package cluster

import (
	"context"
	"errors"
	"io"
	"net"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/cnooljaw/gsr/runtime"
)

// 验证慢 Call 不阻塞同连接其它目标，取消连接后等待任务退出但不强杀业务 Handler。
func TestInboundConnectionIsolatesSlowCall(t *testing.T) {
	adapter, fastID, values := newLocalAdapterTest(t)
	blocked := &inboundRecordingService{entered: make(chan struct{}), release: make(chan struct{})}
	defer close(blocked.release)
	slowID, err := adapter.runtime.CreateService(context.Background(), blocked)
	if err != nil {
		t.Fatal(err)
	}
	owner, peer, frames, cancel, done := startInboundTest(t, adapter, 2)
	writeInboundTestRequest(t, peer, frames, values, slowID, 1, "block")
	select {
	case <-blocked.entered:
	case <-time.After(time.Second):
		t.Fatal("slow Handler did not start")
	}
	writeInboundTestRequest(t, peer, frames, values, fastID, 2, "fast")
	reply, err := frames.readFrame(peer)
	if err != nil || reply.Session != 2 || reply.Kind != FrameResponse {
		t.Fatalf("fast reply = %+v, %v", reply, err)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("connection did not stop while Handler remained blocked")
	}
	if _, err := adapter.runtime.Call(context.Background(), fastID, clusterTestCommand); err != nil {
		t.Fatal(err)
	}
	for failure := range owner.diagnostics.failures {
		if !errors.Is(failure.err, ErrConnectionAgentWrite) && !errors.Is(failure.err, context.Canceled) {
			t.Errorf("unexpected diagnostic: %v", failure.err)
		}
	}
}

