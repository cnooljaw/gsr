package gsr

import (
	"context"
	"crypto/tls"
	"fmt"
	"time"

	"github.com/cnooljaw/gsr/gate"
	"github.com/cnooljaw/gsr/gate/prodgate"
	"github.com/cnooljaw/gsr/runtime"
)

// ProdGateConfig 定义应用层生产 Gate 的监听、认证边界和连接 Agent 构造入口。
type ProdGateConfig struct {
	// Address 是监听的 TCP host:port；端口零可供本机测试。
	Address string
	// TLSConfig 为 nil 时使用 HTTP/WS，适用于可信内网或由 Ingress/LB 终止 TLS 的部署。
	// 公网直连必须配置 TLS；具体约束沿用 prodgate.Config。
	TLSConfig *tls.Config
	// MaxConnections 必须为正，包含尚未完成握手的连接。
	MaxConnections int
	// HandshakeTimeout 限制等待首个握手帧的时长；零值使用 5 秒。
	HandshakeTimeout time.Duration
	// MaxFrameBytes 限制完整 WebSocket 消息大小；零值使用 4 MiB。
	MaxFrameBytes int
	// MaxOutboundQueueBytes 限制每条连接的出站队列字节数。
	MaxOutboundQueueBytes int
	// NewAgent 每条完成握手的连接构造一个业务 Agent，不执行 socket I/O，也不保存 ServiceContext。
	NewAgent func(gate.ClientSession) Service
}

// OpenProdGate 创建生产 Gate，并让连接 Agent 经现有应用适配器进入 Runtime。
// 成功返回实际监听地址；Gate 和 Agent 均由 Runtime 管理生命周期。
func (self *serviceContext) OpenProdGate(ctx context.Context, config ProdGateConfig) (string, error) {
	if config.NewAgent == nil {
		return "", fmt.Errorf("%w: NewAgent is required", prodgate.ErrInvalidConfig)
	}
	node := self.adapter.node
	gateService, err := prodgate.NewGateService(node.local, prodgate.Config{
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
