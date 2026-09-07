package cluster

import (
	"errors"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

// streamConnection owns one socket, not a reader loop or reconnect policy.
// One reader may call read; infrastructure writers are serialized. The outer
// Transport must bound writer concurrency and wait for its own I/O goroutines.
type streamConnection struct {
	socket       net.Conn
	codec        *FrameCodec
	writeTimeout time.Duration
	writeMu      sync.Mutex
	closed       atomic.Bool
	closeOnce    sync.Once
	closeErr     error
}

// Ownership transfers only on successful construction. writeTimeout bounds each
// socket write, not time waiting for the writer lock or remote Handler execution.
func newStreamConnection(socket net.Conn, codec *FrameCodec, writeTimeout time.Duration) (*streamConnection, error) {
	if socket == nil || codec == nil || codec.maxFrame <= frameHeaderSize || writeTimeout <= 0 {
		return nil, errors.New("cluster: socket, initialized frame codec and positive write timeout are required")
	}
	return &streamConnection{socket: socket, codec: codec, writeTimeout: writeTimeout}, nil
}

func (connection *streamConnection) read() (ClusterEnvelope, error) {
	envelope, err := connection.codec.readFrame(connection.socket)
	if err != nil {
		var complete *completeFrameError
		if !errors.As(err, &complete) {
			err = errors.Join(err, connection.close())
		}
	}
	return envelope, err
}

func (connection *streamConnection) write(envelope ClusterEnvelope) error {
	connection.writeMu.Lock()
	defer connection.writeMu.Unlock()
	if connection.closed.Load() {
		return net.ErrClosed
	}
	frame, err := connection.codec.Encode(envelope)
	if err != nil {
		return err
	}
	if err := connection.socket.SetWriteDeadline(time.Now().Add(connection.writeTimeout)); err != nil {
		return errors.Join(err, connection.close())
	}
	if err := writeFrameBytes(connection.socket, frame); err != nil {
		return errors.Join(err, connection.close())
	}
	return nil
}

// close deliberately does not acquire writeMu: closing the socket must unblock
// an in-flight write before waiting writers can observe the closed state.
func (connection *streamConnection) close() error {
	connection.closeOnce.Do(func() {
		connection.closed.Store(true)
		connection.closeErr = connection.socket.Close()
	})
	return connection.closeErr
}
