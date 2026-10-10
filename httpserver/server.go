package httpserver

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"
)

var errInvalidConfig = errors.New("httpserver: invalid config")

// Config 指定宿主持有的 HTTP 监听地址和处理入口。
type Config struct {
	Address string
	Handler http.Handler
}

// Server 持有一个 HTTP Listener 和它的 Serve goroutine。
// 使用方应先关闭 Server，等待在途请求结束，再停止被 Handler 调用的 Runtime。
type Server struct {
	config    Config
	mu        sync.Mutex
	server    *http.Server
	addr      net.Addr
	done      chan error
	closed    bool
	closeDone chan struct{}
	closeErr  error // closeDone 关闭后读取。
}

// New 校验 HTTP 边界；不会提前绑定端口。
func New(config Config) (*Server, error) {
	if config.Address == "" || config.Handler == nil {
		return nil, errInvalidConfig
	}
	return &Server{config: config, closeDone: make(chan struct{})}, nil
}

// Start 绑定地址并启动唯一的 HTTP Serve goroutine。
func (server *Server) Start() error {
	server.mu.Lock()
	defer server.mu.Unlock()
	if server.closed || server.server != nil {
		return fmt.Errorf("httpserver: already started or closed")
	}
	listener, err := net.Listen("tcp", server.config.Address)
	if err != nil {
		return fmt.Errorf("httpserver: listen: %w", err)
	}
	server.server = &http.Server{
		Handler:           server.config.Handler,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       5 * time.Second,
	}
	server.addr = listener.Addr()
	server.done = make(chan error, 1)
	go func() {
		err := server.server.Serve(listener)
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
		server.done <- err
	}()
	return nil
}

// Addr 返回已启动监听的实际地址；启动前返回 nil。
func (server *Server) Addr() net.Addr {
	server.mu.Lock()
	defer server.mu.Unlock()
	return server.addr
}

// Close 停止接纳请求并等待 HTTP Handler 完成；首次调用 ctx 超时后强制关闭连接。
// 重复调用只控制自身等待，不取消原关闭流程，并返回同一最终结果。
func (server *Server) Close(ctx context.Context) error {
	if ctx == nil {
		return errors.New("httpserver: close context is required")
	}
	server.mu.Lock()
	if server.closed {
		server.mu.Unlock()
		return server.waitClosed(ctx)
	}
	server.closed = true
	httpServer := server.server
	done := server.done
	server.mu.Unlock()
	var result error
	if httpServer != nil {
		result = httpServer.Shutdown(ctx)
		if result != nil {
			result = errors.Join(result, httpServer.Close())
		}
		result = errors.Join(result, <-done)
	}
	server.closeErr = result
	close(server.closeDone)
	return result
}

func (server *Server) waitClosed(ctx context.Context) error {
	select {
	case <-server.closeDone:
		return server.closeErr
	default:
	}
	select {
	case <-server.closeDone:
		return server.closeErr
	case <-ctx.Done():
		return ctx.Err()
	}
}
