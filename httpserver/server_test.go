package httpserver

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"sync"
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

// 验证重复 Close 等待同一在途清理；等待取消及最终关闭错误不会被后续调用隐藏。
func TestServerConcurrentCloseSharesCompletion(t *testing.T) {
	for _, force := range []bool{false, true} {
		t.Run(map[bool]string{false: "graceful", true: "forced"}[force], func(t *testing.T) {
			entered, release := make(chan struct{}), make(chan struct{})
			var releaseOnce sync.Once
			unblock := func() { releaseOnce.Do(func() { close(release) }) }
			server, err := New(Config{Address: "127.0.0.1:0", Handler: http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
				close(entered)
				<-release
			})})
			if err != nil {
				t.Fatal(err)
			}
			if err := server.Start(); err != nil {
				t.Fatal(err)
			}
			requestDone := make(chan struct{})
			go func() {
				defer close(requestDone)
				response, _ := (&http.Client{Timeout: time.Second}).Get("http://" + server.Addr().String())
				if response != nil {
					response.Body.Close()
				}
			}()
			defer func() {
				unblock()
				_ = server.Close(context.Background())
				<-requestDone
			}()
			select {
			case <-entered:
			case <-time.After(time.Second):
				t.Fatal("Handler did not start")
			}
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			first := make(chan error, 1)
			go func() { first <- server.Close(ctx) }()
			// 监听关闭证明第一次 Close 已开始，不用 sleep 推测 goroutine 调度。
			for {
				connection, err := net.DialTimeout("tcp", server.Addr().String(), 10*time.Millisecond)
				if err != nil {
					break
				}
				connection.Close()
				if ctx.Err() != nil {
					t.Fatal("Close did not stop listener")
				}
			}
			cancelled, stopWaiting := context.WithCancel(context.Background())
			stopWaiting()
			if err := server.Close(cancelled); !errors.Is(err, context.Canceled) {
				t.Fatalf("repeated Close before completion = %v", err)
			}
			if force {
				cancel()
			} else {
				unblock()
				if err := <-first; err != nil {
					t.Fatalf("first Close = %v", err)
				}
				if err := server.Close(context.Background()); err != nil {
					t.Fatalf("completed Close = %v", err)
				}
			}
			if force {
				if err := <-first; !errors.Is(err, context.Canceled) {
					t.Fatalf("first Close = %v", err)
				}
				if err := server.Close(context.Background()); !errors.Is(err, context.Canceled) {
					t.Fatalf("repeated Close lost final error: %v", err)
				}
			}
		})
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
