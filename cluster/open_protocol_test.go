package cluster

import (
	"context"
	"errors"
	"io"
	"net"
	"reflect"
	"testing"
	"time"
)

// 验证 TCP 半关闭造成的残缺帧不进入 Handler、不携带未验证字段，且其它连接继续调用。
func TestClusterOpenTruncatedFrameIsolatesConnection(t *testing.T) {
	for _, part := range []string{"header", "body"} {
		t.Run(part, func(t *testing.T) {
			adapter, _, codec := newLocalAdapterTest(t)
			service := &tcpCountingService{inboundRecordingService: &inboundRecordingService{}}
			id, err := adapter.runtime.CreateService(context.Background(), service)
			if err != nil {
				t.Fatal(err)
			}
			server, client, caller := newOpenBoundaryLink(t, adapter, nil)
			socket, err := net.DialTimeout("tcp", caller.config.Nodes["b"], time.Second)
			if err != nil {
				t.Fatal(err)
			}
			defer socket.Close()
			if err := socket.SetDeadline(time.Now().Add(2 * time.Second)); err != nil {
				t.Fatal(err)
			}
			payload, err := codec.Encode([]any{"must-not-run"})
			if err != nil {
				t.Fatal(err)
			}
			frame, err := server.frames.Encode(ClusterEnvelope{Kind: FrameRequest, Session: 123, Target: ServiceTarget{ServiceID: id}, Command: clusterTestCommand, Payload: payload})
			if err != nil {
				t.Fatal(err)
			}
			if part == "header" {
				frame = frame[:2]
			} else {
				// 保留看似完整的 Session、Target 和 Command，仅截掉正文末尾。
				frame = frame[:len(frame)-1]
			}
			if err := writeFrameBytes(socket, frame); err != nil {
				t.Fatal(err)
			}
			// 只关闭发送方向，接收方向仍可观察服务端主动断连。
			if err := socket.(*net.TCPConn).CloseWrite(); err != nil {
				t.Fatal(err)
			}
			var probe [1]byte
			if n, err := socket.Read(probe[:]); n != 0 || !errors.Is(err, io.EOF) {
				t.Fatalf("read after truncation: n=%d err=%v", n, err)
			}
			select {
			case event := <-server.Diagnostics():
				if event.Kind != DiagnosticInboundFailure || !errors.Is(event.Err, io.ErrUnexpectedEOF) || event.Session != 0 || event.Command != 0 || event.Target != (ServiceTarget{}) || event.RemoteAddress != socket.LocalAddr().String() {
					t.Fatalf("truncation diagnostic=%+v", event)
				}
			case <-time.After(time.Second):
				t.Fatal("missing truncation diagnostic")
			}
			if service.invocations.Load() != 0 || server.SuppressedDiagnosticCount() != 0 {
				t.Fatal("truncated request invoked Handler or was treated as duplicate")
			}
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			value, err := client.Call(ctx, "b", ServiceTarget{ServiceID: id}, clusterTestCommand, "healthy")
			if err != nil || !reflect.DeepEqual(value, []any{"healthy"}) || service.invocations.Load() != 1 {
				t.Fatalf("healthy call: value=%v err=%v calls=%d", value, err, service.invocations.Load())
			}
		})
	}
}

// 验证完整坏载荷仅丢弃该请求，同连接下一条合法 Call 仍正常回复。
func TestClusterOpenMalformedPayloadKeepsConnection(t *testing.T) {
	adapter, id, codec := newLocalAdapterTest(t)
	server, _, caller := newOpenBoundaryLink(t, adapter, nil)
	socket, err := net.DialTimeout("tcp", caller.config.Nodes["b"], time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer socket.Close()
	if err := socket.SetDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	request := ClusterEnvelope{Kind: FrameRequest, Session: 10, Target: ServiceTarget{ServiceID: id}, Command: clusterTestCommand, Payload: []byte{255}}
	if err := server.frames.writeFrame(socket, request); err != nil {
		t.Fatal(err)
	}
	select {
	case event := <-server.Diagnostics():
		if event.Kind != DiagnosticInboundFailure || !errors.Is(event.Err, ErrMalformedPayload) || event.Session != request.Session || event.Command != request.Command || event.Target != request.Target {
			t.Fatalf("event=%+v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("missing malformed payload diagnostic")
	}
	request.Session = 11
	request.Payload, err = codec.Encode([]any{"alive"})
	if err != nil {
		t.Fatal(err)
	}
	if err := server.frames.writeFrame(socket, request); err != nil {
		t.Fatal(err)
	}
	reply, err := server.frames.readFrame(socket)
	if err != nil || reply.Kind != FrameResponse || reply.Session != 11 {
		t.Fatalf("reply=%+v err=%v", reply, err)
	}
	value, err := codec.Decode(reply.Payload)
	if err != nil || !reflect.DeepEqual(value, []any{"alive"}) {
		t.Fatalf("value=%v err=%v", value, err)
	}
}

// 验证超限长度在读取正文前断开该连接，诊断无伪造请求字段，监听仍能服务其它连接。
func TestClusterOpenOversizedFrameIsolatesConnection(t *testing.T) {
	adapter, id, _ := newLocalAdapterTest(t)
	server, client, caller := newOpenBoundaryLink(t, adapter, nil)
	socket, err := net.DialTimeout("tcp", caller.config.Nodes["b"], time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer socket.Close()
	if err := socket.SetDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	// 只发送超限长度前缀；服务端必须立即拒绝，不能等待声明的巨大正文。
	if _, err := socket.Write([]byte{255, 255, 255, 255}); err != nil {
		t.Fatal(err)
	}
	var probe [1]byte
	_, err = socket.Read(probe[:])
	if err == nil {
		t.Fatal("oversized connection remained readable")
	}
	var networkErr net.Error
	if errors.As(err, &networkErr) && networkErr.Timeout() {
		t.Fatalf("server waited for oversized body: %v", err)
	}
	if !errors.Is(err, io.EOF) && !errors.As(err, &networkErr) {
		t.Fatalf("unexpected read error: %v", err)
	}
	select {
	case event := <-server.Diagnostics():
		if event.Kind != DiagnosticInboundFailure || !errors.Is(event.Err, ErrFrameTooLarge) || event.Session != 0 || event.Command != 0 || event.Target != (ServiceTarget{}) || event.RemoteNode != "" || event.RemoteAddress != socket.LocalAddr().String() {
			t.Fatalf("event=%+v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("missing oversized frame diagnostic")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	value, err := client.Call(ctx, "b", ServiceTarget{ServiceID: id}, clusterTestCommand, "healthy")
	if err != nil || !reflect.DeepEqual(value, []any{"healthy"}) {
		t.Fatalf("healthy connection value=%v err=%v", value, err)
	}
}
