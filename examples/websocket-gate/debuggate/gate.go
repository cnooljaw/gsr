package debuggate

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	gatecontract "github.com/cnooljaw/gsr/examples/websocket-gate/gate"
	"github.com/cnooljaw/gsr/runtime"
	"github.com/gorilla/websocket"
)

const (
	defaultMaxFrameBytes    = 4 << 20
	defaultReadHeaderWait   = 5 * time.Second
	defaultAgentStartWait   = 5 * time.Second
	disconnectRetryInterval = 5 * time.Millisecond
	defaultWriteWait        = 10 * time.Second
	closeFrameWait          = 100 * time.Millisecond

	queueEntryOverhead = 64
)

// Gate Service 的命令只处理连接建立和断开；客户端帧直接投递给 Agent。
const (
	gateConnectCommand runtime.CommandID = 0xffff_ff00 + iota
	gateDisconnectCommand
)

// ClientDisconnectedCommand 是 Gate 通知连接 Agent 已断开的 Core Command。
const ClientDisconnectedCommand = gatecontract.ClientDisconnectedCommand

var (
	// ErrInvalidConfig 表示 DebugGate 配置不完整或监听地址不安全。
	ErrInvalidConfig = errors.New("debuggate: invalid config")
	// ErrClientQueueFull 表示客户端出站队列达到配置的字节上限。
	ErrClientQueueFull = errors.New("debuggate: client outbound queue full")
	// ErrClientSessionClosed 表示连接已关闭，不能再发送消息。
	ErrClientSessionClosed = errors.New("debuggate: client session closed")
	// ErrFrameTooLarge 表示 Protobuf Envelope 超过单帧大小上限。
	ErrFrameTooLarge = errors.New("debuggate: frame too large")
)

var debugUpgrader = websocket.Upgrader{EnableCompression: false}

// ClientSession 是 Gate-Agent 共用的单连接出站能力。
type ClientSession = gatecontract.ClientSession

// Envelope 是 Gate-Agent 共用的 Protobuf 消息外层。
type Envelope = gatecontract.Envelope

// Config 定义仅供本地或可信内网调试的 WebSocket Gate 配置。
type Config struct {
	// Address 必须是回环或私有 IP 地址，端口 0 可供本机测试动态分配。
	Address string
	// MaxConnections 必须显式设置为正数。
	MaxConnections int
	// MaxFrameBytes 限制单条二进制消息；零值使用 4 MiB 默认值。
	MaxFrameBytes int
	// MaxOutboundQueueBytes 限制每个连接的出站队列字节数，必须显式设置。
	MaxOutboundQueueBytes int
	// NewAgent 为每条连接创建独立的 Agent Service。
	NewAgent func(ClientSession) runtime.Service
}

// Metrics 是 DebugGate 的有限容量计数快照。
type Metrics struct {
	// AcceptedConnections 是已创建 Agent 的连接数。
	AcceptedConnections uint64
	// RejectedConnections 是超容量、WebSocket 握手失败或无法创建 Agent 的连接数。
	RejectedConnections uint64
	// FrameTooLarge 是超过单帧限制的收发消息数。
	FrameTooLarge uint64
	// InboundMailboxFull 是 Gate 或 Agent Mailbox 拒绝入站消息的次数。
	InboundMailboxFull uint64
	// OutboundQueueFull 是客户端出站队列拒绝消息的次数。
	OutboundQueueFull uint64
}

type gateCounters struct {
	acceptedConnections atomic.Uint64
	rejectedConnections atomic.Uint64
	frameTooLarge       atomic.Uint64
	inboundMailboxFull  atomic.Uint64
	outboundQueueFull   atomic.Uint64
}

// GateService 是示例用 DebugGate；它通过显式注入的 Runtime 建立每连接 Agent。
type GateService struct {
	runtime *runtime.Runtime
	config  Config

	mu             sync.Mutex
	server         *http.Server
	serviceID      runtime.ServiceID
	address        net.Addr
	stopping       bool
	activeHandlers int
	connections    map[*clientConnection]struct{}
	serveErr       error
	ioWorkers      sync.WaitGroup
	counters       gateCounters

	// agentsByConnection 只用于连接断开时通知 Agent，不参与逐帧路由。
	agentsByConnection map[*clientConnection]runtime.ServiceID
}