// 验证 Call 名额用尽时暂停读取，名额释放后连续 Send 仍按本地接纳顺序执行。
func TestInboundConnectionBackpressureAndSendOrder(t *testing.T) {
	adapter, _, values := newLocalAdapterTest(t)
	service := &inboundRecordingService{entered: make(chan struct{}), release: make(chan struct{})}
	id, err := adapter.runtime.CreateService(context.Background(), service)
	if err != nil {
		t.Fatal(err)
	}
	_, peer, frames, _, _ := startInboundTest(t, adapter, 1)
	released := false
	defer func() {
		if !released {
			close(service.release)
		}
	}()
	writeInboundTestRequest(t, peer, frames, values, id, 1, "block")
	select {
	case <-service.entered:
	case <-time.After(time.Second):
		t.Fatal("Handler did not start")
	}
	if err := peer.SetWriteDeadline(time.Now().Add(30 * time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	// net.Pipe 无缓冲；零字节写入证明读循环尚未读取后续帧头。
	n, err := peer.Write([]byte{0, 0, 0, 1})
	if n != 0 || !errors.Is(err, os.ErrDeadlineExceeded) {
		t.Fatalf("full capacity write = %d, %v", n, err)
	}
	if err := peer.SetWriteDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatal(err)
	}
	close(service.release)
	released = true
	if _, err := frames.readFrame(peer); err != nil {
		t.Fatal(err)
	}
	writeInboundTestRequest(t, peer, frames, values, id, 0, int64(1))
	writeInboundTestRequest(t, peer, frames, values, id, 0, int64(2))
	writeInboundTestRequest(t, peer, frames, values, id, 2, int64(3))
	reply, err := frames.readFrame(peer)
	if err != nil {
		t.Fatal(err)
	}
	result, err := values.Decode(reply.Payload)
	if err != nil || !reflect.DeepEqual(result, []any{"block", int64(1), int64(2), int64(3)}) {
		t.Fatalf("order = %#v, %v", result, err)
	}
}

// 验证解码和名称解析 panic 只进入本地诊断，不使连接退出或泄露 panic 文本。
func TestInboundConnectionRecoversAdapterPanics(t *testing.T) {
	adapter, id, values := newLocalAdapterTest(t)
	adapter.codec = inboundFaultCodec{PayloadCodec: values}
	adapter.resolve = func(ServiceName) (runtime.ServiceID, error) { panic("private resolver detail") }
	owner, peer, frames, cancel, done := startInboundTest(t, adapter, 2)
	payload, err := values.Encode([]any{})
	if err != nil {
		t.Fatal(err)
	}
	for _, request := range []ClusterEnvelope{
		{Kind: FrameRequest, Target: ServiceTarget{ServiceID: id}, Command: clusterTestCommand + 1, Payload: []byte{255}},
		{Kind: FrameRequest, Target: ServiceTarget{ServiceName: "panic"}, Command: clusterTestCommand, Payload: payload},
	} {
		if err := frames.writeFrame(peer, request); err != nil {
			t.Fatal(err)
		}
	}
	encodePanicPayload, err := values.Encode([]any{"encode panic"})
	if err != nil {
		t.Fatal(err)
	}
	if err := frames.writeFrame(peer, ClusterEnvelope{Kind: FrameRequest, Session: 9, Target: ServiceTarget{ServiceID: id}, Command: clusterTestCommand + 2, Payload: encodePanicPayload}); err != nil {
		t.Fatal(err)
	}
	if reply, err := frames.readFrame(peer); err != nil || reply.Session != 9 || !errors.Is(reply.Error, runtime.ErrHandlerFailed) {
		t.Fatalf("encode panic reply = %+v, %v", reply, err)
	}
	writeInboundTestRequest(t, peer, frames, values, id, 1, "alive")
	if reply, err := frames.readFrame(peer); err != nil || reply.Kind != FrameResponse {
		t.Fatalf("reply = %+v, %v", reply, err)
	}
	cancel()
	<-done
	count := 0
	for failure := range owner.diagnostics.failures {
		if !errors.Is(failure.err, errInboundRequestPanicked) {
			t.Errorf("panic diagnostic = %v", failure.err)
		}
		count++
	}
	if count != 3 {
		t.Fatalf("panic diagnostics = %d", count)
	}
}

// 验证重复坏载荷只记录首条和关闭汇总，不挤满队列且合法 Call 继续处理。
func TestInboundConnectionSuppressesRepeatedBadFrames(t *testing.T) {
	adapter, id, values := newLocalAdapterTest(t)
	owner, peer, frames, cancel, done := startInboundTest(t, adapter, 1)
	for i := 0; i < defaultFailureQueueCapacity+3; i++ {
		err := frames.writeFrame(peer, ClusterEnvelope{Kind: FrameRequest, Target: ServiceTarget{ServiceID: id}, Command: clusterTestCommand + 1})
		if err != nil {
			t.Fatal(err)
		}
	}
	writeInboundTestRequest(t, peer, frames, values, id, 1, "alive")
	if _, err := frames.readFrame(peer); err != nil {
		t.Fatal(err)
	}
	cancel()
	<-done
	if owner.diagnostics.dropped.Load() != 0 {
		t.Fatalf("dropped = %d", owner.diagnostics.dropped.Load())
	}
	first := <-owner.diagnostics.failures
	summary := <-owner.diagnostics.failures
	if !errors.Is(first.err, ErrMalformedPayload) || summary.suppressed != defaultFailureQueueCapacity+2 {
		t.Fatalf("first=%+v summary=%+v", first, summary)
	}
	if len(owner.diagnostics.failures) != 0 {
		t.Fatal("repeated details retained")
	}
}

// 验证满载暂停读取时仍可主动取消，且不需要目标 Handler 返回才能回收连接任务。
func TestInboundConnectionCancellationAtCapacity(t *testing.T) {
	adapter, _, values := newLocalAdapterTest(t)
	blocked := &inboundRecordingService{entered: make(chan struct{}), release: make(chan struct{})}
	defer close(blocked.release)
	id, err := adapter.runtime.CreateService(context.Background(), blocked)
	if err != nil {
		t.Fatal(err)
	}
	_, peer, frames, cancel, done := startInboundTest(t, adapter, 1)
	writeInboundTestRequest(t, peer, frames, values, id, 1, "block")
	select {
	case <-blocked.entered:
	case <-time.After(time.Second):
		t.Fatal("Handler did not start")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("full inbound connection did not cancel")
	}
}

// 验证无效并发上限在启动前被拒绝，已运行过的连接不能重复启动读循环。
func TestInboundConnectionRejectsInvalidLifecycle(t *testing.T) {
	adapter, _, _ := newLocalAdapterTest(t)
	connection, _, _ := newTestStreamConnection(t)
	for _, limit := range []int{0, -1} {
		if _, err := newInboundConnection(connection, adapter, limit, newInboundDiagnostics()); !errors.Is(err, errInvalidInboundConnection) {
			t.Fatalf("invalid limit = %v", err)
		}
	}
	owner, err := newInboundConnection(connection, adapter, 1, newInboundDiagnostics())
	if err != nil {
		t.Fatal(err)
	}
	if err := owner.serve(nil); !errors.Is(err, errInvalidInboundConnection) {
		t.Fatalf("nil context = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := owner.serve(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled start = %v", err)
	}
	if err := owner.serve(context.Background()); !errors.Is(err, errInvalidInboundConnection) {
		t.Fatalf("repeated start = %v", err)
	}
}

// 验证一条连接退出后不会关闭共享出口，另一条连接仍能上报带连接信息的诊断。
func TestInboundConnectionKeepsSharedDiagnosticsOpen(t *testing.T) {
	adapter, id, _ := newLocalAdapterTest(t)
	diagnostics := newInboundDiagnostics()
	first, _, _ := newTestStreamConnection(t)
	firstOwner, err := newInboundConnection(first, adapter, 1, diagnostics)
	if err != nil {
		t.Fatal(err)
	}
	firstCtx, firstCancel := context.WithCancel(context.Background())
	firstCancel()
	if err := firstOwner.serve(firstCtx); !errors.Is(err, context.Canceled) {
		t.Fatalf("first exit = %v", err)
	}
	select {
	case _, open := <-diagnostics.failures:
		if !open {
			t.Fatal("connection closed shared diagnostics")
		}
	default:
	}
	second, peer, frames := newTestStreamConnection(t)
	secondOwner, err := newInboundConnection(second, adapter, 1, diagnostics)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- secondOwner.serve(ctx) }()
	defer func() {
		cancel()
		second.close()
		select {
		case <-done:
			close(diagnostics.failures)
		case <-time.After(time.Second):
			t.Error("second connection did not exit")
		}
	}()
	if err := frames.writeFrame(peer, ClusterEnvelope{Kind: FrameRequest, Session: 7, Target: ServiceTarget{ServiceID: id}, Command: clusterTestCommand + 1}); err != nil {
		t.Fatal(err)
	}
	select {
	case failure := <-diagnostics.failures:
		if failure.remoteAddress != peer.LocalAddr().String() || failure.session != 7 || !errors.Is(failure.err, ErrMalformedPayload) {
			t.Fatalf("second connection diagnostic = %+v", failure)
		}
	case <-time.After(time.Second):
		t.Fatal("missing second connection diagnostic")
	}
}

