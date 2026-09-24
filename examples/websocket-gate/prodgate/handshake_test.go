package prodgate

import (
	"bytes"
	"crypto/ecdh"
	"errors"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// TestAcceptClientHandshakeReturnsServerKeyAndAuthenticator 验证固定二进制握手、X25519 派生密钥和后续上行认证使用同一结果。
func TestAcceptClientHandshakeReturnsServerKeyAndAuthenticator(t *testing.T) {
	vector := readClientAuthVector(t)
	clientPublicKey := decodeVectorHex(t, "client public key", vector.ClientPublic)
	request := marshalHandshakeRequest(t, clientPublicKey)
	serverPrivateKey := newX25519PrivateKey(t, ecdh.X25519(), vector.ServerPrivate)

	responseBytes, authenticator, err := acceptClientHandshakeWithKeyGenerator(request, 4<<20, func() (*ecdh.PrivateKey, error) {
		return serverPrivateKey, nil
	})
	if err != nil {
		t.Fatalf("acceptClientHandshakeWithKeyGenerator() error = %v", err)
	}
	if authenticator == nil {
		t.Fatal("acceptClientHandshakeWithKeyGenerator() returned a nil authenticator")
	}
	if len(responseBytes) != handshakeFrameSize || responseBytes[0] != handshakeVersion {
		t.Fatalf("handshake response = %x, want version %d and %d bytes", responseBytes, handshakeVersion, handshakeFrameSize)
	}
	serverPublicBytes := responseBytes[1:]
	assertVectorBytes(t, "server public key", vector.ServerPublic, serverPublicBytes)

	clientPrivateKey := newX25519PrivateKey(t, ecdh.X25519(), vector.ClientPrivate)
	serverPublicKey, err := ecdh.X25519().NewPublicKey(serverPublicBytes)
	if err != nil {
		t.Fatalf("create server public key: %v", err)
	}
	sharedSecret, err := clientPrivateKey.ECDH(serverPublicKey)
	if err != nil {
		t.Fatalf("client X25519 ECDH() error = %v", err)
	}
	macKey, err := deriveClientMACKey(sharedSecret, clientPublicKey, serverPublicBytes)
	if err != nil {
		t.Fatalf("derive client MAC key: %v", err)
	}
	assertVectorBytes(t, "MAC key", vector.MACKey, macKey[:])

	frame := makeVectorFrame(
		t,
		decodeVectorHex(t, "sequence bytes", vector.SequenceBytes),
		decodeVectorHex(t, "Envelope bytes", vector.EnvelopeBytes),
		decodeVectorHex(t, "MAC", vector.MAC),
	)
	sequence, envelope, err := authenticator.verifyFrame(frame, len(frame))
	if err != nil {
		t.Fatalf("verifyFrame() after handshake error = %v", err)
	}
	if sequence != 0xfedcba9876543210 || !bytes.Equal(envelope, decodeVectorHex(t, "Envelope bytes", vector.EnvelopeBytes)) {
		t.Fatalf("verified sequence or Envelope is invalid: sequence=%d envelope=%x", sequence, envelope)
	}
}

// TestAcceptClientHandshakeRejectsInvalidFrames 验证握手只接受固定长度、版本和有效 X25519 公钥。
func TestAcceptClientHandshakeRejectsInvalidFrames(t *testing.T) {
	vector := readClientAuthVector(t)
	clientPublicKey := decodeVectorHex(t, "client public key", vector.ClientPublic)
	serverPrivateKey := newX25519PrivateKey(t, ecdh.X25519(), vector.ServerPrivate)
	generateServerKey := func() (*ecdh.PrivateKey, error) { return serverPrivateKey, nil }
	validRequest := marshalHandshakeRequest(t, clientPublicKey)
	wrongVersion := append([]byte(nil), validRequest...)
	wrongVersion[0] = handshakeVersion + 1
	shortFrame := validRequest[:len(validRequest)-1]
	longFrame := append(append([]byte(nil), validRequest...), 0)
	lowOrderPublicKey := marshalHandshakeRequest(t, make([]byte, x25519KeySize))

	for _, testCase := range []struct {
		name  string
		frame []byte
		limit int
		want  error
	}{
		{name: "malformed frame", frame: []byte{0xff}, limit: 4 << 20, want: errInvalidClientHandshake},
		{name: "wrong version", frame: wrongVersion, limit: 4 << 20, want: errInvalidClientHandshake},
		{name: "short frame", frame: shortFrame, limit: 4 << 20, want: errInvalidClientHandshake},
		{name: "long frame", frame: longFrame, limit: 4 << 20, want: errInvalidClientHandshake},
		{name: "low-order public key", frame: lowOrderPublicKey, limit: 4 << 20, want: errInvalidClientHandshake},
		{name: "oversized frame", frame: validRequest, limit: len(validRequest) - 1, want: errHandshakeFrameTooLarge},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			_, _, err := acceptClientHandshakeWithKeyGenerator(testCase.frame, testCase.limit, generateServerKey)
			if !errors.Is(err, testCase.want) {
				t.Fatalf("acceptClientHandshakeWithKeyGenerator() error = %v, want %v", err, testCase.want)
			}
		})
	}

	if _, _, err := acceptClientHandshakeWithKeyGenerator(validRequest, 4<<20, func() (*ecdh.PrivateKey, error) {
		return nil, errors.New("test key generation failure")
	}); !errors.Is(err, errHandshakeKeyGeneration) {
		t.Fatalf("key generation error = %v, want %v", err, errHandshakeKeyGeneration)
	}
}

