package cluster

import (
	"context"
	"errors"
	"testing"

	runtime "github.com/cnooljaw/gsr/runtime"
)

// 验证 ConnectionAgent 将 Call Request 交给本地适配器，并沿原路由写回 Response。
func TestConnectionAgentDispatchesCallResponse(t *testing.T) {
	requestPayload := []byte("request")
	written := make(chan ClusterEnvelope, 1)
	agent, err := NewConnectionAgent(ConnectionAgentConfig{
		Handler: func(_ context.Context, from NodeID, request ClusterEnvelope) ([]byte, error) {
			if from != "node-a" {
				t.Errorf("handler from = %q, want node-a", from)
			}
			if request.Session == 0 {
				t.Error("handler request session = 0, want non-zero")
			}
			request.Payload[0] = 'X'
			return []byte("response"), nil
		},
		Write: func(response ClusterEnvelope) error {
			written <- response
			return nil
		},
	})
	if err != nil {
		t.Fatalf("NewConnectionAgent() error = %v, want nil", err)
	}
	request := ClusterEnvelope{
		Kind:    FrameRequest,
		Session: 9,
		Target:  ServiceTarget{ServiceID: 7},
		Command: clusterTestCommand,
		Payload: requestPayload,
	}
	if err := agent.Handle(context.Background(), "node-a", request); err != nil {
		t.Fatalf("Handle() error = %v, want nil", err)
	}
	if string(request.Payload) != "request" {
		t.Fatalf("request payload = %q, want unchanged request", request.Payload)
	}
	response := <-written
	if response.Kind != FrameResponse || response.Session != request.Session {
		t.Fatalf("response kind/session = %s/%d, want response/%d", response.Kind, response.Session, request.Session)
	}
	if response.Target != request.Target || response.Command != request.Command {
		t.Fatalf("response route = %+v/%d, want %+v/%d", response.Target, response.Command, request.Target, request.Command)
	}
	if string(response.Payload) != "response" || response.Error != nil {
		t.Fatalf("response payload/error = %q/%v, want response/nil", response.Payload, response.Error)
	}
}

// 验证 Call Handler 的结构化错误会变成同一 Session 的 Error 帧。
func TestConnectionAgentDispatchesStructuredError(t *testing.T) {
	wantErr := runtime.NewStatusError(runtime.ErrorClassDomain, runtime.ErrorCode("room.full"), "room is full")
	written := make(chan ClusterEnvelope, 1)
	agent, err := NewConnectionAgent(ConnectionAgentConfig{
		Handler: func(context.Context, NodeID, ClusterEnvelope) ([]byte, error) {
			return nil, wantErr
		},
		Write: func(response ClusterEnvelope) error {
			written <- response
			return nil
		},
	})
	if err != nil {
		t.Fatalf("NewConnectionAgent() error = %v, want nil", err)
	}
	request := ClusterEnvelope{
		Kind:    FrameRequest,
		Session: 10,
		Target:  ServiceTarget{ServiceName: "room"},
		Command: clusterTestCommand,
	}
	if err := agent.Handle(context.Background(), "node-a", request); err != nil {
		t.Fatalf("Handle() error = %v, want nil", err)
	}
	response := <-written
	if response.Kind != FrameError || response.Session != request.Session {
		t.Fatalf("response kind/session = %s/%d, want error/%d", response.Kind, response.Session, request.Session)
	}
	if response.Payload != nil {
		t.Fatalf("error payload = %v, want nil", response.Payload)
	}
	if !errors.Is(response.Error, wantErr) {
		t.Fatalf("response error = %v, want %v", response.Error, wantErr)
	}
}

// 验证 Send Request 的 Handler 错误只归属接收节点，不产生远端 Response。
func TestConnectionAgentSuppressesSendError(t *testing.T) {
	writeCount := 0
	agent, err := NewConnectionAgent(ConnectionAgentConfig{
		Handler: func(context.Context, NodeID, ClusterEnvelope) ([]byte, error) {
			return nil, runtime.ErrHandlerFailed
		},
		Write: func(ClusterEnvelope) error {
			writeCount++
			return nil
		},
	})
	if err != nil {
		t.Fatalf("NewConnectionAgent() error = %v, want nil", err)
	}
	if err := agent.Handle(context.Background(), "node-a", ClusterEnvelope{
		Kind:    FrameRequest,
		Target:  ServiceTarget{ServiceID: 7},
		Command: clusterTestCommand,
	}); err != nil {
		t.Fatalf("Handle(send) error = %v, want nil", err)
	}
	if writeCount != 0 {
		t.Fatalf("write count = %d, want 0 for Send", writeCount)
	}
}

