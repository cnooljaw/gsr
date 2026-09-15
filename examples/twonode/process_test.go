package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/cnooljaw/gsr/cluster"
)

// 验证两个独立进程在目标停机后继续产生新操作，重启后恢复调用并有序退出。
func TestTwoNodeProcessRecovery(t *testing.T) {
	runProcessRecovery(t, false)
}

// 验证接收端被强杀而未执行退出钩子时，调用端继续运行且重启后的新请求恢复成功。
func TestTwoNodeProcessCrashRecovery(t *testing.T) {
	runProcessRecovery(t, true)
}

func runProcessRecovery(t *testing.T, crash bool) {
	t.Helper()
	if os.Getenv("GSR_PROCESS_TEST") != "1" {
		t.Skip("显式设置 GSR_PROCESS_TEST=1 运行跨进程故障门禁")
	}
	directory := t.TempDir()
	serverConfig := config{Role: "server", LogDirectory: "log/server", Listen: "127.0.0.1:0"}
	serverLog := filepath.Join(directory, "log/server/gsr.log")
	callerLog := filepath.Join(directory, "log/caller/gsr.log")
	server := startExampleProcess(t, directory, "server.json", serverConfig)
	ready := awaitProcessLog(t, server, serverLog, func(entry processLog) bool {
		return entry.Message == "process ready" && entry.Fields.ListenAddress != ""
	})
	// 使用 Open 实际绑定的端口，不先探测空闲端口再释放，避免初次启动端口竞争。
	address := ready.Fields.ListenAddress
	caller := startExampleProcess(t, directory, "caller.json", config{
		Role: "caller", LogDirectory: "log/caller", Target: "battle",
		Nodes: map[cluster.NodeID]string{"battle": address},
	})
	first := awaitProcessLog(t, caller, callerLog, func(entry processLog) bool {
		return entry.Message == "Call result" && entry.Fields.Error == nil
	})
	// 强杀不保证排队日志落盘；先确认首轮记录已输出，再注入连接存续期间的进程故障。
	awaitProcessLog(t, server, serverLog, func(entry processLog) bool {
		return entry.Message == "received" && entry.Fields.Command == uint32(echoCommand) &&
			len(entry.Fields.Args) == 1 && entry.Fields.Args[0] == first.Fields.Operation
	})
	if crash {
		killExampleProcess(t, server)
		for _, entry := range readProcessLogs(t, serverLog) {
			if entry.Message == "process stopped" {
				t.Fatal("killed process unexpectedly completed graceful shutdown")
			}
		}
	} else {
		stopExampleProcess(t, server)
	}
	failed := awaitProcessLog(t, caller, callerLog, func(entry processLog) bool {
		return entry.Message == "Call result" && entry.Fields.Operation > first.Fields.Operation && entry.Fields.Error != nil
	})
	serverConfig.Listen = address
	restarted := startExampleProcess(t, directory, "server-restarted.json", serverConfig)
	recovered := awaitProcessLog(t, caller, callerLog, func(entry processLog) bool {
		return entry.Message == "Call result" && entry.Fields.Operation > failed.Fields.Operation && entry.Fields.Error == nil
	})
	stopExampleProcess(t, caller)
	stopExampleProcess(t, restarted)
	// 全部进程已退出，检查完整日志，不能将短暂空队列视作最终结果。
	callerEntries := readProcessLogs(t, callerLog)
	serverEntries := readProcessLogs(t, serverLog)
	if len(callerEntries) == 0 || len(serverEntries) == 0 || callerEntries[len(callerEntries)-1].Message != "process stopped" || serverEntries[len(serverEntries)-1].Message != "process stopped" {
		t.Fatal("missing final shutdown record")
	}
	notifications := make(map[uint64]bool)
	received := make(map[[2]uint64]bool)
	stops := 0
	for _, entry := range serverEntries {
		if entry.Message == "process stopped" {
			stops++
		}
		if entry.Message == "received" && len(entry.Fields.Args) == 1 {
			key := [2]uint64{uint64(entry.Fields.Command), entry.Fields.Args[0]}
			if received[key] {
				t.Fatalf("duplicate delivery: command=%d operation=%d", key[0], key[1])
			}
			received[key] = true
		}
		if entry.Message == "received" && entry.Fields.Command == uint32(notifyCommand) && len(entry.Fields.Args) == 1 {
			notifications[entry.Fields.Args[0]] = true
		}
	}
	if !notifications[first.Fields.Operation] || !notifications[recovered.Fields.Operation] {
		t.Fatal("Send did not reach server before and after restart")
	}
	wantStops := 2
	if crash {
		wantStops = 1 // 只有重启后的进程正常退出，不能替已崩溃进程伪造收尾记录。
	}
	if stops != wantStops {
		t.Fatalf("server shutdown records=%d want=%d", stops, wantStops)
	}
	t.Logf("成功操作=%d，停机失败操作=%d，恢复成功操作=%d", first.Fields.Operation, failed.Fields.Operation, recovered.Fields.Operation)
}

