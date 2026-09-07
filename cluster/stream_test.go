package cluster

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"testing"
	"testing/iotest"
)

// 验证连续帧即使逐字节到达也能分别读取，正常流结束返回 EOF。
func TestStreamReadsFragmentedFrames(t *testing.T) {
	codec, err := NewFrameCodec(FrameCodecConfig{})
	if err != nil {
		t.Fatal(err)
	}
	var stream bytes.Buffer
	for _, session := range []ClusterSessionID{1, 2} {
		request := ClusterEnvelope{Kind: FrameRequest, Session: session, Target: ServiceTarget{ServiceName: "room"}, Command: 1}
		if err := codec.writeFrame(&stream, request); err != nil {
			t.Fatal(err)
		}
	}
	reader := iotest.OneByteReader(&stream)
	for _, session := range []ClusterSessionID{1, 2} {
		request, err := codec.readFrame(reader)
		if err != nil || request.Session != session || request.Target.ServiceName != "room" {
			t.Fatalf("readFrame = %+v, %v", request, err)
		}
	}
	if _, err := codec.readFrame(reader); !errors.Is(err, io.EOF) {
		t.Fatalf("end of stream = %v, want EOF", err)
	}
}

// 验证截断帧报告不完整读取，非法长度在尝试读取 body 前被拒绝。
func TestStreamRejectsTruncationAndInvalidLength(t *testing.T) {
	codec, err := NewFrameCodec(FrameCodecConfig{MaxFrame: 64})
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name  string
		input []byte
		want  error
	}{
		{name: "partial header", input: []byte{0, 0}, want: io.ErrUnexpectedEOF},
		{name: "missing body", input: []byte{0, 0, 0, 8}, want: io.ErrUnexpectedEOF},
		{name: "partial body", input: []byte{0, 0, 0, 8, 1}, want: io.ErrUnexpectedEOF},
		{name: "zero length", input: []byte{0, 0, 0, 0}, want: ErrMalformedFrame},
		{name: "over limit", input: []byte{0, 0, 0, 61}, want: ErrFrameTooLarge},
		{name: "uint32 max", input: []byte{255, 255, 255, 255}, want: ErrFrameTooLarge},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if _, err := codec.readFrame(bytes.NewReader(testCase.input)); !errors.Is(err, testCase.want) {
				t.Fatalf("readFrame error = %v, want %v", err, testCase.want)
			}
		})
	}
}

// 验证完整坏帧已被消费，调用方选择丢弃后仍能读取下一帧。
func TestStreamPreservesBoundaryAfterMalformedBody(t *testing.T) {
	codec, err := NewFrameCodec(FrameCodecConfig{})
	if err != nil {
		t.Fatal(err)
	}
	request := ClusterEnvelope{Kind: FrameRequest, Target: ServiceTarget{ServiceID: 7}, Command: 1}
	valid, err := codec.Encode(request)
	if err != nil {
		t.Fatal(err)
	}
	bad := make([]byte, 5)
	binary.BigEndian.PutUint32(bad, 1)
	bad[4] = 255
	reader := bytes.NewReader(append(bad, valid...))
	if _, err := codec.readFrame(reader); !errors.Is(err, ErrMalformedFrame) {
		t.Fatalf("bad frame = %v", err)
	}
	if got, err := codec.readFrame(reader); err != nil || got.Target != request.Target {
		t.Fatalf("next frame = %+v, %v", got, err)
	}
}

// 验证写入错误和无错误短写被交回连接 owner，失败后不重放帧或剩余字节。
func TestStreamReportsWriteFailureWithoutReplay(t *testing.T) {
	codec, err := NewFrameCodec(FrameCodecConfig{})
	if err != nil {
		t.Fatal(err)
	}
	writeErr := errors.New("connection lost")
	for _, failure := range []error{nil, writeErr} {
		writer := &streamFailWriter{err: failure}
		request := ClusterEnvelope{Kind: FrameRequest, Target: ServiceTarget{ServiceID: 7}, Command: 1}
		want := failure
		if want == nil {
			want = io.ErrShortWrite
		}
		if err := codec.writeFrame(writer, request); !errors.Is(err, want) {
			t.Fatalf("writeFrame error = %v, want %v", err, want)
		}
		if writer.calls != 1 {
			t.Fatalf("Write called %d times, want 1", writer.calls)
		}
	}
}

type streamFailWriter struct {
	err   error
	calls int
}

// 验证未构造的 codec 在读取流之前失败，避免无效上限导致不受限分配。
func TestStreamRejectsUninitializedCodec(t *testing.T) {
	for _, codec := range []*FrameCodec{nil, {}} {
		if _, err := codec.readFrame(nil); !errors.Is(err, ErrInvalidFrameConfig) {
			t.Fatalf("readFrame error = %v, want ErrInvalidFrameConfig", err)
		}
	}
}

func (writer *streamFailWriter) Write(_ []byte) (int, error) {
	writer.calls++
	return 1, writer.err
}
