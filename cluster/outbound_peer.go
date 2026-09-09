package cluster

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

// outboundPeerConfig 只用于内部组装；超时必须显式提供，不定稿公开配置默认值。
type outboundPeerConfig struct {
	node         NodeID
	address      string
	codec        *FrameCodec
	calls        *ClusterCallRegistry
	dialTimeout  time.Duration
	writeTimeout time.Duration
	// dial 是标准拨号器的测试替换边界，必须遵守 ctx 并转移成功连接的所有权。
	dial              func(context.Context, string, string) (net.Conn, error)
	sharedDiagnostics *clusterDiagnostics
}

// outboundPeer 为一个目标管理按需连接，不拥有业务队列或后台拨号循环。
// 外层每目标唯一 NodeSender 串行调用 Send；此处串行锁也保护关闭与连接替换。
// 每次替换先等待旧读循环结束，因此每个 owner 最多持有一个响应读 goroutine。
type outboundPeer struct {
	config outboundPeerConfig
	ctx    context.Context
	cancel context.CancelFunc

	sendMu    sync.Mutex
	mu        sync.Mutex
	closed    bool
	quiescing bool
	active    *outboundConnection // mu 保护，close 无需等待 sendMu 即可关闭 socket。
	done      chan struct{}       // sendMu 保护，仅关联 active 的读循环。

	diagnostics       chan responseReadFailure
	dropped           atomic.Uint64
	diagnosticsClosed bool // sendMu 保护；所有生产者退出后关闭。
}

func newOutboundPeer(config outboundPeerConfig) (*outboundPeer, error) {
	if config.node == "" || config.address == "" || !config.calls.valid() ||
		config.codec == nil || config.codec.maxFrame <= frameHeaderSize ||
		config.dialTimeout <= 0 || config.writeTimeout <= 0 {
		return nil, fmt.Errorf("%w: target, address, registry, codec and positive timeouts are required", ErrInvalidTransportConfig)
	}
	if config.dial == nil {
		config.dial = (&net.Dialer{}).DialContext
	}
	ctx, cancel := context.WithCancel(context.Background())
	peer := &outboundPeer{
		config: config, ctx: ctx, cancel: cancel,
	}
	if config.sharedDiagnostics == nil {
		peer.diagnostics = make(chan responseReadFailure, defaultFailureQueueCapacity)
	}
	return peer, nil
}

// Send 至多执行一次 Dial 和一次发送尝试，不等待远端业务结果，也不重放。
// 返回的投递错误由 NodeSender 记录；尚未绑定的当前 Call 在这里直接结束等待。
func (peer *outboundPeer) Send(to NodeID, request ClusterEnvelope) (err error) {
	if to != peer.config.node {
		return ErrInvalidNode
	}
	peer.sendMu.Lock()
	defer peer.sendMu.Unlock()
	unbound, err := newConnectionCalls(peer.config.calls, to)
	if err != nil {
		return err
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("%w: outbound send: %v", ErrTransportPanicked, recovered)
			peer.mu.Lock()
			active := peer.active
			peer.mu.Unlock()
			if active != nil {
				active.calls.fail(err)
				active.stream.close()
			}
		}
		if err != nil {
			unbound.reject(request, err)
		}
	}()
	if err := peer.ctx.Err(); err != nil {
		return ErrTransportClosed
	}
	ready, err := unbound.pending(request)
	if err != nil || !ready {
		return err
	}
	connection, err := peer.connection()
	if err != nil {
		return err
	}
	return connection.write(request)
}

