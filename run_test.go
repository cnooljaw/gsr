package gsr

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cnooljaw/gsr/cluster"
	"github.com/cnooljaw/gsr/logging"
	"github.com/cnooljaw/gsr/runtime"
)

// 验证 HTTP 按名称调用且不伪造来源，重复打开不再次构造路由，退出时释放端口。
func TestRunHTTPIngress(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	started := make(chan error, 1)
	finished := make(chan error, 1)
	notified := make(chan runtime.ServiceID, 1)
	entry := &probeService{start: func(ctx context.Context, self ServiceContext) error {
		for _, name := range []cluster.ServiceName{"users", "rooms", "notifications"} {
			name := name
			id, err := self.CreateService(ctx, &probeService{handle: func(current CommandContext) (any, error) {
				if current.Source() != 0 {
					return nil, errors.New("HTTP ingress forged a Service source")
				}
				if name == "notifications" {
					notified <- current.Source()
				}
				return string(name), nil
			}})
			if err != nil {
				return err
			}
			if err := self.Register(name, id); err != nil {
				return err
			}
		}
		err := self.OpenHTTP(address, func(in *HTTPIngress) http.Handler {
			mux := http.NewServeMux()
			for _, name := range []cluster.ServiceName{"users", "rooms"} {
				name := name
				mux.HandleFunc("GET /"+string(name), func(writer http.ResponseWriter, request *http.Request) {
					value, err := in.CallName(request.Context(), name, 1)
					if err != nil {
						http.Error(writer, http.StatusText(http.StatusServiceUnavailable), http.StatusServiceUnavailable)
						return
					}
					fmt.Fprint(writer, value)
				})
			}
			mux.HandleFunc("GET /notify", func(writer http.ResponseWriter, _ *http.Request) {
				if err := in.SendName("notifications", 1); err != nil {
					http.Error(writer, http.StatusText(http.StatusServiceUnavailable), http.StatusServiceUnavailable)
					return
				}
				writer.WriteHeader(http.StatusAccepted)
			})
			return mux
		})
		if err == nil {
			constructedAgain := false
			duplicateErr := self.OpenHTTP(address, func(_ *HTTPIngress) http.Handler {
				constructedAgain = true
				return http.NewServeMux()
			})
			if duplicateErr == nil || constructedAgain {
				err = errors.New("duplicate OpenHTTP must reject before constructing routes")
			}
		}
		started <- err
		return err
	}}
	go func() {
		finished <- Run(ctx, Config{Logging: logging.Config{Writer: io.Discard}}, entry)
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-finished:
			if err != nil {
				t.Errorf("Run() error = %v", err)
			}
			listener, err := net.Listen("tcp", address)
			if err != nil {
				t.Errorf("HTTP listener not released: %v", err)
			} else {
				listener.Close()
			}
		case <-time.After(3 * time.Second):
			t.Error("Run() did not stop")
		}
	})
	select {
	case err := <-started:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("OpenHTTP did not finish")
	}
	client := &http.Client{Timeout: time.Second}
	for _, name := range []string{"users", "rooms"} {
		response, err := client.Get("http://" + address + "/" + name)
		if err != nil {
			t.Fatal(err)
		}
		body, readErr := io.ReadAll(response.Body)
		response.Body.Close()
		if readErr != nil || response.StatusCode != http.StatusOK || string(body) != name {
			t.Fatalf("GET /%s = %d %q, %v", name, response.StatusCode, body, readErr)
		}
	}
	response, err := client.Get("http://" + address + "/notify")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusAccepted {
		t.Fatalf("GET /notify status = %d", response.StatusCode)
	}
	select {
	case source := <-notified:
		if source != 0 {
			t.Fatalf("HTTP Send source = %d, want 0", source)
		}
	case <-time.After(time.Second):
		t.Fatal("HTTP Send was not delivered")
	}
}

