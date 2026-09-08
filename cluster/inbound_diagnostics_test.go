package cluster

import (
	"errors"
	"sync"
	"testing"
)

// 验证并发生产者共享一个有界出口，满载立即丢弃并准确计数，不需要消费者配合。
func TestInboundDiagnosticsConcurrentOverflow(t *testing.T) {
	diagnostics := newInboundDiagnostics()
	cause := errors.New("diagnostic")
	var producers sync.WaitGroup
	for index := 0; index < 8; index++ {
		producers.Add(1)
		go func() {
			defer producers.Done()
			for session := 1; session <= 64; session++ {
				diagnostics.report(inboundFailure{remoteAddress: "peer", session: ClusterSessionID(session), err: cause})
			}
		}()
	}
	producers.Wait()
	close(diagnostics.failures)
	count := 0
	for failure := range diagnostics.failures {
		if failure.remoteAddress != "peer" || failure.session == 0 || !errors.Is(failure.err, cause) {
			t.Fatalf("diagnostic = %+v", failure)
		}
		count++
	}
	if count != defaultFailureQueueCapacity || diagnostics.dropped.Load() != uint64(512-count) {
		t.Fatalf("received = %d, dropped = %d", count, diagnostics.dropped.Load())
	}
}
