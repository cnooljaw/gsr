package prodgate

import (
	"bytes"
	"crypto/ecdh"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"strconv"
	"testing"
)

type clientAuthVector struct {
	Reference     string `json:"reference"`
	ClientPrivate string `json:"client_private_key"`
	ServerPrivate string `json:"server_private_key"`
	ClientPublic  string `json:"client_public_key"`
	ServerPublic  string `json:"server_public_key"`
	SharedSecret  string `json:"shared_secret"`
	SaltLabel     string `json:"salt_label"`
	SaltInput     string `json:"salt_input"`
	Salt          string `json:"salt"`
	Info          string `json:"info"`
	HKDFPRK       string `json:"hkdf_prk"`
	MACKey        string `json:"mac_key"`
	Sequence      string `json:"seq"`
	SequenceBytes string `json:"seq_bytes"`
	EnvelopeBytes string `json:"envelope_bytes"`
	HMACSHA256    string `json:"hmac_sha256"`
	MAC           string `json:"mac"`
}

// TestClientAuthVectorMatchesReference 校验生产认证实现与固定向量的逐阶段字节结果。
func TestClientAuthVectorMatchesReference(t *testing.T) {
	vector := readClientAuthVector(t)
	curve := ecdh.X25519()
	clientPrivate := newX25519PrivateKey(t, curve, vector.ClientPrivate)
	serverPrivate := newX25519PrivateKey(t, curve, vector.ServerPrivate)
	clientPublic := clientPrivate.PublicKey().Bytes()
	serverPublic := serverPrivate.PublicKey().Bytes()
	assertVectorBytes(t, "client public key", vector.ClientPublic, clientPublic)
	assertVectorBytes(t, "server public key", vector.ServerPublic, serverPublic)

	sharedSecret, err := clientPrivate.ECDH(serverPrivate.PublicKey())
	if err != nil {
		t.Fatalf("X25519 ECDH() error = %v", err)
	}
	assertVectorBytes(t, "shared secret", vector.SharedSecret, sharedSecret)
	serverSharedSecret, err := serverPrivate.ECDH(clientPrivate.PublicKey())
	if err != nil {
		t.Fatalf("reverse X25519 ECDH() error = %v", err)
	}
	if !bytes.Equal(sharedSecret, serverSharedSecret) {
		t.Fatalf("X25519 shared secrets differ: client=%x server=%x", sharedSecret, serverSharedSecret)
	}

	saltInput := make([]byte, 0, len(vector.SaltLabel)+len(clientPublic)+len(serverPublic))
	saltInput = append(saltInput, vector.SaltLabel...)
	saltInput = append(saltInput, clientPublic...)
	saltInput = append(saltInput, serverPublic...)
	assertVectorBytes(t, "salt input", vector.SaltInput, saltInput)
	salt := sha256.Sum256(saltInput)
	assertVectorBytes(t, "salt", vector.Salt, salt[:])
	prk := testHMACSHA256(salt[:], sharedSecret)
	assertVectorBytes(t, "HKDF PRK", vector.HKDFPRK, prk)

	macKey, err := deriveClientMACKey(sharedSecret, clientPublic, serverPublic)
	if err != nil {
		t.Fatalf("deriveClientMACKey() error = %v", err)
	}
	assertVectorBytes(t, "MAC key", vector.MACKey, macKey[:])

	sequenceValue, err := strconv.ParseUint(vector.Sequence, 16, 64)
	if err != nil {
		t.Fatalf("parse sequence %q: %v", vector.Sequence, err)
	}
	var sequenceBytes [clientSequenceSize]byte
	binary.BigEndian.PutUint64(sequenceBytes[:], sequenceValue)
	assertVectorBytes(t, "sequence bytes", vector.SequenceBytes, sequenceBytes[:])
	envelope := decodeVectorHex(t, "Envelope bytes", vector.EnvelopeBytes)
	fullMAC := testHMACSHA256(macKey[:], append(append([]byte(nil), sequenceBytes[:]...), envelope...))
	assertVectorBytes(t, "full HMAC-SHA256", vector.HMACSHA256, fullMAC)
	assertVectorBytes(t, "truncated MAC", vector.MAC, fullMAC[:clientMACSize])

	frame := makeVectorFrame(t, sequenceBytes[:], envelope, fullMAC[:clientMACSize])
	auth := newClientAuthenticator(macKey)
	sequence, gotEnvelope, err := auth.verifyFrame(frame, len(frame))
	if err != nil {
		t.Fatalf("verifyFrame() error = %v", err)
	}
	if sequence != sequenceValue {
		t.Fatalf("verified sequence = %d, want %d", sequence, sequenceValue)
	}
	if !bytes.Equal(gotEnvelope, envelope) {
		t.Fatalf("verified Envelope = %x, want original bytes %x", gotEnvelope, envelope)
	}
}