// TestAcceptClientHandshakeGeneratesRandomServerKey 验证生产入口生成的服务端临时公钥可与客户端派生出同一认证结果。
func TestAcceptClientHandshakeGeneratesRandomServerKey(t *testing.T) {
	vector := readClientAuthVector(t)
	clientPrivateKey := newX25519PrivateKey(t, ecdh.X25519(), vector.ClientPrivate)
	clientPublicKey := clientPrivateKey.PublicKey().Bytes()
	request := marshalHandshakeRequest(t, clientPublicKey)

	responseBytes, authenticator, err := acceptClientHandshake(request, 4<<20)
	if err != nil {
		t.Fatalf("acceptClientHandshake() error = %v", err)
	}
	if authenticator == nil {
		t.Fatal("acceptClientHandshake() returned a nil authenticator")
	}
	if len(responseBytes) != handshakeFrameSize || responseBytes[0] != handshakeVersion {
		t.Fatalf("handshake response = %x, want version %d and %d bytes", responseBytes, handshakeVersion, handshakeFrameSize)
	}
	serverPublicKeyBytes := responseBytes[1:]
	serverPublicKey, err := ecdh.X25519().NewPublicKey(serverPublicKeyBytes)
	if err != nil {
		t.Fatalf("create server public key: %v", err)
	}
	sharedSecret, err := clientPrivateKey.ECDH(serverPublicKey)
	if err != nil {
		t.Fatalf("client X25519 ECDH() error = %v", err)
	}
	macKey, err := deriveClientMACKey(sharedSecret, clientPublicKey, serverPublicKeyBytes)
	if err != nil {
		t.Fatalf("derive client MAC key: %v", err)
	}
	sequence := []byte{0, 0, 0, 0, 0, 0, 0, 1}
	envelope := decodeVectorHex(t, "Envelope bytes", vector.EnvelopeBytes)
	unsignedFrame := append(append([]byte(nil), sequence...), envelope...)
	mac := testHMACSHA256(macKey[:], unsignedFrame)
	frame := makeVectorFrame(t, sequence, envelope, mac[:clientMACSize])
	if _, gotEnvelope, err := authenticator.verifyFrame(frame, len(frame)); err != nil || !bytes.Equal(gotEnvelope, envelope) {
		t.Fatalf("verify dynamically authenticated frame: Envelope=%x error=%v", gotEnvelope, err)
	}
}

