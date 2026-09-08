package cluster

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"os"
	"testing"
	"testing/iotest"
	"time"
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
			_, err := codec.readFrame(bytes.NewReader(testCase.input))
			if !errors.Is(err, testCase.want) {
				t.Fatalf("readFrame error = %v, want %v", err, testCase.want)
			}
			var complete *completeFrameError
			if errors.As(err, &complete) {
				t.Fatal("invalid length or truncation marked as a complete frame")
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
	_, err = codec.readFrame(reader)
	if !errors.Is(err, ErrMalformedFrame) {
		t.Fatalf("bad frame = %v", err)
	}
	var complete *completeFrameError
	if !errors.As(err, &complete) {
		t.Fatal("missing complete frame boundary marker")
	}
	if got, err := codec.readFrame(reader); err != nil || got.Target != request.Target {
		t.Fatalf("next frame = %+v, %v", got, err)
	}
}

// 验证写入错误和无错误短写被交回连接管理组件，失败后不重放帧或剩余字节。
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

// 验证未构造的编解码器在读取流之前失败，避免无效上限导致不受限分配。
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

// 验证 ConnectionAgent 通过双向字节流在原连接返回同一 Session 的回复。
func TestStreamConnectionAgentRoundTrip(t *testing.T) {
	codec, err := NewFrameCodec(FrameCodecConfig{})
	if err != nil {
		t.Fatal(err)
	}
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()
	for _, conn := range []net.Conn{client, server} {
		if err := conn.SetDeadline(time.Now().Add(time.Second)); err != nil {
			t.Fatal(err)
		}
	}
	agent, err := NewConnectionAgent(ConnectionAgentConfig{
		Handler: func(_ context.Context, request ClusterEnvelope) ([]byte, error) {
			return request.Payload, nil
		},
		Write: func(response ClusterEnvelope) error {
			return codec.writeFrame(server, response)
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		request, err := codec.readFrame(server)
		if err == nil {
			err = agent.Handle(context.Background(), request)
		}
		done <- err
	}()
	defer func() {
		client.Close()
		server.Close()
		if err := <-done; err != nil {
			t.Errorf("server exchange: %v", err)
		}
	}()
	request := ClusterEnvelope{Kind: FrameRequest, Session: 42, Target: ServiceTarget{ServiceID: 7}, Command: 1, Payload: []byte("hello")}
	if err := codec.writeFrame(client, request); err != nil {
		t.Fatal(err)
	}
	response, err := codec.readFrame(client)
	if err != nil {
		t.Fatal(err)
	}
	if response.Kind != FrameResponse || response.Session != request.Session || response.Target != request.Target ||
		response.Command != request.Command || !bytes.Equal(response.Payload, request.Payload) {
		t.Fatalf("unexpected response: %+v", response)
	}
}

// 验证连接管理组件关闭连接后，等待帧读取或对端消费的写入都能退出。
func TestStreamCloseUnblocksIO(t *testing.T) {
	codec, err := NewFrameCodec(FrameCodecConfig{})
	if err != nil {
		t.Fatal(err)
	}
	for _, operation := range []string{"read", "write"} {
		t.Run(operation, func(t *testing.T) {
			local, peer := net.Pipe()
			defer peer.Close()
			defer local.Close()
			started := make(chan struct{})
			done := make(chan error, 1)
			go func() {
				close(started)
				if operation == "read" {
					_, err := codec.readFrame(local)
					done <- err
					return
				}
				done <- codec.writeFrame(local, ClusterEnvelope{Kind: FrameRequest, Target: ServiceTarget{ServiceID: 7}, Command: 1})
			}()
			<-started
			if err := local.Close(); err != nil {
				t.Error(err)
			}
			select {
			case err := <-done:
				if !errors.Is(err, io.ErrClosedPipe) {
					t.Fatalf("I/O error = %v, want closed pipe", err)
				}
			case <-time.After(time.Second):
				t.Fatal("I/O did not exit after close")
			}
		})
	}
}

// 验证连接管理组件设置的截止时间能终止帧读写，并保留可判断的超时原因。
func TestStreamHonorsConnectionDeadline(t *testing.T) {
	codec, err := NewFrameCodec(FrameCodecConfig{})
	if err != nil {
		t.Fatal(err)
	}
	local, peer := net.Pipe()
	defer local.Close()
	defer peer.Close()
	if err := local.SetDeadline(time.Now().Add(-time.Second)); err != nil {
		t.Fatal(err)
	}
	_, err = codec.readFrame(local)
	if !errors.Is(err, os.ErrDeadlineExceeded) {
		t.Fatalf("read error = %v, want deadline exceeded", err)
	}
	var complete *completeFrameError
	if errors.As(err, &complete) {
		t.Fatal("read timeout marked as a complete frame")
	}
	err = codec.writeFrame(local, ClusterEnvelope{Kind: FrameRequest, Target: ServiceTarget{ServiceID: 7}, Command: 1})
	if !errors.Is(err, os.ErrDeadlineExceeded) {
		t.Fatalf("write error = %v, want deadline exceeded", err)
	}
}
