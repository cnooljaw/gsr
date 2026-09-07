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
	for failure := range owner.failures {
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
	adapter.codecs = MustCommandCodecRegistry([]CommandCodecEntry{
		{CommandID: clusterTestCommand, Codec: values},
		{CommandID: clusterTestCommand + 1, Codec: inboundPanicCodec{}},
		{CommandID: clusterTestCommand + 2, Codec: inboundPanicEncodeCodec{CommandCodec: values}},
	})
	adapter.resolve = func(ServiceName) (runtime.ServiceID, error) { panic("private resolver detail") }
	owner, peer, frames, cancel, done := startInboundTest(t, adapter, 2)
	payload, err := values.Encode([]any{})
	if err != nil {
		t.Fatal(err)
	}
	for _, request := range []ClusterEnvelope{
		{Kind: FrameRequest, Target: ServiceTarget{ServiceID: id}, Command: clusterTestCommand + 1, Payload: payload},
		{Kind: FrameRequest, Target: ServiceTarget{ServiceName: "panic"}, Command: clusterTestCommand, Payload: payload},
	} {
		if err := frames.writeFrame(peer, request); err != nil {
			t.Fatal(err)
		}
	}
	if err := frames.writeFrame(peer, ClusterEnvelope{Kind: FrameRequest, Session: 9, Target: ServiceTarget{ServiceID: id}, Command: clusterTestCommand + 2, Payload: payload}); err != nil {
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
	for failure := range owner.failures {
		if !errors.Is(failure.err, errInboundRequestPanicked) {
			t.Errorf("panic diagnostic = %v", failure.err)
		}
		count++
	}
	if count != 3 {
		t.Fatalf("panic diagnostics = %d", count)
	}
}

// 验证慢诊断消费者不会阻塞入站读取，超出固定容量的事件可通过计数观测。
func TestInboundConnectionDropsExcessDiagnostics(t *testing.T) {
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
	if owner.dropped.Load() != 3 {
		t.Fatalf("dropped = %d", owner.dropped.Load())
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
		if _, err := newInboundConnection(connection, "node-a", adapter, limit); !errors.Is(err, errInvalidInboundConnection) {
			t.Fatalf("invalid limit = %v", err)
		}
	}
	owner, err := newInboundConnection(connection, "node-a", adapter, 1)
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

func startInboundTest(t *testing.T, adapter *localRequestAdapter, limit int) (*inboundConnection, net.Conn, *FrameCodec, context.CancelFunc, <-chan struct{}) {
	t.Helper()
	connection, peer, codec := newTestStreamConnection(t)
	owner, err := newInboundConnection(connection, "node-a", adapter, limit)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
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

type inboundPanicCodec struct{}

func (inboundPanicCodec) Encode(any) ([]byte, error) {
	panic("private codec detail")
}

func (inboundPanicCodec) Decode([]byte) (any, error) {
	panic("private codec detail")
}

type inboundPanicEncodeCodec struct {
	CommandCodec
}

func (inboundPanicEncodeCodec) Encode(any) ([]byte, error) {
	panic("private encoding detail")
}