// connection 只在 sendMu 内调用；只替换已知关闭的连接，不探测或重发当前请求。
func (peer *outboundPeer) connection() (*outboundConnection, error) {
	peer.mu.Lock()
	active := peer.active
	peer.mu.Unlock()
	if active != nil && !active.stream.closed.Load() {
		return active, nil
	}
	if peer.done != nil {
		<-peer.done
	}
	ctx, cancel := context.WithTimeout(peer.ctx, peer.config.dialTimeout)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	socket, err := peer.config.dial(ctx, "tcp", peer.config.address)
	if err != nil {
		if socket != nil {
			err = errors.Join(err, socket.Close())
		}
		return nil, fmt.Errorf("dial %s: %w", peer.config.node, err)
	}
	stream, err := newStreamConnection(socket, peer.config.codec, peer.config.writeTimeout)
	if err != nil {
		if socket != nil {
			err = errors.Join(err, socket.Close())
		}
		return nil, err
	}
	// 即便拨号器在取消竞争中返回成功，也不得发布关闭后的新连接。
	if err := ctx.Err(); err != nil {
		return nil, errors.Join(err, stream.close())
	}
	connection, err := newOutboundConnection(stream, peer.config.calls, peer.config.node)
	if err != nil {
		return nil, errors.Join(err, stream.close())
	}
	peer.mu.Lock()
	if peer.closed || peer.quiescing {
		peer.mu.Unlock()
		return nil, errors.Join(ErrTransportClosed, stream.close())
	}
	peer.active = connection
	peer.done = make(chan struct{})
	go peer.receive(connection, peer.done)
	peer.mu.Unlock()
	return connection, nil
}

func (peer *outboundPeer) receive(connection *outboundConnection, done chan struct{}) {
	var terminal error
	defer func() {
		if recovered := recover(); recovered != nil {
			terminal = fmt.Errorf("%w: response reader: %v", ErrTransportPanicked, recovered)
		}
		terminal = errors.Join(terminal, connection.stream.close())
		connection.calls.fail(terminal)
		peer.mu.Lock()
		closing := peer.closed
		peer.mu.Unlock()
		expectedClose := closing && (errors.Is(terminal, net.ErrClosed) || errors.Is(terminal, io.ErrClosedPipe) || errors.Is(terminal, context.Canceled)) && !errors.Is(terminal, ErrTransportPanicked)
		if terminal != nil && !expectedClose && peer.config.sharedDiagnostics != nil {
			peer.config.sharedDiagnostics.report(Diagnostic{Kind: DiagnosticConnectionFailure, RemoteNode: peer.config.node, Err: terminal})
		} else if terminal != nil && peer.config.sharedDiagnostics == nil {
			select {
			case peer.diagnostics <- responseReadFailure{node: peer.config.node, err: terminal}:
			default:
				peer.dropped.Add(1)
			}
		}
		close(done)
	}()
	dropped, err := connection.stream.receiveResponsesTo(connection.calls, peer.diagnostics, peer.config.sharedDiagnostics)
	peer.dropped.Add(dropped)
	terminal = err
}

// close 先取消拨号并关闭 socket，随后等待正在发送的调用及响应读循环退出。
// 标准 Dial/写入有有限超时；测试拨号器也必须遵守 context，不能被强杀。
func (peer *outboundPeer) close() error {
	err := peer.signalClose()
	peer.waitClosed()
	return err
}

// quiesce 只取消拨号，不关闭已经建立的 socket。
func (peer *outboundPeer) quiesce() {
	peer.mu.Lock()
	peer.quiescing = true
	peer.cancel()
	peer.mu.Unlock()
}

// signalClose 解除连接 I/O 等待，不等待发送器或响应读循环。
func (peer *outboundPeer) signalClose() error {
	peer.mu.Lock()
	peer.closed = true
	peer.cancel()
	active := peer.active
	peer.mu.Unlock()
	var err error
	if active != nil {
		err = active.stream.close()
	}
	return err
}

func (peer *outboundPeer) waitClosed() {
	peer.sendMu.Lock()
	defer peer.sendMu.Unlock()
	if peer.done != nil {
		<-peer.done
	}
	if !peer.diagnosticsClosed {
		if peer.diagnostics != nil {
			close(peer.diagnostics)
		}
		peer.diagnosticsClosed = true
	}
}
