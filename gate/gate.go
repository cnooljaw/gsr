package gate

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cnooljaw/gsr/logging"
	"github.com/cnooljaw/gsr/runtime"
	"github.com/gorilla/websocket"
)

const (
	defaultMaxFrameBytes                          = 4 << 20
	defaultHandshakeTimeout                       = 5 * time.Second
	defaultReadHeaderTimeout                      = 5 * time.Second
	defaultHTTPIdleTimeout                        = 5 * time.Second
	defaultAgentStartWait                         = 5 * time.Second
	defaultWriteWait                              = 10 * time.Second
	closeFrameWait                                = 100 * time.Millisecond
	gateConnectCommand          runtime.CommandID = 0xffff_ff00
	gateDisconnectCommand       runtime.CommandID = 0xffff_ff01
	websocketHandshakeCloseCode                   = websocket.ClosePolicyViolation
)

var (
	// ErrInvalidConfig 表示 Gate 协议模式、监听或有限容量配置无效。
	ErrInvalidConfig = errors.New("gate: invalid config")
	// ErrClientQueueFull 表示客户端出站队列达到配置的字节上限。
	ErrClientQueueFull = errors.New("gate: client outbound queue full")
	// ErrClientSessionClosed 表示连接已关闭，不能再发送消息。
	ErrClientSessionClosed = errors.New("gate: client session closed")
	// ErrFrameTooLarge 表示完整 WebSocket 消息超过单帧大小上限。
	ErrFrameTooLarge = errors.New("gate: frame too large")
	errGateStopping  = errors.New("gate: Gate is stopping")
)

var gateUpgrader = websocket.Upgrader{
	EnableCompression: false,
	// Origin 不是客户端身份；业务 Agent 仍须显式完成 token 鉴权。
	CheckOrigin: func(*http.Request) bool { return true },
}

// Protocol 是单个 Gate 监听固定使用的上行协议，不按连接猜测或自动降级。
type Protocol uint8

const (
	// ProtocolMAC 是默认模式：X25519 握手后校验每帧的 seq 和 MAC。
	ProtocolMAC Protocol = iota
	// ProtocolPlain 直接收发字节消息，仅允许显式绑定回环或私有 IP。
	ProtocolPlain
)

// Config 定义 WebSocket Gate 的固定协议模式、监听、容量和连接 Agent。
type Config struct {
	// Protocol 的零值为 MAC；Plain 必须显式选择，TLS 与协议模式独立。
	Protocol Protocol
	// Address 是 Gate 绑定的 TCP host:port；Plain 仅允许字面回环或私有 IP。
	Address string
	// TLSConfig 为 nil 时监听普通 HTTP WebSocket，适用于可信内网或由 Ingress 终止 TLS 的部署；
	// 公网直连由部署方提供 TLS 配置。Gate 克隆非 nil 配置，只协商 HTTP/1.1，
	// 并拒绝可能替换整份 TLS 配置的 GetConfigForClient；GetCertificate 可用于动态选证书。
	TLSConfig *tls.Config
	// MaxConnections 必须为正，限制原始 TCP 连接，包括未完成 HTTP 请求头或握手的连接。
	MaxConnections int
	// HandshakeTimeout 仅在 MAC 模式限制首个握手帧等待，零值使用 5 秒；Plain 不执行握手。
	HandshakeTimeout time.Duration
	// MaxFrameBytes 限制单条 WebSocket 二进制消息；零值使用 4 MiB。
	MaxFrameBytes int
	// MaxOutboundQueueBytes 限制每个连接的出站队列总字节数，必须至少容纳一个队列条目开销。
	MaxOutboundQueueBytes int
	// NewAgent 在连接接纳（MAC 模式还须完成握手）后创建独立 Agent Service。
	NewAgent func(ClientSession) runtime.Service
}

