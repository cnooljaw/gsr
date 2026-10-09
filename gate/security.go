package gate

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/binary"
	"errors"
)

const (
	x25519KeySize              = 32
	clientMACKeySize           = sha256.Size
	clientMACSize              = 16
	clientSequenceSize         = 8
	authenticatedFrameOverhead = clientSequenceSize + clientMACSize
	clientHKDFSaltLabel        = "gsr/wss/hkdf-salt/v1"
	clientHKDFInfo             = "gsr/wss/c2s-hmac-sha256/v1"
)

var (
	errInvalidKeyMaterial    = errors.New("gate: invalid client key material")
	errInvalidAuthenticator  = errors.New("gate: invalid client authenticator")
	errMalformedAuthFrame    = errors.New("gate: malformed authenticated frame")
	errAuthFrameTooLarge     = errors.New("gate: authenticated frame too large")
	errClientMACInvalid      = errors.New("gate: client frame authentication failed")
	errClientSequenceInvalid = errors.New("gate: client sequence is not increasing")
)

// deriveClientMACKey 按固定公钥顺序与 HKDF 标签派生客户端上行 MAC 密钥。
func deriveClientMACKey(sharedSecret, clientPublicKey, serverPublicKey []byte) ([clientMACKeySize]byte, error) {
	var empty [clientMACKeySize]byte
	if len(sharedSecret) != x25519KeySize || len(clientPublicKey) != x25519KeySize || len(serverPublicKey) != x25519KeySize {
		return empty, errInvalidKeyMaterial
	}
	if subtle.ConstantTimeCompare(sharedSecret, make([]byte, x25519KeySize)) == 1 {
		return empty, errInvalidKeyMaterial
	}

	saltInput := make([]byte, 0, len(clientHKDFSaltLabel)+2*x25519KeySize)
	saltInput = append(saltInput, clientHKDFSaltLabel...)
	saltInput = append(saltInput, clientPublicKey...)
	saltInput = append(saltInput, serverPublicKey...)
	salt := sha256.Sum256(saltInput)
	prk := hmacSHA256(salt[:], sharedSecret)

	// 输出长度正好是 SHA-256 摘要长度，因此 HKDF-Expand 只需计算 T(1)。
	info := append([]byte(clientHKDFInfo), 1)
	return hmacSHA256(prk[:], info), nil
}

// clientAuthenticator 串行验证一条连接的上行帧并维护该连接已接纳的最大序号。
type clientAuthenticator struct {
	macKey       [clientMACKeySize]byte
	lastSequence uint64
}

// newClientAuthenticator 复制派生密钥，创建独占该连接的序号验证状态。
func newClientAuthenticator(macKey [clientMACKeySize]byte) *clientAuthenticator {
	return &clientAuthenticator{macKey: macKey}
}

// verifyFrame 先校验整帧 MAC，再接纳严格递增的 seq，并返回原始 Envelope 字节切片。
func (auth *clientAuthenticator) verifyFrame(frame []byte, maxFrameBytes int) (uint64, []byte, error) {
	if auth == nil || maxFrameBytes <= 0 {
		return 0, nil, errInvalidAuthenticator
	}
	if len(frame) > maxFrameBytes {
		return 0, nil, errAuthFrameTooLarge
	}
	if len(frame) < authenticatedFrameOverhead {
		return 0, nil, errMalformedAuthFrame
	}

	sequence := binary.BigEndian.Uint64(frame[:clientSequenceSize])
	envelopeEnd := len(frame) - clientMACSize
	expected := hmacSHA256(auth.macKey[:], frame[:envelopeEnd])
	if subtle.ConstantTimeCompare(expected[:clientMACSize], frame[envelopeEnd:]) != 1 {
		return 0, nil, errClientMACInvalid
	}
	if sequence == 0 || sequence <= auth.lastSequence {
		return 0, nil, errClientSequenceInvalid
	}

	auth.lastSequence = sequence
	return sequence, frame[clientSequenceSize:envelopeEnd], nil
}

func hmacSHA256(key, message []byte) [sha256.Size]byte {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write(message)
	var sum [sha256.Size]byte
	copy(sum[:], mac.Sum(nil))
	return sum
}
