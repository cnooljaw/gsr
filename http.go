package gsr

import (
	"context"
	"errors"
	"net/http"

	"github.com/cnooljaw/gsr/cluster"
	"github.com/cnooljaw/gsr/httpserver"
	"github.com/cnooljaw/gsr/runtime"
)

// HTTPIngress 是供 HTTP Handler 保存和使用的受信任本地入口，不是 ServiceContext。
// SendName 只保证本地消息被接纳；CallName 等待目标回复，二者的 Source 均为零。
// 入站鉴权、参数校验、超时和 HTTP 响应格式由应用 Handler 决定。
type HTTPIngress struct {
	node *application
}

// SendName 按当前名称表解析目标，并以基础设施来源投递本地 Command。
func (in *HTTPIngress) SendName(name cluster.ServiceName, command runtime.CommandID, args ...any) error {
	if in == nil || in.node == nil || in.node.local == nil {
		return runtime.ErrRuntimeClosed
	}
	id, err := in.node.resolve(name)
	if err != nil {
		return err
	}
	return in.node.local.Send(id, command, args...)
}

// CallName 按当前名称表解析目标，并等待本地 Service 的回复；ctx 可缩短等待时间。
func (in *HTTPIngress) CallName(ctx context.Context, name cluster.ServiceName, command runtime.CommandID, args ...any) (any, error) {
	if ctx == nil {
		return nil, runtime.ErrInvalidCallContext
	}
	if in == nil || in.node == nil || in.node.local == nil {
		return nil, runtime.ErrRuntimeClosed
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	id, err := in.node.resolve(name)
	if err != nil {
		return nil, err
	}
	return in.node.local.Call(ctx, id, command, args...)
}

// OpenHTTP 在本节点开启唯一的 HTTP 监听。Handler 构造函数收到可长期保存的入站入口；
// Run 在关闭 Runtime 前停止监听，并限时等待在途 Handler；超时不能强杀 Handler。
// 监听失败后可再次尝试；已打开或已关闭时不调用路由构造函数。
// 构造函数只组装路由，不启动资源或执行业务通信；并发打开可能各自完成构造，但只接纳一个监听。
func (capability *Node) OpenHTTP(address string, handler func(*HTTPIngress) http.Handler) error {
	if handler == nil {
		return errors.New("gsr: HTTP handler is required")
	}
	node := capability.node
	node.httpMu.Lock()
	closed, opened := node.httpClosed, node.httpServer != nil
	node.httpMu.Unlock()
	if closed {
		return runtime.ErrRuntimeClosed
	}
	if opened {
		return errors.New("gsr: HTTP listener already open")
	}
	// 业务构造函数在锁外执行；构造后再次检查状态，避免并发打开或停机漏过接纳检查。
	server, err := httpserver.New(httpserver.Config{
		Address: address,
		Handler: handler(&HTTPIngress{node: node}),
	})
	if err != nil {
		return err
	}
	node.httpMu.Lock()
	defer node.httpMu.Unlock()
	if node.httpClosed {
		return runtime.ErrRuntimeClosed
	}
	if node.httpServer != nil {
		return errors.New("gsr: HTTP listener already open")
	}
	if err := server.Start(); err != nil {
		return err
	}
	node.httpServer = server
	return nil
}
