package main

import (
	"testing"
	"time"

	codec "github.com/cnooljaw/gosproto"
	"github.com/cnooljaw/gsr/examples/websocket-gate/gen/sproto/c2s"
	"github.com/cnooljaw/gsr/examples/websocket-gate/gen/sproto/s2c"
	"github.com/gorilla/websocket"
)

func exchangeSproto(t *testing.T, client *echoTestClient, frame []byte) []byte {
	t.Helper()
	if err := client.connection.WriteMessage(websocket.BinaryMessage, frame); err != nil {
		t.Fatal(err)
	}
	if err := client.connection.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	_, response, err := client.connection.ReadMessage()
	if err != nil {
		t.Fatal(err)
	}
	return response
}

// 验证入口退出后 Sproto 请求经真实 Gate/Agent/EchoService 返回原 session 的业务响应与拒绝。
func TestSprotoEchoRequestAndBusinessRejection(t *testing.T) {
	client := startEchoTestClientWithProtocol(t, "sproto")
	peer, err := codec.NewRpc(c2s.Protocols)
	if err != nil {
		t.Fatal(err)
	}
	for index, text := range []string{"中文🙂", ""} {
		session := int32(index + 1)
		frame, err := peer.RequestEncode("echo", session, &c2s.EchoRequest{Text: codec.String(text)})
		if err != nil {
			t.Fatal(err)
		}
		response := exchangeSproto(t, client, frame)
		mode, name, returned, payload, err := peer.Dispatch(response)
		if err != nil || mode != codec.RpcResponseMode || name != "echo" || returned != session {
			t.Fatalf("response mode=%v name=%s session=%d err=%v", mode, name, returned, err)
		}
		value := payload.(*c2s.EchoResponse)
		if text == "" {
			if value.Code == nil || *value.Code != 2 || value.Message == nil || *value.Message != "text is required" {
				t.Fatalf("business rejection: %+v", value)
			}
		} else if value.Text == nil || *value.Text != text || value.Code == nil || *value.Code != 1 {
			t.Fatalf("echo: %+v", value)
		}
	}
}

// 验证 Sproto Notify 经业务 Service 发回普通 Command，再由 Agent 主动 Push；坏包关闭当前连接。
func TestSprotoNotifyPushAndBadFrame(t *testing.T) {
	client := startEchoTestClientWithProtocol(t, "sproto")
	requests, err := codec.NewRpc(c2s.Protocols)
	if err != nil {
		t.Fatal(err)
	}
	pushes, err := codec.NewRpc(s2c.Protocols)
	if err != nil {
		t.Fatal(err)
	}
	frame, err := requests.RequestEncode("notify", 0, &c2s.NotifyRequest{Text: codec.String("notice")})
	if err != nil {
		t.Fatal(err)
	}
	response := exchangeSproto(t, client, frame)
	mode, name, session, payload, err := pushes.Dispatch(response)
	if err != nil || mode != codec.RpcRequestMode || name != "push" || session != 0 {
		t.Fatalf("push mode=%v name=%s session=%d err=%v", mode, name, session, err)
	}
	if value := payload.(*s2c.PushRequest); value.Text == nil || *value.Text != "notice" {
		t.Fatalf("push: %+v", value)
	}
	if err := client.connection.WriteMessage(websocket.BinaryMessage, []byte{255}); err != nil {
		t.Fatal(err)
	}
	if err := client.connection.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := client.connection.ReadMessage(); err == nil {
		t.Fatal("invalid Sproto frame did not close connection")
	}
}
