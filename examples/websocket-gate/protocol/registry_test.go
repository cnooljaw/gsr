package protocol

import (
	"errors"
	"testing"

	clientv1 "github.com/cnooljaw/gsr/examples/websocket-gate/gen/go/gsr/client/v1"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"
)

// TestRegistryDecodesTypedRequest 验证注册的客户端 Request 会解码为指定 Protobuf 类型并带出响应编号。
func TestRegistryDecodesTypedRequest(t *testing.T) {
	registry := newTestRegistry(t)
	encodedPayload, err := proto.Marshal(&emptypb.Empty{})
	if err != nil {
		t.Fatalf("marshal request payload: %v", err)
	}

	message, err := registry.Decode(ClientToServer, &clientv1.Envelope{
		MsgId:     1001,
		RequestId: 17,
		Payload:   encodedPayload,
	})
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if message.MsgID != 1001 || message.RequestID != 17 || message.Kind != Request || message.ResponseMsgID != 1002 {
		t.Fatalf("Decode() metadata = %+v", message)
	}
	if _, ok := message.Payload.(*emptypb.Empty); !ok {
		t.Fatalf("Decode() payload type = %T, want *emptypb.Empty", message.Payload)
	}
}

// TestRegistryRejectsMalformedPayload 验证已知 msg_id 携带非法 Protobuf 字节时返回 payload 错误。
func TestRegistryRejectsMalformedPayload(t *testing.T) {
	registry := newTestRegistry(t)
	_, err := registry.Decode(ClientToServer, &clientv1.Envelope{
		MsgId:     1001,
		RequestId: 17,
		Payload:   []byte{0xff},
	})
	if !errors.Is(err, ErrInvalidPayload) {
		t.Fatalf("Decode() error = %v, want ErrInvalidPayload", err)
	}
}

// TestRegistryRejectsNilEnvelopeAndInvalidDirection 验证缺失 Envelope 或未定义方向不会进入消息查找与解码。
func TestRegistryRejectsNilEnvelopeAndInvalidDirection(t *testing.T) {
	registry := newTestRegistry(t)
	if _, err := registry.Decode(ClientToServer, nil); !errors.Is(err, ErrInvalidPayload) {
		t.Fatalf("Decode(nil) error = %v, want ErrInvalidPayload", err)
	}
	if _, err := registry.Decode(0, &clientv1.Envelope{MsgId: 1001, RequestId: 1}); !errors.Is(err, ErrWrongDirection) {
		t.Fatalf("Decode(invalid direction) error = %v, want ErrWrongDirection", err)
	}
}

// TestRegistryRejectsInvalidDirectionAndRequestID 验证方向和 request_id 错误会在 payload 分发前拒绝。
func TestRegistryRejectsInvalidDirectionAndRequestID(t *testing.T) {
	registry := newTestRegistry(t)
	tests := []struct {
		name      string
		direction Direction
		msgID     uint32
		requestID uint32
		wantError error
	}{
		{name: "wrong direction", direction: ServerToClient, msgID: 1001, requestID: 1, wantError: ErrWrongDirection},
		{name: "unknown message", direction: ClientToServer, msgID: 7777, requestID: 1, wantError: ErrUnknownMessage},
		{name: "reserved handshake message", direction: ClientToServer, msgID: uint32(clientv1.MsgId_C2S_KEY_EXCHANGE_REQUEST), requestID: 1, wantError: ErrUnknownMessage},
		{name: "request without id", direction: ClientToServer, msgID: 1001, wantError: ErrInvalidRequestID},
		{name: "notification with id", direction: ClientToServer, msgID: 2001, requestID: 1, wantError: ErrInvalidRequestID},
		{name: "push with id", direction: ServerToClient, msgID: 3001, requestID: 1, wantError: ErrInvalidRequestID},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := registry.Decode(test.direction, &clientv1.Envelope{
				MsgId:     test.msgID,
				RequestId: test.requestID,
			})
			if !errors.Is(err, test.wantError) {
				t.Fatalf("Decode() error = %v, want %v", err, test.wantError)
			}
		})
	}
}

// TestRegistryEncodesResponsesAndGenericErrors 验证专属响应遵守请求映射，通用错误只依赖 Envelope 的 request_id。
func TestRegistryEncodesResponsesAndGenericErrors(t *testing.T) {
	registry := newTestRegistry(t)
	tests := []Message{
		{
			MsgID:        1002,
			RequestID:    21,
			RequestMsgID: 1001,
			Payload:      &emptypb.Empty{},
		},
		{
			MsgID:     9001,
			RequestID: 22,
			Payload:   &clientv1.ErrorResponse{Code: 7, Message: "unavailable"},
		},
	}
	for _, message := range tests {
		envelope, err := registry.Encode(ServerToClient, message)
		if err != nil {
			t.Fatalf("Encode(%d) error = %v", message.MsgID, err)
		}
		if envelope.GetMsgId() != message.MsgID || envelope.GetRequestId() != message.RequestID {
			t.Fatalf("Encode() envelope = %+v, want msg_id=%d request_id=%d", envelope, message.MsgID, message.RequestID)
		}
	}
}

