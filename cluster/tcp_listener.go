package cluster

import (
	"context"
	"errors"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

var (
	errInvalidTCPListener     = errors.New("cluster: invalid TCP listener config or lifecycle")
	errInboundConnectionLimit = errors.New("cluster: inbound connection limit reached")
)

// tcpListenerConfig 仅组装入站能力，不包含节点身份、出站路由或认证协议。
// 所有容量和写入等待显式配置；listener 只能由这个 owner 接纳连接。
type tcpListenerConfig struct {
	listener          *net.TCPListener
	codec             *FrameCodec
	adapter           *localRequestAdapter
	maxConnections    int
	maxCalls          int
	writeTimeout      time.Duration
	sharedDiagnostics *clusterDiagnostics
}

// tcpListener 拥有一个监听 socket 及全部已接纳连接，serve 返回前等待任务退出。
// 每个已接纳连接有一个处理任务，受 maxConnections 约束；诊断直接写入共享出口。
type tcpListener struct {
	config           tcpListenerConfig
	started          atomic.Bool
	mu               sync.Mutex
	connections      map[*streamConnection]struct{}
	diagnostics      *inboundDiagnostics
	admissionStopped atomic.Bool
	quiesceOnce      sync.Once
	quiesceErr       error
}

// newTCPListener 成功后转移监听 socket 管理权；失败时仍由调用方负责关闭。
func newTCPListener(config tcpListenerConfig) (*tcpListener, error) {
	if config.listener == nil || config.codec == nil || config.codec.maxFrame <= frameHeaderSize ||
		config.adapter == nil || config.adapter.runtime == nil || config.adapter.codecs == nil ||
		config.maxConnections <= 0 || config.maxCalls <= 0 || config.writeTimeout <= 0 {
		return nil, errInvalidTCPListener
	}
	diagnostics := &inboundDiagnostics{shared: config.sharedDiagnostics}
	if config.sharedDiagnostics == nil {
		diagnostics = newInboundDiagnostics()
	}
	return &tcpListener{
		config:      config,
		connections: make(map[*streamConnection]struct{}),
		diagnostics: diagnostics,
	}, nil
}

// serve 只允许启动一次。取消或异常 Accept 失败时关闭监听和连接，取消 Call 并等待收尾。
// quiesce 关闭监听属于正常停止接纳，仍保留连接直到最终取消。
// 不强杀业务 Handler 或无限阻塞的扩展代码；终止后不重新监听。
func (owner *tcpListener) serve(parent context.Context) error {
	if owner == nil || parent == nil || !owner.started.CompareAndSwap(false, true) {
		return errInvalidTCPListener
	}
	ctx, cancel := context.WithCancel(parent)
	watchDone := make(chan struct{})
	go func() {
		defer close(watchDone)
		<-ctx.Done()
		owner.shutdown()
	}()
	var workers sync.WaitGroup
	defer func() {
		cancel()
		owner.shutdown()
		workers.Wait()
		<-watchDone
		// Accept 与全部连接生产者均已退出，至此才能关闭共享出口。
		if owner.diagnostics.shared == nil {
			close(owner.diagnostics.failures)
		}
	}()
	for {
		socket, err := owner.config.listener.AcceptTCP()
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if owner.admissionStopped.Load() {
				// 停止 Accept 不等于最终清理；已有 Call 继续使用原连接回包。
				<-ctx.Done()
				return ctx.Err()
			}
			return err
		}
		connection, err := newStreamConnection(socket, owner.config.codec, owner.config.writeTimeout)
		if err != nil {
			socket.Close()
			return err
		}
		owner.mu.Lock()
		if ctx.Err() != nil || owner.admissionStopped.Load() {
			owner.mu.Unlock()
			connection.close()
			continue
		}
		if len(owner.connections) >= owner.config.maxConnections {
			owner.mu.Unlock()
			connection.close()
			owner.diagnostics.report(inboundFailure{remoteAddress: socket.RemoteAddr().String(), err: errInboundConnectionLimit})
			continue
		}
		owner.connections[connection] = struct{}{}
		owner.mu.Unlock()
		workers.Add(1)
		go func() {
			defer workers.Done()
			owner.serveConnection(ctx, connection)
		}()
	}
}

func (owner *tcpListener) serveConnection(ctx context.Context, connection *streamConnection) {
	defer func() {
		connection.close()
		owner.mu.Lock()
		delete(owner.connections, connection)
		owner.mu.Unlock()
	}()
	address := connection.socket.RemoteAddr().String()
	inbound, err := newInboundConnection(connection, owner.config.adapter, owner.config.maxCalls, owner.diagnostics)
	if err != nil {
		owner.diagnostics.report(inboundFailure{remoteAddress: address, err: err})
		return
	}
	inbound.admissionStopped = &owner.admissionStopped
	err = inbound.serve(ctx)
	if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, net.ErrClosed) && !errors.Is(err, io.EOF) {
		owner.diagnostics.report(inboundFailure{remoteAddress: address, err: err})
	}
}

// quiesce 只关闭监听和请求门禁，不取消已接纳请求，也不等待连接任务。
// 共享原子门禁覆盖已存在和刚注册的连接；只进行一次监听关闭。
func (owner *tcpListener) quiesce() error {
	owner.quiesceOnce.Do(func() {
		owner.mu.Lock()
		owner.admissionStopped.Store(true)
		owner.mu.Unlock()
		owner.quiesceErr = owner.config.listener.Close()
	})
	return owner.quiesceErr
}

// shutdown 不等待任务；先解除 Accept 与 socket I/O 等待，再由 serve 统一等待退出。
func (owner *tcpListener) shutdown() {
	owner.config.listener.Close()
	owner.mu.Lock()
	defer owner.mu.Unlock()
	for connection := range owner.connections {
		connection.close()
	}
}
