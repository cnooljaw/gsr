package cluster

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"

	runtime "github.com/cnooljaw/gsr/runtime"
)

const frameHeaderSize = 4

// FrameCodecConfig configures the length-prefixed Cluster frame codec.
type FrameCodecConfig struct {
	// MaxFrame bounds the complete frame, including its four-byte length prefix.
	// Zero uses the default limit.
	MaxFrame int
}

var (
	// ErrInvalidFrameConfig means a FrameCodec configuration is invalid.
	ErrInvalidFrameConfig = errors.New("cluster: invalid frame config")
	// ErrMalformedFrame means a frame is truncated, has trailing bytes, or has invalid fields.
	ErrMalformedFrame = errors.New("cluster: malformed frame")
	// ErrFrameTooLarge means a frame exceeds its configured length limit.
	ErrFrameTooLarge = errors.New("cluster: frame too large")
)

// FrameCodec encodes and decodes one complete length-prefixed Cluster frame.
// It does not negotiate versions or perform stream I/O; those responsibilities
// belong to a future Transport/connection owner.
type FrameCodec struct {
	maxFrame int
}

// NewFrameCodec constructs a bounded length-prefixed frame codec.
func NewFrameCodec(config FrameCodecConfig) (*FrameCodec, error) {
	if config.MaxFrame == 0 {
		config.MaxFrame = defaultMaxPayload
	}
	if config.MaxFrame <= frameHeaderSize || uint64(config.MaxFrame-frameHeaderSize) > math.MaxUint32 {
		return nil, fmt.Errorf("%w: MaxFrame must be greater than %d and fit a uint32 body length", ErrInvalidFrameConfig, frameHeaderSize)
	}
	return &FrameCodec{maxFrame: config.MaxFrame}, nil
}

// Encode validates an envelope and returns one complete length-prefixed frame.
func (codec *FrameCodec) Encode(envelope ClusterEnvelope) ([]byte, error) {
	if codec == nil {
		return nil, fmt.Errorf("%w: codec is nil", ErrInvalidFrameConfig)
	}
	if err := envelope.Validate(); err != nil {
		return nil, err
	}

	writer := frameWriter{
		maxBody: codec.maxFrame - frameHeaderSize,
		data:    make([]byte, 0, 64),
	}
	if err := writer.writeByte(byte(envelope.Kind)); err != nil {
		return nil, err
	}
	if err := writer.writeUint64(uint64(envelope.Session)); err != nil {
		return nil, err
	}
	if err := writer.writeTarget(envelope.Target); err != nil {
		return nil, err
	}
	if err := writer.writeUint32(uint32(envelope.Command)); err != nil {
		return nil, err
	}
	if envelope.Kind == FrameError {
		if err := writer.writeString(string(envelope.Error.Class)); err != nil {
			return nil, err
		}
		if err := writer.writeString(string(envelope.Error.Code)); err != nil {
			return nil, err
		}
		if err := writer.writeString(envelope.Error.Message); err != nil {
			return nil, err
		}
	} else if err := writer.writeBytes(envelope.Payload); err != nil {
		return nil, err
	}

	frame := make([]byte, frameHeaderSize+len(writer.data))
	binary.BigEndian.PutUint32(frame[:frameHeaderSize], uint32(len(writer.data)))
	copy(frame[frameHeaderSize:], writer.data)
	return frame, nil
}

// Decode validates and decodes one complete length-prefixed frame.
func (codec *FrameCodec) Decode(frame []byte) (ClusterEnvelope, error) {
	if codec == nil {
		return ClusterEnvelope{}, fmt.Errorf("%w: codec is nil", ErrInvalidFrameConfig)
	}
	if len(frame) > codec.maxFrame {
		return ClusterEnvelope{}, fmt.Errorf("%w: got %d, limit %d", ErrFrameTooLarge, len(frame), codec.maxFrame)
	}
	if len(frame) < frameHeaderSize {
		return ClusterEnvelope{}, malformedFrame("missing length prefix")
	}
	bodyLength := binary.BigEndian.Uint32(frame[:frameHeaderSize])
	if uint64(bodyLength) > uint64(codec.maxFrame-frameHeaderSize) {
		return ClusterEnvelope{}, fmt.Errorf("%w: body length %d", ErrFrameTooLarge, bodyLength)
	}
	expectedLength := frameHeaderSize + int(bodyLength)
	if expectedLength != len(frame) {
		return ClusterEnvelope{}, malformedFrame("length prefix says %d body bytes, got %d", bodyLength, len(frame)-frameHeaderSize)
	}

	decoder := frameDecoder{
		data: frame[frameHeaderSize:],
	}
	kind, err := decoder.readByte()
	if err != nil {
		return ClusterEnvelope{}, err
	}
	session, err := decoder.readUint64()
	if err != nil {
		return ClusterEnvelope{}, err
	}
	target, err := decoder.readTarget()
	if err != nil {
		return ClusterEnvelope{}, err
	}
	command, err := decoder.readUint32()
	if err != nil {
		return ClusterEnvelope{}, err
	}
	envelope := ClusterEnvelope{
		Kind:    FrameKind(kind),
		Session: ClusterSessionID(session),
		Target:  target,
		Command: runtime.CommandID(command),
	}
	switch envelope.Kind {
	case FrameRequest, FrameResponse:
		payload, err := decoder.readBytes()
		if err != nil {
			return ClusterEnvelope{}, err
		}
		envelope.Payload = append([]byte(nil), payload...)
	case FrameError:
		class, err := decoder.readString()
		if err != nil {
			return ClusterEnvelope{}, err
		}
		code, err := decoder.readString()
		if err != nil {
			return ClusterEnvelope{}, err
		}
		message, err := decoder.readString()
		if err != nil {
			return ClusterEnvelope{}, err
		}
		envelope.Error = runtime.NewStatusError(runtime.ErrorClass(class), runtime.ErrorCode(code), message)
	default:
		return ClusterEnvelope{}, malformedFrame("unknown frame kind %d", kind)
	}
	if decoder.offset != len(decoder.data) {
		return ClusterEnvelope{}, malformedFrame("trailing body bytes")
	}
	if err := envelope.Validate(); err != nil {
		return ClusterEnvelope{}, fmt.Errorf("%w: %v", ErrMalformedFrame, err)
	}
	return envelope, nil
}

