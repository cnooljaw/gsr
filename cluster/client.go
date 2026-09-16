package cluster

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"time"

	runtime "github.com/cnooljaw/gsr/runtime"
)

// OutboundSender 是 ClusterClient 接纳请求所需的最小能力，
// 标准实现为 NodeSender。
type OutboundSender interface {
	Send(ClusterEnvelope) error
}

// NodeSenderResolver 提供远端节点唯一的出站发送器。
// 发送器的资源管理、建连和重连均由每 Service 的 ClusterClient 之外的组件负责。
type NodeSenderResolver interface {
	ResolveNodeSender(NodeID) (OutboundSender, error)
}

// ClusterClientConfig 将当前 Service 的来源身份绑定到 Cluster 基础设施。
type ClusterClientConfig struct {
	Source       runtime.ServiceID
	Senders      NodeSenderResolver
	Codec        PayloadCodec
	PendingCalls *ClusterCallRegistry
	MaxCallWait  time.Duration
}

var (
	// ErrInvalidClusterClientConfig 表示 ClusterClient 的依赖无效。
	ErrInvalidClusterClientConfig = errors.New("cluster: invalid client config")
	// ErrNodeSenderUnavailable 表示解析器未提供发送器。
	ErrNodeSenderUnavailable = errors.New("cluster: node sender unavailable")
)

// ClusterClient 是绑定来源身份、只提供少量远端请求能力的 Cluster 客户端。
// 发送器管理、响应完成和远端执行仍由 Cluster 层负责。
type ClusterClient struct {
	source       runtime.ServiceID
	senders      NodeSenderResolver
	codec        PayloadCodec
	pendingCalls *ClusterCallRegistry
	maxCallWait  time.Duration
}

// NewClusterClient 校验并构造绑定来源身份的客户端。
func NewClusterClient(config ClusterClientConfig) (*ClusterClient, error) {
	if config.Source == 0 || isNilNodeSenderResolver(config.Senders) || isNilPayloadCodec(config.Codec) || !config.PendingCalls.valid() || config.MaxCallWait <= 0 {
		return nil, fmt.Errorf("%w: Source, Senders, Codec, PendingCalls, and positive MaxCallWait are required", ErrInvalidClusterClientConfig)
	}
	return &ClusterClient{
		source:       config.Source,
		senders:      config.Senders,
		codec:        config.Codec,
		pendingCalls: config.PendingCalls,
		maxCallWait:  config.MaxCallWait,
	}, nil
}

// Source 返回客户端绑定时记录的 Runtime ServiceID。
// 此值只读，不能为单次请求替换。
func (client *ClusterClient) Source() runtime.ServiceID {
	if client == nil {
		return 0
	}
	return client.source
}

// Send 将 args 编码为命令参数列表，并交给解析出的远端 NodeSender 接纳。
// 返回 nil 仅表示本地接纳成功。
func (client *ClusterClient) Send(node NodeID, target ServiceTarget, command runtime.CommandID, args ...any) error {
	_, sender, payload, err := client.prepareRequest(node, target, command, args...)
	if err != nil {
		return err
	}
	return sender.Send(ClusterEnvelope{
		Kind:    FrameRequest,
		Target:  target,
		Command: command,
		Payload: payload,
	})
}

// Call 发送远端请求并等待对应的 Response 或 Error。
//
// 有效等待时间不超过 MaxCallWait，ctx 可以要求更短的等待。
// 不接受 nil ctx；成功返回只表示已收到并解码匹配的远端响应，
// 远端业务错误以 runtime.StatusError 返回。
func (client *ClusterClient) Call(ctx context.Context, node NodeID, target ServiceTarget, command runtime.CommandID, args ...any) (any, error) {
	if ctx == nil {
		return nil, runtime.ErrInvalidCallContext
	}
	if err := client.validateConfig(); err != nil {
		return nil, err
	}
	effectiveContext, cancel := context.WithTimeout(ctx, client.maxCallWait)
	defer cancel()
	if err := effectiveContext.Err(); err != nil {
		return nil, clusterCallWaitError(ctx, effectiveContext)
	}
	codec, sender, payload, err := client.prepareRequest(node, target, command, args...)
	if err != nil {
		return nil, err
	}
	if err := effectiveContext.Err(); err != nil {
		return nil, clusterCallWaitError(ctx, effectiveContext)
	}
	pending, err := client.pendingCalls.begin(client.source, node, target, command)
	if err != nil {
		return nil, err
	}
	request := ClusterEnvelope{
		Kind:    FrameRequest,
		Session: pending.session,
		Target:  target,
		Command: command,
		Payload: payload,
	}
	if err := sender.Send(request); err != nil {
		client.pendingCalls.cancel(pending.session, err)
		return nil, err
	}
	select {
	case result := <-pending.done:
		return decodeClusterCallResult(codec, command, result)
	case <-effectiveContext.Done():
		waitErr := clusterCallWaitError(ctx, effectiveContext)
		if client.pendingCalls.cancel(pending.session, waitErr) {
			return nil, waitErr
		}
		result := <-pending.done
		return decodeClusterCallResult(codec, command, result)
	}
}

func (client *ClusterClient) validateConfig() error {
	if client == nil || client.source == 0 || isNilNodeSenderResolver(client.senders) || isNilPayloadCodec(client.codec) || !client.pendingCalls.valid() || client.maxCallWait <= 0 {
		return ErrInvalidClusterClientConfig
	}
	return nil
}

func (client *ClusterClient) prepareRequest(node NodeID, target ServiceTarget, command runtime.CommandID, args ...any) (PayloadCodec, OutboundSender, []byte, error) {
	if err := client.validateConfig(); err != nil {
		return nil, nil, nil, err
	}
	if node == "" {
		return nil, nil, nil, ErrInvalidNode
	}
	if err := target.validate(); err != nil {
		return nil, nil, nil, err
	}
	if command == 0 {
		return nil, nil, nil, ErrInvalidEnvelope
	}
	codec := client.codec
	ownedArgs := append([]any(nil), args...)
	payload, err := codec.Encode(ownedArgs)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("encode command %d: %w", command, err)
	}
	sender, err := client.senders.ResolveNodeSender(node)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("resolve sender for node %q: %w", node, err)
	}
	if isNilOutboundSender(sender) {
		return nil, nil, nil, fmt.Errorf("%w: node %q", ErrNodeSenderUnavailable, node)
	}
	return codec, sender, payload, nil
}

func clusterCallWaitError(ctx context.Context, effectiveContext context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if errors.Is(effectiveContext.Err(), context.DeadlineExceeded) {
		return fmt.Errorf("%w: %w", runtime.ErrCallTimeout, context.DeadlineExceeded)
	}
	return effectiveContext.Err()
}

func decodeClusterCallResult(codec PayloadCodec, command runtime.CommandID, result clusterCallResult) (any, error) {
	if result.err != nil {
		return nil, result.err
	}
	value, err := codec.Decode(result.payload)
	if err != nil {
		return nil, fmt.Errorf("decode response for command %d: %w", command, err)
	}
	return value, nil
}

func isNilNodeSenderResolver(resolver NodeSenderResolver) bool {
	if resolver == nil {
		return true
	}
	value := reflect.ValueOf(resolver)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}

func isNilOutboundSender(sender OutboundSender) bool {
	if sender == nil {
		return true
	}
	value := reflect.ValueOf(sender)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}
