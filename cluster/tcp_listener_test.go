package cluster

import (
	"context"
	"errors"
	"net"
	"reflect"
	"sync"
	"testing"
	"time"
)

// 验证 listener 接纳无来源身份的连接，满额拒绝新连接但不影响已接纳连接。
func TestTCPListenerBoundsConnections(t *testing.T) {
	owner, target, cancel, done := newTCPListenerTest(t, 1)
	defer cancel()
	first := dialListenerTest(t, owner)
	listenerCallTest(t, first, target, 1)
	second := dialListenerTest(t, owner)
	select {
	case failure := <-owner.diagnostics.failures:
		if !errors.Is(failure.err, errInboundConnectionLimit) || failure.remoteAddress == "" {
			t.Fatalf("connection rejection = %+v", failure)
		}
	case <-time.After(time.Second):
		t.Fatal("missing connection limit diagnostic")
	}
	if _, err := second.read(); err == nil {
		t.Fatal("excess connection remains open")
	}
	listenerCallTest(t, first, target, 2)
	cancel()
	awaitListenerTest(t, done)
	if _, err := first.read(); err == nil {
		t.Fatal("accepted connection remains open after cancellation")
	}
}

// 验证关闭监听组件能解除阻塞的 Accept，不要求曾经接入客户端。
func TestTCPListenerCancelIdleAccept(t *testing.T) {
	_, _, cancel, done := newTCPListenerTest(t, 2)
	cancel()
	awaitListenerTest(t, done)
}