// Metrics 是 Gate 两种模式共用的有限容量计数快照。
type Metrics struct {
	// AcceptedConnections 只计连接创建 Call 返回有效 AgentID 的连接；MAC 还须握手成功，不包含迟到创建。
	AcceptedConnections uint64
	// RejectedConnections 是容量、握手、升级或 Agent 创建失败的连接数。
	RejectedConnections uint64
	// HandshakeTimeouts 是 MAC 首帧握手超时数；Plain 模式始终为零。
	HandshakeTimeouts uint64
	// FrameTooLarge 是超出单帧限制的收发消息数。
	FrameTooLarge uint64
	// InboundMailboxFull 是 Runtime 拒绝客户端消息入队的次数。
	InboundMailboxFull uint64
	// OutboundQueueFull 是 Gate 拒绝 Agent 出站消息的次数。
	OutboundQueueFull uint64
}

type gateCounters struct {
	acceptedConnections atomic.Uint64
	rejectedConnections atomic.Uint64
	handshakeTimeouts   atomic.Uint64
	frameTooLarge       atomic.Uint64
	inboundMailboxFull  atomic.Uint64
	outboundQueueFull   atomic.Uint64
}

// GateService 是两种协议共用的连接 owner，持有监听、客户端连接和 Agent 映射。
type GateService struct {
	runtime *runtime.Runtime
	config  Config

	mu             sync.RWMutex
	server         *http.Server
	serviceID      runtime.ServiceID
	address        net.Addr
	stopping       bool
	activeHandlers int
	connections    map[*clientConnection]struct{}
	serveErr       error
	ioWorkers      sync.WaitGroup
	counters       gateCounters
	logger         *logging.Logger
	nextConnection atomic.Uint64

	// agentsByConnection 只在连接断开时定位需要通知的 Agent。
	agentsByConnection map[*clientConnection]runtime.ServiceID
}

var _ runtime.Service = (*GateService)(nil)

// NewGateService 校验固定协议模式、可选 TLS 与容量配置，不按首帧猜测模式。
func NewGateService(local *runtime.Runtime, config Config) (*GateService, error) {
	if local == nil {
		return nil, fmt.Errorf("%w: Runtime is required", ErrInvalidConfig)
	}
	if config.Protocol != ProtocolMAC && config.Protocol != ProtocolPlain {
		return nil, fmt.Errorf("%w: unknown protocol mode", ErrInvalidConfig)
	}
	if config.HandshakeTimeout == 0 {
		config.HandshakeTimeout = defaultHandshakeTimeout
	}
	if config.MaxFrameBytes == 0 {
		config.MaxFrameBytes = defaultMaxFrameBytes
	}
	if config.MaxConnections <= 0 || config.HandshakeTimeout < 0 || config.MaxFrameBytes <= 0 ||
		config.MaxOutboundQueueBytes < queueEntryOverhead {
		return nil, fmt.Errorf("%w: connection, handshake, frame, or outbound queue limit is invalid", ErrInvalidConfig)
	}
	if config.Protocol == ProtocolMAC && config.MaxFrameBytes < handshakeFrameSize {
		return nil, fmt.Errorf("%w: frame limit must accommodate the handshake", ErrInvalidConfig)
	}
	if config.NewAgent == nil {
		return nil, fmt.Errorf("%w: NewAgent is required", ErrInvalidConfig)
	}
	if err := validateListenAddress(config.Address); err != nil {
		return nil, err
	}
	if config.Protocol == ProtocolPlain {
		if err := validatePrivateAddress(config.Address); err != nil {
			return nil, err
		}
	}
	tlsConfig, err := productionTLSConfig(config.TLSConfig)
	if err != nil {
		return nil, err
	}
	config.TLSConfig = tlsConfig

	return &GateService{
		runtime:            local,
		config:             config,
		connections:        make(map[*clientConnection]struct{}),
		agentsByConnection: make(map[*clientConnection]runtime.ServiceID),
		logger:             local.Config().Logger,
	}, nil
}