// 验证监听建立后的启动错误或 panic、停止错误或 panic 均保留原错误并释放资源。
func TestRunCleanupAfterOpen(t *testing.T) {
	for _, mode := range []string{"startup_error", "startup_panic", "shutdown_failure", "stop_panic"} {
		t.Run(mode, func(t *testing.T) {
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
					if mode == "startup_error" {
						return startFailure
					}
					if mode == "startup_panic" {
						panic("entry startup panicked")
					}
					cancel()
					return nil
				},
				stop: func(_ context.Context) error {
					entryStops.Add(1)
					if mode == "stop_panic" {
						panic("entry stop panicked")
					}
					return nil
				},
			}
			err := Run(ctx, Config{
				Logging: logging.Config{Writer: &output},
				Cluster: cluster.ClusterConfig{Nodes: map[cluster.NodeID]string{"server": "127.0.0.1:0"}},
			}, entry)
			if !errors.Is(err, stopFailure) || errors.Is(err, startFailure) != (mode == "startup_error") || errors.Is(err, runtime.ErrServicePanicked) != (mode == "startup_panic" || mode == "stop_panic") {
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

// 验证 Runtime、Cluster 或入口构造失败仍排空日志并保留原始错误。
func TestRunConstructionRollback(t *testing.T) {
	var invalidEntry *probeService
	for _, test := range []struct {
		name   string
		config Config
		entry  Service
		want   error
	}{
		{name: "runtime", config: Config{Runtime: runtime.Config{MaxCallWait: -1}}, want: runtime.ErrInvalidConfig},
		{name: "cluster", config: Config{Cluster: cluster.ClusterConfig{MaxCallWait: -1}}, want: cluster.ErrInvalidClusterConfig},
		{name: "entry", entry: invalidEntry, want: runtime.ErrInvalidService},
	} {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer
			var starts, stops atomic.Int32
			entry := test.entry
			if test.name != "entry" {
				entry = &probeService{
					start: func(context.Context, ServiceContext) error {
						starts.Add(1)
						return nil
					},
					stop: func(context.Context) error {
						stops.Add(1)
						return nil
					},
				}
			}
			test.config.Logging = logging.Config{Writer: &output}
			err := Run(context.Background(), test.config, entry)
			if !errors.Is(err, test.want) {
				t.Fatalf("construction error = %v, want %v", err, test.want)
			}
			if starts.Load() != 0 || stops.Load() != 0 {
				t.Fatalf("business lifecycle ran before assembly completed: starts=%d stops=%d", starts.Load(), stops.Load())
			}
			if !bytes.Contains(output.Bytes(), []byte("process stopped")) {
				t.Fatal("logger not drained")
			}
		})
	}
}

// 验证应用按节点名 Open 绑定失败后不发布监听地址，并可在同一节点重试。
func TestRunOpenRetryAfterBindFailure(t *testing.T) {
	bindAddress, err := net.ResolveTCPAddr("tcp", "0.0.0.0:0")
	if err != nil {
		t.Fatal(err)
	}
	busy, err := net.ListenTCP("tcp", bindAddress)
	if err != nil {
		t.Fatal(err)
	}
	defer busy.Close()
	busyAddress := busy.Addr().String()
	_, wantPort, err := net.SplitHostPort(busyAddress)
	if err != nil {
		t.Fatal(err)
	}
	nodeAddress := net.JoinHostPort("127.0.0.1", wantPort)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	entry := &probeService{start: func(_ context.Context, self ServiceContext) error {
		if err := self.Cluster().Open("server"); err == nil {
			return fmt.Errorf("occupied port accepted: busy=%s, opened=%s", busyAddress, self.Cluster().ListenAddress())
		}
		if self.Cluster().ListenAddress() != "" {
			return errors.New("failed bind published a listener")
		}
		if err := busy.Close(); err != nil {
			return err
		}
		if err := self.Cluster().Open("server"); err != nil {
			return err
		}
		address := self.Cluster().ListenAddress()
		_, port, err := net.SplitHostPort(address)
		if err != nil || port != wantPort {
			return errors.New("retry opened the wrong port")
		}
		cancel()
		return nil
	}}
	if err := Run(ctx, Config{
		Logging: logging.Config{Writer: io.Discard},
		Cluster: cluster.ClusterConfig{Nodes: map[cluster.NodeID]string{"server": nodeAddress}},
	}, entry); err != nil {
		t.Fatal(err)
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

// 验证两个 Run 的名称及连接独立，目标退出后远端 Send/Call 均能到达同名新实例。
func TestRunNodesCommunicateWithoutSingleton(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	ready, serverDone := make(chan string, 1), make(chan error, 1)
	stopped := make(chan struct{})
	delivered := make(chan runtime.ServiceID, 1)
	server := &probeService{start: func(ctx context.Context, self ServiceContext) error {
		id, err := self.CreateService(ctx, &probeService{handle: func(current CommandContext) (any, error) {
			if current.Source() != 0 {
				return nil, errors.New("remote source must remain zero")
			}
			current.Exit()
			return "remote", nil
		}, stop: func(context.Context) error {
			close(stopped)
			return nil
		}})
		if err != nil {
			return err
		}
		if err := self.Register("echo", id); err != nil {
			return err
		}
		managerID, err := self.CreateService(ctx, &probeService{handle: func(current CommandContext) (any, error) {
			createContext, cancelCreate := context.WithTimeout(context.Background(), time.Second)
			defer cancelCreate()
			replacementID, err := current.CreateService(createContext, &probeService{handle: func(current CommandContext) (any, error) {
				delivered <- current.Self()
				return "replacement", nil
			}})
			if err != nil {
				return nil, err
			}
			return nil, current.Register("echo", replacementID)
		}})
		if err != nil {
			return err
		}
		if err := self.Register("manager", managerID); err != nil {
			return err
		}
		if err := self.Register("echo", id); !errors.Is(err, ErrServiceNameRegistered) {
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
		if err := self.Register("echo", self.Self()); err != nil {
			return err
		}
		value, err := self.Cluster().Call(ctx, "server", "echo", 1)
		if err != nil {
			return err
		}
		if value != "remote" {
			return errors.New("node registry leaked")
		}
		select {
		case <-stopped:
		case <-ctx.Done():
			return ctx.Err()
		}
		if _, err := self.Cluster().Call(ctx, "server", "echo", 1); !errors.Is(err, runtime.ErrServiceNotFound) {
			return errors.New("stopped service name remained remotely resolvable")
		}
		if _, err := self.Cluster().Call(ctx, "server", "manager", 2); err != nil {
			return err
		}
		if err := self.Cluster().Send("server", "echo", 3); err != nil {
			return err
		}
		select {
		case id := <-delivered:
			if id == 0 {
				return errors.New("remote Send reached invalid service")
			}
		case <-ctx.Done():
			return ctx.Err()
		}
		value, err = self.Cluster().Call(ctx, "server", "echo", 1)
		if err != nil || value != "replacement" {
			return errors.New("remote name did not reach replacement service")
		}
		stopCaller()
		return nil
	}}
	if err := Run(callerContext, Config{Logging: logging.Config{Writer: io.Discard}, Cluster: cluster.ClusterConfig{Nodes: map[cluster.NodeID]string{"server": net.JoinHostPort("127.0.0.1", port)}}}, caller); err != nil {
		t.Fatal(err)
	}
}

// 验证本地按名调用复用 Service 来源与 Mailbox 顺序，并明确拒绝无效或未知名称。
func TestServiceContextCallsLocalServiceByName(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	entry := &probeService{start: func(ctx context.Context, self ServiceContext) error {
		var handled int
		id, err := self.CreateService(ctx, &probeService{handle: func(current CommandContext) (any, error) {
			if current.Source() != self.Self() {
				return nil, errors.New("local name call lost source")
			}
			handled++
			return handled, nil
		}})
		if err != nil {
			return err
		}
		if err := self.Register("echo", id); err != nil {
			return err
		}
		if err := self.Register("entry", self.Self()); err != nil {
			return err
		}
		if err := self.Register("", id); !errors.Is(err, runtime.ErrServiceNotFound) {
			return errors.New("empty name accepted")
		}
		if err := self.Register("zero", 0); !errors.Is(err, runtime.ErrServiceNotFound) {
			return errors.New("zero service ID accepted")
		}
		if err := self.Register("unknown", runtime.ServiceID(999999)); !errors.Is(err, runtime.ErrServiceNotFound) {
			return errors.New("unknown service ID accepted")
		}
		if err := self.SendName("missing", 1); !errors.Is(err, runtime.ErrServiceNotFound) {
			return errors.New("unknown send name accepted")
		}
		if _, err := self.CallName(ctx, "missing", 1); !errors.Is(err, runtime.ErrServiceNotFound) {
			return errors.New("unknown call name accepted")
		}
		if _, err := self.CallName(nil, "echo", 1); !errors.Is(err, runtime.ErrInvalidCallContext) {
			return errors.New("nil call context accepted")
		}
		cancelled, cancelCall := context.WithCancel(ctx)
		cancelCall()
		if _, err := self.CallName(cancelled, "missing", 1); !errors.Is(err, context.Canceled) {
			return errors.New("cancelled call did not preserve context error")
		}
		if _, err := self.CallName(ctx, "entry", 1); !errors.Is(err, runtime.ErrCallCycle) {
			return errors.New("named self call accepted")
		}
		if err := self.SendName("echo", 1); err != nil {
			return err
		}
		value, err := self.CallName(ctx, "echo", 1)
		if err != nil {
			return err
		}
		if value != 2 {
			return errors.New("name delivery bypassed mailbox order")
		}
		cancel()
		return nil
	}}
	if err := Run(ctx, Config{Logging: logging.Config{Writer: io.Discard}}, entry); err != nil {
		t.Fatal(err)
	}
}

// 验证业务 Handler 按名 Send/Call 时以自身为 Source，目标按 Mailbox 顺序处理。
func TestCommandContextCallsLocalServiceByName(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	result := make(chan error, 1)
	entry := &probeService{start: func(ctx context.Context, self ServiceContext) error {
		var callerID runtime.ServiceID
		var handled int
		targetID, err := self.CreateService(ctx, &probeService{handle: func(current CommandContext) (any, error) {
			if current.Source() != callerID {
				return nil, errors.New("named handler call lost caller source")
			}
			handled++
			return handled, nil
		}})
		if err != nil {
			return err
		}
		if err := self.Register("target", targetID); err != nil {
			return err
		}
		callerID, err = self.CreateService(ctx, &probeService{handle: func(current CommandContext) (any, error) {
			if err := current.SendName("target", 1); err != nil {
				result <- err
				cancel()
				return nil, nil
			}
			callContext, stopCall := context.WithTimeout(context.Background(), time.Second)
			defer stopCall()
			value, err := current.CallName(callContext, "target", 1)
			if err == nil && value != 2 {
				err = errors.New("named handler calls bypassed mailbox order")
			}
			result <- err
			cancel()
			return nil, nil
		}})
		if err != nil {
			return err
		}
		return self.Send(callerID, 1)
	}}
	if err := Run(ctx, Config{Logging: logging.Config{Writer: io.Discard}}, entry); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-result:
		if err != nil {
			t.Fatal(err)
		}
	default:
		t.Fatal("handler did not complete named calls")
	}
}

// 验证启动失败且 OnStop panic 时仍清理名称，后续实例可复用同名入口。
func TestServiceNameClearedAfterStartupFailure(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	startFailure := errors.New("start failed")
	entry := &probeService{start: func(ctx context.Context, self ServiceContext) error {
		_, err := self.CreateService(ctx, &probeService{
			start: func(_ context.Context, child ServiceContext) error {
				// 故意在正式发布前注册，用于验证异常启动时的防御性清理。
				if err := child.Register("echo", child.Self()); err != nil {
					return err
				}
				return startFailure
			},
			stop: func(context.Context) error {
				panic("stop failed")
			},
		})
		if !errors.Is(err, startFailure) || !errors.Is(err, runtime.ErrServicePanicked) {
			return errors.New("startup failure was not preserved")
		}
		id, err := self.CreateService(ctx, &probeService{handle: func(CommandContext) (any, error) {
			return "replacement", nil
		}})
		if err != nil {
			return err
		}
		if err := self.Register("echo", id); err != nil {
			return err
		}
		value, err := self.CallName(ctx, "echo", 1)
		if err != nil || value != "replacement" {
			return errors.New("replacement did not receive named call")
		}
		cancel()
		return nil
	}}
	if err := Run(ctx, Config{Logging: logging.Config{Writer: io.Discard}}, entry); err != nil {
		t.Fatal(err)
	}
}

// 验证目标 Service 退出后移除它的全部名称，后续实例可以复用这些名称。
func TestServiceNameClearedAfterExit(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	stopped := make(chan struct{})
	entry := &probeService{start: func(ctx context.Context, self ServiceContext) error {
		survivorID, err := self.CreateService(ctx, &probeService{handle: func(CommandContext) (any, error) {
			return "survivor", nil
		}})
		if err != nil {
			return err
		}
		if err := self.Register("survivor", survivorID); err != nil {
			return err
		}
		stoppedID, err := self.CreateService(ctx, &probeService{
			start: func(_ context.Context, child ServiceContext) error {
				if err := child.Register("echo", child.Self()); err != nil {
					return err
				}
				return child.Register("alias", child.Self())
			},
			handle: func(current CommandContext) (any, error) {
				current.Exit()
				return "stopped", nil
			},
			stop: func(context.Context) error {
				close(stopped)
				return nil
			},
		})
		if err != nil {
			return err
		}
		value, err := self.CallName(ctx, "echo", 1)
		if err != nil || value != "stopped" {
			return errors.New("original named call failed")
		}
		select {
		case <-stopped:
		case <-ctx.Done():
			return ctx.Err()
		}
		value, err = self.CallName(ctx, "survivor", 1)
		if err != nil || value != "survivor" {
			return errors.New("stopping one service removed another service name")
		}
		id, err := self.CreateService(ctx, &probeService{handle: func(CommandContext) (any, error) {
			return "replacement", nil
		}})
		if err != nil {
			return err
		}
		if err := self.Register("echo", id); err != nil {
			return err
		}
		if err := self.Register("alias", id); err != nil {
			return err
		}
		if err := self.Register("stale", stoppedID); !errors.Is(err, runtime.ErrServiceNotFound) {
			return errors.New("stopped service ID was registered again")
		}
		value, err = self.CallName(ctx, "echo", 1)
		if err != nil || value != "replacement" {
			return errors.New("replacement did not receive named call")
		}
		cancel()
		return nil
	}}
	if err := Run(ctx, Config{Logging: logging.Config{Writer: io.Discard}}, entry); err != nil {
		t.Fatal(err)
	}
}

// 验证并发注册与目标退出竞争后不会留下名称或重新发布已退出的 ServiceID。
func TestServiceNameRegistrationRacesCleanup(t *testing.T) {
	const registrations = 128
	node := &application{
		names:     make(map[cluster.ServiceName]runtime.ServiceID),
		namesByID: make(map[runtime.ServiceID][]cluster.ServiceName),
		active:    make(map[runtime.ServiceID]struct{}),
	}
	id := runtime.ServiceID(1)
	node.activateID(id)
	if err := node.register("initial", id); err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	errorsSeen := make(chan error, registrations)
	var workers sync.WaitGroup
	for i := range registrations {
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			errorsSeen <- node.register(cluster.ServiceName(strconv.Itoa(i)), id)
		}()
	}
	workers.Add(1)
	go func() {
		defer workers.Done()
		<-start
		node.unregisterID(id)
	}()
	close(start)
	workers.Wait()
	close(errorsSeen)
	for err := range errorsSeen {
		if err != nil && !errors.Is(err, runtime.ErrServiceNotFound) {
			t.Fatalf("concurrent Register error = %v", err)
		}
	}
	for i := range registrations {
		if _, err := node.resolve(cluster.ServiceName(strconv.Itoa(i))); !errors.Is(err, runtime.ErrServiceNotFound) {
			t.Fatalf("name %d remained after cleanup: %v", i, err)
		}
	}
	if _, err := node.resolve("initial"); !errors.Is(err, runtime.ErrServiceNotFound) {
		t.Fatalf("initial name remained after cleanup: %v", err)
	}
	if len(node.namesByID) != 0 {
		t.Fatalf("reverse index retained stopped ServiceID: %d entries", len(node.namesByID))
	}
	if err := node.register("late", id); !errors.Is(err, runtime.ErrServiceNotFound) {
		t.Fatalf("stopped ServiceID registered again: %v", err)
	}
}