// TestProdGateHandshakeStopsWhenSocketWriteDeadlineFails 验证握手响应无法设置底层写 deadline 时会返回错误并关闭连接。
func TestProdGateHandshakeStopsWhenSocketWriteDeadlineFails(t *testing.T) {
	serverConnection, clientConnection, serverSocket := newProdWriteFailureWebSocketPair(t)
	deadlineFailure := errors.New("test handshake write deadline failure")
	serverSocket.failWriteDeadline(deadlineFailure)

	vector := readClientAuthVector(t)
	request := marshalHandshakeRequest(t, decodeVectorHex(t, "client public key", vector.ClientPublic))
	if err := clientConnection.WriteMessage(websocket.BinaryMessage, request); err != nil {
		t.Fatalf("WriteMessage(handshake request) error = %v", err)
	}

	clientSession := newClientConnection(serverConnection, 4<<20, 8<<20, &gateCounters{}, 1)
	gate := &GateService{config: Config{HandshakeTimeout: defaultHandshakeTimeout, MaxFrameBytes: 4 << 20}}
	if _, err := gate.performHandshake(clientSession); !errors.Is(err, deadlineFailure) {
		t.Fatalf("performHandshake() error = %v, want to preserve %v", err, deadlineFailure)
	}
	if err := clientSession.Close(); !errors.Is(err, deadlineFailure) {
		t.Fatalf("ClientSession.Close() error = %v, want to preserve %v", err, deadlineFailure)
	}
	if _, _, err := clientConnection.ReadMessage(); err == nil {
		t.Fatal("客户端在握手响应 deadline 设置失败后仍能读取 WebSocket 消息")
	}
}

// TestProdGateHandshakeStopsWhenSocketReadDeadlineFails 验证底层读 deadline 设置失败时握手立即返回并关闭连接。
func TestProdGateHandshakeStopsWhenSocketReadDeadlineFails(t *testing.T) {
	serverConnection, clientConnection, serverSocket := newProdWriteFailureWebSocketPair(t)
	deadlineFailure := errors.New("test handshake read deadline failure")
	serverSocket.failReadDeadline(deadlineFailure)
	clientSession := newClientConnection(serverConnection, 4<<20, 8<<20, &gateCounters{}, 1)
	gate := &GateService{config: Config{HandshakeTimeout: defaultHandshakeTimeout, MaxFrameBytes: 4 << 20}}

	if _, err := gate.performHandshake(clientSession); !errors.Is(err, deadlineFailure) {
		t.Fatalf("performHandshake() error = %v, want to preserve %v", err, deadlineFailure)
	}
	if err := clientConnection.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatalf("SetReadDeadline(client WebSocket) error = %v", err)
	}
	if _, _, err := clientConnection.ReadMessage(); !websocket.IsCloseError(err, websocket.CloseInternalServerErr) {
		t.Fatalf("ReadMessage() error = %v, want WebSocket close %d", err, websocket.CloseInternalServerErr)
	}
}

// TestProdGateHandshakeClosesWhenReadDeadlineCannotBeCleared 验证握手响应后无法清除临时读 deadline 时连接会按错误结束。
func TestProdGateHandshakeClosesWhenReadDeadlineCannotBeCleared(t *testing.T) {
	serverConnection, clientConnection, serverSocket := newProdWriteFailureWebSocketPair(t)
	deadlineFailure := errors.New("test handshake read deadline reset failure")
	serverSocket.failReadDeadlineOnCall(2, deadlineFailure)
	vector := readClientAuthVector(t)
	request := marshalHandshakeRequest(t, decodeVectorHex(t, "client public key", vector.ClientPublic))
	if err := clientConnection.WriteMessage(websocket.BinaryMessage, request); err != nil {
		t.Fatalf("WriteMessage(handshake request) error = %v", err)
	}

	clientSession := newClientConnection(serverConnection, 4<<20, 8<<20, &gateCounters{}, 1)
	gate := &GateService{config: Config{HandshakeTimeout: defaultHandshakeTimeout, MaxFrameBytes: 4 << 20}}
	if _, err := gate.performHandshake(clientSession); !errors.Is(err, deadlineFailure) {
		t.Fatalf("performHandshake() error = %v, want to preserve %v", err, deadlineFailure)
	}
	if err := clientConnection.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatalf("SetReadDeadline(client WebSocket) error = %v", err)
	}
	if messageType, response, err := clientConnection.ReadMessage(); err != nil || messageType != websocket.BinaryMessage || len(response) != handshakeFrameSize {
		t.Fatalf("ReadMessage(handshake response) = type %d, bytes %d, error %v", messageType, len(response), err)
	}
	if _, _, err := clientConnection.ReadMessage(); !websocket.IsCloseError(err, websocket.CloseInternalServerErr) {
		t.Fatalf("ReadMessage() after handshake response error = %v, want WebSocket close %d", err, websocket.CloseInternalServerErr)
	}
}

func marshalHandshakeRequest(t *testing.T, clientPublicKey []byte) []byte {
	t.Helper()
	frame := make([]byte, handshakeFrameSize)
	frame[0] = handshakeVersion
	copy(frame[1:], clientPublicKey)
	return frame
}
