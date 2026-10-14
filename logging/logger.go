package logging

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

var (
	// ErrQueueFull 表示本条日志未被接纳，业务无需等待磁盘。
	ErrQueueFull = errors.New("logging: queue full")
	// ErrClosed 表示写入者已停止接纳。
	ErrClosed = errors.New("logging: closed")
	// ErrInvalidRecord 表示字段非键值对、无法快照或记录超过大小上限。
	ErrInvalidRecord = errors.New("logging: invalid record")
	// ErrSequenceExhausted 表示序号已耗尽，不允许回绕。
	ErrSequenceExhausted = errors.New("logging: sequence exhausted")
)

// Config 配置唯一写入者；Directory 和 Writer 都未设置时使用 stdout，Capacity 为零时使用 1024。
// Writer 由应用拥有，不会被关闭；应用须在 Logger.Close 完成后再释放它。
type Config struct {
	// Directory 非空选择文件模式，与 Writer 互斥；路径在 New 时解析为绝对路径并固定。
	// 当前文件为 gsr.log，按 100 MiB 滚动并保留 10 份历史；目录须由本 Logger 独占。
	Directory string
	Writer    io.Writer
	Capacity  int
}

// Logger 是共享日志的 owner 入口，负责来源绑定、观测和最终关闭。
type Logger struct {
	core *writerState
}

// ServiceLogger 是来源绑定的记录能力，只共享日志 core，不拥有关闭或重新绑定能力。
type ServiceLogger struct {
	core    *writerState
	service uint64
}

type writerState struct {
	mu       sync.Mutex
	queue    chan []byte
	done     chan struct{}
	sequence uint64
	closed   bool
	writer   io.Writer
	file     *fileWriter // 仅内部文件由 Logger 负责关闭。
	dropped  atomic.Uint64
	failures atomic.Uint64
	result   error // mu 保护；仅保存首次输出错误，避免无限错误链。
}

type record struct {
	Time     time.Time       `json:"time"`
	Sequence uint64          `json:"seq"`
	Level    string          `json:"level"`
	Service  uint64          `json:"service"`
	File     string          `json:"file"`
	Line     int             `json:"line"`
	Message  string          `json:"message"`
	Fields   json.RawMessage `json:"fields,omitempty"`
}

// New 启动一个串行写入任务，输出每行一条 JSON，不等待日志消费者或云服务。
func New(config Config) (*Logger, error) {
	if config.Directory != "" && config.Writer != nil {
		return nil, errors.New("logging: Directory and Writer are mutually exclusive")
	}
	if config.Capacity < 0 {
		return nil, errors.New("logging: negative capacity")
	}
	if config.Capacity == 0 {
		config.Capacity = 1024
	}
	if config.Writer == nil {
		config.Writer = os.Stdout
	}
	state := &writerState{queue: make(chan []byte, config.Capacity), done: make(chan struct{}), writer: config.Writer}
	if config.Directory != "" {
		file, err := newFileWriter(config.Directory)
		if err != nil {
			return nil, err
		}
		state.file = file
		state.writer = file
	}
	go state.run()
	return &Logger{core: state}, nil
}

// WithService 绑定日志来源；ID 由受信任组装代码传入，不读取当前 goroutine 身份。
func (logger *Logger) WithService(id uint64) *ServiceLogger {
	if logger == nil {
		return nil
	}
	return &ServiceLogger{core: logger.core, service: id}
}

// Info 在调用处记录时间、代码位置和字段快照；nil 入口禁用输出。
func (logger *Logger) Info(message string, fields ...any) error {
	return write(loggerState(logger), 0, "INFO", message, fields)
}

// Error 与 Info 使用同一入队路径，不单独排队，也不等待写盘。
func (logger *Logger) Error(message string, fields ...any) error {
	return write(loggerState(logger), 0, "ERROR", message, fields)
}

// Info 在调用处记录来源、时间、位置及字段快照；nil 入口禁用输出。
func (logger *ServiceLogger) Info(message string, fields ...any) error {
	if logger == nil {
		return nil
	}
	return write(logger.core, logger.service, "INFO", message, fields)
}

// Error 与 Info 共享同一接纳路径，成功不表示已写盘。
func (logger *ServiceLogger) Error(message string, fields ...any) error {
	if logger == nil {
		return nil
	}
	return write(logger.core, logger.service, "ERROR", message, fields)
}

func loggerState(logger *Logger) *writerState {
	if logger == nil {
		return nil
	}
	return logger.core
}