// Addr 返回 Runtime 创建 GateService 后实际监听的地址。
func (gate *GateService) Addr() net.Addr {
	gate.mu.Lock()
	defer gate.mu.Unlock()
	return gate.address
}

// Metrics 返回 Gate 的容量与接纳计数快照。
func (gate *GateService) Metrics() Metrics {
	return Metrics{
		AcceptedConnections: gate.counters.acceptedConnections.Load(),
		RejectedConnections: gate.counters.rejectedConnections.Load(),
		HandshakeTimeouts:   gate.counters.handshakeTimeouts.Load(),
		FrameTooLarge:       gate.counters.frameTooLarge.Load(),
		InboundMailboxFull:  gate.counters.inboundMailboxFull.Load(),
		OutboundQueueFull:   gate.counters.outboundQueueFull.Load(),
	}
}

// OnStart 先绑定地址，再启动 Gate 自己拥有的 HTTP 或 HTTPS WebSocket 接入循环。
func (gate *GateService) OnStart(_ context.Context, self runtime.ServiceContext) error {
	listener, err := net.Listen("tcp", gate.config.Address)
	if err != nil {
		return fmt.Errorf("gate: listen: %w", err)
	}
	limitedListener := newConnectionLimitListener(listener, gate.config.MaxConnections, func() {
		gate.counters.rejectedConnections.Add(1)
		gate.logError("Gate connection rejected", "reason", "capacity")
	})
	server := &http.Server{
		Handler:           gate,
		ReadHeaderTimeout: defaultReadHeaderTimeout,
		IdleTimeout:       defaultHTTPIdleTimeout,
	}
	if gate.config.TLSConfig != nil {
		server.TLSConfig = gate.config.TLSConfig.Clone()
	}

	gate.mu.Lock()
	gate.server = server
	gate.serviceID = self.Self()
	gate.logger = gate.logger.WithService(uint64(self.Self()))
	server.ErrorLog = newHTTPServerErrorLog(gate.logger)
	gate.address = listener.Addr()
	gate.ioWorkers.Add(1)
	gate.mu.Unlock()

	go func() {
		defer gate.ioWorkers.Done()
		var serveErr error
		if gate.config.TLSConfig != nil {
			serveErr = server.ServeTLS(limitedListener, "", "")
		} else {
			serveErr = server.Serve(limitedListener)
		}
		if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			gate.mu.Lock()
			gate.serveErr = serveErr
			gate.mu.Unlock()
			gate.logError("Gate listener failed", "phase", "serve", "error", serveErr)
		}
	}()
	return nil
}

// Handle 在 Gate runner 上创建 Agent，并在连接断开时通知对应 Agent。
func (gate *GateService) Handle(self runtime.CommandContext, command runtime.CommandID, args ...any) (any, error) {
	switch command {
	case gateConnectCommand:
		clientConn, err := clientConnArgument(args)
		if err != nil {
			return nil, err
		}
		if _, exists := gate.agentsByConnection[clientConn]; exists {
			return nil, errors.New("gate: duplicate client connection")
		}
		agent, err := createAgent(gate.config.NewAgent, clientConn)
		if err != nil {
			gate.logConnectionFailure(clientConn, "agent_create", websocket.CloseInternalServerErr, err)
			return nil, err
		}
		startContext, cancel := context.WithTimeout(context.Background(), defaultAgentStartWait)
		defer cancel()
		agentID, err := self.CreateService(startContext, agent)
		if err != nil {
			gate.logConnectionFailure(clientConn, "agent_create", websocket.CloseInternalServerErr, err)
			// 已进入 Gate Handler；创建失败不能被接线重试误判为 Mailbox admission 失败。
			return nil, runtime.ErrHandlerFailed
		}
		gate.agentsByConnection[clientConn] = agentID
		return agentID, nil

	case gateDisconnectCommand:
		clientConn, err := clientConnArgument(args)
		if err != nil {
			return nil, err
		}
		agentID, exists := gate.agentsByConnection[clientConn]
		if !exists {
			return nil, nil
		}
		if err := self.Send(agentID, ClientDisconnectedCommand); err != nil && !isRuntimeStoppingError(err) {
			gate.logConnectionFailure(clientConn, "disconnect_notify", websocket.CloseInternalServerErr, err)
			if errors.Is(err, runtime.ErrMailboxFull) {
				// 保留映射，让连接处理协程重试，避免断开通知被 Agent 背压吞掉。
				return nil, err
			}
			delete(gate.agentsByConnection, clientConn)
			return nil, fmt.Errorf("gate: notify Agent disconnect: %w", err)
		}
		delete(gate.agentsByConnection, clientConn)
		return nil, nil

	default:
		return nil, fmt.Errorf("gate: unknown internal command %d", command)
	}
}

