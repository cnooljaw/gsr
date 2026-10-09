package gate

import (
	"net"
	"sync"
)

// connectionLimitListener 在 HTTP 层之前限制已接纳的 TCP 连接数。
type connectionLimitListener struct {
	net.Listener
	slots  chan struct{}
	reject func()
}

func newConnectionLimitListener(listener net.Listener, limit int, reject func()) *connectionLimitListener {
	return &connectionLimitListener{
		Listener: listener,
		slots:    make(chan struct{}, limit),
		reject:   reject,
	}
}

// Accept 只向 HTTP Server 返回有容量名额的连接，超限连接立即关闭。
func (listener *connectionLimitListener) Accept() (net.Conn, error) {
	for {
		connection, err := listener.Listener.Accept()
		if err != nil {
			return nil, err
		}
		select {
		case listener.slots <- struct{}{}:
			return &limitedNetConn{Conn: connection, slots: listener.slots}, nil
		default:
			if listener.reject != nil {
				listener.reject()
			}
			_ = connection.Close()
		}
	}
}

type limitedNetConn struct {
	net.Conn
	slots chan struct{}
	once  sync.Once
}

// Close 释放连接容量名额，重复关闭不会重复释放。
func (connection *limitedNetConn) Close() error {
	err := connection.Conn.Close()
	connection.once.Do(func() {
		<-connection.slots
	})
	return err
}
