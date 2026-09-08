package cluster

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
)

var (
	errInvalidInboundConnection = errors.New("cluster: invalid inbound connection")
	errInboundRequestPanicked   = errors.New("cluster: inbound request adapter panicked")
)

// inboundConnection 管理单个入站连接的有界 Call 任务和非阻塞诊断。
// 每条连接拥有独立 owner，不要求来源 NodeID 或身份交换。
// 组装方限制节点级连接总数，再调用一次 serve 并等待其返回。
type inboundConnection struct {
	connection    *streamConnection
	adapter       *localRequestAdapter
	maxCalls      int
	started       atomic.Bool
	diagnostics   *inboundDiagnostics
	remoteAddress string
}

func newInboundConnection(connection *streamConnection, adapter *localRequestAdapter, maxCalls int, diagnostics *inboundDiagnostics) (*inboundConnection, error) {
	if connection == nil || adapter == nil || adapter.runtime == nil || adapter.codecs == nil || maxCalls <= 0 || diagnostics == nil || diagnostics.failures == nil {
		return nil, errInvalidInboundConnection
	}
	return &inboundConnection{
		connection: connection, adapter: adapter, maxCalls: maxCalls,
		diagnostics: diagnostics, remoteAddress: connection.socket.RemoteAddr().String(),
	}, nil
}

// serve 自身是唯一读循环，额外创建一个取消监视任务和至多 maxCalls 个 Call 任务。
// 返回前关闭连接、取消基础设施 Call、等待所属任务退出；不关闭共享诊断出口。
// codec/解析器必须协作返回；Go 不能强杀无限阻塞的扩展代码。
func (owner *inboundConnection) serve(parent context.Context) (result error) {
	if owner == nil || parent == nil || !owner.started.CompareAndSwap(false, true) {
		return errInvalidInboundConnection
	}
	ctx, cancel := context.WithCancel(parent)
	watchDone := make(chan struct{})
	go func() {
		defer close(watchDone)
		<-ctx.Done()
		owner.connection.close()
	}()
	var workers sync.WaitGroup
	defer func() {
		cancel()
		result = errors.Join(result, owner.connection.close())
		workers.Wait()
		<-watchDone
	}()
	slots := make(chan struct{}, owner.maxCalls)
	for {
		// 在读下一帧前预留名额，满载时连后续 Send 也背压；最多保留一帧读取中的数据。
		select {
		case slots <- struct{}{}:
		case <-ctx.Done():
			return ctx.Err()
		}
		request, err := owner.connection.read()
		if err != nil {
			<-slots
			if ctx.Err() != nil {
				return ctx.Err()
			}
			var complete *completeFrameError
			if errors.As(err, &complete) {
				owner.report(0, err)
				continue
			}
			return err
		}
		if request.Kind == FrameRequest && request.Session != 0 {
			workers.Add(1)
			go func() {
				defer workers.Done()
				defer func() { <-slots }()
				owner.handle(ctx, request)
			}()
			continue
		}
		owner.handle(ctx, request)
		<-slots
	}
}

func (owner *inboundConnection) handle(ctx context.Context, request ClusterEnvelope) {
	defer func() {
		if recover() != nil {
			owner.report(request.Session, errInboundRequestPanicked)
		}
	}()
	if err := owner.adapter.handle(ctx, request, owner.connection.write); err != nil {
		owner.report(request.Session, err)
		if errors.Is(err, ErrConnectionAgentWrite) {
			owner.connection.close()
		}
	}
}

func (owner *inboundConnection) report(session ClusterSessionID, err error) {
	owner.diagnostics.report(inboundFailure{remoteAddress: owner.remoteAddress, session: session, err: err})
}