// 验证停止接纳拒绝新连接及旧连接的新请求，同时让已接纳 Call 沿原连接回复。
func TestTCPListenerQuiescePreservesReply(t *testing.T) {
	owner, target, cancel, done := newTCPListenerTest(t, 2)
	blocked := &inboundRecordingService{entered: make(chan struct{}), release: make(chan struct{})}
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(blocked.release) }) }
	defer release()
	id, err := owner.config.adapter.runtime.CreateService(context.Background(), blocked)
	if err != nil {
		t.Fatal(err)
	}
	counter := &tcpCountingService{inboundRecordingService: &inboundRecordingService{}}
	counterID, err := owner.config.adapter.runtime.CreateService(context.Background(), counter)
	if err != nil {
		t.Fatal(err)
	}
	connection := dialListenerTest(t, owner)
	idle := dialListenerTest(t, owner)
	listenerCallTest(t, idle, target, 10)
	values, err := NewValueCodec(ValueCodecConfig{})
	if err != nil {
		t.Fatal(err)
	}
	payload, err := values.Encode([]any{"block"})
	if err != nil {
		t.Fatal(err)
	}
	request := ClusterEnvelope{Kind: FrameRequest, Session: 1, Target: ServiceTarget{ServiceID: id}, Command: clusterTestCommand, Payload: payload}
	if err := connection.write(request); err != nil {
		t.Fatal(err)
	}
	select {
	case <-blocked.entered:
	case <-time.After(time.Second):
		t.Fatal("Handler did not start")
	}
	if err := owner.quiesce(); err != nil {
		t.Fatal(err)
	}
	if err := owner.quiesce(); err != nil {
		t.Fatal(err)
	}
	extra, err := net.DialTimeout("tcp4", owner.config.listener.Addr().String(), time.Second)
	if err == nil {
		extra.Close()
		t.Fatal("listener still accepts connections")
	}
	request.Session = 2
	request.Target = ServiceTarget{ServiceID: counterID}
	request.Payload, err = values.Encode([]any{"after-stop"})
	if err != nil {
		t.Fatal(err)
	}
	if err := connection.write(request); err != nil {
		t.Fatal(err)
	}
	// 新 Send 也不能从另一条已存在的连接进入业务。
	request.Session = 0
	if err := idle.write(request); err != nil {
		t.Fatal(err)
	}
	release()
	response, err := connection.read()
	if err != nil || response.Kind != FrameResponse || response.Session != 1 {
		t.Fatalf("admitted reply = %+v, %v", response, err)
	}
	if err := connection.socket.SetReadDeadline(time.Now().Add(30 * time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	if _, err := connection.read(); err == nil {
		t.Fatal("new request was handled after quiesce")
	} else {
		var timeout net.Error
		if !errors.As(err, &timeout) || !timeout.Timeout() {
			t.Fatalf("connection closed before final cleanup: %v", err)
		}
	}
	select {
	case err := <-done:
		t.Fatalf("quiesce terminated owner: %v", err)
	default:
	}
	cancel()
	awaitListenerTest(t, done)
	// 本地 Call 作为 Mailbox 屏障：若错误投递了前面的 Send，它必已执行。
	if _, err := owner.config.adapter.runtime.Call(context.Background(), counterID, clusterTestCommand); err != nil {
		t.Fatal(err)
	}
	if counter.invocations.Load() != 1 {
		t.Fatalf("post-quiesce requests reached Handler: %d", counter.invocations.Load()-1)
	}
	for failure := range owner.diagnostics.failures {
		t.Fatalf("unexpected shutdown diagnostic: %+v", failure)
	}
}

// 验证停止接纳后最终取消能解除满载 Call 名额等待，不等待被阻塞的业务 Handler。
func TestTCPListenerQuiesceCancelSaturatedCalls(t *testing.T) {
	owner, _, cancel, done := newTCPListenerTest(t, 1)
	connection := dialListenerTest(t, owner)
	values, err := NewValueCodec(ValueCodecConfig{})
	if err != nil {
		t.Fatal(err)
	}
	payload, err := values.Encode([]any{"block"})
	if err != nil {
		t.Fatal(err)
	}
	for session := ClusterSessionID(1); session <= 2; session++ {
		blocked := &inboundRecordingService{entered: make(chan struct{}), release: make(chan struct{})}
		defer close(blocked.release)
		id, err := owner.config.adapter.runtime.CreateService(context.Background(), blocked)
		if err != nil {
			t.Fatal(err)
		}
		if err := connection.write(ClusterEnvelope{Kind: FrameRequest, Session: session, Target: ServiceTarget{ServiceID: id}, Command: clusterTestCommand, Payload: payload}); err != nil {
			t.Fatal(err)
		}
		select {
		case <-blocked.entered:
		case <-time.After(time.Second):
			t.Fatal("Handler did not start")
		}
	}
	var stops sync.WaitGroup
	for index := 0; index < 8; index++ {
		stops.Add(1)
		go func() {
			defer stops.Done()
			if err := owner.quiesce(); err != nil {
				t.Error(err)
			}
		}()
	}
	stops.Wait()
	cancel()
	awaitListenerTest(t, done)
}

// 验证停止空闲监听后仍等待最终取消，既不提前结束 owner，也不泄漏 Accept 任务。
func TestTCPListenerQuiesceIdle(t *testing.T) {
	owner, _, cancel, done := newTCPListenerTest(t, 1)
	if err := owner.quiesce(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		t.Fatalf("quiesce terminated owner: %v", err)
	default:
	}
	cancel()
	awaitListenerTest(t, done)
}

// 验证坏帧关闭只归属当前连接，其它入站连接仍能调用目标 Service。
func TestTCPListenerIsolatesBrokenConnection(t *testing.T) {
	owner, target, _, _ := newTCPListenerTest(t, 2)
	bad := dialListenerTest(t, owner)
	good := dialListenerTest(t, owner)
	listenerCallTest(t, good, target, 1)
	if _, err := bad.socket.Write([]byte{0, 0, 0, 0}); err != nil {
		t.Fatal(err)
	}
	select {
	case failure := <-owner.diagnostics.failures:
		if !errors.Is(failure.err, ErrMalformedFrame) || failure.remoteAddress == "" {
			t.Fatalf("bad frame diagnostic = %+v", failure)
		}
	case <-time.After(time.Second):
		t.Fatal("missing bad connection diagnostic")
	}
	listenerCallTest(t, good, target, 2)
}

// 验证 listener 关闭会取消连接内的 Call 等待，不等待或强杀仍在执行的业务 Handler。
func TestTCPListenerCancelPendingCall(t *testing.T) {
	owner, _, cancel, done := newTCPListenerTest(t, 1)
	blocked := &inboundRecordingService{entered: make(chan struct{}), release: make(chan struct{})}
	defer close(blocked.release)
	id, err := owner.config.adapter.runtime.CreateService(context.Background(), blocked)
	if err != nil {
		t.Fatal(err)
	}
	connection := dialListenerTest(t, owner)
	values, err := NewValueCodec(ValueCodecConfig{})
	if err != nil {
		t.Fatal(err)
	}
	payload, err := values.Encode([]any{"block"})
	if err != nil {
		t.Fatal(err)
	}
	if err := connection.write(ClusterEnvelope{Kind: FrameRequest, Session: 1, Target: ServiceTarget{ServiceID: id}, Command: clusterTestCommand, Payload: payload}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-blocked.entered:
	case <-time.After(time.Second):
		t.Fatal("Handler did not start")
	}
	cancel()
	awaitListenerTest(t, done)
}

// 验证无人消费且诊断队列满载时，监听仍能拒绝超额连接并服务已有连接。
func TestTCPListenerDropsDiagnosticsWithoutBlocking(t *testing.T) {
	owner, target, cancel, done := newTCPListenerTest(t, 1)
	first := dialListenerTest(t, owner)
	listenerCallTest(t, first, target, 1)
	for index := 0; index < defaultFailureQueueCapacity+3; index++ {
		extra := dialListenerTest(t, owner)
		if _, err := extra.read(); err == nil {
			t.Fatal("excess connection remains open")
		}
	}
	listenerCallTest(t, first, target, 2)
	cancel()
	awaitListenerTest(t, done)
	if owner.diagnostics.dropped.Load() != 3 {
		t.Fatalf("dropped = %d, want 3", owner.diagnostics.dropped.Load())
	}
}

// 验证缺失依赖和非正数上限被拒绝，nil context 与重复启动不创建额外接纳循环。
func TestTCPListenerRejectsInvalidConfigAndRestart(t *testing.T) {
	owner, _, cancel, done := newTCPListenerTest(t, 1)
	for _, invalidate := range []func(*tcpListenerConfig){
		func(config *tcpListenerConfig) { config.listener = nil },
		func(config *tcpListenerConfig) { config.codec = nil },
		func(config *tcpListenerConfig) { config.adapter = nil },
		func(config *tcpListenerConfig) { config.maxConnections = 0 },
		func(config *tcpListenerConfig) { config.maxCalls = 0 },
		func(config *tcpListenerConfig) { config.writeTimeout = 0 },
	} {
		config := owner.config
		invalidate(&config)
		if _, err := newTCPListener(config); !errors.Is(err, errInvalidTCPListener) {
			t.Fatalf("invalid config = %v", err)
		}
	}
	if err := owner.serve(nil); !errors.Is(err, errInvalidTCPListener) {
		t.Fatalf("nil context = %v", err)
	}
	cancel()
	awaitListenerTest(t, done)
	if err := owner.serve(context.Background()); !errors.Is(err, errInvalidTCPListener) {
		t.Fatalf("restart = %v", err)
	}
}

func newTCPListenerTest(t *testing.T, maxConnections int) (*tcpListener, ServiceTarget, context.CancelFunc, <-chan error) {
	t.Helper()
	adapter, id, _ := newLocalAdapterTest(t)
	socket, err := net.ListenTCP("tcp4", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { socket.Close() })
	codec, err := NewFrameCodec(FrameCodecConfig{})
	if err != nil {
		t.Fatal(err)
	}
	owner, err := newTCPListener(tcpListenerConfig{
		listener: socket, codec: codec, adapter: adapter,
		maxConnections: maxConnections, maxCalls: 2, writeTimeout: time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		done <- owner.serve(ctx)
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-finished:
		case <-time.After(2 * time.Second):
			t.Error("listener owner did not exit")
		}
	})
	return owner, ServiceTarget{ServiceID: id}, cancel, done
}