// 验证子进程运行与示例相同的配置、信号和资源收尾入口；普通测试不执行此入口。
func TestExampleProcessHelper(t *testing.T) {
	if os.Getenv("GSR_EXAMPLE_HELPER") != "1" {
		t.Skip("仅由跨进程测试启动")
	}
	if err := runFile(os.Getenv("GSR_EXAMPLE_CONFIG")); err != nil {
		t.Fatal(err)
	}
}

type exampleProcess struct {
	command *exec.Cmd
	done    chan struct{}
	output  bytes.Buffer // 仅在 Wait 完成后读取。
	err     error
}

func startExampleProcess(t *testing.T, directory, name string, cfg config) *exampleProcess {
	t.Helper()
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, name)
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	process := &exampleProcess{done: make(chan struct{})}
	process.command = exec.CommandContext(ctx, os.Args[0], "-test.run=^TestExampleProcessHelper$", "-test.timeout=35s")
	process.command.Dir = directory
	process.command.Env = append(os.Environ(), "GSR_EXAMPLE_HELPER=1", "GSR_EXAMPLE_CONFIG="+path)
	process.command.Stdout = &process.output
	process.command.Stderr = &process.output
	if err := process.command.Start(); err != nil {
		cancel()
		t.Fatal(err)
	}
	go func() {
		defer close(process.done)
		process.err = process.command.Wait()
	}()
	t.Cleanup(func() {
		// 断言失败时也不留下子进程；正常退出已经由 stopExampleProcess 验证。
		cancel()
		<-process.done
	})
	return process
}

func stopExampleProcess(t *testing.T, process *exampleProcess) {
	t.Helper()
	if err := process.command.Process.Signal(os.Interrupt); err != nil {
		t.Fatal(err)
	}
	select {
	case <-process.done:
		if process.err != nil {
			t.Fatalf("process exit: %v\n%s", process.err, process.output.String())
		}
	case <-time.After(8 * time.Second):
		t.Fatal("process did not exit after interrupt")
	}
}

func killExampleProcess(t *testing.T, process *exampleProcess) {
	t.Helper()
	// 只终止本测试 Start 后持有的子进程对象，不按名称搜索或批量杀进程。
	if err := process.command.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-process.done:
		var exit *exec.ExitError
		if !errors.As(process.err, &exit) {
			t.Fatalf("expected killed exit: %v", process.err)
		}
		status, ok := exit.Sys().(syscall.WaitStatus)
		if !ok || !status.Signaled() || status.Signal() != syscall.SIGKILL {
			t.Fatalf("unexpected exit status: %v", exit)
		}
	case <-time.After(8 * time.Second):
		t.Fatal("killed process was not reaped")
	}
}

type processLog struct {
	Message string `json:"message"`
	Fields  struct {
		ListenAddress string   `json:"listen_address"`
		Operation     uint64   `json:"operation"`
		Command       uint32   `json:"command"`
		Args          []uint64 `json:"args"`
		Error         any      `json:"error"`
	} `json:"fields"`
}

func readProcessLogs(t *testing.T, path string) []processLog {
	t.Helper()
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	var entries []processLog
	// 活跃写入者可能尚未写完最后一行，只解析带换行结束的完整记录。
	lines := bytes.Split(data, []byte{'\n'})
	for _, line := range lines[:len(lines)-1] {
		var entry processLog
		if err := json.Unmarshal(line, &entry); err != nil {
			t.Fatalf("decode %s: %v", path, err)
		}
		entries = append(entries, entry)
	}
	return entries
}

func awaitProcessLog(t *testing.T, process *exampleProcess, path string, matches func(processLog) bool) processLog {
	t.Helper()
	deadline := time.NewTimer(12 * time.Second)
	defer deadline.Stop()
	poll := time.NewTicker(20 * time.Millisecond)
	defer poll.Stop()
	for {
		for _, entry := range readProcessLogs(t, path) {
			if matches(entry) {
				return entry
			}
		}
		select {
		case <-process.done:
			t.Fatalf("process exited before expected log: %v\n%s", process.err, process.output.String())
		case <-deadline.C:
			t.Fatalf("missing expected log in %s", path)
		case <-poll.C:
		}
	}
}
