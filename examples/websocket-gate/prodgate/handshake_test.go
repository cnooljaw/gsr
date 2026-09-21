package prodgate

import (
	"bytes"
	"crypto/ecdh"
	"errors"
	"testing"

	commonv1 "github.com/cnooljaw/gsr/examples/websocket-gate/gen/go/gsr/common/v1"
	"google.golang.org/protobuf/proto"
)

// TestAcceptClientHandshakeReturnsServerKeyAndAuthenticator 验证握手响应、X25519 派生密钥和后续上行认证使用同一结果。
func TestAcceptClientHandshakeReturnsServerKeyAndAuthenticator(t *testing.T) {
	vector := readClientAuthVector(t)
	clientPublicKey := decodeVectorHex(t, "client public key", vector.ClientPublic)
	request := marshalHandshakeRequest(t, uint32(commonv1.MsgId_C2S_KEY_EXCHANGE_REQUEST), 0, clientPublicKey)
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

	var response commonv1.Envelope
	if err := proto.Unmarshal(responseBytes, &response); err != nil {
		t.Fatalf("decode handshake response: %v", err)
	}
	if response.GetMsgId() != uint32(commonv1.MsgId_S2C_KEY_EXCHANGE_RESPONSE) {
		t.Fatalf("response msg_id = %d, want %d", response.GetMsgId(), commonv1.MsgId_S2C_KEY_EXCHANGE_RESPONSE)
	}
	if response.GetRequestId() != 0 {
		t.Fatalf("response request_id = %d, want 0", response.GetRequestId())
	}

	var keyExchange commonv1.KeyExchangeResponse
	if err := proto.Unmarshal(response.GetPayload(), &keyExchange); err != nil {
		t.Fatalf("decode key exchange response: %v", err)
	}
	assertVectorBytes(t, "server public key", vector.ServerPublic, keyExchange.GetServerPublicKey())

	clientPrivateKey := newX25519PrivateKey(t, ecdh.X25519(), vector.ClientPrivate)
	serverPublicKey, err := ecdh.X25519().NewPublicKey(keyExchange.GetServerPublicKey())
	if err != nil {
		t.Fatalf("create server public key: %v", err)
	}
	sharedSecret, err := clientPrivateKey.ECDH(serverPublicKey)
	if err != nil {
		t.Fatalf("client X25519 ECDH() error = %v", err)
	}
	macKey, err := deriveClientMACKey(sharedSecret, clientPublicKey, keyExchange.GetServerPublicKey())
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

// TestAcceptClientHandshakeRejectsInvalidFrames 验证握手只接受合法的控制 Envelope 和有效 X25519 公钥。
func TestAcceptClientHandshakeRejectsInvalidFrames(t *testing.T) {
	vector := readClientAuthVector(t)
	clientPublicKey := decodeVectorHex(t, "client public key", vector.ClientPublic)
	serverPrivateKey := newX25519PrivateKey(t, ecdh.X25519(), vector.ServerPrivate)
	generateServerKey := func() (*ecdh.PrivateKey, error) { return serverPrivateKey, nil }
	validRequest := marshalHandshakeRequest(t, uint32(commonv1.MsgId_C2S_KEY_EXCHANGE_REQUEST), 0, clientPublicKey)
	wrongCommand := marshalHandshakeRequest(t, uint32(commonv1.MsgId_S2C_KEY_EXCHANGE_RESPONSE), 0, clientPublicKey)
	nonzeroRequestID := marshalHandshakeRequest(t, uint32(commonv1.MsgId_C2S_KEY_EXCHANGE_REQUEST), 7, clientPublicKey)
	shortPublicKey := marshalHandshakeRequest(t, uint32(commonv1.MsgId_C2S_KEY_EXCHANGE_REQUEST), 0, clientPublicKey[:len(clientPublicKey)-1])
	lowOrderPublicKey := marshalHandshakeRequest(t, uint32(commonv1.MsgId_C2S_KEY_EXCHANGE_REQUEST), 0, make([]byte, x25519KeySize))
	malformedPayload, err := proto.Marshal(&commonv1.Envelope{
		MsgId:   uint32(commonv1.MsgId_C2S_KEY_EXCHANGE_REQUEST),
		Payload: []byte{0xff},
	})
	if err != nil {
		t.Fatalf("marshal malformed handshake payload: %v", err)
	}

	for _, testCase := range []struct {
		name  string
		frame []byte
		limit int
		want  error
	}{
		{name: "malformed Envelope", frame: []byte{0xff}, limit: 4 << 20, want: errInvalidClientHandshake},
		{name: "malformed payload", frame: malformedPayload, limit: 4 << 20, want: errInvalidClientHandshake},
		{name: "wrong command", frame: wrongCommand, limit: 4 << 20, want: errInvalidClientHandshake},
		{name: "nonzero request_id", frame: nonzeroRequestID, limit: 4 << 20, want: errInvalidClientHandshake},
		{name: "short public key", frame: shortPublicKey, limit: 4 << 20, want: errInvalidClientHandshake},
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
	request := marshalHandshakeRequest(t, uint32(commonv1.MsgId_C2S_KEY_EXCHANGE_REQUEST), 0, clientPublicKey)

	responseBytes, authenticator, err := acceptClientHandshake(request, 4<<20)
	if err != nil {
		t.Fatalf("acceptClientHandshake() error = %v", err)
	}
	if authenticator == nil {
		t.Fatal("acceptClientHandshake() returned a nil authenticator")
	}
	var response commonv1.Envelope
	if err := proto.Unmarshal(responseBytes, &response); err != nil {
		t.Fatalf("decode handshake response: %v", err)
	}
	var keyExchange commonv1.KeyExchangeResponse
	if err := proto.Unmarshal(response.GetPayload(), &keyExchange); err != nil {
		t.Fatalf("decode key exchange response: %v", err)
	}
	serverPublicKey, err := ecdh.X25519().NewPublicKey(keyExchange.GetServerPublicKey())
	if err != nil {
		t.Fatalf("create server public key: %v", err)
	}
	sharedSecret, err := clientPrivateKey.ECDH(serverPublicKey)
	if err != nil {
		t.Fatalf("client X25519 ECDH() error = %v", err)
	}
	macKey, err := deriveClientMACKey(sharedSecret, clientPublicKey, keyExchange.GetServerPublicKey())
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

func marshalHandshakeRequest(t *testing.T, msgID, requestID uint32, clientPublicKey []byte) []byte {
	t.Helper()
	payload, err := proto.Marshal(&commonv1.KeyExchangeRequest{ClientPublicKey: clientPublicKey})
	if err != nil {
		t.Fatalf("marshal key exchange request: %v", err)
	}
	frame, err := proto.Marshal(&commonv1.Envelope{MsgId: msgID, RequestId: requestID, Payload: payload})
	if err != nil {
		t.Fatalf("marshal handshake Envelope: %v", err)
	}
	return frame
}