// NewGateService 校验配置并创建示例用 DebugGate Service。
func NewGateService(local *runtime.Runtime, config Config) (*GateService, error) {
	if local == nil {
		return nil, fmt.Errorf("%w: Runtime is required", ErrInvalidConfig)
	}
	if config.MaxFrameBytes == 0 {
		config.MaxFrameBytes = defaultMaxFrameBytes
	}
	if config.MaxConnections <= 0 || config.MaxFrameBytes < 0 || config.MaxOutboundQueueBytes < queueEntryOverhead {
		return nil, fmt.Errorf("%w: connection, frame, or outbound queue limit is invalid", ErrInvalidConfig)
	}
	if config.NewAgent == nil {
		return nil, fmt.Errorf("%w: NewAgent is required", ErrInvalidConfig)
	}
	if err := validatePrivateAddress(config.Address); err != nil {
		return nil, err
	}

	return &GateService{
		runtime:            local,
		config:             config,
		connections:        make(map[*clientConnection]struct{}),
		agentsByConnection: make(map[*clientConnection]runtime.ServiceID),
	}, nil
}

// Addr 返回 Runtime 创建 GateService 后实际监听的地址。
func (gate *GateService) Addr() net.Addr {
	gate.mu.Lock()
	defer gate.mu.Unlock()
	return gate.address
}

// Metrics 返回 Gate 的容量和接纳计数快照。
func (gate *GateService) Metrics() Metrics {
	return Metrics{
		AcceptedConnections: gate.counters.acceptedConnections.Load(),
		RejectedConnections: gate.counters.rejectedConnections.Load(),
		FrameTooLarge:       gate.counters.frameTooLarge.Load(),
		InboundMailboxFull:  gate.counters.inboundMailboxFull.Load(),
		OutboundQueueFull:   gate.counters.outboundQueueFull.Load(),
	}
}

// OnStart 先绑定受限地址，再启动 Gate 自己拥有的 HTTP/WebSocket 接入循环。
func (gate *GateService) OnStart(_ context.Context, self runtime.ServiceContext) error {
	listener, err := net.Listen("tcp", gate.config.Address)
	if err != nil {
		return fmt.Errorf("debuggate: listen: %w", err)
	}
	server := &http.Server{
		Handler:           gate,
		ReadHeaderTimeout: defaultReadHeaderWait,
	}

	gate.mu.Lock()
	gate.server = server
	gate.serviceID = self.Self()
	gate.address = listener.Addr()
	gate.ioWorkers.Add(1)
	gate.mu.Unlock()

	go func() {
		defer gate.ioWorkers.Done()
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			gate.mu.Lock()
			gate.serveErr = err
			gate.mu.Unlock()
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
			return nil, errors.New("debuggate: duplicate client connection")
		}
		agent, err := createAgent(gate.config.NewAgent, clientConn)
		if err != nil {
			gate.counters.rejectedConnections.Add(1)
			return nil, err
		}
		startContext, cancel := context.WithTimeout(context.Background(), defaultAgentStartWait)
		defer cancel()
		agentID, err := self.CreateService(startContext, agent)
		if err != nil {
			gate.counters.rejectedConnections.Add(1)
			return nil, fmt.Errorf("debuggate: create Agent: %w", err)
		}
		gate.agentsByConnection[clientConn] = agentID
		gate.counters.acceptedConnections.Add(1)
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
		if err := self.Send(agentID, ClientDisconnectedCommand); err != nil {
			if isRuntimeStoppingError(err) {
				delete(gate.agentsByConnection, clientConn)
				return nil, nil
			}
			if errors.Is(err, runtime.ErrMailboxFull) {
				// 保留映射，让连接处理协程重试，避免断开通知被 Agent 背压吞掉。
				return nil, err
			}
			delete(gate.agentsByConnection, clientConn)
			return nil, fmt.Errorf("debuggate: notify Agent disconnect: %w", err)
		}
		delete(gate.agentsByConnection, clientConn)
		return nil, nil
	default:
		return nil, fmt.Errorf("debuggate: unknown internal command %d", command)
	}
}

