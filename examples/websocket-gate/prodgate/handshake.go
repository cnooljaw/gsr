package prodgate

import (
	"crypto/ecdh"
	"crypto/rand"
	"errors"
	"fmt"

	commonv1 "github.com/cnooljaw/gsr/examples/websocket-gate/gen/go/gsr/common/v1"
	"google.golang.org/protobuf/proto"
)

var (
	errInvalidClientHandshake = errors.New("prodgate: invalid client handshake")
	errHandshakeFrameTooLarge = errors.New("prodgate: handshake frame too large")
	errHandshakeKeyGeneration = errors.New("prodgate: server key generation failed")
	errInvalidHandshakeConfig = errors.New("prodgate: invalid handshake configuration")
)

// acceptClientHandshake 校验未认证的首个 Envelope，返回服务端握手回复和该连接的认证器。
func acceptClientHandshake(frame []byte, maxFrameBytes int) ([]byte, *clientAuthenticator, error) {
	return acceptClientHandshakeWithKeyGenerator(frame, maxFrameBytes, func() (*ecdh.PrivateKey, error) {
		return ecdh.X25519().GenerateKey(rand.Reader)
	})
}

// acceptClientHandshakeWithKeyGenerator 允许测试固定服务端密钥，同时保持生产路径使用系统 CSPRNG。
func acceptClientHandshakeWithKeyGenerator(
	frame []byte,
	maxFrameBytes int,
	generateServerKey func() (*ecdh.PrivateKey, error),
) ([]byte, *clientAuthenticator, error) {
	if maxFrameBytes <= 0 || generateServerKey == nil {
		return nil, nil, errInvalidHandshakeConfig
	}
	if len(frame) > maxFrameBytes {
		return nil, nil, errHandshakeFrameTooLarge
	}

	var envelope commonv1.Envelope
	if err := proto.Unmarshal(frame, &envelope); err != nil {
		return nil, nil, errInvalidClientHandshake
	}
	if envelope.GetMsgId() != uint32(commonv1.MsgId_C2S_KEY_EXCHANGE_REQUEST) || envelope.GetRequestId() != 0 {
		return nil, nil, errInvalidClientHandshake
	}

	var request commonv1.KeyExchangeRequest
	if err := proto.Unmarshal(envelope.GetPayload(), &request); err != nil || len(request.GetClientPublicKey()) != x25519KeySize {
		return nil, nil, errInvalidClientHandshake
	}
	curve := ecdh.X25519()
	clientPublicKey, err := curve.NewPublicKey(request.GetClientPublicKey())
	if err != nil {
		return nil, nil, errInvalidClientHandshake
	}
	serverPrivateKey, err := generateServerKey()
	if err != nil || serverPrivateKey == nil {
		return nil, nil, errHandshakeKeyGeneration
	}
	sharedSecret, err := serverPrivateKey.ECDH(clientPublicKey)
	if err != nil {
		return nil, nil, errInvalidClientHandshake
	}
	serverPublicKey := serverPrivateKey.PublicKey().Bytes()
	macKey, err := deriveClientMACKey(sharedSecret, request.GetClientPublicKey(), serverPublicKey)
	if err != nil {
		return nil, nil, errInvalidClientHandshake
	}

	responsePayload, err := proto.Marshal(&commonv1.KeyExchangeResponse{ServerPublicKey: serverPublicKey})
	if err != nil {
		return nil, nil, fmt.Errorf("prodgate: marshal key exchange response: %w", err)
	}
	response, err := proto.Marshal(&commonv1.Envelope{
		MsgId:   uint32(commonv1.MsgId_S2C_KEY_EXCHANGE_RESPONSE),
		Payload: responsePayload,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("prodgate: marshal handshake Envelope: %w", err)
	}
	if len(response) > maxFrameBytes {
		return nil, nil, errHandshakeFrameTooLarge
	}
	return response, newClientAuthenticator(macKey), nil
}
