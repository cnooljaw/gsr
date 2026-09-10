package cluster

import (
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"
)

// ClusterConfig 配置静态出站路由；不包含本地身份、Runtime 或监听能力。
// 数值零使用有限默认值，负数无效；构造复制 Nodes，Codecs 复用不可变注册表。
type ClusterConfig struct {
	Nodes  map[NodeID]string
	Codecs *CommandCodecRegistry
	// MaxCallWait 默认 5 秒，调用方 context 只能缩短等待。
	MaxCallWait time.Duration
	// DialTimeout 默认 3 秒，限制一次拨号而不是整个 Call。
	DialTimeout time.Duration
	// WriteTimeout 默认 3 秒，不包含排队或远端执行时间。
	WriteTimeout time.Duration
	// OutboundQueueCapacity 默认每目标 1024 条。
	OutboundQueueCapacity int
	// MaxFrame 默认 4 MiB，包含四字节长度前缀。
	MaxFrame int
}

func (config ClusterConfig) normalized() (ClusterConfig, error) {
	if config.Codecs == nil || config.Codecs.codecs == nil || config.MaxCallWait < 0 ||
		config.DialTimeout < 0 || config.WriteTimeout < 0 || config.OutboundQueueCapacity < 0 || config.MaxFrame < 0 {
		return ClusterConfig{}, fmt.Errorf("%w: initialized codecs and nonnegative limits are required", ErrInvalidClusterConfig)
	}
	if config.MaxCallWait == 0 {
		config.MaxCallWait = 5 * time.Second
	}
	if config.DialTimeout == 0 {
		config.DialTimeout = 3 * time.Second
	}
	if config.WriteTimeout == 0 {
		config.WriteTimeout = 3 * time.Second
	}
	if config.OutboundQueueCapacity == 0 {
		config.OutboundQueueCapacity = defaultOutboundQueueCapacity
	}
	if config.MaxFrame == 0 {
		config.MaxFrame = defaultMaxPayload
	}
	nodes := make(map[NodeID]string, len(config.Nodes))
	for node, address := range config.Nodes {
		if node == "" || !validTargetAddress(address) {
			return ClusterConfig{}, fmt.Errorf("%w: invalid TCP route for %q", ErrInvalidClusterConfig, node)
		}
		nodes[node] = address
	}
	config.Nodes = nodes
	return config, nil
}

func validTargetAddress(address string) bool {
	return validTCPAddress(address, 1)
}

// validTCPAddress 只检查地址格式；监听允许零端口，出站目标必须使用实际端口。
// DNS 解析与实际绑定/拨号仍由网络边界负责。
func validTCPAddress(address string, minPort int) bool {
	host, port, err := net.SplitHostPort(address)
	if err != nil || host == "" || strings.ContainsAny(host, "/\\ \t\r\n") || port == "" {
		return false
	}
	for _, digit := range port {
		if digit < '0' || digit > '9' {
			return false
		}
	}
	number, err := strconv.Atoi(port)
	return err == nil && number >= minPort && number <= 65535
}
