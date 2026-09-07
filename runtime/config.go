package runtime

import (
	"fmt"
	"time"
)

const (
	defaultMaxCallWait     = 5 * time.Second
	defaultMailboxCapacity = 1024
	defaultShutdownTimeout = 5 * time.Second
)

// Config 定义 Core Runtime 的配置。
type Config struct {
	// MaxCallWait 是本地 Call 的最长等待时间。
	MaxCallWait time.Duration
	// MailboxCapacity 限制每个 Service Mailbox 的容量。
	MailboxCapacity int
	// ShutdownTimeout 限制 Runtime 停止工作的等待时间。
	ShutdownTimeout time.Duration
}

// DefaultConfig 返回有效的 Runtime 默认配置。
func DefaultConfig() Config {
	return Config{
		MaxCallWait:     defaultMaxCallWait,
		MailboxCapacity: defaultMailboxCapacity,
		ShutdownTimeout: defaultShutdownTimeout,
	}
}

// Validate 检查已填充完整的 Runtime 配置。
func (config Config) Validate() error {
	switch {
	case config.MaxCallWait <= 0:
		return invalidConfig("MaxCallWait must be greater than zero")
	case config.MailboxCapacity <= 0:
		return invalidConfig("MailboxCapacity must be greater than zero")
	case config.ShutdownTimeout <= 0:
		return invalidConfig("ShutdownTimeout must be greater than zero")
	default:
		return nil
	}
}

func (config Config) normalized() (Config, error) {
	defaults := DefaultConfig()
	if config.MaxCallWait == 0 {
		config.MaxCallWait = defaults.MaxCallWait
	}
	if config.MailboxCapacity == 0 {
		config.MailboxCapacity = defaults.MailboxCapacity
	}
	if config.ShutdownTimeout == 0 {
		config.ShutdownTimeout = defaults.ShutdownTimeout
	}
	if err := config.Validate(); err != nil {
		return Config{}, err
	}
	return config, nil
}

func invalidConfig(detail string) error {
	return fmt.Errorf("%w: %s", ErrInvalidConfig, detail)
}
