package cluster

import (
	"errors"
	"io"
	"net"
	"os"
	"testing"
	"time"
)

// 验证同一连接上的并发回复保持完整帧，连接关闭后拒绝后续写入。
func TestStreamConnectionSerializesWrites(t *testing.T) {
	connection, peer, codec := newTestStreamConnection(t)
	const count = 8
	done := make(chan error, count)
	for i := 1; i <= count; i++ {
		go func(session ClusterSessionID) {
			done <- connection.write(ClusterEnvelope{Kind: FrameResponse, Session: session, Target: ServiceTarget{ServiceID: 1}, Command: 1})
		}(ClusterSessionID(i))
	}
	t.Cleanup(func() {
		connection.close()
		for i := 0; i < count; i++ {
			if err := <-done; err != nil {
				t.Error(err)
			}
		}
	})
	seen := make(map[ClusterSessionID]bool)
	for i := 0; i < count; i++ {
		frame, err := codec.readFrame(peer)
		if err != nil {
			t.Fatal(err)
		}
		if frame.Session < 1 || frame.Session > count || seen[frame.Session] {
			t.Fatalf("unexpected session %d", frame.Session)
		}
		seen[frame.Session] = true
	}
	if err := connection.close(); err != nil {
		t.Fatal(err)
	}
	if err := connection.close(); err != nil {
		t.Fatalf("repeated close: %v", err)
	}
	err := connection.write(ClusterEnvelope{Kind: FrameRequest, Target: ServiceTarget{ServiceID: 1}, Command: 1})
	if !errors.Is(err, net.ErrClosed) {
		t.Fatalf("write after close = %v", err)
	}
}

// 验证本地编码失败不关闭连接，后续有效帧仍可发送。
func TestStreamConnectionEncodingFailureKeepsConnection(t *testing.T) {
	connection, peer, codec := newTestStreamConnection(t)
	if err := connection.write(ClusterEnvelope{}); err == nil {
		t.Fatal("invalid envelope accepted")
	}
	done := make(chan error, 1)
	go func() {
		done <- connection.write(ClusterEnvelope{Kind: FrameRequest, Target: ServiceTarget{ServiceID: 1}, Command: 1})
	}()
	defer func() {
		connection.close()
		if err := <-done; err != nil {
			t.Error(err)
		}
	}()
	if _, err := codec.readFrame(peer); err != nil {
		t.Fatal(err)
	}
}

// 验证写入超时会关闭连接并解除读取等待，后续写入不会重放到损坏连接。
func TestStreamConnectionWriteTimeoutClosesConnection(t *testing.T) {
	connection, _, _ := newTestStreamConnection(t)
	connection.writeTimeout = 20 * time.Millisecond
	readDone := make(chan error, 1)
	go func() {
		_, err := connection.read()
		readDone <- err
	}()
	defer func() {
		connection.close()
		if err := <-readDone; !errors.Is(err, io.ErrClosedPipe) {
			t.Errorf("blocked read = %v", err)
		}
	}()
	request := ClusterEnvelope{Kind: FrameRequest, Target: ServiceTarget{ServiceID: 1}, Command: 1}
	if err := connection.write(request); !errors.Is(err, os.ErrDeadlineExceeded) {
		t.Fatalf("write = %v, want deadline exceeded", err)
	}
	if err := connection.write(request); !errors.Is(err, net.ErrClosed) {
		t.Fatalf("subsequent write = %v, want closed", err)
	}
}

// 验证完整坏帧保持连接对齐，而非法长度会关闭连接并拒绝后续写入。
func TestStreamConnectionReadFailureDisposition(t *testing.T) {
	for _, complete := range []bool{true, false} {
		name := "invalid length"
		if complete {
			name = "complete bad frame"
		}
		t.Run(name, func(t *testing.T) {
			connection, peer, codec := newTestStreamConnection(t)
			done := make(chan error, 1)
			go func() {
				frame := []byte{0, 0, 0, 0}
				if complete {
					frame = []byte{0, 0, 0, 1, 255}
				}
				err := writeFrameBytes(peer, frame)
				if err == nil && complete {
					err = codec.writeFrame(peer, ClusterEnvelope{Kind: FrameRequest, Target: ServiceTarget{ServiceID: 1}, Command: 1})
				}
				done <- err
			}()
			defer func() {
				connection.close()
				if err := <-done; err != nil {
					t.Error(err)
				}
			}()
			if _, err := connection.read(); !errors.Is(err, ErrMalformedFrame) {
				t.Fatalf("read error = %v", err)
			}
			if complete {
				if _, err := connection.read(); err != nil {
					t.Fatalf("next frame: %v", err)
				}
				return
			}
			err := connection.write(ClusterEnvelope{Kind: FrameRequest, Target: ServiceTarget{ServiceID: 1}, Command: 1})
			if !errors.Is(err, net.ErrClosed) {
				t.Fatalf("write after invalid length = %v", err)
			}
		})
	}
}

// 验证连接构造拒绝缺失依赖与无限写等待，构造失败不接管或关闭调用方 socket。
func TestStreamConnectionRejectsInvalidConfig(t *testing.T) {
	_, peer, codec := newTestStreamConnection(t)
	for _, config := range []struct {
		socket net.Conn
		codec  *FrameCodec
		wait   time.Duration
	}{
		{codec: codec, wait: time.Second},
		{socket: peer, wait: time.Second},
		{socket: peer, codec: &FrameCodec{}, wait: time.Second},
		{socket: peer, codec: codec},
		{socket: peer, codec: codec, wait: -time.Second},
	} {
		if _, err := newStreamConnection(config.socket, config.codec, config.wait); err == nil {
			t.Fatal("invalid connection config accepted")
		}
	}
	if err := peer.SetWriteDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatalf("caller socket closed after failed construction: %v", err)
	}
}

func newTestStreamConnection(t *testing.T) (*streamConnection, net.Conn, *FrameCodec) {
	t.Helper()
	local, peer := net.Pipe()
	t.Cleanup(func() { peer.Close() })
	t.Cleanup(func() { local.Close() })
	if err := peer.SetDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatal(err)
	}
	codec, err := NewFrameCodec(FrameCodecConfig{})
	if err != nil {
		t.Fatal(err)
	}
	connection, err := newStreamConnection(local, codec, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { connection.close() })
	return connection, peer, codec
}
