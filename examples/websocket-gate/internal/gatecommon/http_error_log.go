package gatecommon

import (
	"bytes"
	"log"

	"github.com/cnooljaw/gsr/logging"
)

// NewHTTPServerErrorLog 把 net/http 的内部错误收口为安全类别，不转录未认证输入。
func NewHTTPServerErrorLog(logger *logging.Logger) *log.Logger {
	return log.New(httpServerErrorWriter{logger: logger}, "", 0)
}

type httpServerErrorWriter struct {
	logger *logging.Logger
}

func (writer httpServerErrorWriter) Write(data []byte) (int, error) {
	category := "http_server"
	switch {
	case bytes.HasPrefix(data, []byte("http: TLS handshake error")):
		category = "tls_handshake"
	case bytes.HasPrefix(data, []byte("http: Accept error")):
		category = "accept"
	case bytes.HasPrefix(data, []byte("http: panic serving")):
		category = "handler_panic"
	}
	if writer.logger != nil {
		_ = writer.logger.Error("Gate HTTP server error", "category", category)
	} else {
		log.Printf("Gate HTTP server error: category=%s", category)
	}
	return len(data), nil
}
