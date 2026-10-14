package gate

import (
	"bytes"
	"context"
	"log"
	"strings"
	"testing"
	"time"

	"github.com/cnooljaw/gsr/logging"
)

// TestHTTPServerErrorLogWithoutSharedLogger 验证无共享 Logger 时仍只输出安全类别，不转录原始错误。
func TestHTTPServerErrorLogWithoutSharedLogger(t *testing.T) {
	var output bytes.Buffer
	previous := log.Writer()
	log.SetOutput(&output)
	t.Cleanup(func() {
		log.SetOutput(previous)
	})

	for _, test := range []struct {
		name     string
		input    string
		category string
	}{
		{name: "TLS", input: "http: TLS handshake error from private-client-data", category: "tls_handshake"},
		{name: "Accept", input: "http: Accept error: private-client-data", category: "accept"},
		{name: "Panic", input: "http: panic serving private-client-data", category: "handler_panic"},
		{name: "Other", input: "http: private-client-data", category: "http_server"},
	} {
		t.Run(test.name, func(t *testing.T) {
			output.Reset()
			newHTTPServerErrorLog(nil).Print(test.input)
			if !strings.Contains(output.String(), "category="+test.category) {
				t.Fatalf("HTTP error log = %q, want category %q", output.String(), test.category)
			}
			if strings.Contains(output.String(), "private-client-data") {
				t.Fatalf("HTTP error log contains untrusted text: %q", output.String())
			}
		})
	}
}

// TestHTTPServerErrorLogDoesNotBlockOnFullQueue 验证共享日志队列满载时，HTTP 错误记录仍立即返回。
func TestHTTPServerErrorLogDoesNotBlockOnFullQueue(t *testing.T) {
	output := &blockedHTTPLogWriter{entered: make(chan struct{}, 1), release: make(chan struct{})}
	logger, err := logging.New(logging.Config{Writer: output, Capacity: 1})
	if err != nil {
		t.Fatalf("logging.New() error = %v", err)
	}
	t.Cleanup(func() {
		close(output.release)
		if err := logger.Close(context.Background()); err != nil {
			t.Errorf("Logger.Close() error = %v", err)
		}
	})
	if err := logger.Info("first"); err != nil {
		t.Fatalf("Logger.Info(first) error = %v", err)
	}
	select {
	case <-output.entered:
	case <-time.After(time.Second):
		t.Fatal("log writer did not receive the first entry")
	}
	if err := logger.Info("second"); err != nil {
		t.Fatalf("Logger.Info(second) error = %v", err)
	}

	type writeResult struct {
		length int
		err    error
	}
	result := make(chan writeResult, 1)
	const serverError = "http: Accept error: private-client-data"
	go func() {
		length, err := newHTTPServerErrorLog(logger.WithService(0)).Writer().Write([]byte(serverError))
		result <- writeResult{length: length, err: err}
	}()
	select {
	case got := <-result:
		if got.length != len(serverError) || got.err != nil {
			t.Fatalf("HTTP error Write() = %d, %v; want %d, nil", got.length, got.err, len(serverError))
		}
	case <-time.After(time.Second):
		t.Fatal("HTTP error logging blocked on a full queue")
	}
	if got := logger.DroppedCount(); got != 1 {
		t.Fatalf("Logger.DroppedCount() = %d, want 1", got)
	}
}

type blockedHTTPLogWriter struct {
	entered chan struct{}
	release chan struct{}
}

func (writer *blockedHTTPLogWriter) Write(data []byte) (int, error) {
	select {
	case writer.entered <- struct{}{}:
	default:
	}
	<-writer.release
	return len(data), nil
}