// OnStop 关闭监听与所有 WebSocket，再等待 Gate 自己的接入和 I/O goroutine 退出。
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
	if deadline, ok := ctx.Deadline(); ok && deadline.Before(closeDeadline) {
		closeDeadline = deadline
	}
	for _, clientConn := range clientConnections {
		if err := clientConn.closeWith(websocket.CloseGoingAway, "going_away", closeDeadline); err != nil {
			result = errors.Join(result, fmt.Errorf("debuggate: close client connection: %w", err))
		}
	}
	gate.ioWorkers.Wait()
	result = errors.Join(result, gate.notifyMappedAgentsOnStop(ctx))
	clear(gate.agentsByConnection)

	gate.mu.Lock()
	serveErr := gate.serveErr
	gate.mu.Unlock()
	return errors.Join(result, serveErr)
}

// ServeHTTP 接收 WebSocket 二进制 Envelope，并由连接读循环直接投递给对应 Agent。
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
		http.Error(writer, "debug gate unavailable", http.StatusServiceUnavailable)
		return
	}

	var clientConn *clientConnection
	defer func() {
		if clientConn != nil {
			gate.untrackConnection(clientConn)
		} else {
			gate.finishHandler()
		}
		gate.ioWorkers.Done()
	}()

	connection, err := debugUpgrader.Upgrade(writer, request, nil)
	if err != nil {
		gate.counters.rejectedConnections.Add(1)
		return
	}
	clientConn = newClientConnection(connection, gate.config.MaxFrameBytes, gate.config.MaxOutboundQueueBytes, &gate.counters)
	if !gate.trackConnection(clientConn) {
		clientConn.closeWith(websocket.CloseGoingAway, "going_away", time.Now().Add(closeFrameWait))
		return
	}
	var agentID runtime.ServiceID
	defer func() {
		clientConn.closeWith(websocket.CloseNormalClosure, "normal", time.Now().Add(closeFrameWait))
		gate.notifyDisconnected(serviceID, clientConn)
	}()

	connectContext, cancel := context.WithTimeout(request.Context(), defaultAgentStartWait)
	result, err := gate.runtime.Call(connectContext, serviceID, gateConnectCommand, clientConn)
	cancel()
	if err != nil {
		gate.counters.rejectedConnections.Add(1)
		clientConn.closeWith(websocket.CloseInternalServerErr, "server_error", time.Now().Add(closeFrameWait))
		return
	}
	createdAgentID, ok := result.(runtime.ServiceID)
	if !ok || createdAgentID == 0 {
		gate.counters.rejectedConnections.Add(1)
		clientConn.closeWith(websocket.CloseInternalServerErr, "server_error", time.Now().Add(closeFrameWait))
		return
	}
	agentID = createdAgentID

	gate.ioWorkers.Add(1)
	go func() {
		defer gate.ioWorkers.Done()
		clientConn.writeLoop()
	}()

	connection.SetReadLimit(int64(gate.config.MaxFrameBytes))
	for {
		messageType, frame, err := connection.ReadMessage()
		if err != nil {
			if errors.Is(err, websocket.ErrReadLimit) {
				gate.counters.frameTooLarge.Add(1)
				clientConn.closeWith(websocket.CloseMessageTooBig, "message_too_big", time.Now().Add(closeFrameWait))
			}
			return
		}
		if messageType != websocket.BinaryMessage {
			clientConn.closeWith(websocket.CloseUnsupportedData, "unsupported_data", time.Now().Add(closeFrameWait))
			return
		}
		if err := gate.runtime.DeliverClientMessage(agentID, frame); err != nil {
			if errors.Is(err, runtime.ErrMailboxFull) {
				gate.counters.inboundMailboxFull.Add(1)
			}
			clientConn.closeWith(websocket.CloseInternalServerErr, "server_error", time.Now().Add(closeFrameWait))
			return
		}
	}
}

