package httpserver

import (
	"context"
	"io"
	"net/http"
	"testing"
	"time"
)

// TestServerServesAndCloses 验证 HTTP 监听由组件启动，并在关闭后释放端口。
func TestServerServesAndCloses(t *testing.T) {
	server, err := New(Config{
		Address: "127.0.0.1:0",
		Handler: http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			writer.Write([]byte("ready"))
		}),
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if err := server.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	address := server.Addr()
	if address == nil {
		t.Fatal("Addr() = nil after Start")
	}
	client := &http.Client{Timeout: time.Second}
	response, err := client.Get("http://" + address.String())
	if err != nil {
		t.Fatalf("GET error = %v", err)
	}
	data, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil || string(data) != "ready" {
		t.Fatalf("GET body = %q, %v; want ready", data, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := server.Close(ctx); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if err := server.Close(ctx); err != nil {
		t.Fatalf("second Close() error = %v", err)
	}
	if err := server.Start(); err == nil {
		t.Fatal("Start() succeeded after Close")
	}
}

// TestServerCloseWaitsForHandler 验证有序关闭在在途 HTTP Handler 完成前不返回。
func TestServerCloseWaitsForHandler(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	server, err := New(Config{
		Address: "127.0.0.1:0",
		Handler: http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			close(entered)
			<-release
			writer.WriteHeader(http.StatusNoContent)
		}),
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if err := server.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		server.Close(ctx)
	})
	client := &http.Client{Timeout: time.Second}
	requestDone := make(chan error, 1)
	go func() {
		response, err := client.Get("http://" + server.Addr().String())
		if err == nil {
			response.Body.Close()
		}
		requestDone <- err
	}()
	select {
	case <-entered:
	case <-time.After(time.Second):
		close(release)
		t.Fatal("HTTP Handler did not start")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	closeDone := make(chan error, 1)
	go func() {
		closeDone <- server.Close(ctx)
	}()
	select {
	case err := <-closeDone:
		close(release)
		t.Fatalf("Close() returned before Handler completed: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	if err := <-closeDone; err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if err := <-requestDone; err != nil {
		t.Fatalf("GET error = %v", err)
	}
}

// TestNewRejectsMissingHTTPBoundary 验证监听地址和 Handler 必须由宿主明确提供。
func TestNewRejectsMissingHTTPBoundary(t *testing.T) {
	if _, err := New(Config{Handler: http.NotFoundHandler()}); err == nil {
		t.Fatal("New() accepted missing address")
	}
	if _, err := New(Config{Address: "127.0.0.1:0"}); err == nil {
		t.Fatal("New() accepted missing Handler")
	}
}