// OnStop 停止接纳、关闭客户端连接，并等待 Gate 自己的 I/O goroutine 退出。
func (gate *GateService) OnStop(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	gate.mu.Lock()
	gate.stopping = true
	server := gate.server
	clientConnections := make([]*clientConnection, 0, len(gate.connections))
	for clientConn := range gate.connections {
		clientConnections = append(clientConnections, clientConn)
	}
	gate.mu.Unlock()

	var result error
	if server != nil {
		if err := server.Close(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			result = errors.Join(result, err)
		}
	}
	closeDeadline := time.Now().Add(closeFrameWait)
	if ctx != nil {
		if deadline, ok := ctx.Deadline(); ok && deadline.Before(closeDeadline) {
			closeDeadline = deadline
		}
	}
	for _, clientConn := range clientConnections {
		if err := clientConn.closeWith(websocket.CloseGoingAway, "going_away", closeDeadline); err != nil {
			result = errors.Join(result, fmt.Errorf("gate: close client connection %d: %w", clientConn.connectionID, err))
		}
	}
	gate.ioWorkers.Wait()
	result = errors.Join(result, gate.notifyMappedAgentsOnStop(ctx))
	clear(gate.agentsByConnection)
	gate.logError("Gate stopped", "phase", "stop")

	gate.mu.Lock()
	serveErr := gate.serveErr
	gate.mu.Unlock()
	return errors.Join(result, serveErr)
}

