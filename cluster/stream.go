package cluster

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

// completeFrameError records that the entire bounded frame was consumed before
// decoding failed. It proves stream alignment, not that the input is acceptable;
// authentication and protocol policy can still require closing the connection.
type completeFrameError struct {
	err error
}

func (err *completeFrameError) Error() string {
	return "decode complete cluster frame: " + err.err.Error()
}

func (err *completeFrameError) Unwrap() error {
	return err.err
}

// readFrame reads exactly one bounded frame. The connection owner supplies
// deadlines/cancellation and decides whether an error requires closing the stream.
// No goroutine or connection is owned here.
func (codec *FrameCodec) readFrame(reader io.Reader) (ClusterEnvelope, error) {
	if codec == nil || codec.maxFrame <= frameHeaderSize {
		return ClusterEnvelope{}, ErrInvalidFrameConfig
	}
	var header [frameHeaderSize]byte
	if _, err := io.ReadFull(reader, header[:]); err != nil {
		return ClusterEnvelope{}, fmt.Errorf("read cluster frame header: %w", err)
	}
	bodyLength := binary.BigEndian.Uint32(header[:])
	if uint64(bodyLength) > uint64(codec.maxFrame-frameHeaderSize) {
		return ClusterEnvelope{}, fmt.Errorf("%w: body length %d", ErrFrameTooLarge, bodyLength)
	}
	if bodyLength == 0 {
		return ClusterEnvelope{}, malformedFrame("empty body")
	}
	frame := make([]byte, frameHeaderSize+int(bodyLength))
	copy(frame, header[:])
	if _, err := io.ReadFull(reader, frame[frameHeaderSize:]); err != nil {
		// EOF after a complete header is still a truncated frame, not clean shutdown.
		if errors.Is(err, io.EOF) {
			err = io.ErrUnexpectedEOF
		}
		return ClusterEnvelope{}, fmt.Errorf("read cluster frame body: %w", err)
	}
	envelope, err := codec.Decode(frame)
	if err != nil {
		return ClusterEnvelope{}, &completeFrameError{err: err}
	}
	return envelope, nil
}

// writeFrame requires the connection owner to serialize writes. Partial failures
// are returned without replay; some bytes may already have reached the peer.
func (codec *FrameCodec) writeFrame(writer io.Writer, envelope ClusterEnvelope) error {
	frame, err := codec.Encode(envelope)
	if err != nil {
		return err
	}
	return writeFrameBytes(writer, frame)
}

func writeFrameBytes(writer io.Writer, frame []byte) error {
	written, err := writer.Write(frame)
	if err != nil {
		return fmt.Errorf("write cluster frame: %w", err)
	}
	if written != len(frame) {
		return fmt.Errorf("write cluster frame: %w", io.ErrShortWrite)
	}
	return nil
}
