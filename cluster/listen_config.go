package cluster

import (
	"errors"
	"fmt"

	"github.com/cnooljaw/gsr/runtime"
)

// ErrInvalidListenConfig 表示监听地址或入站容量配置无效。
var ErrInvalidListenConfig = errors.New("cluster: invalid listen config")

// ListenConfig 配置单个入站监听，不包含本地节点身份或外部发布地址。
// 编码注册表、帧长和写超时由 ClusterConfig 提供，不在此重复配置。
type ListenConfig struct {
	// Address 必须显式包含主机和数字端口；端口 0 允许系统分配端口。
	Address string
	// MaxConnections 为零时使用 128，正数限制整个监听的入站连接数。
	MaxConnections int
	// MaxConcurrentCalls 为零时使用 32，正数限制每条连接的 Call 等待数。
	// 满载暂停该连接读取，包括后续 Send；两个容量字段均不允许负数。
	MaxConcurrentCalls int
	// ResolveService 可选；nil 时只能投递到 ServiceID，名称目标返回 Service 不存在错误。
	// 解析器必须并发安全且不执行无限阻塞 I/O；它不转移 Registry 的所有权。
	ResolveService func(ServiceName) (runtime.ServiceID, error)
}

func (config ListenConfig) normalized() (ListenConfig, error) {
	if !validTCPAddress(config.Address, 0) {
		return ListenConfig{}, fmt.Errorf("%w: invalid TCP listen address %q", ErrInvalidListenConfig, config.Address)
	}
	if config.MaxConnections < 0 || config.MaxConcurrentCalls < 0 {
		return ListenConfig{}, fmt.Errorf("%w: nonnegative limits are required", ErrInvalidListenConfig)
	}
	if config.MaxConnections == 0 {
		config.MaxConnections = 128
	}
	if config.MaxConcurrentCalls == 0 {
		config.MaxConcurrentCalls = 32
	}
	return config, nil
}