// ServeHTTP 按构造时固定的认证模式接纳连接，读循环直接向 Agent 投递不透明字节。
func (gate *GateService) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	if request.URL.Path != "/ws" {
		http.NotFound(writer, request)
		return
	}
	if request.Method != http.MethodGet {
		http.Error(writer, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	serviceID, admitted := gate.beginHandler()
	if !admitted {
		http.Error(writer, "gate unavailable", http.StatusServiceUnavailable)
		return
	}

	var clientConn *clientConnection
	var agentID runtime.ServiceID
	var agentCreationRequested bool
	defer func() {
		if clientConn == nil {
			gate.finishHandler()
		} else {
			clientConn.closeWith(websocket.CloseNormalClosure, "normal", time.Now().Add(closeFrameWait))
			if agentCreationRequested {
				gate.notifyDisconnected(serviceID, clientConn)
			}
			gate.flushConnectionDiagnostics(clientConn)
			gate.untrackConnection(clientConn)
		}
		gate.ioWorkers.Done()
	}()

	connection, err := gateUpgrader.Upgrade(writer, request, nil)
	if err != nil {
		gate.logError("Gate connection rejected", "reason", "upgrade", "error", err)
		gate.counters.rejectedConnections.Add(1)
		return
	}
	clientConn = newClientConnection(connection, gate.config.MaxFrameBytes, gate.config.MaxOutboundQueueBytes, &gate.counters, gate.nextConnection.Add(1))
	if !gate.trackConnection(clientConn) {
		clientConn.closeWith(websocket.CloseGoingAway, "going_away", time.Now().Add(closeFrameWait))
		return
	}

	connection.SetReadLimit(int64(gate.config.MaxFrameBytes))
	var authenticator *clientAuthenticator
	if gate.config.Protocol == ProtocolMAC {
		authenticator, err = gate.performHandshake(clientConn)
		if err != nil {
			gate.counters.rejectedConnections.Add(1)
			return
		}
	}

	// Call 超时不会取消 Gate Handler；断开事件仍要排在创建事件之后处理，因此不继承 WebSocket 请求的取消信号。
	agentCreationRequested = true
	connectContext, cancel := context.WithTimeout(context.Background(), defaultAgentStartWait)
	result, err := callGateConnect(connectContext, gate.runtime, serviceID, gateConnectCommand, clientConn)
	cancel()
	if err != nil {
		// Handler 内部的创建失败已经由 Gate runner 记录；这里只记录等待创建超时，避免同一失败重复落日志。
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, runtime.ErrCallTimeout) {
			gate.logConnectionFailure(clientConn, "agent_start_timeout", websocket.CloseInternalServerErr, err)
		}
		gate.counters.rejectedConnections.Add(1)
		clientConn.closeWith(websocket.CloseInternalServerErr, "server_error", time.Now().Add(closeFrameWait))
		return
	}
	createdAgentID, ok := result.(runtime.ServiceID)
	if !ok || createdAgentID == 0 {
		gate.logConnectionFailure(clientConn, "agent_result", websocket.CloseInternalServerErr, errors.New("invalid Agent ServiceID"))
		gate.counters.rejectedConnections.Add(1)
		clientConn.closeWith(websocket.CloseInternalServerErr, "server_error", time.Now().Add(closeFrameWait))
		return
	}
	agentID = createdAgentID
	gate.counters.acceptedConnections.Add(1)

	gate.startClientWriter(clientConn)

	for {
		messageType, frame, err := connection.ReadMessage()
		if err != nil {
			if errors.Is(err, websocket.ErrReadLimit) {
				gate.logConnectionFailure(clientConn, "frame_too_large", websocket.CloseMessageTooBig, err)
				gate.counters.frameTooLarge.Add(1)
				clientConn.closeWith(websocket.CloseMessageTooBig, "message_too_big", time.Now().Add(closeFrameWait))
			}
			return
		}
		if messageType != websocket.BinaryMessage {
			gate.logConnectionFailure(clientConn, "unsupported_data", websocket.CloseUnsupportedData, errors.New("non-binary WebSocket message"))
			clientConn.closeWith(websocket.CloseUnsupportedData, "unsupported_data", time.Now().Add(closeFrameWait))
			return
		}
		if gate.config.Protocol == ProtocolMAC {
			err = gate.deliverAuthenticatedFrame(agentID, clientConn, authenticator, frame)
		} else {
			err = gate.deliverFrame(agentID, clientConn, frame)
		}
		if err != nil {
			if errors.Is(err, errGateStopping) {
				<-clientConn.outbound.Done()
			}
			return
		}
	}
}

func (gate *GateService) startClientWriter(clientConn *clientConnection) {
	gate.ioWorkers.Add(1)
	go func() {
		defer gate.ioWorkers.Done()
		if err := clientConn.writeLoop(); err != nil {
			gate.logConnectionFailure(clientConn, "write", websocket.CloseGoingAway, err)
		}
	}()
}

