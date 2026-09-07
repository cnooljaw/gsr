package cluster

import (
	"errors"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

// streamConnection 管理一个 socket，不管理读循环或重连策略。
// read 只允许一个读取方调用；基础设施的写入调用串行执行。
// 外层 Transport 必须限制并发写入方数量，并等待其 I/O goroutine 退出。
type streamConnection struct {
	socket       net.Conn
	codec        *FrameCodec
	writeTimeout time.Duration
	writeMu      sync.Mutex
	closed       atomic.Bool
	closeOnce    sync.Once
	closeErr     error
}

// 只有构造成功才转移 socket 管理权。writeTimeout 限制每次 socket 写入耗时，
// 不包含等待写锁或远端 Handler 执行的时间。
func newStreamConnection(socket net.Conn, codec *FrameCodec, writeTimeout time.Duration) (*streamConnection, error) {
	if socket == nil || codec == nil || codec.maxFrame <= frameHeaderSize || writeTimeout <= 0 {
		return nil, errors.New("cluster: socket, initialized frame codec and positive write timeout are required")
	}
	return &streamConnection{socket: socket, codec: codec, writeTimeout: writeTimeout}, nil
}

func (connection *streamConnection) read() (ClusterEnvelope, error) {
	envelope, err := connection.codec.readFrame(connection.socket)
	if err != nil {
		var complete *completeFrameError
		if !errors.As(err, &complete) {
			err = errors.Join(err, connection.close())
		}
	}
	return envelope, err
}

func (connection *streamConnection) write(envelope ClusterEnvelope) error {
	connection.writeMu.Lock()
	defer connection.writeMu.Unlock()
	if connection.closed.Load() {
		return net.ErrClosed
	}
	frame, err := connection.codec.Encode(envelope)
	if err != nil {
		return err
	}
	if err := connection.socket.SetWriteDeadline(time.Now().Add(connection.writeTimeout)); err != nil {
		return errors.Join(err, connection.close())
	}
	if err := writeFrameBytes(connection.socket, frame); err != nil {
		return errors.Join(err, connection.close())
	}
	return nil
}

// close 不获取 writeMu：必须先关闭 socket 解除正在进行的写入阻塞，
// 等待写锁的调用方才能继续并观察到已关闭状态。
func (connection *streamConnection) close() error {
	connection.closeOnce.Do(func() {
		connection.closed.Store(true)
		connection.closeErr = connection.socket.Close()
	})
	return connection.closeErr
}
