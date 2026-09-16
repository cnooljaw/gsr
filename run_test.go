package gsr

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cnooljaw/gsr/cluster"
	"github.com/cnooljaw/gsr/logging"
	"github.com/cnooljaw/gsr/runtime"
)

// 验证监听建立后的启动失败或停止失败仍释放端口、停止子 Service 并排空日志，保留各阶段错误。
func TestRunCleanupAfterOpen(t *testing.T) {
	for _, startupFailure := range []bool{true, false} {
		name := "shutdown_failure"
		if startupFailure {
			name = "startup_rollback"
		}
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			startFailure := errors.New("entry startup failed")
			stopFailure := errors.New("child stop failed")
			var output bytes.Buffer
			var entryStops, childStops atomic.Int32
			var address string
			var childLogger *logging.Logger
			child := &probeService{
				start: func(_ context.Context, self ServiceContext) error {
					childLogger = self.Logger()
					return nil
				},
				stop: func(_ context.Context) error {
					childStops.Add(1)
					return errors.Join(stopFailure, childLogger.Info("child stopped"))
				},
			}
			entry := &probeService{
				start: func(ctx context.Context, self ServiceContext) error {
					if _, err := self.CreateService(ctx, child); err != nil {
						return err
					}
					if err := self.Cluster().Open("server"); err != nil {
						return err
					}
					address = self.Cluster().ListenAddress()
					if startupFailure {
						return startFailure
					}
					cancel()
					return nil
				},
				stop: func(_ context.Context) error {
					entryStops.Add(1)
					return nil
				},
			}
			err := Run(ctx, Config{
				Logging: logging.Config{Writer: &output},
				Cluster: cluster.ClusterConfig{Nodes: map[cluster.NodeID]string{"server": "127.0.0.1:0"}},
			}, entry)
			if !errors.Is(err, stopFailure) || errors.Is(err, startFailure) != startupFailure {
				t.Fatalf("lost lifecycle error: %v", err)
			}
			if entryStops.Load() != 1 || childStops.Load() != 1 {
				t.Fatalf("stop counts: entry=%d child=%d", entryStops.Load(), childStops.Load())
			}
			childLog := bytes.Index(output.Bytes(), []byte("child stopped"))
			finalLog := bytes.Index(output.Bytes(), []byte("process stopped"))
			if childLog < 0 || finalLog <= childLog {
				t.Fatalf("shutdown logs missing or out of order: %s", output.String())
			}
			if address == "" {
				t.Fatal("listener never opened")
			}
			// 重绑实际端口验证监听已经释放，不以探测连接失败代替资源回收证据。
			listener, err := net.Listen("tcp", address)
			if err != nil {
				t.Fatalf("listener not released: %v", err)
			}
			if err := listener.Close(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// 验证后续组件构造失败仍排空日志；错误不被回滚覆盖，也不需要调用者持有 Host。
func TestRunConstructionRollback(t *testing.T) {
	var output bytes.Buffer
	err := Run(context.Background(), Config{Logging: logging.Config{Writer: &output}, Cluster: cluster.ClusterConfig{MaxCallWait: -1}}, &probeService{})
	if !errors.Is(err, cluster.ErrInvalidClusterConfig) {
		t.Fatal(err)
	}
	if !bytes.Contains(output.Bytes(), []byte("process stopped")) {
		t.Fatal("logger not drained")
	}
}

// 验证取消和无效入口都有明确结果，不借用外部 Logger 或遗留运行环境。
func TestRunInvalidInputs(t *testing.T) {
	config := Config{Logging: logging.Config{Writer: io.Discard}}
	var entry *probeService
	if err := Run(context.Background(), config, entry); !errors.Is(err, runtime.ErrInvalidService) {
		t.Fatal(err)
	}
	if err := Run(nil, config, &probeService{}); err == nil {
		t.Fatal("accepted nil context")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := Run(ctx, config, &probeService{}); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	config.Runtime.Logger = &logging.Logger{}
	if err := Run(context.Background(), config, &probeService{}); err == nil {
		t.Fatal("accepted borrowed logger")
	}
}

// 验证多个 Service 并发执行时各自绑定 Self，不使用共享的当前 Service 变量。
func TestConcurrentServiceClusterBindings(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	results := make(chan error, 8)
	release := make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	defer unblock()
	entry := &probeService{start: func(ctx context.Context, self ServiceContext) error {
		defer unblock()
		for i := 0; i < 8; i++ {
			id, err := self.CreateService(ctx, &probeService{handle: func(current CommandContext) (any, error) {
				<-release
				var result error
				if current.Cluster().client.Source() != current.Self() || current.Source() == current.Self() {
					result = errors.New("source leaked between services")
				}
				results <- result
				current.Exit()
				return nil, nil
			}})
			if err != nil {
				return err
			}
			if err := self.Send(id, 1); err != nil {
				return err
			}
		}
		unblock()
		for i := 0; i < 8; i++ {
			select {
			case err := <-results:
				if err != nil {
					return err
				}
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		cancel()
		return nil
	}}
	if err := Run(ctx, Config{Logging: logging.Config{Writer: io.Discard}}, entry); err != nil {
		t.Fatal(err)
	}
}

// 验证两个 Run 的名称及连接独立，入口退出后监听继续服务，纯调用节点无需 Open。
func TestRunNodesCommunicateWithoutSingleton(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	ready, serverDone := make(chan string, 1), make(chan error, 1)
	server := &probeService{start: func(ctx context.Context, self ServiceContext) error {
		id, err := self.CreateService(ctx, &probeService{handle: func(current CommandContext) (any, error) {
			if current.Source() != 0 {
				return nil, errors.New("remote source must remain zero")
			}
			return "remote", nil
		}})
		if err != nil {
			return err
		}
		if err := self.Cluster().Register("echo", id); err != nil {
			return err
		}
		if err := self.Cluster().Register("echo", id); !errors.Is(err, ErrServiceNameRegistered) {
			return errors.New("duplicate name accepted")
		}
		if err := self.Cluster().Open("missing"); !errors.Is(err, cluster.ErrNodeNotFound) {
			return errors.New("missing route accepted")
		}
		if err := self.Cluster().Open("server"); err != nil {
			return err
		}
		if err := self.Cluster().Open("server"); !errors.Is(err, cluster.ErrClusterAlreadyOpen) {
			return errors.New("second listener accepted")
		}
		ready <- self.Cluster().ListenAddress()
		self.Exit()
		return nil
	}}
	go func() {
		serverDone <- Run(ctx, Config{Logging: logging.Config{Writer: io.Discard}, Cluster: cluster.ClusterConfig{Nodes: map[cluster.NodeID]string{"server": "127.0.0.1:0"}}}, server)
	}()
	defer func() {
		cancel()
		if err := <-serverDone; err != nil {
			t.Error(err)
		}
	}()
	var address string
	select {
	case address = <-ready:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	_, port, err := net.SplitHostPort(address)
	if err != nil {
		t.Fatal(err)
	}
	callerContext, stopCaller := context.WithCancel(ctx)
	defer stopCaller()
	caller := &probeService{start: func(ctx context.Context, self ServiceContext) error {
		// 同名发布只影响本节点，不能覆盖服务端的 echo。
		if err := self.Cluster().Register("echo", self.Self()); err != nil {
			return err
		}
		value, err := self.Cluster().Call(ctx, "server", "echo", 1)
		if err != nil {
			return err
		}
		if value != "remote" {
			return errors.New("node registry leaked")
		}
		stopCaller()
		return nil
	}}
	if err := Run(callerContext, Config{Logging: logging.Config{Writer: io.Discard}, Cluster: cluster.ClusterConfig{Nodes: map[cluster.NodeID]string{"server": net.JoinHostPort("127.0.0.1", port)}}}, caller); err != nil {
		t.Fatal(err)
	}
}