func write(state *writerState, service uint64, level, message string, fields []any) (err error) {
	if state == nil {
		return nil
	}
	if err := state.err(); err != nil {
		return err
	}
	entry := record{Time: time.Now(), Level: level, Service: service, Message: message}
	_, entry.File, entry.Line, _ = runtime.Caller(2)
	// 扩展 error/JSON 格式化可能 panic，不能逃出业务记录点；不在队列锁内执行扩展。
	defer func() {
		if recover() != nil {
			err = ErrInvalidRecord
		}
	}()
	if len(fields)%2 != 0 {
		return ErrInvalidRecord
	}
	values := make(map[string]any, len(fields)/2)
	for index := 0; index < len(fields); index += 2 {
		key, ok := fields[index].(string)
		if !ok || key == "" {
			return ErrInvalidRecord
		}
		if _, exists := values[key]; exists {
			return ErrInvalidRecord
		}
		value := fields[index+1]
		if cause, ok := value.(error); ok {
			value = cause.Error()
		}
		values[key] = value
	}
	entry.Fields, err = json.Marshal(values)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidRecord, err)
	}
	if len(entry.Fields)+len(message) > 64<<10 {
		return ErrInvalidRecord
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.result != nil {
		return state.result
	}
	if state.closed {
		return ErrClosed
	}
	if state.sequence == math.MaxUint64 {
		return ErrSequenceExhausted
	}
	entry.Sequence = state.sequence + 1
	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	if len(data) > 64<<10 {
		return ErrInvalidRecord
	}
	select {
	case state.queue <- append(data, '\n'):
		state.sequence = entry.Sequence
		return nil
	default:
		state.dropped.Add(1)
		return ErrQueueFull
	}
}

// DroppedCount 返回队列满载未接纳的日志数；不与 Diagnostics 丢弃数混用。
func (logger *Logger) DroppedCount() uint64 {
	if logger == nil || logger.core == nil {
		return 0
	}
	return logger.core.dropped.Load()
}

// WriteFailureCount 返回已接纳但输出失败的记录数。
func (logger *Logger) WriteFailureCount() uint64 {
	if logger == nil || logger.core == nil {
		return 0
	}
	return logger.core.failures.Load()
}

// Err 返回首次输出故障；可在运行中查询，不表示队列满载或入参错误。
func (logger *Logger) Err() error {
	if logger == nil || logger.core == nil {
		return nil
	}
	return logger.core.err()
}

func (state *writerState) err() error {
	state.mu.Lock()
	defer state.mu.Unlock()
	return state.result
}

// Close 停止接纳并等待排空和 Flush，关闭内部文件；ctx 只限制本次等待，不强杀阻塞 Writer。
// 可再次等待同一流程；返回 nil 代表已写入 Writer，不代表文件已 fsync 或已被 SLS 收集。
func (logger *Logger) Close(ctx context.Context) error {
	if ctx == nil {
		return errors.New("logging: nil close context")
	}
	if logger == nil || logger.core == nil {
		return nil
	}
	state := logger.core
	state.mu.Lock()
	if !state.closed {
		state.closed = true
		close(state.queue)
	}
	state.mu.Unlock()
	select {
	case <-state.done:
		return state.result
	default:
	}
	select {
	case <-state.done:
		return state.result
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (state *writerState) run() {
	defer close(state.done)
	failed := false
	for data := range state.queue {
		if failed {
			state.failures.Add(1)
			continue
		}
		if err := writeRecord(state.writer, data); err != nil {
			state.failures.Add(1)
			state.fail(err)
			failed = true
		}
	}
	if !failed {
		if err := flushWriter(state.writer); err != nil {
			state.fail(err)
		}
	}
	if state.file != nil {
		if err := state.file.Close(); err != nil {
			state.fail(err)
		}
	}
}

func (state *writerState) fail(err error) {
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.result == nil {
		state.result = err
	}
	if !state.closed {
		state.closed = true
		close(state.queue)
	}
}

func writeRecord(writer io.Writer, data []byte) (err error) {
	defer func() {
		if recover() != nil {
			err = errors.New("logging: writer panicked")
		}
	}()
	n, err := writer.Write(data)
	if err == nil && n != len(data) {
		return io.ErrShortWrite
	}
	return err
}

func flushWriter(writer io.Writer) (err error) {
	defer func() {
		if recover() != nil {
			err = errors.New("logging: flush panicked")
		}
	}()
	if flusher, ok := writer.(interface{ Flush() error }); ok {
		return flusher.Flush()
	}
	return nil
}
