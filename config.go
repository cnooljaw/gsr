package gsr

import (
	"time"

	"github.com/cnooljaw/gsr/cluster"
	"github.com/cnooljaw/gsr/logging"
	"github.com/cnooljaw/gsr/runtime"
)

// Config 声明应用运行环境，Logging 创建唯一日志 owner，底层组件日志由 Run 注入。
type Config struct {
	Logging logging.Config
	Runtime RuntimeConfig
	Cluster ClusterConfig
}

// RuntimeConfig 只包含应用可调的 Core 参数，零值使用有限默认值，负值拒绝。
type RuntimeConfig struct {
	// MaxCallWait 限制本地 Call 的最长等待，调用方 context 可以缩短。
	MaxCallWait time.Duration
	// MailboxCapacity 限制每个 Service 的队列容量。
	MailboxCapacity int
	// ShutdownTimeout 是 Core 有序停止的共享等待预算，不强杀 Handler。
	ShutdownTimeout time.Duration
}

func (config RuntimeConfig) core(logger *logging.Logger) runtime.Config {
	return runtime.Config{
		Logger:          logger,
		MaxCallWait:     config.MaxCallWait,
		MailboxCapacity: config.MailboxCapacity,
		ShutdownTimeout: config.ShutdownTimeout,
	}
}

// ClusterConfig 配置共同节点表与通信预算，零值采用有限默认值，负值拒绝。
// 节点表 host 是访问地址，port 也用于本机通配监听；端口零只供监听测试。
type ClusterConfig struct {
	Nodes map[cluster.NodeID]string
	// Codec 为空使用默认 ValueCodec，自定义实现必须并发安全。
	Codec cluster.PayloadCodec
	// MaxCallWait 限制整个远程 Call 的等待。
	MaxCallWait time.Duration
	// DialTimeout 限制单次拨号，不是整个 Call 的预算。
	DialTimeout time.Duration
	// WriteTimeout 限制单次写入，不包含排队或业务执行。
	WriteTimeout time.Duration
	// OutboundQueueCapacity 限制每目标队列条目数。
	OutboundQueueCapacity int
	// MaxFrame 包含四字节长度前缀。
	MaxFrame int
}

func (config ClusterConfig) outbound(logger *logging.Logger) cluster.ClusterConfig {
	return cluster.ClusterConfig{
		Logger:                logger,
		Nodes:                 config.Nodes,
		Codec:                 config.Codec,
		MaxCallWait:           config.MaxCallWait,
		DialTimeout:           config.DialTimeout,
		WriteTimeout:          config.WriteTimeout,
		OutboundQueueCapacity: config.OutboundQueueCapacity,
		MaxFrame:              config.MaxFrame,
	}
}
