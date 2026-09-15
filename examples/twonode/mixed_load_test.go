//go:build darwin || linux

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/cnooljaw/gsr/cluster"
	gsr "github.com/cnooljaw/gsr/runtime"
)

type mixedConfig struct {
	Address string
	IDs     []gsr.ServiceID
	Start   int64
	Seconds int
}

// 验证一万个 Service 的混合 Send 负载经本地和独立进程 TCP 投递后计数、来源路径及关闭清理一致。
func TestMixedServiceLoad(t *testing.T) {
	if os.Getenv("GSR_MIXED_LOAD") != "1" {
		t.Skip("显式启用三分钟混合负载")
	}
	seconds := 180
	if value := os.Getenv("GSR_MIXED_SECONDS"); value != "" {
		var err error
		seconds, err = strconv.Atoi(value)
		if err != nil || seconds < 5 || seconds > 180 || seconds%5 != 0 {
			t.Fatal("duration must be 5..180 seconds and divisible by 5")
		}
	}
	local, err := gsr.NewRuntime(gsr.Config{})
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
	var delivered atomic.Int64
	services := make([]*mixedService, 10000)
	cfg := mixedConfig{Seconds: seconds}
	for index := range services {
		service := &mixedService{rate: mixedRate(index), offset: index % 5, delivered: &delivered,
			latencies: make([]time.Duration, 0, mixedRate(index)*seconds)}
		services[index] = service
		id, err := local.CreateService(context.Background(), service)
		if err != nil {
			t.Fatal(err)
		}
		cfg.IDs = append(cfg.IDs, id)
	}
	network := newMixedCluster(t, nil)
	cfg.Address, err = network.Open(local, cluster.ListenConfig{Address: "127.0.0.1:0"})
	if err != nil {
		t.Fatal(err)
	}
	// 两端共享未来开始时刻；五秒预热不计入正式延迟统计。
	cfg.Start = time.Now().Add(3 * time.Second).UnixNano()
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(seconds+30)*time.Second)
	defer cancel()
	child := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestMixedSenderHelper$", "-test.v", "-test.timeout=4m")
	child.Env = append(os.Environ(), "GSR_MIXED_CHILD="+string(data), "GODEBUG=")
	var output bytes.Buffer
	child.Stdout, child.Stderr = &output, &output
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		done <- child.Wait()
	}()
	joined := false
	t.Cleanup(func() {
		cancel()
		if !joined {
			<-done
		}
	})
	t.Logf("target_pid=%d sender_pid=%d services=10000 duration=%ds", os.Getpid(), child.Process.Pid, seconds)
	var before runtime.MemStats
	var cpuBefore syscall.Rusage
	var measuredStart time.Time
	attempts := 0
	mixedSchedule(t, cfg, false, func(id gsr.ServiceID, sequence int64, remote bool) {
		attempts++
		if err := local.Send(id, notifyCommand, sequence, time.Now().UnixNano(), remote); err != nil {
			t.Fatalf("local admission: %v", err)
		}
	}, func(second int) {
		if second == 5 {
			runtime.ReadMemStats(&before)
			if err := syscall.Getrusage(syscall.RUSAGE_SELF, &cpuBefore); err != nil {
				t.Fatal(err)
			}
			measuredStart = time.Now()
		}
		if second >= 5 && (second-5)%10 == 0 {
			mixedResources(t, second-5, delivered.Load())
		}
	})
	elapsed := time.Since(measuredStart)
	var after runtime.MemStats
	runtime.ReadMemStats(&after)
	var cpuAfter syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &cpuAfter); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		joined = true
		if err != nil {
			t.Fatalf("sender: %v\n%s", err, output.String())
		}
		t.Logf("sender: %s", output.String())
	case <-ctx.Done():
		t.Fatal("sender timeout")
	}
	want := int64((seconds + 5) * 9800)
	deadline := time.Now().Add(5 * time.Second)
	for delivered.Load() < want && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if delivered.Load() != want {
		t.Fatalf("delivered=%d want=%d", delivered.Load(), want)
	}
	network.Quiesce()
	if err := local.Close(); err != nil {
		t.Fatal(err)
	}
	inspection := local.Inspect()
	if inspection.PendingCallCount != 0 || inspection.TimerCount != 0 || inspection.Metrics.AdmissionRejected != 0 || inspection.Metrics.HandlerErrors != 0 || inspection.State != gsr.RuntimeClosed {
		t.Fatalf("final inspection: %+v", inspection)
	}
	groups := make(map[int][]time.Duration)
	for index, service := range services {
		if service.count != service.rate*(seconds+5) || service.invalid || len(service.latencies) != service.rate*seconds {
			t.Fatalf("service %d count=%d invalid=%t", index, service.count, service.invalid)
		}
		groups[service.rate] = append(groups[service.rate], service.latencies...)
	}
	for _, rate := range []int{1, 5, 10, 100} {
		values := groups[rate]
		sort.Slice(values, func(i, j int) bool { return values[i] < values[j] })
		t.Logf("rate=%d samples=%d p50=%s p95=%s p99=%s max=%s", rate, len(values), values[(len(values)*50+99)/100-1], values[(len(values)*95+99)/100-1], values[(len(values)*99+99)/100-1], values[len(values)-1])
	}
	cpuSeconds := float64(cpuAfter.Utime.Nano()+cpuAfter.Stime.Nano()-cpuBefore.Utime.Nano()-cpuBefore.Stime.Nano()) / 1e9
	t.Logf("measured_elapsed=%s input_per_sec=%.2f local_attempts_with_warmup=%d delivered_with_warmup=%d cpu_cores=%.4f gc_cycles=%d gc_pause=%s allocated_bytes=%d heap_before=%d heap_after=%d MailboxFull=%d", elapsed, float64(seconds*9800)/elapsed.Seconds(), attempts, delivered.Load(), cpuSeconds/elapsed.Seconds(), after.NumGC-before.NumGC, time.Duration(after.PauseTotalNs-before.PauseTotalNs), after.TotalAlloc-before.TotalAlloc, before.HeapAlloc, after.HeapAlloc, inspection.Metrics.AdmissionRejected)
}

