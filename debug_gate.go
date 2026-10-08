package gsr

import (
	"context"
	"fmt"

	"github.com/cnooljaw/gsr/gate"
	"github.com/cnooljaw/gsr/gate/debuggate"
	"github.com/cnooljaw/gsr/runtime"
)

// DebugGateConfig 定义应用层 DebugGate 的容量和连接 Agent 构造入口。
// DebugGate 仅用于本地调试，不能暴露到公网。
type DebugGateConfig struct {
	// Address 只允许回环或私有 IP，端口零可供本地测试。
	Address string
	// MaxConnections 必须为正，包含已接纳的客户端连接。
	MaxConnections int
	// MaxFrameBytes 限制单帧大小；零值沿用 DebugGate 默认值。
	MaxFrameBytes int
	// MaxOutboundQueueBytes 限制每条连接的出站队列字节数。
	MaxOutboundQueueBytes int
	// NewAgent 每连接构造一个业务 Agent，不执行 socket I/O，也不保存 ServiceContext。
	NewAgent func(gate.ClientSession) Service
}

// OpenDebugGate 创建 DebugGate，并让每条连接的应用 Agent 经现有适配器进入 Runtime。
// 成功返回实际监听地址；Gate 和 Agent 均由 Runtime 管理生命周期。
func (self *serviceContext) OpenDebugGate(ctx context.Context, config DebugGateConfig) (string, error) {
	if config.NewAgent == nil {
		return "", fmt.Errorf("%w: NewAgent is required", debuggate.ErrInvalidConfig)
	}
	node := self.adapter.node
	gateService, err := debuggate.NewGateService(node.local, debuggate.Config{
		Address:               config.Address,
		MaxConnections:        config.MaxConnections,
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
