// Package cluster 提供建立在本地 Runtime Core 之上的传输与路由层。
package cluster

import (
	"errors"
	"fmt"

	runtime "github.com/cnooljaw/gsr/runtime"
)

// NodeID 标识目标远端 Runtime 节点，不编码进 Core ServiceID。
type NodeID string

// ServiceName 是稳定的 Service 逻辑名称，由 Core 之外的组件解析。
type ServiceName string

// ClusterSessionID 将一次远端 Call 与其响应或错误关联。
// 零值表示单向 Send，不需要响应。
type ClusterSessionID uint64

// FrameKind 标识 ClusterEnvelope 的方向与含义。
type FrameKind uint8

const (
	// FrameRequest 携带远端 Send 或 Call 请求。
	FrameRequest FrameKind = iota + 1
	// FrameResponse 携带远端 Call 的成功结果。
	FrameResponse
	// FrameError 携带结构化的远端 Call 或投递错误。
	FrameError
)

// String 返回 FrameKind 的稳定显示名称。
func (kind FrameKind) String() string {
	switch kind {
	case FrameRequest:
		return "request"
	case FrameResponse:
		return "response"
	case FrameError:
		return "error"
	default:
		return "unknown"
	}
}

// ServiceTarget 通过远端 Service 的本地临时 ID 或稳定逻辑名称标识目标。
// 两个字段必须且只能设置一个。
type ServiceTarget struct {
	ServiceID   runtime.ServiceID
	ServiceName ServiceName
}

// ClusterEnvelope 是 Cluster 与 Transport 之间的值边界。
//
// 来源与目标 NodeID 由 Transport 提供。
// 消息封包不携带 Core Message 指针、NodeID 或任意指定的来源身份。
type ClusterEnvelope struct {
	Kind    FrameKind
	Session ClusterSessionID
	Target  ServiceTarget
	Command runtime.CommandID
	Payload []byte
	Error   *runtime.StatusError
}

var (
	// ErrInvalidEnvelope 表示消息封包违反 Cluster 帧契约。
	ErrInvalidEnvelope = errors.New("cluster: invalid envelope")
	// ErrPayloadTooLarge 表示传输载荷超出配置上限。
	ErrPayloadTooLarge = errors.New("cluster: payload too large")
)

// Validate 检查所有 Transport 共享的结构契约。
func (envelope ClusterEnvelope) Validate() error {
	if envelope.Kind != FrameRequest && envelope.Kind != FrameResponse && envelope.Kind != FrameError {
		return fmt.Errorf("%w: unknown frame kind %d", ErrInvalidEnvelope, envelope.Kind)
	}
	if err := envelope.Target.validate(); err != nil {
		return err
	}
	if envelope.Command == 0 {
		return fmt.Errorf("%w: command is zero", ErrInvalidEnvelope)
	}

	switch envelope.Kind {
	case FrameRequest:
		if envelope.Error != nil {
			return fmt.Errorf("%w: request contains error", ErrInvalidEnvelope)
		}
	case FrameResponse:
		if envelope.Session == 0 {
			return fmt.Errorf("%w: response session is zero", ErrInvalidEnvelope)
		}
		if envelope.Error != nil {
			return fmt.Errorf("%w: response contains error", ErrInvalidEnvelope)
		}
	case FrameError:
		if envelope.Session == 0 {
			return fmt.Errorf("%w: error session is zero", ErrInvalidEnvelope)
		}
		if err := validateStatusError(envelope.Error); err != nil {
			return err
		}
		if len(envelope.Payload) != 0 {
			return fmt.Errorf("%w: error contains payload", ErrInvalidEnvelope)
		}
	}
	return nil
}

func (target ServiceTarget) validate() error {
	hasID := target.ServiceID != 0
	hasName := target.ServiceName != ""
	if hasID == hasName {
		return fmt.Errorf("%w: exactly one ServiceID or ServiceName is required", ErrInvalidEnvelope)
	}
	return nil
}

func validateStatusError(status *runtime.StatusError) error {
	if status == nil || status.Class == "" || status.Code == "" {
		return fmt.Errorf("%w: error status must include class and code", ErrInvalidEnvelope)
	}
	return nil
}

// Clone 返回独占可变载荷与错误值的消息封包副本，用于 Transport 接纳边界。
func (envelope ClusterEnvelope) Clone() ClusterEnvelope {
	clone := envelope
	clone.Payload = append([]byte(nil), envelope.Payload...)
	if envelope.Error != nil {
		status := *envelope.Error
		clone.Error = &status
	}
	return clone
}