func (gate *GateService) performHandshake(clientConn *clientConnection) (*clientAuthenticator, error) {
	connection := clientConn.connection
	if err := connection.SetReadDeadline(time.Now().Add(gate.config.HandshakeTimeout)); err != nil {
		gate.logConnectionFailure(clientConn, "handshake_deadline", websocket.CloseInternalServerErr, err)
		clientConn.closeWith(websocket.CloseInternalServerErr, "server_error", time.Now().Add(closeFrameWait))
		return nil, err
	}
	messageType, frame, err := connection.ReadMessage()
	if err != nil {
		if errors.Is(err, websocket.ErrReadLimit) {
			gate.logConnectionFailure(clientConn, "frame_too_large", websocket.CloseMessageTooBig, err)
			gate.counters.frameTooLarge.Add(1)
			clientConn.closeWith(websocket.CloseMessageTooBig, "message_too_big", time.Now().Add(closeFrameWait))
		} else if isTimeoutError(err) {
			gate.logConnectionFailure(clientConn, "handshake_timeout", websocketHandshakeCloseCode, err)
			gate.counters.handshakeTimeouts.Add(1)
			clientConn.closeWith(websocketHandshakeCloseCode, "policy_violation", time.Now().Add(closeFrameWait))
		} else {
			gate.logConnectionFailure(clientConn, "handshake_read", websocket.CloseInternalServerErr, err)
			clientConn.closeWith(websocket.CloseInternalServerErr, "server_error", time.Now().Add(closeFrameWait))
		}
		return nil, err
	}
	if messageType != websocket.BinaryMessage {
		gate.logConnectionFailure(clientConn, "unsupported_data", websocket.CloseUnsupportedData, errors.New("handshake must be binary"))
		clientConn.closeWith(websocket.CloseUnsupportedData, "unsupported_data", time.Now().Add(closeFrameWait))
		return nil, errors.New("gate: handshake must be a binary message")
	}

	response, authenticator, err := acceptClientHandshake(frame, gate.config.MaxFrameBytes)
	if err != nil {
		gate.logConnectionFailure(clientConn, handshakeErrorCategory(err), closeCodeForHandshakeError(err), err)
		switch {
		case errors.Is(err, errHandshakeFrameTooLarge):
			gate.counters.frameTooLarge.Add(1)
			clientConn.closeWith(websocket.CloseMessageTooBig, "message_too_big", time.Now().Add(closeFrameWait))
		case errors.Is(err, errHandshakeKeyGeneration):
			clientConn.closeWith(websocket.CloseInternalServerErr, "server_error", time.Now().Add(closeFrameWait))
		default:
			clientConn.closeWith(websocket.CloseProtocolError, "protocol_error", time.Now().Add(closeFrameWait))
		}
		return nil, err
	}
	if err := setWebSocketWriteDeadline(connection, time.Now().Add(defaultWriteWait)); err != nil {
		gate.logConnectionFailure(clientConn, "handshake_write", websocket.CloseInternalServerErr, err)
		clientConn.closeWith(websocket.CloseInternalServerErr, "server_error", time.Now().Add(closeFrameWait))
		return nil, err
	}
	if err := connection.WriteMessage(websocket.BinaryMessage, response); err != nil {
		gate.logConnectionFailure(clientConn, "handshake_write", websocket.CloseInternalServerErr, err)
		clientConn.closeWith(websocket.CloseInternalServerErr, "server_error", time.Now().Add(closeFrameWait))
		return nil, err
	}
	if err := connection.SetReadDeadline(time.Time{}); err != nil {
		gate.logConnectionFailure(clientConn, "handshake_deadline", websocket.CloseInternalServerErr, err)
		clientConn.closeWith(websocket.CloseInternalServerErr, "server_error", time.Now().Add(closeFrameWait))
		return nil, err
	}
	return authenticator, nil
}

