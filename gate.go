package gsr

import (
	"context"
	"crypto/tls"
	"fmt"
	"time"

	"github.com/cnooljaw/gsr/gate"
	"github.com/cnooljaw/gsr/runtime"
)

// GateConfig 定义应用层 Gate 的固定协议模式、监听和连接 Agent 构造入口。
type GateConfig struct {
	// Protocol 默认使用 gate.ProtocolMAC；内网调试须显式选择 gate.ProtocolPlain。
	Protocol gate.Protocol
	// Address 是 TCP host:port；Plain 仅允许字面回环或私有 IP，端口零可供本机测试。
	Address string
	// TLSConfig 为 nil 时使用 HTTP/WS，适用于可信内网或由 Ingress/LB 终止 TLS 的部署。
	// 公网直连必须配置 TLS；具体约束沿用 gate.Config。
	TLSConfig *tls.Config
	// MaxConnections 必须为正，限制原始 TCP 连接（包括未完成 HTTP 请求头或握手的连接）。
	MaxConnections int
	// HandshakeTimeout 仅在 MAC 模式限制首帧握手等待；零值使用 5 秒，Plain 不执行握手。
	HandshakeTimeout time.Duration
	// MaxFrameBytes 限制完整 WebSocket 消息大小；零值使用 4 MiB。
	MaxFrameBytes int
	// MaxOutboundQueueBytes 限制每条连接的出站队列字节数。
	MaxOutboundQueueBytes int
	// NewAgent 为每条接纳的连接构造业务 Agent，不执行 socket I/O，也不保存 ServiceContext。
	NewAgent func(gate.ClientSession) Service
}

// OpenGate 创建固定协议模式的 Gate，并让连接 Agent 经现有应用适配器进入 Runtime。
// 成功返回实际监听地址；Gate 和 Agent 均由 Runtime 管理生命周期。
func (self *serviceContext) OpenGate(ctx context.Context, config GateConfig) (string, error) {
	if config.NewAgent == nil {
		return "", fmt.Errorf("%w: NewAgent is required", gate.ErrInvalidConfig)
	}
	node := self.adapter.node
	gateService, err := gate.NewGateService(node.local, gate.Config{
		Protocol:              config.Protocol,
		Address:               config.Address,
		TLSConfig:             config.TLSConfig,
		MaxConnections:        config.MaxConnections,
		HandshakeTimeout:      config.HandshakeTimeout,
		MaxFrameBytes:         config.MaxFrameBytes,
		MaxOutboundQueueBytes: config.MaxOutboundQueueBytes,
		NewAgent: func(client gate.ClientSession) runtime.Service {
			agent, err := node.wrap(config.NewAgent(client))
			if err != nil {
				return nil
			}
			return agent
		},
	})
	if err != nil {
		return "", err
	}
	if _, err := self.ServiceContext.CreateService(ctx, gateService); err != nil {
		return "", err
	}
	return gateService.Addr().String(), nil
}