// TestClientAuthenticatorRejectsTampering 验证认证失败不推进序号，且拒绝重放、超限帧及错误 MAC 长度。
func TestClientAuthenticatorRejectsTampering(t *testing.T) {
	vector := readClientAuthVector(t)
	macKey := decodeVectorHex(t, "MAC key", vector.MACKey)
	sequence := decodeVectorHex(t, "sequence bytes", vector.SequenceBytes)
	envelope := decodeVectorHex(t, "Envelope bytes", vector.EnvelopeBytes)
	mac := decodeVectorHex(t, "MAC", vector.MAC)
	frame := makeVectorFrame(t, sequence, envelope, mac)
	key := fixedMACKey(t, macKey)

	changedMAC := append([]byte(nil), frame...)
	changedMAC[len(changedMAC)-1] ^= 0x01
	changedSequence := append([]byte(nil), frame...)
	changedSequence[0] ^= 0x01
	changedEnvelope := append([]byte(nil), frame...)
	changedEnvelope[clientSequenceSize] ^= 0x01
	fullMAC := decodeVectorHex(t, "full HMAC-SHA256", vector.HMACSHA256)
	fullMACFrame := makeVectorFrame(t, sequence, envelope, fullMAC)
	sharedSecret := decodeVectorHex(t, "shared secret", vector.SharedSecret)
	clientPublic := decodeVectorHex(t, "client public key", vector.ClientPublic)
	serverPublic := decodeVectorHex(t, "server public key", vector.ServerPublic)
	wrongInfoKey := fixedMACKey(t, deriveTestMACKey(
		sharedSecret,
		clientPublic,
		serverPublic,
		clientHKDFInfo+"-wrong",
	))
	wrongPublicOrderKey := fixedMACKey(t, deriveTestMACKey(
		sharedSecret,
		serverPublic,
		clientPublic,
		clientHKDFInfo,
	))
	zeroSequence := make([]byte, clientSequenceSize)
	zeroSequenceMAC := testHMACSHA256(key[:], append(append([]byte(nil), zeroSequence...), envelope...))
	zeroSequenceFrame := makeVectorFrame(t, zeroSequence, envelope, zeroSequenceMAC[:clientMACSize])

	for _, testCase := range []struct {
		name  string
		frame []byte
		key   [clientMACKeySize]byte
		limit int
		want  error
	}{
		{name: "modified MAC", frame: changedMAC, key: key, limit: len(frame), want: errClientMACInvalid},
		{name: "modified sequence", frame: changedSequence, key: key, limit: len(frame), want: errClientMACInvalid},
		{name: "modified Envelope", frame: changedEnvelope, key: key, limit: len(frame), want: errClientMACInvalid},
		{name: "15-byte MAC", frame: frame[:len(frame)-1], key: key, limit: len(frame), want: errClientMACInvalid},
		{name: "untruncated HMAC", frame: fullMACFrame, key: key, limit: len(fullMACFrame), want: errClientMACInvalid},
		{name: "wrong HKDF info", frame: frame, key: wrongInfoKey, limit: len(frame), want: errClientMACInvalid},
		{name: "swapped public key order", frame: frame, key: wrongPublicOrderKey, limit: len(frame), want: errClientMACInvalid},
		{name: "zero sequence", frame: zeroSequenceFrame, key: key, limit: len(zeroSequenceFrame), want: errClientSequenceInvalid},
		{name: "short frame", frame: make([]byte, authenticatedFrameOverhead-1), key: key, limit: len(frame), want: errMalformedAuthFrame},
		{name: "too large", frame: frame, key: key, limit: len(frame) - 1, want: errAuthFrameTooLarge},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			auth := newClientAuthenticator(testCase.key)
			if _, _, err := auth.verifyFrame(testCase.frame, testCase.limit); !errors.Is(err, testCase.want) {
				t.Fatalf("verifyFrame() error = %v, want %v", err, testCase.want)
			}
		})
	}

	// 一份坏 MAC 不应消耗正确帧的序号。
	auth := newClientAuthenticator(key)
	if _, _, err := auth.verifyFrame(changedSequence, len(changedSequence)); !errors.Is(err, errClientMACInvalid) {
		t.Fatalf("verify tampered sequence error = %v, want authentication failure", err)
	}
	if _, _, err := auth.verifyFrame(frame, len(frame)); err != nil {
		t.Fatalf("valid frame after rejected tampering error = %v", err)
	}
	if _, _, err := auth.verifyFrame(frame, len(frame)); !errors.Is(err, errClientSequenceInvalid) {
		t.Fatalf("replayed frame error = %v, want non-increasing sequence", err)
	}

	sequenceValue, err := strconv.ParseUint(vector.Sequence, 16, 64)
	if err != nil {
		t.Fatalf("parse sequence %q: %v", vector.Sequence, err)
	}
	var lowerSequence [clientSequenceSize]byte
	binary.BigEndian.PutUint64(lowerSequence[:], sequenceValue-1)
	lowerMAC := testHMACSHA256(key[:], append(append([]byte(nil), lowerSequence[:]...), envelope...))
	lowerFrame := makeVectorFrame(t, lowerSequence[:], envelope, lowerMAC[:clientMACSize])
	if _, _, err := auth.verifyFrame(lowerFrame, len(lowerFrame)); !errors.Is(err, errClientSequenceInvalid) {
		t.Fatalf("lower sequence error = %v, want non-increasing sequence", err)
	}
}

