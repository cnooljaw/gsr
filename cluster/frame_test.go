package cluster

import (
	"encoding/binary"
	"errors"
	"reflect"
	"testing"

	runtime "github.com/cnooljaw/gsr/runtime"
)

const frameTestCommand runtime.CommandID = 0x05000103

// 验证 FrameCodec 能往返 Request、Response、Error 及两种远端目标。
func TestFrameCodecRoundTripsEnvelopeKinds(t *testing.T) {
	codec, err := NewFrameCodec(FrameCodecConfig{})
	if err != nil {
		t.Fatalf("NewFrameCodec() error = %v, want nil", err)
	}

	cases := []ClusterEnvelope{
		{
			Kind:    FrameRequest,
			Target:  ServiceTarget{ServiceID: 7},
			Command: frameTestCommand,
			Payload: []byte("request"),
		},
		{
			Kind:    FrameRequest,
			Session: 11,
			Target:  ServiceTarget{ServiceName: "room"},
			Command: frameTestCommand,
			Payload: []byte("call"),
		},
		{
			Kind:    FrameResponse,
			Session: 11,
			Target:  ServiceTarget{ServiceID: 7},
			Command: frameTestCommand,
			Payload: []byte("response"),
		},
		{
			Kind:    FrameError,
			Session: 12,
			Target:  ServiceTarget{ServiceName: "room"},
			Command: frameTestCommand,
			Error:   runtime.NewStatusError(runtime.ErrorClassRuntime, runtime.ErrorCodeServiceNotFound, "not found"),
		},
	}

	for index, want := range cases {
		frame, err := codec.Encode(want)
		if err != nil {
			t.Fatalf("case %d Encode() error = %v, want nil", index, err)
		}
		if gotLength := int(binary.BigEndian.Uint32(frame[:4])); gotLength != len(frame)-4 {
			t.Fatalf("case %d length prefix = %d, want %d", index, gotLength, len(frame)-4)
		}
		got, err := codec.Decode(frame)
		if err != nil {
			t.Fatalf("case %d Decode() error = %v, want nil", index, err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("case %d Decode() = %#v, want %#v", index, got, want)
		}
	}
}

// 验证 FrameCodec 拒绝截断、尾随和未知目标标签，避免把错误边界交给业务层。
func TestFrameCodecRejectsMalformedFrames(t *testing.T) {
	codec, err := NewFrameCodec(FrameCodecConfig{})
	if err != nil {
		t.Fatalf("NewFrameCodec() error = %v, want nil", err)
	}
	frame, err := codec.Encode(ClusterEnvelope{
		Kind:    FrameRequest,
		Target:  ServiceTarget{ServiceID: 7},
		Command: frameTestCommand,
		Payload: []byte("request"),
	})
	if err != nil {
		t.Fatalf("Encode() error = %v, want nil", err)
	}

	cases := []struct {
		name    string
		frame   []byte
		wantErr error
	}{
		{name: "truncated", frame: frame[:len(frame)-1], wantErr: ErrMalformedFrame},
		{name: "trailing", frame: append(append([]byte(nil), frame...), 0), wantErr: ErrMalformedFrame},
		{name: "oversized length", frame: []byte{0xff, 0xff, 0xff, 0xff}, wantErr: ErrFrameTooLarge},
	}
	for _, testCase := range cases {
		if _, err := codec.Decode(testCase.frame); !errors.Is(err, testCase.wantErr) {
			t.Fatalf("%s Decode() error = %v, want %v", testCase.name, err, testCase.wantErr)
		}
	}

	unknownTarget := append([]byte(nil), frame...)
	unknownTarget[4+1+8] = 0xff
	if _, err := codec.Decode(unknownTarget); !errors.Is(err, ErrMalformedFrame) {
		t.Fatalf("unknown target Decode() error = %v, want ErrMalformedFrame", err)
	}
}

// 验证 FrameCodec 对完整帧大小和 payload 大小执行有限边界检查。
func TestFrameCodecEnforcesFrameLimit(t *testing.T) {
	codec, err := NewFrameCodec(FrameCodecConfig{MaxFrame: 64})
	if err != nil {
		t.Fatalf("NewFrameCodec() error = %v, want nil", err)
	}
	envelope := ClusterEnvelope{
		Kind:    FrameRequest,
		Target:  ServiceTarget{ServiceID: 7},
		Command: frameTestCommand,
		Payload: []byte("this payload makes the frame too large"),
	}
	if _, err := codec.Encode(envelope); !errors.Is(err, ErrFrameTooLarge) {
		t.Fatalf("large Encode() error = %v, want ErrFrameTooLarge", err)
	}

	defaultCodec, err := NewFrameCodec(FrameCodecConfig{})
	if err != nil {
		t.Fatalf("NewFrameCodec(default) error = %v, want nil", err)
	}
	frame, err := defaultCodec.Encode(envelope)
	if err != nil {
		t.Fatalf("default Encode() error = %v, want nil", err)
	}
	if _, err := codec.Decode(frame); !errors.Is(err, ErrFrameTooLarge) {
		t.Fatalf("large Decode() error = %v, want ErrFrameTooLarge", err)
	}
}

// 验证 FrameCodec 构造阶段拒绝过小的 framing 上限。
func TestNewFrameCodecRejectsInvalidConfig(t *testing.T) {
	if _, err := NewFrameCodec(FrameCodecConfig{MaxFrame: 3}); !errors.Is(err, ErrInvalidFrameConfig) {
		t.Fatalf("small MaxFrame error = %v, want ErrInvalidFrameConfig", err)
	}
}