func dialListenerTest(t *testing.T, owner *tcpListener) *streamConnection {
	t.Helper()
	socket, err := net.DialTimeout("tcp4", owner.config.listener.Addr().String(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { socket.Close() })
	if err := socket.SetDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatal(err)
	}
	connection, err := newStreamConnection(socket, owner.config.codec, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	return connection
}

func listenerCallTest(t *testing.T, connection *streamConnection, target ServiceTarget, session ClusterSessionID) {
	t.Helper()
	values, err := NewValueCodec(ValueCodecConfig{})
	if err != nil {
		t.Fatal(err)
	}
	payload, err := values.Encode([]any{"hello"})
	if err != nil {
		t.Fatal(err)
	}
	if err := connection.write(ClusterEnvelope{Kind: FrameRequest, Session: session, Target: target, Command: clusterTestCommand, Payload: payload}); err != nil {
		t.Fatal(err)
	}
	response, err := connection.read()
	if err != nil {
		t.Fatal(err)
	}
	value, err := values.Decode(response.Payload)
	if err != nil || response.Kind != FrameResponse || response.Session != session || !reflect.DeepEqual(value, []any{"hello"}) {
		t.Fatalf("listener response = %+v, %#v, %v", response, value, err)
	}
}

func awaitListenerTest(t *testing.T, done <-chan error) {
	t.Helper()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("listener exit = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("listener did not finish")
	}
}
