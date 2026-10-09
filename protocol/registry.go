// Package protocol 提供客户端 Protobuf 消息的解码、分发和响应封装。
package protocol

import (
	"errors"
	"fmt"
	"reflect"

	"github.com/cnooljaw/gsr/protocol/catalog"
	commonv1 "github.com/cnooljaw/gsr/protocol/gen/go/gsr/common/v1"
	"google.golang.org/protobuf/proto"
)

var (
	// ErrUnknownMessageID 表示消息编号没有对应的 Protobuf 类型。
	ErrUnknownMessageID = errors.New("protocol: unknown msg_id")
	// ErrWrongDirection 表示消息编号的方向不允许当前收发操作。
	ErrWrongDirection = errors.New("protocol: message direction does not match operation")
	// ErrPayloadTypeMismatch 表示消息编号与 Protobuf payload 类型不匹配。
	ErrPayloadTypeMismatch = errors.New("protocol: payload type does not match msg_id")
	// ErrInvalidPayload 表示消息 payload 不是对应类型的有效 Protobuf 编码。
	ErrInvalidPayload = errors.New("protocol: invalid payload")
	// ErrHandlerNotRegistered 表示协议已定义，但业务尚未注册对应处理函数。
	ErrHandlerNotRegistered = errors.New("protocol: handler not registered")
	// ErrResponseRequired 表示有 request_id 的请求未得到响应。
	ErrResponseRequired = errors.New("protocol: request requires a response")
	// ErrRequestIDMismatch 表示 request_id 与请求或通知类型不匹配。
	ErrRequestIDMismatch = errors.New("protocol: request_id does not match message kind")
)

// Registry 汇总由 Protobuf 生成的消息目录，并建立编号与类型的双向索引。
type Registry struct {
	byID   map[uint32]catalog.Entry
	byType map[reflect.Type]catalog.Entry
}

// NewRegistry 自动纳入框架通用目录，再合并业务生成目录；调用方只传业务目录。
// 通用消息编号和类型不可覆盖或重映射，重复传入通用目录也会返回冲突错误。
func NewRegistry(catalogs ...[]catalog.Entry) (*Registry, error) {
	registry := &Registry{
		byID:   make(map[uint32]catalog.Entry),
		byType: make(map[reflect.Type]catalog.Entry),
	}
	for _, entry := range commonv1.MessageCatalogGsrCommonV1Common {
		if err := registry.add(entry); err != nil {
			return nil, err
		}
	}
	for _, entries := range catalogs {
		for _, entry := range entries {
			if err := registry.add(entry); err != nil {
				return nil, err
			}
		}
	}
	return registry, nil
}

func (registry *Registry) add(entry catalog.Entry) error {
	if entry.ID == 0 {
		return errors.New("protocol: message catalog contains zero msg_id")
	}
	if entry.Direction != catalog.ClientToServer && entry.Direction != catalog.ServerToClient {
		return fmt.Errorf("protocol: msg_id %d has invalid direction %d", entry.ID, entry.Direction)
	}
	if entry.Direction == catalog.ClientToServer && entry.Kind != catalog.Request && entry.Kind != catalog.Notify {
		return fmt.Errorf("protocol: client-to-server msg_id %d must be a request or notification", entry.ID)
	}
	if entry.Direction == catalog.ServerToClient && entry.Kind != catalog.Response && entry.Kind != catalog.Push {
		return fmt.Errorf("protocol: server-to-client msg_id %d must be a response or push", entry.ID)
	}
	if entry.NewMessage == nil {
		return fmt.Errorf("protocol: msg_id %d has no payload constructor", entry.ID)
	}
	if _, exists := registry.byID[entry.ID]; exists {
		return fmt.Errorf("protocol: duplicate msg_id %d", entry.ID)
	}
	message := entry.NewMessage()
	if isNil(message) {
		return fmt.Errorf("protocol: msg_id %d produced a nil payload", entry.ID)
	}
	messageType := reflect.TypeOf(message)
	if existing, exists := registry.byType[messageType]; exists {
		return fmt.Errorf("protocol: payload type %s is mapped by msg_id %d and %d", messageType, existing.ID, entry.ID)
	}
	registry.byID[entry.ID] = entry
	registry.byType[messageType] = entry
	return nil
}

// Decode 根据 msg_id 创建并解码对应的客户端上行消息，并校验 request_id 语义。
func (registry *Registry) Decode(msgID, requestID uint32, payload []byte) (proto.Message, error) {
	entry, exists := registry.byID[msgID]
	if !exists {
		return nil, fmt.Errorf("%w: %d", ErrUnknownMessageID, msgID)
	}
	if entry.Direction != catalog.ClientToServer {
		return nil, fmt.Errorf("%w: %d is server-to-client", ErrWrongDirection, msgID)
	}
	if (entry.Kind == catalog.Request && requestID == 0) || (entry.Kind == catalog.Notify && requestID != 0) {
		return nil, fmt.Errorf("%w: msg_id %d", ErrRequestIDMismatch, msgID)
	}
	message := entry.NewMessage()
	if err := proto.Unmarshal(payload, message); err != nil {
		return nil, fmt.Errorf("%w: msg_id %d: %v", ErrInvalidPayload, msgID, err)
	}
	return message, nil
}

func (registry *Registry) encodeServerMessage(msgID uint32, kind catalog.Kind, message proto.Message) ([]byte, error) {
	entry, exists := registry.byID[msgID]
	if !exists {
		return nil, fmt.Errorf("%w: %d", ErrUnknownMessageID, msgID)
	}
	if entry.Direction != catalog.ServerToClient {
		return nil, fmt.Errorf("%w: %d is client-to-server", ErrWrongDirection, msgID)
	}
	if entry.Kind != kind {
		return nil, fmt.Errorf("protocol: msg_id %d is %v, not %v", msgID, entry.Kind, kind)
	}
	messageEntry, registered := registry.byType[reflect.TypeOf(message)]
	if isNil(message) || !registered || messageEntry.ID != msgID {
		return nil, fmt.Errorf("%w: msg_id %d expects %s", ErrPayloadTypeMismatch, msgID, reflect.TypeOf(entry.NewMessage()))
	}
	payload, err := proto.Marshal(message)
	if err != nil {
		return nil, fmt.Errorf("encode msg_id %d payload: %w", msgID, err)
	}
	return payload, nil
}

func (registry *Registry) entryForType(messageType reflect.Type) (catalog.Entry, bool) {
	entry, exists := registry.byType[messageType]
	return entry, exists
}

func isNil(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Ptr, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}