type frameWriter struct {
	maxBody int
	data    []byte
}

func (writer *frameWriter) writeTarget(target ServiceTarget) error {
	if target.ServiceID != 0 {
		if err := writer.writeByte(targetIDTag); err != nil {
			return err
		}
		return writer.writeUint64(uint64(target.ServiceID))
	}
	if err := writer.writeByte(targetNameTag); err != nil {
		return err
	}
	return writer.writeString(string(target.ServiceName))
}

func (writer *frameWriter) writeByte(value byte) error {
	return writer.write([]byte{value})
}

func (writer *frameWriter) writeUint32(value uint32) error {
	var bytes [4]byte
	binary.BigEndian.PutUint32(bytes[:], value)
	return writer.write(bytes[:])
}

func (writer *frameWriter) writeUint64(value uint64) error {
	var bytes [8]byte
	binary.BigEndian.PutUint64(bytes[:], value)
	return writer.write(bytes[:])
}

func (writer *frameWriter) writeString(value string) error {
	return writer.writeBytes([]byte(value))
}

func (writer *frameWriter) writeBytes(value []byte) error {
	if uint64(len(value)) > math.MaxUint32 {
		return fmt.Errorf("%w: value length %d", ErrFrameTooLarge, len(value))
	}
	if err := writer.writeUint32(uint32(len(value))); err != nil {
		return err
	}
	return writer.write(value)
}

func (writer *frameWriter) write(value []byte) error {
	if len(value) > writer.maxBody-len(writer.data) {
		return fmt.Errorf("%w: body limit %d", ErrFrameTooLarge, writer.maxBody+frameHeaderSize)
	}
	writer.data = append(writer.data, value...)
	return nil
}

type frameDecoder struct {
	data   []byte
	offset int
}

const (
	targetIDTag   byte = 1
	targetNameTag byte = 2
)

func (decoder *frameDecoder) readTarget() (ServiceTarget, error) {
	tag, err := decoder.readByte()
	if err != nil {
		return ServiceTarget{}, err
	}
	switch tag {
	case targetIDTag:
		serviceID, err := decoder.readUint64()
		if err != nil {
			return ServiceTarget{}, err
		}
		return ServiceTarget{ServiceID: runtime.ServiceID(serviceID)}, nil
	case targetNameTag:
		serviceName, err := decoder.readString()
		if err != nil {
			return ServiceTarget{}, err
		}
		return ServiceTarget{ServiceName: ServiceName(serviceName)}, nil
	default:
		return ServiceTarget{}, malformedFrame("unknown target tag %d", tag)
	}
}

func (decoder *frameDecoder) readByte() (byte, error) {
	if decoder.offset >= len(decoder.data) {
		return 0, malformedFrame("unexpected end of body")
	}
	value := decoder.data[decoder.offset]
	decoder.offset++
	return value, nil
}

func (decoder *frameDecoder) readUint32() (uint32, error) {
	bytes, err := decoder.read(4)
	if err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint32(bytes), nil
}

func (decoder *frameDecoder) readUint64() (uint64, error) {
	bytes, err := decoder.read(8)
	if err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint64(bytes), nil
}

func (decoder *frameDecoder) readString() (string, error) {
	value, err := decoder.readBytes()
	if err != nil {
		return "", err
	}
	return string(value), nil
}

func (decoder *frameDecoder) readBytes() ([]byte, error) {
	length, err := decoder.readUint32()
	if err != nil {
		return nil, err
	}
	if uint64(length) > uint64(len(decoder.data)-decoder.offset) {
		return nil, malformedFrame("value length %d exceeds remaining body", length)
	}
	return decoder.read(int(length))
}

func (decoder *frameDecoder) read(length int) ([]byte, error) {
	if length < 0 || length > len(decoder.data)-decoder.offset {
		return nil, malformedFrame("truncated body")
	}
	value := decoder.data[decoder.offset : decoder.offset+length]
	decoder.offset += length
	return value, nil
}

func malformedFrame(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrMalformedFrame, fmt.Sprintf(format, args...))
}
