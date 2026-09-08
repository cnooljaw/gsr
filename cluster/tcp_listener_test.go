package cluster

import (
	"context"
	"errors"
	"net"
	"reflect"
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
