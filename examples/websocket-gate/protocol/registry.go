package protocol

import (
	"errors"
	"fmt"

	clientv1 "github.com/cnooljaw/gsr/examples/websocket-gate/gen/go/gsr/client/v1"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

var (
	// ErrInvalidRegistry 表示消息注册表定义不完整或彼此冲突。
	ErrInvalidRegistry = errors.New("invalid client protocol registry")
	// ErrUnknownMessage 表示 Envelope 的 msg_id 未注册。
	ErrUnknownMessage = errors.New("unknown client protocol message")
	// ErrWrongDirection 表示消息方向与当前连接方向不符。
	ErrWrongDirection = errors.New("wrong client protocol message direction")
	// ErrInvalidRequestID 表示 request_id 与消息类型的规则不符。
	ErrInvalidRequestID = errors.New("invalid client request_id")
	// ErrInvalidPayload 表示 Payload 类型不符或 Protobuf 编解码失败。
	ErrInvalidPayload = errors.New("invalid client protocol payload")
	// ErrInvalidResponse 表示响应没有对应有效的客户端请求。
	ErrInvalidResponse = errors.New("invalid client protocol response")
)

// Direction 表示消息在客户端与服务端之间的传输方向。
type Direction uint8

const (
	// ClientToServer 表示从客户端进入服务端。
	ClientToServer Direction = iota + 1
	// ServerToClient 表示从服务端发往客户端。
	ServerToClient
)

// MessageKind 表示客户端 Envelope 所承载的消息语义。
type MessageKind uint8

const (
	// Request 表示需要服务端业务响应的客户端请求。
	Request MessageKind = iota + 1
	// Notify 表示不等待响应的客户端通知。
	Notify
	// Response 表示服务端对客户端请求的响应。
	Response
	// Push 表示服务端主动推送。
	Push
)

// MessageDefinition 描述一个客户端消息编号及其 payload 类型。
//
// Payload 只用于读取 Protobuf 类型，不会被注册表保留或修改。
// Request 的 ResponseMsgID 必须指向对应的 Response；专属 Response 的
// RequestMsgID 必须反向指向该 Request。通用 ErrorResponse 的 RequestMsgID 为零。
type MessageDefinition struct {
	MsgID         uint32
	Direction     Direction
	Kind          MessageKind
	Payload       proto.Message
	ResponseMsgID uint32
	RequestMsgID  uint32
}

// Message 是已经按注册表完成方向、request_id 和 payload 校验的协议消息。
//
// Encode 对专属 Response 要求 RequestMsgID 指向原始客户端 Request；通用 ErrorResponse 只使用 Envelope 的 RequestID。
type Message struct {
	MsgID        uint32
	RequestID    uint32
	RequestMsgID uint32
	Payload      proto.Message
}

// DecodedMessage 包含解码后的业务 payload 和注册表中的消息关系。
type DecodedMessage struct {
	MsgID         uint32
	RequestID     uint32
	Kind          MessageKind
	ResponseMsgID uint32
	Payload       proto.Message
}

type registeredMessage struct {
	definition  MessageDefinition
	payloadType protoreflect.MessageType
}

// Registry 是创建后不可变的客户端消息注册表。
type Registry struct {
	messages map[uint32]registeredMessage
}

// NewRegistry 校验并复制客户端消息定义。
func NewRegistry(definitions ...MessageDefinition) (*Registry, error) {
	if len(definitions) == 0 {
		return nil, fmt.Errorf("%w: no messages", ErrInvalidRegistry)
	}

	registry := &Registry{messages: make(map[uint32]registeredMessage, len(definitions))}
	for _, definition := range definitions {
		if err := validateDefinition(definition); err != nil {
			return nil, err
		}
		if _, exists := registry.messages[definition.MsgID]; exists {
			return nil, fmt.Errorf("%w: duplicate msg_id %d", ErrInvalidRegistry, definition.MsgID)
		}

		payloadType := definition.Payload.ProtoReflect().Type()
		definition.Payload = nil
		registry.messages[definition.MsgID] = registeredMessage{
			definition:  definition,
			payloadType: payloadType,
		}
	}

	if err := validateResponseRelations(registry.messages); err != nil {
		return nil, err
	}
	return registry, nil
}

// Decode 按消息编号查找 payload 类型，并验证方向及 request_id 语义。
func (registry *Registry) Decode(direction Direction, envelope *clientv1.Envelope) (DecodedMessage, error) {
	if registry == nil {
		return DecodedMessage{}, ErrInvalidRegistry
	}
	if !validDirection(direction) {
		return DecodedMessage{}, ErrWrongDirection
	}
	if envelope == nil {
		return DecodedMessage{}, fmt.Errorf("%w: nil Envelope", ErrInvalidPayload)
	}

	registered, exists := registry.messages[envelope.GetMsgId()]
	if !exists {
		return DecodedMessage{}, fmt.Errorf("%w: msg_id %d", ErrUnknownMessage, envelope.GetMsgId())
	}
	if registered.definition.Direction != direction {
		return DecodedMessage{}, fmt.Errorf("%w: msg_id %d", ErrWrongDirection, envelope.GetMsgId())
	}
	if err := validateRequestID(registered.definition.Kind, envelope.GetRequestId()); err != nil {
		return DecodedMessage{}, err
	}

	payload := registered.payloadType.New().Interface()
	if err := proto.Unmarshal(envelope.GetPayload(), payload); err != nil {
		return DecodedMessage{}, fmt.Errorf("%w: msg_id %d: %v", ErrInvalidPayload, envelope.GetMsgId(), err)
	}

	return DecodedMessage{
		MsgID:         registered.definition.MsgID,
		RequestID:     envelope.GetRequestId(),
		Kind:          registered.definition.Kind,
		ResponseMsgID: registered.definition.ResponseMsgID,
		Payload:       payload,
	}, nil
}

// Encode 验证方向、消息关系、request_id 和 payload 后构造 Envelope。
func (registry *Registry) Encode(direction Direction, message Message) (*clientv1.Envelope, error) {
	if registry == nil {
		return nil, ErrInvalidRegistry
	}
	if !validDirection(direction) {
		return nil, ErrWrongDirection
	}

	registered, exists := registry.messages[message.MsgID]
	if !exists {
		return nil, fmt.Errorf("%w: msg_id %d", ErrUnknownMessage, message.MsgID)
	}
	if registered.definition.Direction != direction {
		return nil, fmt.Errorf("%w: msg_id %d", ErrWrongDirection, message.MsgID)
	}
	if err := validateRequestID(registered.definition.Kind, message.RequestID); err != nil {
		return nil, err
	}
	if err := validateResponseRequest(registered.definition, message.RequestMsgID); err != nil {
		return nil, err
	}
	if message.Payload == nil || !message.Payload.ProtoReflect().IsValid() {
		return nil, fmt.Errorf("%w: nil payload", ErrInvalidPayload)
	}
	if message.Payload.ProtoReflect().Descriptor().FullName() != registered.payloadType.Descriptor().FullName() {
		return nil, fmt.Errorf("%w: msg_id %d expects %s, got %s", ErrInvalidPayload, message.MsgID,
			registered.payloadType.Descriptor().FullName(), message.Payload.ProtoReflect().Descriptor().FullName())
	}

	payload, err := proto.Marshal(message.Payload)
	if err != nil {
		return nil, fmt.Errorf("%w: msg_id %d: %v", ErrInvalidPayload, message.MsgID, err)
	}
	return &clientv1.Envelope{
		MsgId:     message.MsgID,
		RequestId: message.RequestID,
		Payload:   payload,
	}, nil
}

func validateDefinition(definition MessageDefinition) error {
	if definition.MsgID == 0 || !validDirection(definition.Direction) || definition.Payload == nil {
		return fmt.Errorf("%w: msg_id, direction, or payload is invalid", ErrInvalidRegistry)
	}
	if definition.MsgID == uint32(clientv1.MsgId_C2S_KEY_EXCHANGE_REQUEST) ||
		definition.MsgID == uint32(clientv1.MsgId_S2C_KEY_EXCHANGE_RESPONSE) {
		return fmt.Errorf("%w: handshake msg_id %d is reserved for Gate", ErrInvalidRegistry, definition.MsgID)
	}
	if !validKind(definition.Kind) {
		return fmt.Errorf("%w: msg_id %d has invalid kind", ErrInvalidRegistry, definition.MsgID)
	}
	if !validKindDirection(definition.Kind, definition.Direction) {
		return fmt.Errorf("%w: msg_id %d has incompatible kind and direction", ErrInvalidRegistry, definition.MsgID)
	}
	if definition.Payload.ProtoReflect().Type() == nil {
		return fmt.Errorf("%w: msg_id %d has invalid payload type", ErrInvalidRegistry, definition.MsgID)
	}
	if definition.MsgID == uint32(clientv1.MsgId_S2C_ERROR_RESPONSE) &&
		(definition.Kind != Response || definition.Direction != ServerToClient || definition.RequestMsgID != 0 ||
			definition.Payload.ProtoReflect().Descriptor().FullName() != "gsr.client.v1.ErrorResponse") {
		return fmt.Errorf("%w: msg_id %d is reserved for the generic ErrorResponse", ErrInvalidRegistry, definition.MsgID)
	}

	switch definition.Kind {
	case Request:
		if definition.ResponseMsgID == 0 || definition.RequestMsgID != 0 {
			return fmt.Errorf("%w: request msg_id %d must define one response", ErrInvalidRegistry, definition.MsgID)
		}
	case Response:
		if definition.ResponseMsgID != 0 {
			return fmt.Errorf("%w: response msg_id %d cannot define another response", ErrInvalidRegistry, definition.MsgID)
		}
		if definition.RequestMsgID == 0 && definition.MsgID != uint32(clientv1.MsgId_S2C_ERROR_RESPONSE) {
			return fmt.Errorf("%w: response msg_id %d must reference its request", ErrInvalidRegistry, definition.MsgID)
		}
	case Notify, Push:
		if definition.ResponseMsgID != 0 || definition.RequestMsgID != 0 {
			return fmt.Errorf("%w: one-way msg_id %d cannot define a response relation", ErrInvalidRegistry, definition.MsgID)
		}
	}
	return nil
}

func validateResponseRelations(messages map[uint32]registeredMessage) error {
	for _, registered := range messages {
		definition := registered.definition
		switch definition.Kind {
		case Request:
			response, exists := messages[definition.ResponseMsgID]
			if !exists || response.definition.Kind != Response || response.definition.Direction != ServerToClient ||
				response.definition.RequestMsgID != definition.MsgID {
				return fmt.Errorf("%w: request msg_id %d has invalid response msg_id %d",
					ErrInvalidRegistry, definition.MsgID, definition.ResponseMsgID)
			}
		case Response:
			if definition.RequestMsgID == 0 {
				continue
			}
			request, exists := messages[definition.RequestMsgID]
			if !exists || request.definition.Kind != Request || request.definition.ResponseMsgID != definition.MsgID {
				return fmt.Errorf("%w: response msg_id %d has invalid request msg_id %d",
					ErrInvalidRegistry, definition.MsgID, definition.RequestMsgID)
			}
		}
	}
	return nil
}

func validateResponseRequest(definition MessageDefinition, requestMsgID uint32) error {
	if definition.Kind != Response {
		if requestMsgID != 0 {
			return fmt.Errorf("%w: non-response msg_id %d has request msg_id %d", ErrInvalidResponse, definition.MsgID, requestMsgID)
		}
		return nil
	}
	if definition.RequestMsgID == 0 {
		if requestMsgID != 0 {
			return fmt.Errorf("%w: generic response msg_id %d does not use request msg_id", ErrInvalidResponse, definition.MsgID)
		}
		return nil
	}
	if definition.RequestMsgID != requestMsgID {
		return fmt.Errorf("%w: response msg_id %d does not match request msg_id %d",
			ErrInvalidResponse, definition.MsgID, requestMsgID)
	}
	return nil
}

func validateRequestID(kind MessageKind, requestID uint32) error {
	switch kind {
	case Request, Response:
		if requestID == 0 {
			return ErrInvalidRequestID
		}
	case Notify, Push:
		if requestID != 0 {
			return ErrInvalidRequestID
		}
	}
	return nil
}

func validDirection(direction Direction) bool {
	return direction == ClientToServer || direction == ServerToClient
}

func validKind(kind MessageKind) bool {
	return kind >= Request && kind <= Push
}

func validKindDirection(kind MessageKind, direction Direction) bool {
	switch kind {
	case Request, Notify:
		return direction == ClientToServer
	case Response, Push:
		return direction == ServerToClient
	default:
		return false
	}
}
