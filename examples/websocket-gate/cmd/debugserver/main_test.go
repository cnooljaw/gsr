package main

import "testing"

// TestParseServerOptionsDefaults 验证示例服务默认启动 DebugGate 并使用本地调试端口。
func TestParseServerOptionsDefaults(t *testing.T) {
	options, err := parseServerOptions(nil)
	if err != nil {
		t.Fatalf("parseServerOptions() error = %v", err)
	}
	if options.production || options.address != "127.0.0.1:7888" {
		t.Fatalf("default options = %#v, want DebugGate on 127.0.0.1:7888", options)
	}
}

// TestParseServerOptionsProductionDefaults 验证生产模式默认避开 DebugGate 端口。
func TestParseServerOptionsProductionDefaults(t *testing.T) {
	options, err := parseServerOptions([]string{"--prod"})
	if err != nil {
		t.Fatalf("parseServerOptions(--prod) error = %v", err)
	}
	if !options.production || options.address != "127.0.0.1:7889" {
		t.Fatalf("production options = %#v, want ProdGate on 127.0.0.1:7889", options)
	}
}

// TestParseServerOptionsAcceptsExplicitAddress 验证两种模式都可以显式覆盖监听地址。
func TestParseServerOptionsAcceptsExplicitAddress(t *testing.T) {
	options, err := parseServerOptions([]string{"--prod", "--addr", "127.0.0.1:9001"})
	if err != nil {
		t.Fatalf("parseServerOptions(explicit address) error = %v", err)
	}
	if !options.production || options.address != "127.0.0.1:9001" {
		t.Fatalf("explicit options = %#v, want production address 127.0.0.1:9001", options)
	}
}

// TestParseServerOptionsKeepsExplicitDebugPort 验证生产模式不会覆盖调用方明确指定的默认端口。
func TestParseServerOptionsKeepsExplicitDebugPort(t *testing.T) {
	options, err := parseServerOptions([]string{"--addr", "127.0.0.1:7888", "--prod"})
	if err != nil {
		t.Fatalf("parseServerOptions(explicit default port) error = %v", err)
	}
	if !options.production || options.address != "127.0.0.1:7888" {
		t.Fatalf("explicit default port options = %#v, want production address 127.0.0.1:7888", options)
	}
}

// TestParseServerOptionsAcceptsDirectTLS 验证生产示例可显式指定直连 WSS 的证书和私钥文件。
func TestParseServerOptionsAcceptsDirectTLS(t *testing.T) {
	options, err := parseServerOptions([]string{"--prod", "--tls-cert", "server.crt", "--tls-key", "server.key"})
	if err != nil {
		t.Fatalf("parseServerOptions(TLS files) error = %v", err)
	}
	if !options.production || options.certificateFile != "server.crt" || options.privateKeyFile != "server.key" {
		t.Fatalf("TLS options = %#v, want ProdGate with both certificate files", options)
	}
}

// TestParseServerOptionsRejectsIncompleteTLS 验证证书与私钥必须成对提供，且不能用于 DebugGate。
func TestParseServerOptionsRejectsIncompleteTLS(t *testing.T) {
	for _, arguments := range [][]string{
		{"--prod", "--tls-cert", "server.crt"},
		{"--prod", "--tls-key", "server.key"},
		{"--tls-cert", "server.crt", "--tls-key", "server.key"},
	} {
		if _, err := parseServerOptions(arguments); err == nil {
			t.Fatalf("parseServerOptions(%q) error = nil, want usage error", arguments)
		}
	}
}

// TestParseServerOptionsRejectsUnknownArguments 验证示例命令不会静默接受拼写错误的启动参数。
func TestParseServerOptionsRejectsUnknownArguments(t *testing.T) {
	if _, err := parseServerOptions([]string{"--secure"}); err == nil {
		t.Fatal("parseServerOptions(--secure) error = nil, want usage error")
	}
}
