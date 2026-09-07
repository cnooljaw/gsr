package cluster

import (
	"errors"
	"fmt"
	"reflect"

	runtime "github.com/cnooljaw/gsr/runtime"
)

// CommandCodec 在 Command 载荷与字节之间进行编解码。
// 可以使用默认 ValueCodec，也可以提供命令专用的强类型实现。
type CommandCodec interface {
	Encode(value any) ([]byte, error)
	Decode(payload []byte) (any, error)
}

// CommandCodecEntry 将非零 CommandID 绑定到载荷编解码器。
type CommandCodecEntry struct {
	CommandID runtime.CommandID
	Codec     CommandCodec
}

// CommandCodecRegistry 是不可变的命令到编解码器的查找表。
type CommandCodecRegistry struct {
	codecs map[runtime.CommandID]CommandCodec
}

var (
	// ErrInvalidCommandRegistry 表示注册项或注册表接收者无效。
	ErrInvalidCommandRegistry = errors.New("cluster: invalid command codec registry")
	// ErrDuplicateCommandCodec 表示同一 CommandID 被重复注册。
	ErrDuplicateCommandCodec = errors.New("cluster: duplicate command codec")
	// ErrCommandCodecNotFound 表示 CommandID 未注册编解码器。
	ErrCommandCodecNotFound = errors.New("cluster: command codec not found")
)

// NewCommandCodecRegistry 校验并复制启动期的命令编解码器注册项。
func NewCommandCodecRegistry(entries []CommandCodecEntry) (*CommandCodecRegistry, error) {
	codecs := make(map[runtime.CommandID]CommandCodec, len(entries))
	for _, entry := range entries {
		if entry.CommandID == 0 || isNilCommandCodec(entry.Codec) {
			return nil, fmt.Errorf("%w: CommandID and Codec are required", ErrInvalidCommandRegistry)
		}
		if _, exists := codecs[entry.CommandID]; exists {
			return nil, fmt.Errorf("%w: command %d", ErrDuplicateCommandCodec, entry.CommandID)
		}
		codecs[entry.CommandID] = entry.Codec
	}
	return &CommandCodecRegistry{codecs: codecs}, nil
}

// MustCommandCodecRegistry 构造静态注册表，遇到编程错误时触发 panic。
func MustCommandCodecRegistry(entries []CommandCodecEntry) *CommandCodecRegistry {
	registry, err := NewCommandCodecRegistry(entries)
	if err != nil {
		panic(err)
	}
	return registry
}

// Lookup 返回 CommandID 对应的已注册不可变编解码器。
func (registry *CommandCodecRegistry) Lookup(command runtime.CommandID) (CommandCodec, error) {
	if registry == nil || command == 0 {
		return nil, ErrInvalidCommandRegistry
	}
	codec, exists := registry.codecs[command]
	if !exists {
		return nil, fmt.Errorf("%w: command %d", ErrCommandCodecNotFound, command)
	}
	return codec, nil
}

func isNilCommandCodec(codec CommandCodec) bool {
	if codec == nil {
		return true
	}
	value := reflect.ValueOf(codec)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}
