package cluster

import (
	"errors"
	"fmt"
	"reflect"

	runtime "github.com/cnooljaw/gsr/runtime"
)

// CommandCodec converts one Command payload to and from bytes. A codec may be
// the default ValueCodec or a command-specific typed implementation.
type CommandCodec interface {
	Encode(value any) ([]byte, error)
	Decode(payload []byte) (any, error)
}

// CommandCodecEntry binds one non-zero CommandID to a payload codec.
type CommandCodecEntry struct {
	CommandID runtime.CommandID
	Codec     CommandCodec
}

// CommandCodecRegistry is an immutable command-to-codec lookup table.
type CommandCodecRegistry struct {
	codecs map[runtime.CommandID]CommandCodec
}

var (
	// ErrInvalidCommandRegistry means a registry entry or registry receiver is invalid.
	ErrInvalidCommandRegistry = errors.New("cluster: invalid command codec registry")
	// ErrDuplicateCommandCodec means one CommandID was registered more than once.
	ErrDuplicateCommandCodec = errors.New("cluster: duplicate command codec")
	// ErrCommandCodecNotFound means no codec is registered for a CommandID.
	ErrCommandCodecNotFound = errors.New("cluster: command codec not found")
)

// NewCommandCodecRegistry validates and copies startup command codec entries.
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

// MustCommandCodecRegistry constructs a static registry or panics on a programmer error.
func MustCommandCodecRegistry(entries []CommandCodecEntry) *CommandCodecRegistry {
	registry, err := NewCommandCodecRegistry(entries)
	if err != nil {
		panic(err)
	}
	return registry
}

// Lookup returns the immutable codec registered for a CommandID.
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