// 验证远端负载发生器只经 Cluster Send 输出，不 Open，也不借助业务 Call 确认投递。
func TestMixedSenderHelper(t *testing.T) {
	value := os.Getenv("GSR_MIXED_CHILD")
	if value == "" {
		t.Skip("仅供混合负载父测试启动")
	}
	var cfg mixedConfig
	if err := json.Unmarshal([]byte(value), &cfg); err != nil {
		t.Fatal(err)
	}
	network := newMixedCluster(t, map[cluster.NodeID]string{"target": cfg.Address})
	client, err := network.Client(1)
	if err != nil {
		t.Fatal(err)
	}
	attempts := 0
	mixedSchedule(t, cfg, true, func(id gsr.ServiceID, sequence int64, remote bool) {
		attempts++
		if err := client.Send("target", cluster.ServiceTarget{ServiceID: id}, notifyCommand, sequence, time.Now().UnixNano(), remote); err != nil {
			t.Fatalf("remote admission: %v", err)
		}
	}, func(int) {})
	// 给已接纳的末批异步写入留出时间；是否真正投递由父进程最终计数裁定。
	time.Sleep(100 * time.Millisecond)
	t.Logf("remote_attempts_with_warmup=%d", attempts)
}

func newMixedCluster(t *testing.T, nodes map[cluster.NodeID]string) *cluster.Cluster {
	t.Helper()
	codec, err := cluster.NewValueCodec(cluster.ValueCodecConfig{})
	if err != nil {
		t.Fatal(err)
	}
	network, err := cluster.NewCluster(cluster.ClusterConfig{Nodes: nodes, Codecs: cluster.MustCommandCodecRegistry([]cluster.CommandCodecEntry{{CommandID: notifyCommand, Codec: codec}})})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := network.Close(ctx); err != nil {
			t.Error(err)
		}
		for event := range network.Diagnostics() {
			t.Errorf("cluster diagnostic: %+v", event)
		}
	})
	return network
}

func mixedRate(index int) int {
	switch {
	case index < 7000:
		return 0
	case index < 9000:
		return 1
	case index < 9800:
		return 5
	case index < 9980:
		return 10
	default:
		return 100
	}
}

func mixedSchedule(t *testing.T, cfg mixedConfig, remote bool, send func(gsr.ServiceID, int64, bool), observe func(int)) {
	t.Helper()
	slots := mixedSlots()
	start := time.Unix(0, cfg.Start)
	maxLag := time.Duration(0)
	for tick := 0; tick < (cfg.Seconds+5)*100; tick++ {
		scheduled := start.Add(time.Duration(tick) * 10 * time.Millisecond)
		time.Sleep(time.Until(scheduled))
		if lag := time.Since(scheduled); lag > maxLag {
			maxLag = lag
		}
		if tick%100 == 0 {
			observe(tick / 100)
		}
		for _, index := range slots[tick%100] {
			sequence := mixedSequence(tick, index)
			if ((sequence+int64(index%5))%5 == 0) != remote {
				continue
			}
			send(cfg.IDs[index], sequence, remote)
		}
	}
	time.Sleep(time.Until(start.Add(time.Duration(cfg.Seconds+5) * time.Second)))
	t.Logf("remote=%t generator_max_lag=%s", remote, maxLag)
}

