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
	badFrames     badFrameDiagnostics
	// listener 组装时共享门禁；独立连接测试不配置时保持原接纳行为。
	admissionStopped *atomic.Bool
}

func newInboundConnection(connection *streamConnection, adapter *localRequestAdapter, maxCalls int, diagnostics *inboundDiagnostics) (*inboundConnection, error) {
	if connection == nil || adapter == nil || adapter.runtime == nil || isNilPayloadCodec(adapter.codec) || maxCalls <= 0 || diagnostics == nil || (diagnostics.failures == nil && diagnostics.shared == nil) {
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
		if count := owner.badFrames.suppressed; count > 0 {
			owner.diagnostics.report(inboundFailure{remoteAddress: owner.remoteAddress,
				err: errBadFramesSuppressed, suppressed: count})
		}
		<-watchDone
	}()
	slots := make(chan struct{}, owner.maxCalls)
	for {
		if owner.stoppedAdmission() {
			<-ctx.Done()
			return ctx.Err()
		}
		// 在读下一帧前预留名额，满载时连后续 Send 也背压；最多保留一帧读取中的数据。
		select {
		case slots <- struct{}{}:
		case <-ctx.Done():
			return ctx.Err()
		}
		request, err := owner.connection.read()
		// 这次原子检查是完整帧的 admission 边界；先于停止者才可交给处理任务。
		// 即使读取期间发生停止，也不能把新请求投递给本地 Runtime。
		if owner.stoppedAdmission() {
			<-slots
			<-ctx.Done()
			return ctx.Err()
		}
		if err != nil {
			<-slots
			if ctx.Err() != nil {
				return ctx.Err()
			}
			var complete *completeFrameError
			if errors.As(err, &complete) {
				owner.report(ClusterEnvelope{}, err)
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

func (owner *inboundConnection) stoppedAdmission() bool {
	return owner.admissionStopped != nil && owner.admissionStopped.Load()
}

func (owner *inboundConnection) handle(ctx context.Context, request ClusterEnvelope) {
	defer func() {
		if recover() != nil {
			owner.report(request, errInboundRequestPanicked)
		}
	}()
	if err := owner.adapter.handle(ctx, request, owner.connection.write); err != nil {
		owner.report(request, err)
		if errors.Is(err, errInboundResponseWrite) {
			owner.connection.close()
		}
	}
}

func (owner *inboundConnection) report(request ClusterEnvelope, err error) {
	if !owner.badFrames.accept(err) {
		if shared := owner.diagnostics.shared; shared != nil {
			shared.suppressed.Add(1)
		}
		return
	}
	if request.Validate() != nil {
		request = ClusterEnvelope{}
	}
	owner.diagnostics.report(inboundFailure{remoteAddress: owner.remoteAddress, session: request.Session,
		target: request.Target, command: request.Command, err: err})
}