func (gate *GateService) deliverAuthenticatedFrame(
	agentID runtime.ServiceID,
	clientConn *clientConnection,
	authenticator *clientAuthenticator,
	frame []byte,
) error {
	_, message, err := authenticator.verifyFrame(frame, gate.config.MaxFrameBytes)
	if err != nil {
		switch {
		case errors.Is(err, errAuthFrameTooLarge):
			gate.logConnectionFailure(clientConn, "frame_too_large", websocket.CloseMessageTooBig, err)
			gate.counters.frameTooLarge.Add(1)
			clientConn.closeWith(websocket.CloseMessageTooBig, "message_too_big", time.Now().Add(closeFrameWait))
		case errors.Is(err, errClientMACInvalid), errors.Is(err, errClientSequenceInvalid):
			gate.logConnectionFailure(clientConn, authErrorCategory(err), websocket.ClosePolicyViolation, err)
			clientConn.closeWith(websocket.ClosePolicyViolation, "policy_violation", time.Now().Add(closeFrameWait))
		case errors.Is(err, errMalformedAuthFrame):
			gate.logConnectionFailure(clientConn, "protocol", websocket.CloseProtocolError, err)
			clientConn.closeWith(websocket.CloseProtocolError, "protocol_error", time.Now().Add(closeFrameWait))
		default:
			gate.logConnectionFailure(clientConn, "authentication", websocket.CloseInternalServerErr, err)
			clientConn.closeWith(websocket.CloseInternalServerErr, "server_error", time.Now().Add(closeFrameWait))
		}
		return err
	}
	return gate.deliverFrame(agentID, clientConn, message)
}

func (gate *GateService) deliverFrame(agentID runtime.ServiceID, clientConn *clientConnection, frame []byte) error {
	if err := gate.deliverClientMessage(agentID, frame); err != nil {
		if errors.Is(err, errGateStopping) {
			return err
		}
		gate.logConnectionFailure(clientConn, "inbound_delivery", websocket.CloseInternalServerErr, err)
		if errors.Is(err, runtime.ErrMailboxFull) {
			gate.counters.inboundMailboxFull.Add(1)
		}
		clientConn.closeWith(websocket.CloseInternalServerErr, "server_error", time.Now().Add(closeFrameWait))
		return err
	}
	return nil
}

func (gate *GateService) deliverClientMessage(agentID runtime.ServiceID, frame []byte) error {
	gate.mu.RLock()
	defer gate.mu.RUnlock()
	if gate.stopping {
		return errGateStopping
	}
	return gate.runtime.DeliverClientMessage(agentID, frame)
}

func (gate *GateService) notifyDisconnected(serviceID runtime.ServiceID, clientConn *clientConnection) {
	retryDisconnect(gate.runtime, serviceID, gateDisconnectCommand, clientConn, func() bool {
		gate.mu.RLock()
		defer gate.mu.RUnlock()
		return gate.stopping
	})
}

func (gate *GateService) notifyMappedAgentsOnStop(ctx context.Context) error {
	pending := make([]runtime.ServiceID, 0, len(gate.agentsByConnection))
	for _, agentID := range gate.agentsByConnection {
		pending = append(pending, agentID)
	}
	return notifyAgentsOnStop(ctx, gate.runtime, pending)
}

func (gate *GateService) logError(message string, fields ...any) {
	gate.mu.Lock()
	logger := gate.logger
	gate.mu.Unlock()
	if logger != nil {
		_ = logger.Error(message, fields...)
	}
}

func (gate *GateService) logConnectionFailure(clientConn *clientConnection, category string, closeCode int, err error) {
	if clientConn == nil || clientConn.noteDiagnostic(category) {
		fields := []any{"category", category, "close_code", closeCode}
		if clientConn != nil {
			fields = append(fields, "connection_id", clientConn.connectionID)
		}
		fields = append(fields, safeWebSocketErrorFields(err)...)
		gate.logError("Gate connection failure", fields...)
	}
}

func (gate *GateService) flushConnectionDiagnostics(clientConn *clientConnection) {
	if clientConn == nil {
		return
	}
	for category, count := range clientConn.takeSuppressedDiagnostics() {
		gate.logError("Gate repeated failures", "connection_id", clientConn.connectionID,
			"category", category, "suppressed", count)
	}
}

func handshakeErrorCategory(err error) string {
	switch {
	case errors.Is(err, errHandshakeFrameTooLarge):
		return "frame_too_large"
	case errors.Is(err, errHandshakeKeyGeneration):
		return "key_generation"
	default:
		return "protocol"
	}
}