func (gate *GateService) notifyDisconnected(serviceID runtime.ServiceID, clientConn *clientConnection) {
	ticker := time.NewTicker(disconnectRetryInterval)
	defer ticker.Stop()
	for {
		attemptContext, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		_, err := gate.runtime.Call(attemptContext, serviceID, gateDisconnectCommand, clientConn)
		cancel()
		if err == nil || isRuntimeStoppingError(err) || !isDisconnectRetryableError(err) {
			return
		}
		select {
		case <-ticker.C:
		}
	}
}

func (gate *GateService) notifyMappedAgentsOnStop(ctx context.Context) error {
	pending := make([]runtime.ServiceID, 0, len(gate.agentsByConnection))
	for _, agentID := range gate.agentsByConnection {
		pending = append(pending, agentID)
	}
	sort.Slice(pending, func(left, right int) bool { return pending[left] < pending[right] })
	retry := make([]runtime.ServiceID, 0, len(pending))
	ticker := time.NewTicker(disconnectRetryInterval)
	defer ticker.Stop()
	var result error
	for len(pending) > 0 {
		retry = retry[:0]
		for _, agentID := range pending {
			if err := ctx.Err(); err != nil {
				return errors.Join(result, err)
			}
			err := gate.runtime.Send(agentID, ClientDisconnectedCommand)
			if err == nil || isRuntimeStoppingError(err) {
				continue
			}
			if errors.Is(err, runtime.ErrMailboxFull) {
				retry = append(retry, agentID)
				continue
			}
			result = errors.Join(result, fmt.Errorf("debuggate: notify Agent %d on Gate stop: %w", agentID, err))
		}
		// 交换两个缓冲切片，避免每轮重试重新分配 ServiceID 列表。
		pending, retry = retry, pending
		if len(pending) == 0 {
			return result
		}
		select {
		case <-ctx.Done():
			return errors.Join(result, ctx.Err())
		case <-ticker.C:
		}
	}
	return result
}

func isRuntimeStoppingError(err error) bool {
	return errors.Is(err, runtime.ErrServiceNotFound) ||
		errors.Is(err, runtime.ErrServiceClosed) ||
		errors.Is(err, runtime.ErrServiceStopping) ||
		errors.Is(err, runtime.ErrRuntimeStopping) ||
		errors.Is(err, runtime.ErrRuntimeStopped) ||
		errors.Is(err, runtime.ErrRuntimeClosed)
}

func isDisconnectRetryableError(err error) bool {
	return errors.Is(err, runtime.ErrMailboxFull) ||
		errors.Is(err, context.DeadlineExceeded) ||
		errors.Is(err, runtime.ErrCallTimeout)
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

func clientConnArgument(args []any) (*clientConnection, error) {
	if len(args) != 1 {
		return nil, errors.New("debuggate: invalid connection event")
	}
	clientConn, ok := args[0].(*clientConnection)
	if !ok || clientConn == nil {
		return nil, errors.New("debuggate: invalid client connection")
	}
	return clientConn, nil
}

func createAgent(factory func(ClientSession) runtime.Service, clientSession ClientSession) (service runtime.Service, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			service = nil
			err = fmt.Errorf("debuggate: Agent constructor panicked: %v", recovered)
		}
	}()
	service = factory(clientSession)
	if service == nil {
		return nil, errors.New("debuggate: Agent constructor returned nil")
	}
	return service, nil
}

func validatePrivateAddress(address string) error {
	host, portText, err := net.SplitHostPort(address)
	if err != nil || host == "" {
		return fmt.Errorf("%w: Address must contain a literal private IP and port", ErrInvalidConfig)
	}
	for _, digit := range portText {
		if digit < '0' || digit > '9' {
			return fmt.Errorf("%w: Address port is invalid", ErrInvalidConfig)
		}
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port < 0 || port > 65535 {
		return fmt.Errorf("%w: Address port is invalid", ErrInvalidConfig)
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return fmt.Errorf("%w: Address host must be a literal IP", ErrInvalidConfig)
	}
	if !ip.IsLoopback() && !ip.IsPrivate() {
		return fmt.Errorf("%w: DebugGate may bind only to loopback or private IP addresses", ErrInvalidConfig)
	}
	return nil
}