func mixedSlots() [100][]int {
	var slots [100][]int
	for index := 7000; index < 10000; index++ {
		period := 100 / mixedRate(index)
		for slot := index % period; slot < 100; slot += period {
			slots[slot] = append(slots[slot], index)
		}
	}
	return slots
}

func mixedSequence(tick, index int) int64 {
	return int64(tick/100*mixedRate(index) + (tick%100)/(100/mixedRate(index)))
}

type mixedService struct {
	rate      int
	offset    int
	count     int
	last      [2]int64
	seen      [2]bool
	invalid   bool
	latencies []time.Duration
	delivered *atomic.Int64
}

func (*mixedService) OnStart(context.Context, gsr.ServiceContext) error {
	return nil
}

func (service *mixedService) Handle(_ gsr.CommandContext, _ gsr.CommandID, args ...any) (any, error) {
	sequence, sent, remote := args[0].(int64), args[1].(int64), args[2].(bool)
	path := 0
	if remote {
		path = 1
	}
	expected := int64(0)
	if service.seen[path] {
		expected = service.last[path] + 1
	}
	for ((expected+int64(service.offset))%5 == 0) != remote {
		expected++
	}
	if sequence != expected {
		service.invalid = true
	}
	service.seen[path], service.last[path] = true, sequence
	service.count++
	if sequence >= int64(service.rate*5) {
		service.latencies = append(service.latencies, time.Duration(time.Now().UnixNano()-sent))
	}
	service.delivered.Add(1)
	return nil, nil
}
func (*mixedService) OnStop(context.Context) error {
	return nil
}

func mixedResources(t *testing.T, second int, delivered int64) {
	t.Helper()
	var memory runtime.MemStats
	runtime.ReadMemStats(&memory)
	rss, err := exec.Command("ps", "-o", "rss=", "-p", strconv.Itoa(os.Getpid())).Output()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("second=%d delivered=%d goroutines=%d heap=%d gc=%d rss_kib=%s", second, delivered, runtime.NumGoroutine(), memory.HeapAlloc, memory.NumGC, strings.TrimSpace(string(rss)))
}

// 验证五秒计划严格保留空闲组和四档频率，每个活跃 Service 恰有五分之一消息走远端。
func TestMixedLoadDistribution(t *testing.T) {
	slots := mixedSlots()
	counts := make([]int, 10000)
	remoteCounts := make([]int, 10000)
	for tick := 0; tick < 500; tick++ {
		for _, index := range slots[tick%100] {
			sequence := mixedSequence(tick, index)
			if sequence != int64(counts[index]) {
				t.Fatalf("non-contiguous plan: index=%d sequence=%d", index, sequence)
			}
			counts[index]++
			if (sequence+int64(index%5))%5 == 0 {
				remoteCounts[index]++
			}
		}
	}
	total, remote := 0, 0
	for index, count := range counts {
		if count != mixedRate(index)*5 || remoteCounts[index]*5 != count {
			t.Fatalf("distribution: index=%d count=%d remote=%d", index, count, remoteCounts[index])
		}
		total += count
		remote += remoteCounts[index]
	}
	if total != 49000 || remote != 9800 {
		t.Fatalf("total=%d remote=%d", total, remote)
	}
}

// 验证本地与远端可交错到达但各路径不能重复或跳号，预热消息不进入正式延迟样本。
func TestMixedServiceAccounting(t *testing.T) {
	for offset := 0; offset < 5; offset++ {
		var delivered atomic.Int64
		service := &mixedService{rate: 1, offset: offset, delivered: &delivered}
		// 先收完远端，再收本地，证明不强求两条独立路径之间的全局到达顺序。
		for _, remote := range []bool{true, false} {
			for sequence := int64(0); sequence < 10; sequence++ {
				if ((sequence+int64(offset))%5 == 0) != remote {
					continue
				}
				_, _ = service.Handle(nil, notifyCommand, sequence, time.Now().UnixNano(), remote)
			}
		}
		if service.invalid || service.count != 10 || len(service.latencies) != 5 || delivered.Load() != 10 {
			t.Fatalf("offset=%d accounting=%+v", offset, service)
		}
		_, _ = service.Handle(nil, notifyCommand, int64(0), time.Now().UnixNano(), offset == 0)
		if !service.invalid {
			t.Fatal("duplicate not detected")
		}
	}
	var delivered atomic.Int64
	service := &mixedService{rate: 1, delivered: &delivered}
	_, _ = service.Handle(nil, notifyCommand, int64(2), time.Now().UnixNano(), false)
	if !service.invalid {
		t.Fatal("missing first local sequence not detected")
	}
}
