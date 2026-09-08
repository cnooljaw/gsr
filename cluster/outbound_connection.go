package cluster

import (
	"errors"
	"net"
	"time"
)

// outboundConnection 将一条已建立的出站 socket 与其等待归属绑定。
// 不负责 Dial、重连或创建 goroutine；外层负责唯一读循环、关闭与等待。
type outboundConnection struct {
	stream *streamConnection
	calls  *connectionCalls
}

func newOutboundConnection(stream *streamConnection, registry *ClusterCallRegistry, node NodeID) (*outboundConnection, error) {
	if stream == nil {
		return nil, net.ErrClosed
	}
	calls, err := newConnectionCalls(registry, node)
	if err != nil {
		return nil, err
	}
	return &outboundConnection{stream: stream, calls: calls}, nil
}

// write 在串行写入权内检查 Call 是否仍有效，绑定后至多尝试写入一次。
// 绑定前取消则跳过发送；绑定后取消不承诺撤销已开始的发送尝试。
func (connection *outboundConnection) write(request ClusterEnvelope) error {
	stream := connection.stream
	stream.writeMu.Lock()
	defer stream.writeMu.Unlock()
	frame, encodeErr := stream.codec.Encode(request)
	if stream.closed.Load() {
		connection.calls.fail(net.ErrClosed)
	}
	ready, err := connection.calls.bind(request)
	if err != nil {
		connection.calls.reject(request, err)
		return err
	}
	if !ready {
		return nil
	}
	if encodeErr != nil {
		if request.Session != 0 {
			connection.calls.registry.cancel(request.Session, encodeErr)
		}
		return encodeErr
	}
	if err = stream.socket.SetWriteDeadline(time.Now().Add(stream.writeTimeout)); err == nil {
		err = writeFrameBytes(stream.socket, frame)
	}
	if err != nil {
		err = errors.Join(err, stream.close())
		connection.calls.fail(err)
	}
	return err
}
