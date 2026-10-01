package prodgate

import (
	"crypto/ecdh"
	"crypto/rand"
	"errors"
)

const (
	handshakeVersion   byte = 1
	handshakeFrameSize      = 1 + x25519KeySize
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
	if len(frame) != handshakeFrameSize || frame[0] != handshakeVersion {
		return nil, nil, errInvalidClientHandshake
	}
	curve := ecdh.X25519()
	clientPublicKey, err := curve.NewPublicKey(frame[1:])
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
	macKey, err := deriveClientMACKey(sharedSecret, frame[1:], serverPublicKey)
	if err != nil {
		return nil, nil, errInvalidClientHandshake
	}
	response := make([]byte, handshakeFrameSize)
	response[0] = handshakeVersion
	copy(response[1:], serverPublicKey)
	if len(response) > maxFrameBytes {
		return nil, nil, errHandshakeFrameTooLarge
	}
	return response, newClientAuthenticator(macKey), nil
}
