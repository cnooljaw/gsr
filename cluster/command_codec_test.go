package cluster

import (
	"errors"
	"testing"

	runtime "github.com/cnooljaw/gsr/runtime"
)

const registryTestCommand runtime.CommandID = 0x05000102

// 验证 Command codec registry 能查找已注册 codec，并隔离启动 entries 的后续修改。
func TestCommandCodecRegistryLooksUpImmutableEntries(t *testing.T) {
	codec, err := NewValueCodec(ValueCodecConfig{})
	if err != nil {
		t.Fatalf("NewValueCodec() error = %v, want nil", err)
	}
	entries := []CommandCodecEntry{{CommandID: registryTestCommand, Codec: codec}}
	registry, err := NewCommandCodecRegistry(entries)
	if err != nil {
		t.Fatalf("NewCommandCodecRegistry() error = %v, want nil", err)
	}
	entries[0] = CommandCodecEntry{}

	registered, err := registry.Lookup(registryTestCommand)
	if err != nil {
		t.Fatalf("Lookup() error = %v, want nil", err)
	}
	payload, err := registered.Encode("room")
	if err != nil {
		t.Fatalf("registered Encode() error = %v, want nil", err)
	}
	value, err := registered.Decode(payload)
	if err != nil {
		t.Fatalf("registered Decode() error = %v, want nil", err)
	}
	if value != "room" {
		t.Fatalf("registered Decode() = %#v, want room", value)
	}
}

// 验证 registry 拒绝零 CommandID、nil codec 和重复注册，避免启动后覆盖协议定义。
func TestCommandCodecRegistryRejectsInvalidEntries(t *testing.T) {
	codec, err := NewValueCodec(ValueCodecConfig{})
	if err != nil {
		t.Fatalf("NewValueCodec() error = %v, want nil", err)
	}
	cases := []struct {
		name    string
		entries []CommandCodecEntry
		wantErr error
	}{
		{
			name:    "zero command",
			entries: []CommandCodecEntry{{Codec: codec}},
			wantErr: ErrInvalidCommandRegistry,
		},
		{
			name:    "nil codec",
			entries: []CommandCodecEntry{{CommandID: registryTestCommand}},
			wantErr: ErrInvalidCommandRegistry,
		},
		{
			name: "duplicate command",
			entries: []CommandCodecEntry{
				{CommandID: registryTestCommand, Codec: codec},
				{CommandID: registryTestCommand, Codec: codec},
			},
			wantErr: ErrDuplicateCommandCodec,
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if _, err := NewCommandCodecRegistry(testCase.entries); !errors.Is(err, testCase.wantErr) {
				t.Fatalf("NewCommandCodecRegistry() error = %v, want %v", err, testCase.wantErr)
			}
		})
	}
}

// 验证未知命令和无效 registry 查找返回稳定错误，而不是 nil codec 或 panic。
func TestCommandCodecRegistryLookupErrors(t *testing.T) {
	registry, err := NewCommandCodecRegistry(nil)
	if err != nil {
		t.Fatalf("NewCommandCodecRegistry(nil) error = %v, want nil", err)
	}
	if _, err := registry.Lookup(registryTestCommand); !errors.Is(err, ErrCommandCodecNotFound) {
		t.Fatalf("unknown Lookup() error = %v, want ErrCommandCodecNotFound", err)
	}
	if _, err := registry.Lookup(0); !errors.Is(err, ErrInvalidCommandRegistry) {
		t.Fatalf("zero Lookup() error = %v, want ErrInvalidCommandRegistry", err)
	}
	var nilRegistry *CommandCodecRegistry
	if _, err := nilRegistry.Lookup(registryTestCommand); !errors.Is(err, ErrInvalidCommandRegistry) {
		t.Fatalf("nil registry Lookup() error = %v, want ErrInvalidCommandRegistry", err)
	}
}
