package app

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	"github.com/cnooljaw/gsr/cluster"
	"github.com/cnooljaw/gsr/logging"
	"github.com/cnooljaw/gsr/runtime"
)

// 验证入口完成组装后可退出，取消应用等待会统一关闭组件且重复关闭安全。
func TestHostRunAndClose(t *testing.T) {
	host, err := New(Config{Logging: logging.Config{Writer: io.Discard}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := host.Close(); err != nil {
			t.Error(err)
		}
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	entry := &testService{start: func(ctx context.Context, scoped runtime.ServiceContext) error {
		if _, err := scoped.CreateService(ctx, &testService{}); err != nil {
			return err
		}
		scoped.Exit()
		cancel()
		return nil
	}}
	if err := host.Run(ctx, entry); err != nil {
		t.Fatal(err)
	}
	if err := host.Logger().Info("after close"); !errors.Is(err, logging.ErrClosed) {
		t.Fatal(err)
	}
	if _, err := host.Cluster().Client(1); !errors.Is(err, cluster.ErrClusterQuiescing) {
		t.Fatal(err)
	}
}

// 验证入口启动失败仍保留原错误并关闭宿主日志，不要求应用手工回滚。
func TestHostStartupFailureClosesResources(t *testing.T) {
	host, err := New(Config{Logging: logging.Config{Writer: io.Discard}})
	if err != nil {
		t.Fatal(err)
	}
	failure := errors.New("entry failed")
	entry := &testService{start: func(context.Context, runtime.ServiceContext) error { return failure }}
	if err := host.Run(context.Background(), entry); !errors.Is(err, failure) {
		t.Fatal(err)
	}
	if err := host.Logger().Info("after failure"); !errors.Is(err, logging.ErrClosed) {
		t.Fatal(err)
	}
}

// 验证宿主拒绝含糊的双重日志所有权配置，不静默替换借用资源。
func TestHostRejectsBorrowedLogger(t *testing.T) {
	_, err := New(Config{Runtime: runtime.Config{Logger: &logging.Logger{}}})
	if err == nil {
		t.Fatal("accepted external logger")
	}
}

type testService struct {
	start func(context.Context, runtime.ServiceContext) error
}

func (service *testService) OnStart(ctx context.Context, scoped runtime.ServiceContext) error {
	if service.start != nil {
		return service.start(ctx, scoped)
	}
	return nil
}

func (service *testService) Handle(_ runtime.CommandContext, _ runtime.CommandID, _ ...any) (any, error) {
	return nil, nil
}

func (service *testService) OnStop(_ context.Context) error {
	return nil
}

// 验证后续组件配置失败时已启动的日志任务被排空，且不关闭应用借用的 Writer。
func TestHostConstructionRollback(t *testing.T) {
	var output bytes.Buffer
	host, err := New(Config{
		Logging: logging.Config{Writer: &output},
		Cluster: cluster.ClusterConfig{MaxCallWait: -1},
	})
	if host != nil || !errors.Is(err, cluster.ErrInvalidClusterConfig) {
		t.Fatalf("host=%v error=%v", host, err)
	}
	if !bytes.Contains(output.Bytes(), []byte("process stopped")) {
		t.Fatal("rollback did not drain logger")
	}
}

// 验证默认编码无需注册 Command，合法参数继续到目标节点解析阶段。
func TestHostDefaultCodecNeedsNoCommandList(t *testing.T) {
	host, err := New(Config{Logging: logging.Config{Writer: io.Discard}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := host.Close(); err != nil {
			t.Error(err)
		}
	})
	client, err := host.Cluster().Client(1)
	if err != nil {
		t.Fatal(err)
	}
	err = client.Send("absent", cluster.ServiceTarget{ServiceID: 1}, 2)
	if !errors.Is(err, cluster.ErrNodeNotFound) {
		t.Fatal(err)
	}
}
