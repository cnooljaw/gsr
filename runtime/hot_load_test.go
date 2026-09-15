package runtime

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"sync"
	"testing"
	"time"
)

// 验证固定容量热点 Mailbox 的接纳和满载计数守恒，所有已接纳消息恰好处理一次并记录延迟分位数。
func TestRuntimeHotMailboxLoad(t *testing.T) {
	if os.Getenv("GSR_LOAD_TEST") != "1" {
		t.Skip("显式设置 GSR_LOAD_TEST=1 运行热点负载测量")
	}
	for _, capacity := range []int{64, 1024} {
		t.Run(fmt.Sprintf("capacity_%d", capacity), func(t *testing.T) {
			measureHotMailbox(t, capacity)
		})
	}
}

func measureHotMailbox(t *testing.T, capacity int) {
	t.Helper()
	const attempts = 100000
	const producers = 4
	local, err := NewRuntime(Config{MailboxCapacity: capacity})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := local.Close(); err != nil {
			t.Error(err)
		}
	})
	if err := local.Start(); err != nil {
		t.Fatal(err)
	}
	// 测量结果缓冲有固定上限，避免采样消费者给 Handler 施加额外背压。
	samples := make(chan hotSample, attempts)
	id, err := local.CreateService(context.Background(), &hotLoadService{samples: samples})
	if err != nil {
		t.Fatal(err)
	}
	accepted := make([]bool, attempts)
	failures := make(chan error, producers)
	var senders sync.WaitGroup
	startGate := make(chan struct{})
	for producer := 0; producer < producers; producer++ {
		senders.Add(1)
		go func(first int) {
			defer senders.Done()
			<-startGate
			// 每个生产者独占下标区间，完成后才由主测试读取接纳结果。
			for index := first; index < first+attempts/producers; index++ {
				err := local.Send(id, scaleSendCommand, hotMessage{index: index, sent: time.Now()})
				if err == nil {
					accepted[index] = true
					continue
				}
				if !errors.Is(err, ErrMailboxFull) {
					failures <- err
					return
				}
			}
		}(producer * (attempts / producers))
	}
	start := time.Now()
	close(startGate)
	senders.Wait()
	admissionElapsed := time.Since(start)
	close(failures)
	for err := range failures {
		t.Fatalf("unexpected Send failure: %v", err)
	}
	admitted := 0
	for _, ok := range accepted {
		if ok {
			admitted++
		}
	}
	if admitted == 0 {
		t.Fatal("no messages admitted")
	}
	latencies := make([]time.Duration, 0, admitted)
	seen := make([]bool, attempts)
	wait, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for len(latencies) < admitted {
		select {
		case sample := <-samples:
			if sample.index < 0 || sample.index >= attempts || !accepted[sample.index] || seen[sample.index] {
				t.Fatalf("unexpected or duplicate message: %d", sample.index)
			}
			seen[sample.index] = true
			latencies = append(latencies, sample.latency)
		case <-wait.Done():
			t.Fatalf("processed=%d admitted=%d: %v", len(latencies), admitted, wait.Err())
		}
	}
	observedElapsed := time.Since(start)
	// 先等已接纳消息到达 Handler，再关闭；Stop 丢弃队列不能被当成正常处理。
	if err := local.Close(); err != nil {
		t.Fatal(err)
	}
	if len(samples) != 0 {
		t.Fatal("extra handler observations after Close")
	}
	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
	percentile := func(percent int) time.Duration {
		return latencies[(len(latencies)*percent+99)/100-1]
	}
	t.Logf("capacity=%d producers=%d attempts=%d admitted=%d full=%d handled=%d admission=%s observed=%s latency_p50=%s latency_p95=%s latency_p99=%s latency_max=%s",
		capacity, producers, attempts, admitted, attempts-admitted, len(latencies), admissionElapsed, observedElapsed,
		percentile(50), percentile(95), percentile(99), latencies[len(latencies)-1])
}

type hotMessage struct {
	index int
	sent  time.Time
}

type hotSample struct {
	index   int
	latency time.Duration
}

type hotLoadService struct {
	samples chan<- hotSample
}

func (service *hotLoadService) OnStart(context.Context, ServiceContext) error {
	return nil
}

func (service *hotLoadService) Handle(_ CommandContext, _ CommandID, args ...any) (any, error) {
	message := args[0].(hotMessage)
	service.samples <- hotSample{index: message.index, latency: time.Since(message.sent)}
	return nil, nil
}

func (service *hotLoadService) OnStop(context.Context) error {
	return nil
}