func closeCodeForHandshakeError(err error) int {
	switch {
	case errors.Is(err, errHandshakeFrameTooLarge):
		return websocket.CloseMessageTooBig
	case errors.Is(err, errHandshakeKeyGeneration):
		return websocket.CloseInternalServerErr
	default:
		return websocket.CloseProtocolError
	}
}

func authErrorCategory(err error) string {
	if errors.Is(err, errClientMACInvalid) {
		return "mac"
	}
	if errors.Is(err, errClientSequenceInvalid) {
		return "sequence"
	}
	return "authentication"
}

func (gate *GateService) beginHandler() (runtime.ServiceID, bool) {
	gate.mu.Lock()
	defer gate.mu.Unlock()
	if gate.stopping || gate.activeHandlers >= gate.config.MaxConnections {
		gate.counters.rejectedConnections.Add(1)
		return 0, false
	}
	gate.activeHandlers++
	gate.ioWorkers.Add(1)
	return gate.serviceID, true
}

func (gate *GateService) finishHandler() {
	gate.mu.Lock()
	gate.activeHandlers--
	gate.mu.Unlock()
}

func (gate *GateService) trackConnection(clientConn *clientConnection) bool {
	gate.mu.Lock()
	defer gate.mu.Unlock()
	if gate.stopping {
		return false
	}
	gate.connections[clientConn] = struct{}{}
	return true
}

func (gate *GateService) untrackConnection(clientConn *clientConnection) {
	gate.mu.Lock()
	delete(gate.connections, clientConn)
	gate.activeHandlers--
	gate.mu.Unlock()
}

func validateListenAddress(address string) error {
	_, portText, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("%w: Address must be a valid TCP host:port", ErrInvalidConfig)
	}
	for _, digit := range portText {
		if digit < '0' || digit > '9' {
			return fmt.Errorf("%w: Address port must be numeric", ErrInvalidConfig)
		}
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port < 0 || port > 65535 {
		return fmt.Errorf("%w: Address port is invalid", ErrInvalidConfig)
	}
	return nil
}

func validatePrivateAddress(address string) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("%w: Address must contain a literal private IP and port", ErrInvalidConfig)
	}
	ip := net.ParseIP(host)
	if ip == nil || (!ip.IsLoopback() && !ip.IsPrivate()) {
		return fmt.Errorf("%w: Plain mode requires a literal loopback or private IP", ErrInvalidConfig)
	}
	return nil
}

func productionTLSConfig(config *tls.Config) (*tls.Config, error) {
	if config == nil {
		return nil, nil
	}
	if config.GetConfigForClient != nil {
		return nil, fmt.Errorf("%w: GetConfigForClient cannot replace Gate TLS constraints", ErrInvalidConfig)
	}
	if len(config.Certificates) == 0 && config.GetCertificate == nil {
		return nil, fmt.Errorf("%w: TLSConfig must provide a server certificate", ErrInvalidConfig)
	}
	clone := config.Clone()
	if clone.MinVersion == 0 {
		clone.MinVersion = tls.VersionTLS12
	}
	if clone.MinVersion < tls.VersionTLS12 ||
		(clone.MaxVersion != 0 && (clone.MaxVersion < tls.VersionTLS12 || clone.MaxVersion < clone.MinVersion)) {
		return nil, fmt.Errorf("%w: TLS 1.2 or newer is required", ErrInvalidConfig)
	}
	clone.NextProtos = []string{"http/1.1"}
	return clone, nil
}

func clientConnArgument(args []any) (*clientConnection, error) {
	if len(args) != 1 {
		return nil, errors.New("gate: invalid connection event")
	}
	clientConn, ok := args[0].(*clientConnection)
	if !ok || clientConn == nil {
		return nil, errors.New("gate: invalid client connection")
	}
	return clientConn, nil
}

func isTimeoutError(err error) bool {
	var networkError net.Error
	return errors.As(err, &networkError) && networkError.Timeout()
}