// TestClientMACKeyRejectsInvalidMaterial 验证派生密钥只接受完整且非全零的 X25519 shared secret 与公钥。
func TestClientMACKeyRejectsInvalidMaterial(t *testing.T) {
	vector := readClientAuthVector(t)
	sharedSecret := decodeVectorHex(t, "shared secret", vector.SharedSecret)
	clientPublic := decodeVectorHex(t, "client public key", vector.ClientPublic)
	serverPublic := decodeVectorHex(t, "server public key", vector.ServerPublic)

	if _, err := deriveClientMACKey(sharedSecret[:len(sharedSecret)-1], clientPublic, serverPublic); !errors.Is(err, errInvalidKeyMaterial) {
		t.Fatalf("short shared secret error = %v, want invalid key material", err)
	}
	if _, err := deriveClientMACKey(make([]byte, x25519KeySize), clientPublic, serverPublic); !errors.Is(err, errInvalidKeyMaterial) {
		t.Fatalf("all-zero shared secret error = %v, want invalid key material", err)
	}
	if _, err := deriveClientMACKey(sharedSecret, clientPublic[:len(clientPublic)-1], serverPublic); !errors.Is(err, errInvalidKeyMaterial) {
		t.Fatalf("short client public key error = %v, want invalid key material", err)
	}
	if _, err := deriveClientMACKey(sharedSecret, clientPublic, serverPublic[:len(serverPublic)-1]); !errors.Is(err, errInvalidKeyMaterial) {
		t.Fatalf("short server public key error = %v, want invalid key material", err)
	}
}

func readClientAuthVector(t *testing.T) clientAuthVector {
	t.Helper()
	data, err := os.ReadFile("../testdata/client-auth-vector.json")
	if err != nil {
		t.Fatalf("read client auth vector: %v", err)
	}
	var vector clientAuthVector
	if err := json.Unmarshal(data, &vector); err != nil {
		t.Fatalf("decode client auth vector: %v", err)
	}
	return vector
}

func newX25519PrivateKey(t *testing.T, curve ecdh.Curve, encoded string) *ecdh.PrivateKey {
	t.Helper()
	key, err := curve.NewPrivateKey(decodeVectorHex(t, "X25519 private key", encoded))
	if err != nil {
		t.Fatalf("X25519 NewPrivateKey() error = %v", err)
	}
	return key
}

func assertVectorBytes(t *testing.T, name, expectedHex string, got []byte) {
	t.Helper()
	expected := decodeVectorHex(t, name, expectedHex)
	if !bytes.Equal(got, expected) {
		t.Fatalf("%s = %x, want %x", name, got, expected)
	}
}

func decodeVectorHex(t *testing.T, name, encoded string) []byte {
	t.Helper()
	decoded, err := hex.DecodeString(encoded)
	if err != nil {
		t.Fatalf("decode %s hex: %v", name, err)
	}
	return decoded
}

func testHMACSHA256(key, message []byte) []byte {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write(message)
	return mac.Sum(nil)
}

func deriveTestMACKey(sharedSecret, clientPublicKey, serverPublicKey []byte, info string) []byte {
	saltInput := append(append(append([]byte(nil), clientHKDFSaltLabel...), clientPublicKey...), serverPublicKey...)
	salt := sha256.Sum256(saltInput)
	prk := testHMACSHA256(salt[:], sharedSecret)
	return testHMACSHA256(prk, append([]byte(info), 1))
}

func makeVectorFrame(t *testing.T, sequence, envelope, mac []byte) []byte {
	t.Helper()
	frame := make([]byte, 0, len(sequence)+len(envelope)+len(mac))
	frame = append(frame, sequence...)
	frame = append(frame, envelope...)
	frame = append(frame, mac...)
	return frame
}

func fixedMACKey(t *testing.T, encoded []byte) [clientMACKeySize]byte {
	t.Helper()
	if len(encoded) != clientMACKeySize {
		t.Fatalf("MAC key length = %d, want %d", len(encoded), clientMACKeySize)
	}
	var key [clientMACKeySize]byte
	copy(key[:], encoded)
	return key
}
