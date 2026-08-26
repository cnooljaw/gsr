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

// Config configures the Core Runtime.
type Config struct {
	// MaxCallWait is the maximum duration of a local Call.
	MaxCallWait time.Duration
	// MailboxCapacity bounds each Service mailbox.
	MailboxCapacity int
	// ShutdownTimeout bounds Runtime-owned shutdown work.
	ShutdownTimeout time.Duration
}

// DefaultConfig returns the valid default Runtime configuration.
func DefaultConfig() Config {
	return Config{
		MaxCallWait:     defaultMaxCallWait,
		MailboxCapacity: defaultMailboxCapacity,
		ShutdownTimeout: defaultShutdownTimeout,
	}
}

// Validate checks a fully populated Runtime configuration.
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