// TestRegistryRejectsUnmatchedResponseAndPayload 验证响应关联错误与错误 Protobuf 类型均不能编码。
func TestRegistryRejectsUnmatchedResponseAndPayload(t *testing.T) {
	registry := newTestRegistry(t)
	tests := []struct {
		name      string
		direction Direction
		message   Message
		wantError error
	}{
		{
			name:      "unknown request association",
			direction: ServerToClient,
			message: Message{
				MsgID: 1002, RequestID: 31, RequestMsgID: 8888, Payload: &emptypb.Empty{},
			},
			wantError: ErrInvalidResponse,
		},
		{
			name:      "wrong request response pair",
			direction: ServerToClient,
			message: Message{
				MsgID: 1002, RequestID: 31, RequestMsgID: 2001, Payload: &emptypb.Empty{},
			},
			wantError: ErrInvalidResponse,
		},
		{
			name:      "generic error with unused request message id",
			direction: ServerToClient,
			message: Message{
				MsgID: 9001, RequestID: 31, RequestMsgID: 1001, Payload: &clientv1.ErrorResponse{Code: 7},
			},
			wantError: ErrInvalidResponse,
		},
		{
			name:      "wrong payload type",
			direction: ClientToServer,
			message: Message{
				MsgID: 1001, RequestID: 31, Payload: &clientv1.ErrorResponse{},
			},
			wantError: ErrInvalidPayload,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := registry.Encode(test.direction, test.message)
			if !errors.Is(err, test.wantError) {
				t.Fatalf("Encode() error = %v, want %v", err, test.wantError)
			}
		})
	}
}

// TestRegistryRejectsInvalidDefinitions 验证重复编号、缺失响应关系和无类型消息在构造时失败。
func TestRegistryRejectsInvalidDefinitions(t *testing.T) {
	base := []MessageDefinition{
		{MsgID: 1001, Direction: ClientToServer, Kind: Request, Payload: &emptypb.Empty{}, ResponseMsgID: 1002},
		{MsgID: 1002, Direction: ServerToClient, Kind: Response, Payload: &emptypb.Empty{}, RequestMsgID: 1001},
	}
	tests := []struct {
		name        string
		definitions []MessageDefinition
	}{
		{name: "empty registry"},
		{name: "duplicate message id", definitions: append(append([]MessageDefinition(nil), base...), base[0])},
		{name: "missing response", definitions: base[:1]},
		{name: "missing payload", definitions: []MessageDefinition{{MsgID: 1001, Direction: ClientToServer, Kind: Notify}}},
		{name: "reserved handshake id", definitions: []MessageDefinition{{MsgID: uint32(clientv1.MsgId_C2S_KEY_EXCHANGE_REQUEST), Direction: ClientToServer, Kind: Notify, Payload: &emptypb.Empty{}}}},
		{name: "unlinked business response", definitions: []MessageDefinition{{MsgID: 1002, Direction: ServerToClient, Kind: Response, Payload: &emptypb.Empty{}}}},
		{name: "wrong common error payload", definitions: []MessageDefinition{{MsgID: uint32(clientv1.MsgId_S2C_ERROR_RESPONSE), Direction: ServerToClient, Kind: Response, Payload: &emptypb.Empty{}}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := NewRegistry(test.definitions...)
			if !errors.Is(err, ErrInvalidRegistry) {
				t.Fatalf("NewRegistry() error = %v, want ErrInvalidRegistry", err)
			}
		})
	}
}

func newTestRegistry(t *testing.T) *Registry {
	t.Helper()
	registry, err := NewRegistry(
		MessageDefinition{MsgID: 1001, Direction: ClientToServer, Kind: Request, Payload: &emptypb.Empty{}, ResponseMsgID: 1002},
		MessageDefinition{MsgID: 1002, Direction: ServerToClient, Kind: Response, Payload: &emptypb.Empty{}, RequestMsgID: 1001},
		MessageDefinition{MsgID: 2001, Direction: ClientToServer, Kind: Notify, Payload: &emptypb.Empty{}},
		MessageDefinition{MsgID: 3001, Direction: ServerToClient, Kind: Push, Payload: &emptypb.Empty{}},
		MessageDefinition{MsgID: 9001, Direction: ServerToClient, Kind: Response, Payload: &clientv1.ErrorResponse{}},
	)
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}
	return registry
}
