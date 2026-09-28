package gatecommon

import (
	"bytes"
	"log"
	"strings"
	"testing"
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
			NewHTTPServerErrorLog(nil).Print(test.input)
			if !strings.Contains(output.String(), "category="+test.category) {
				t.Fatalf("HTTP error log = %q, want category %q", output.String(), test.category)
			}
			if strings.Contains(output.String(), "private-client-data") {
				t.Fatalf("HTTP error log contains untrusted text: %q", output.String())
			}
		})
	}
}