// 验证 Handler 的 panic 会转成安全的 ServicePanicked 错误帧，而不是传播到连接管理组件。
func TestConnectionAgentRecoversHandlerPanic(t *testing.T) {
	written := make(chan ClusterEnvelope, 1)
	agent, err := NewConnectionAgent(ConnectionAgentConfig{
		Handler: func(context.Context, NodeID, ClusterEnvelope) ([]byte, error) {
			panic("secret handler detail")
		},
		Write: func(response ClusterEnvelope) error {
			written <- response
			return nil
		},
	})
	if err != nil {
		t.Fatalf("NewConnectionAgent() error = %v, want nil", err)
	}
	request := ClusterEnvelope{
		Kind:    FrameRequest,
		Session: 11,
		Target:  ServiceTarget{ServiceID: 7},
		Command: clusterTestCommand,
	}
	if err := agent.Handle(context.Background(), "node-a", request); err != nil {
		t.Fatalf("Handle() error = %v, want nil", err)
	}
	response := <-written
	if !errors.Is(response.Error, runtime.ErrServicePanicked) {
		t.Fatalf("response error = %v, want ErrServicePanicked", response.Error)
	}
	var expectedStatus *runtime.StatusError
	if !errors.As(runtime.ErrServicePanicked, &expectedStatus) {
		t.Fatal("runtime.ErrServicePanicked is not a StatusError")
	}
	if response.Error.Message != expectedStatus.Message {
		t.Fatalf("response error message = %q, want safe message %q", response.Error.Message, expectedStatus.Message)
	}
}

// 验证 Agent 在构造和请求边界拒绝缺失依赖、空来源、错误帧和 nil context。
func TestConnectionAgentRejectsInvalidConfigAndRequest(t *testing.T) {
	validHandler := func(context.Context, NodeID, ClusterEnvelope) ([]byte, error) { return nil, nil }
	validWriter := func(ClusterEnvelope) error { return nil }
	if _, err := NewConnectionAgent(ConnectionAgentConfig{Write: validWriter}); !errors.Is(err, ErrInvalidConnectionAgentConfig) {
		t.Fatalf("missing Handler error = %v, want ErrInvalidConnectionAgentConfig", err)
	}
	if _, err := NewConnectionAgent(ConnectionAgentConfig{Handler: validHandler}); !errors.Is(err, ErrInvalidConnectionAgentConfig) {
		t.Fatalf("missing Write error = %v, want ErrInvalidConnectionAgentConfig", err)
	}
	agent, err := NewConnectionAgent(ConnectionAgentConfig{Handler: validHandler, Write: validWriter})
	if err != nil {
		t.Fatalf("NewConnectionAgent() error = %v, want nil", err)
	}
	validRequest := ClusterEnvelope{Kind: FrameRequest, Target: ServiceTarget{ServiceID: 7}, Command: clusterTestCommand}
	cases := []struct {
		name string
		ctx  context.Context
		from NodeID
		data ClusterEnvelope
	}{
		{name: "nil context", from: "node-a", data: validRequest},
		{name: "empty source", ctx: context.Background(), data: validRequest},
		{name: "wrong kind", ctx: context.Background(), from: "node-a", data: ClusterEnvelope{Kind: FrameResponse, Session: 1, Target: validRequest.Target, Command: validRequest.Command}},
		{name: "invalid target", ctx: context.Background(), from: "node-a", data: ClusterEnvelope{Kind: FrameRequest, Command: validRequest.Command}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if err := agent.Handle(testCase.ctx, testCase.from, testCase.data); !errors.Is(err, ErrInvalidConnectionAgentRequest) {
				t.Fatalf("Handle() error = %v, want ErrInvalidConnectionAgentRequest", err)
			}
		})
	}
	var nilAgent *ConnectionAgent
	if err := nilAgent.Handle(context.Background(), "node-a", validRequest); !errors.Is(err, ErrInvalidConnectionAgentConfig) {
		t.Fatalf("nil agent Handle() error = %v, want ErrInvalidConnectionAgentConfig", err)
	}
}

// 验证 ResponseWriter 的错误和 panic 都不会被静默吞掉，且暴露稳定写回错误。
func TestConnectionAgentReportsWriterFailure(t *testing.T) {
	writeErr := errors.New("write failed")
	agent, err := NewConnectionAgent(ConnectionAgentConfig{
		Handler: func(context.Context, NodeID, ClusterEnvelope) ([]byte, error) {
			return []byte("response"), nil
		},
		Write: func(ClusterEnvelope) error {
			return writeErr
		},
	})
	if err != nil {
		t.Fatalf("NewConnectionAgent() error = %v, want nil", err)
	}
	request := ClusterEnvelope{Kind: FrameRequest, Session: 12, Target: ServiceTarget{ServiceID: 7}, Command: clusterTestCommand}
	if err := agent.Handle(context.Background(), "node-a", request); !errors.Is(err, ErrConnectionAgentWrite) || !errors.Is(err, writeErr) {
		t.Fatalf("Handle() error = %v, want ErrConnectionAgentWrite and writeErr", err)
	}

	agent, err = NewConnectionAgent(ConnectionAgentConfig{
		Handler: func(context.Context, NodeID, ClusterEnvelope) ([]byte, error) {
			return []byte("response"), nil
		},
		Write: func(ClusterEnvelope) error {
			panic("secret writer detail")
		},
	})
	if err != nil {
		t.Fatalf("NewConnectionAgent(panic writer) error = %v, want nil", err)
	}
	if err := agent.Handle(context.Background(), "node-a", request); !errors.Is(err, ErrConnectionAgentWrite) {
		t.Fatalf("Handle(panic writer) error = %v, want ErrConnectionAgentWrite", err)
	}
}
