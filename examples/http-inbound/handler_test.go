package httpinbound

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/cnooljaw/gsr/runtime"
)

// TestRoomCountHTTPHandlerQueriesService 验证 HTTP 入口通过本地 Call 读取 Service 状态并返回 JSON。
func TestRoomCountHTTPHandlerQueriesService(t *testing.T) {
	local := startInboundRuntime(t, 3*time.Second)
	roomID, err := local.CreateService(context.Background(), &roomService{players: 3})
	if err != nil {
		t.Fatalf("CreateService(room) error = %v", err)
	}
	server := newRoomCountServer(local, roomID)
	t.Cleanup(server.Close)

	client := server.Client()
	client.Timeout = time.Second
	response, err := client.Get(server.URL + "/rooms/count")
	if err != nil {
		t.Fatalf("GET /rooms/count error = %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("GET /rooms/count status = %d, want 200", response.StatusCode)
	}
	if contentType := response.Header.Get("Content-Type"); contentType != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", contentType)
	}
	var body struct {
		Count int `json:"count"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode HTTP response: %v", err)
	}
	if body.Count != 3 {
		t.Fatalf("HTTP count = %d, want 3", body.Count)
	}
}

// TestRoomCountHTTPHandlerHidesServiceError 验证业务内部错误只变为安全的 HTTP 状态，不泄露错误文本。
func TestRoomCountHTTPHandlerHidesServiceError(t *testing.T) {
	local := startInboundRuntime(t, 3*time.Second)
	roomID, err := local.CreateService(context.Background(), &failingRoomService{})
	if err != nil {
		t.Fatalf("CreateService(failing room) error = %v", err)
	}
	server := newRoomCountServer(local, roomID)
	t.Cleanup(server.Close)

	client := server.Client()
	client.Timeout = time.Second
	response, err := client.Get(server.URL + "/rooms/count")
	if err != nil {
		t.Fatalf("GET /rooms/count error = %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("GET /rooms/count status = %d, want 503", response.StatusCode)
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read HTTP response: %v", err)
	}
	if strings.Contains(string(body), "private database detail") {
		t.Fatalf("HTTP response leaked internal error: %q", body)
	}
}

// TestRoomCountHTTPHandlerBoundsCall 验证本地 Service 未及时回复时 HTTP 入口有界返回 504。
func TestRoomCountHTTPHandlerBoundsCall(t *testing.T) {
	local := startInboundRuntime(t, 50*time.Millisecond)
	release := make(chan struct{})
	roomID, err := local.CreateService(context.Background(), &waitingRoomService{release: release})
	if err != nil {
		t.Fatalf("CreateService(waiting room) error = %v", err)
	}
	server := newRoomCountServer(local, roomID)
	t.Cleanup(server.Close)
	t.Cleanup(func() { close(release) })

	client := server.Client()
	client.Timeout = time.Second
	response, err := client.Get(server.URL + "/rooms/count")
	if err != nil {
		t.Fatalf("GET /rooms/count error = %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusGatewayTimeout {
		t.Fatalf("GET /rooms/count status = %d, want 504", response.StatusCode)
	}
}

func newRoomCountServer(local *runtime.Runtime, roomID runtime.ServiceID) *httptest.Server {
	mux := http.NewServeMux()
	mux.Handle("GET /rooms/count", roomCountHandler(local, roomID))
	return httptest.NewServer(mux)
}

func startInboundRuntime(t *testing.T, maxCallWait time.Duration) *runtime.Runtime {
	t.Helper()
	local, err := runtime.NewRuntime(runtime.Config{MaxCallWait: maxCallWait})
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

type failingRoomService struct{}

func (service *failingRoomService) OnStart(_ context.Context, _ runtime.ServiceContext) error {
	return nil
}

func (service *failingRoomService) Handle(_ runtime.CommandContext, _ runtime.CommandID, _ ...any) (any, error) {
	return nil, errors.New("private database detail")
}

func (service *failingRoomService) OnStop(_ context.Context) error {
	return nil
}

type waitingRoomService struct {
	release <-chan struct{}
}

func (service *waitingRoomService) OnStart(_ context.Context, _ runtime.ServiceContext) error {
	return nil
}

func (service *waitingRoomService) Handle(_ runtime.CommandContext, _ runtime.CommandID, _ ...any) (any, error) {
	<-service.release
	return 3, nil
}

func (service *waitingRoomService) OnStop(_ context.Context) error {
	return nil
}
