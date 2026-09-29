package httpoutbound

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cnooljaw/gsr/runtime"
)

const pingCommand runtime.CommandID = 100

// TestProfileRefreshDoesNotBlockOtherServices 验证资料刷新等待 HTTP 时，其他 Service 仍能处理消息。
func TestProfileRefreshDoesNotBlockOtherServices(t *testing.T) {
	requestStarted := make(chan struct{})
	releaseResponse := make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseResponse) }) }
	t.Cleanup(release)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		close(requestStarted)
		select {
		case <-releaseResponse:
		case <-request.Context().Done():
			return
		}
		_ = json.NewEncoder(writer).Encode(map[string]string{"display_name": "小明"})
	}))
	t.Cleanup(server.Close)

	client := server.Client()
	client.Timeout = time.Second
	profile, err := newProfileService(client, server.URL)
	if err != nil {
		t.Fatalf("newProfileService() error = %v", err)
	}
	local := startTestRuntime(t)
	profileID, err := local.CreateService(context.Background(), profile)
	if err != nil {
		t.Fatalf("CreateService(profile) error = %v", err)
	}
	pingID, err := local.CreateService(context.Background(), &pingService{})
	if err != nil {
		t.Fatalf("CreateService(ping) error = %v", err)
	}
	refreshDone := make(chan error, 1)
	go func() {
		_, err := local.Call(context.Background(), profileID, refreshProfileCommand)
		refreshDone <- err
	}()
	select {
	case <-requestStarted:
	case <-time.After(time.Second):
		t.Fatal("profile Service did not start HTTP request")
	}
	pingContext, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if result, err := local.Call(pingContext, pingID, pingCommand); err != nil || result != "pong" {
		t.Fatalf("Call(ping) = %v, %v; want pong while HTTP is waiting", result, err)
	}
	select {
	case err := <-refreshDone:
		t.Fatalf("profile refresh completed before HTTP response was released: %v", err)
	default:
	}
	release()
	select {
	case err := <-refreshDone:
		if err != nil {
			t.Fatalf("Call(refresh) error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("profile refresh did not complete after HTTP response")
	}
	if name, err := local.Call(context.Background(), profileID, getProfileCommand); err != nil || name != "小明" {
		t.Fatalf("Call(get profile) = %v, %v; want 小明", name, err)
	}
}

// TestProfileRefreshFailurePreservesState 验证 HTTP 超时与上游故障返回安全错误，且不会覆盖已有资料。
func TestProfileRefreshFailurePreservesState(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch requests.Add(1) {
		case 1:
			_ = json.NewEncoder(writer).Encode(map[string]string{"display_name": "原名字"})
		case 2:
			<-request.Context().Done()
		default:
			writer.WriteHeader(http.StatusServiceUnavailable)
		}
	}))
	t.Cleanup(server.Close)
	profile, err := newProfileService(&http.Client{Timeout: 500 * time.Millisecond}, server.URL)
	if err != nil {
		t.Fatalf("newProfileService() error = %v", err)
	}
	local := startTestRuntime(t)
	profileID, err := local.CreateService(context.Background(), profile)
	if err != nil {
		t.Fatalf("CreateService(profile) error = %v", err)
	}
	if _, err := local.Call(context.Background(), profileID, refreshProfileCommand); err != nil {
		t.Fatalf("first Call(refresh) error = %v", err)
	}
	if _, err := local.Call(context.Background(), profileID, refreshProfileCommand); !errors.Is(err, errProfileTimeout) {
		t.Fatalf("second Call(refresh) error = %v, want profile timeout", err)
	}
	if _, err := local.Call(context.Background(), profileID, refreshProfileCommand); !errors.Is(err, errProfileUnavailable) {
		t.Fatalf("third Call(refresh) error = %v, want profile unavailable", err)
	}
	if name, err := local.Call(context.Background(), profileID, getProfileCommand); err != nil || name != "原名字" {
		t.Fatalf("Call(get profile) = %v, %v; want unchanged name", name, err)
	}
}

// TestProfileServiceRequiresBoundedClient 验证资料 Service 不接受可能无限等待的 HTTP Client。
func TestProfileServiceRequiresBoundedClient(t *testing.T) {
	for _, client := range []*http.Client{nil, {}, {Timeout: -time.Second}} {
		if _, err := newProfileService(client, "http://example.test/profile"); err == nil {
			t.Fatalf("newProfileService(%v) error = nil, want bounded client rejection", client)
		}
	}
}

// TestProfileServiceRejectsInvalidCommands 验证未知命令与已知命令的非法参数返回各自稳定错误。
func TestProfileServiceRejectsInvalidCommands(t *testing.T) {
	service, err := newProfileService(&http.Client{Timeout: time.Second}, "http://example.test/profile")
	if err != nil {
		t.Fatalf("newProfileService() error = %v", err)
	}
	if _, err := service.Handle(nil, runtime.CommandID(999), "bad"); !errors.Is(err, runtime.ErrUnknownCommand) {
		t.Fatalf("Handle(unknown) error = %v, want ErrUnknownCommand", err)
	}
	if _, err := service.Handle(nil, getProfileCommand, "bad"); !errors.Is(err, runtime.ErrInvalidCommandArguments) {
		t.Fatalf("Handle(get with argument) error = %v, want ErrInvalidCommandArguments", err)
	}
}

func startTestRuntime(t *testing.T) *runtime.Runtime {
	t.Helper()
	local, err := runtime.NewRuntime(runtime.Config{MaxCallWait: 3 * time.Second})
	if err != nil {
		t.Fatalf("NewRuntime() error = %v", err)
	}
	if err := local.Start(); err != nil {
		t.Fatalf("Runtime.Start() error = %v", err)
	}
	t.Cleanup(func() {
		if err := local.Close(); err != nil {
			t.Errorf("Runtime.Close() error = %v", err)
		}
	})
	return local
}

type pingService struct{}

func (service *pingService) OnStart(_ context.Context, _ runtime.ServiceContext) error {
	return nil
}

func (service *pingService) Handle(_ runtime.CommandContext, command runtime.CommandID, _ ...any) (any, error) {
	if command != pingCommand {
		return nil, runtime.ErrUnknownCommand
	}
	return "pong", nil
}

func (service *pingService) OnStop(_ context.Context) error {
	return nil
}
