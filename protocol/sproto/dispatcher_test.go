package sproto

import (
	"errors"
	"reflect"
	"testing"

	codec "github.com/cnooljaw/gosproto"
)

type echoRequest struct {
	Text string `sproto:"string,0"`
}

type echoResponse struct {
	Text string `sproto:"string,0"`
}

type testHeader struct {
	Type    *int64 `sproto:"integer,0"`
	Session *int64 `sproto:"integer,1"`
}

type captureSender struct {
	frames [][]byte
}

func (sender *captureSender) Send(frame []byte) error {
	sender.frames = append(sender.frames, append([]byte(nil), frame...))
	return nil
}

func echoConfig() Config {
	return Config{
		C2S: []*codec.Protocol{
			{Type: 1, Name: "echo", Request: reflect.TypeOf(&echoRequest{}), Response: reflect.TypeOf(&echoResponse{})},
			{Type: 2, Name: "notify", Request: reflect.TypeOf(&echoRequest{})},
		},
		S2C: []*codec.Protocol{{Type: 1, Name: "push", Request: reflect.TypeOf(&echoRequest{})}},
	}
}

func encodeRequest(t *testing.T, typ int64, session *int64, text string) []byte {
	t.Helper()
	header, err := codec.Encode(&testHeader{Type: &typ, Session: session})
	if err != nil {
		t.Fatal(err)
	}
	body, err := codec.Encode(&echoRequest{Text: text})
	if err != nil {
		t.Fatal(err)
	}
	return codec.Pack(append(header, body...))
}

// 验证原生 session 响应关联、单次响应、Scope 透传及不带 session 的主动推送。
func TestRequestResponseAndPush(t *testing.T) {
	sender := &captureSender{}
	var saved Context
	dispatcher, err := NewDispatcher[int](echoConfig(), sender, map[string]any{
		"echo": func(scope int, request Context, value *echoRequest) error {
			if scope != 42 || request.Session() != 7 {
				t.Fatal("request scope or session changed")
			}
			saved = request
			if err := request.Response(&echoResponse{Text: value.Text}); err != nil {
				return err
			}
			if err := request.Response(&echoResponse{}); !errors.Is(err, ErrDuplicateResponse) {
				t.Fatalf("duplicate response: %v", err)
			}
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	session := int64(7)
	if err := dispatcher.HandleFrame(42, encodeRequest(t, 1, &session, "中文🙂")); err != nil {
		t.Fatal(err)
	}
	if err := saved.Response(&echoResponse{}); !errors.Is(err, ErrInvalidContext) {
		t.Fatalf("escaped context: %v", err)
	}
	if err := dispatcher.Push("push", &echoRequest{Text: "pushed"}); err != nil {
		t.Fatal(err)
	}
	if len(sender.frames) != 2 {
		t.Fatalf("frames=%d", len(sender.frames))
	}
	data, _ := codec.Unpack(sender.frames[0])
	var header testHeader
	used, err := codec.Decode(data, &header)
	if err != nil || header.Type != nil || header.Session == nil || *header.Session != 7 {
		t.Fatalf("response header: %+v %v", header, err)
	}
	var response echoResponse
	if _, err := codec.Decode(data[used:], &response); err != nil || response.Text != "中文🙂" {
		t.Fatalf("response: %+v %v", response, err)
	}
	data, _ = codec.Unpack(sender.frames[1])
	if _, err := codec.Decode(data, &header); err != nil || header.Type == nil || *header.Type != 1 || header.Session != nil {
		t.Fatalf("push header: %+v %v", header, err)
	}
}

// 验证 Notify 不建立响应会话，非法方向、未知协议、缺失 session 与无响应都可观察失败。
func TestNotifyAndRejectedFrames(t *testing.T) {
	sender := &captureSender{}
	calls := 0
	dispatcher, err := NewDispatcher[int](echoConfig(), sender, map[string]any{
		"notify": func(_ int, request Context, _ *echoRequest) error {
			calls++
			if err := request.Response(&echoResponse{}); !errors.Is(err, ErrResponseRequired) {
				t.Fatalf("notify response: %v", err)
			}
			return nil
		},
		"echo": func(int, Context, *echoRequest) error { return nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := dispatcher.HandleFrame(0, encodeRequest(t, 2, nil, "notice")); err != nil {
		t.Fatal(err)
	}
	session := int64(1)
	for _, frame := range [][]byte{
		encodeRequest(t, 1, nil, "missing session"),
		encodeRequest(t, 2, &session, "unexpected session"),
		encodeRequest(t, 99, nil, "unknown"),
		{255},
	} {
		if err := dispatcher.HandleFrame(0, frame); err == nil {
			t.Fatal("invalid input accepted")
		}
	}
	if err := dispatcher.HandleFrame(0, encodeRequest(t, 1, &session, "no reply")); !errors.Is(err, ErrResponseRequired) {
		t.Fatalf("missing response: %v", err)
	}
	if calls != 1 || len(sender.frames) != 0 {
		t.Fatalf("calls=%d frames=%d", calls, len(sender.frames))
	}
}

// 验证构造器建立协议、方向、预算及 Handler 参数类型的不变量。
func TestDispatcherConstruction(t *testing.T) {
	sender := &captureSender{}
	invalid := []map[string]any{
		{"unknown": func(int, Context, *echoRequest) error { return nil }},
		{"echo": func(int, Context, *echoResponse) error { return nil }},
		{"echo": func(int, *echoRequest) error { return nil }},
	}
	for _, handlers := range invalid {
		if _, err := NewDispatcher[int](echoConfig(), sender, handlers); err == nil {
			t.Fatal("invalid binding accepted")
		}
	}
	config := echoConfig()
	config.Limits.MaxDepth = -1
	if _, err := NewDispatcher[int](config, sender, nil); err == nil {
		t.Fatal("invalid limits accepted")
	}
	config = echoConfig()
	config.S2C[0].Response = reflect.TypeOf(&echoResponse{})
	if _, err := NewDispatcher[int](config, sender, nil); err == nil {
		t.Fatal("server request-client RPC accepted")
	}
	var missing *captureSender
	if _, err := NewDispatcher[int](echoConfig(), missing, nil); err == nil {
		t.Fatal("typed nil sender accepted")
	}
}

// 验证原生 Sproto tag 零不被误当成 Protobuf 的保留编号。
func TestNativeZeroProtocolTag(t *testing.T) {
	config := echoConfig()
	config.C2S[0].Type = 0
	dispatcher, err := NewDispatcher[int](config, &captureSender{}, map[string]any{
		"echo": func(_ int, request Context, value *echoRequest) error {
			return request.Response(&echoResponse{Text: value.Text})
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	session := int64(1)
	if err := dispatcher.HandleFrame(0, encodeRequest(t, 0, &session, "zero tag")); err != nil {
		t.Fatal(err)
	}
}