func startInboundTest(t *testing.T, adapter *localRequestAdapter, limit int) (*inboundConnection, net.Conn, *FrameCodec, context.CancelFunc, <-chan struct{}) {
	t.Helper()
	connection, peer, codec := newTestStreamConnection(t)
	owner, err := newInboundConnection(connection, adapter, limit, newInboundDiagnostics())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		// 测试组装方持有出口，单连接返回后才关闭它。
		defer close(owner.diagnostics.failures)
		if err := owner.serve(ctx); !errors.Is(err, context.Canceled) && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrClosedPipe) {
			t.Errorf("inbound loop exited unexpectedly: %v", err)
		}
	}()
	t.Cleanup(func() { cancel(); connection.close(); <-done })
	return owner, peer, codec, cancel, done
}

func writeInboundTestRequest(t *testing.T, peer net.Conn, frames *FrameCodec, values *ValueCodec, target runtime.ServiceID, session ClusterSessionID, arg any) {
	t.Helper()
	payload, err := values.Encode([]any{arg})
	if err != nil {
		t.Fatal(err)
	}
	if err := frames.writeFrame(peer, ClusterEnvelope{Kind: FrameRequest, Session: session, Target: ServiceTarget{ServiceID: target}, Command: clusterTestCommand, Payload: payload}); err != nil {
		t.Fatal(err)
	}
}

type inboundRecordingService struct {
	entered chan struct{}
	release chan struct{}
	seen    []any
}

func (*inboundRecordingService) OnStart(context.Context, runtime.ServiceContext) error {
	return nil
}

func (service *inboundRecordingService) Handle(_ runtime.CommandContext, _ runtime.CommandID, args ...any) (any, error) {
	if len(args) > 0 && args[0] == "block" {
		close(service.entered)
		<-service.release
	}
	service.seen = append(service.seen, args...)
	return append([]any(nil), service.seen...), nil
}

func (*inboundRecordingService) OnStop(context.Context) error {
	return nil
}

type inboundFaultCodec struct {
	PayloadCodec
}

func (codec inboundFaultCodec) Encode(value any) ([]byte, error) {
	if reflect.DeepEqual(value, []any{"encode panic"}) {
		panic("private encoding detail")
	}
	return codec.PayloadCodec.Encode(value)
}

func (codec inboundFaultCodec) Decode(payload []byte) (any, error) {
	if len(payload) == 1 && payload[0] == 255 {
		panic("private codec detail")
	}
	return codec.PayloadCodec.Decode(payload)
}
