package cluster

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

// completeFrameError 表示解码失败前已消费整个有长度上限的完整帧。
// 它只证明字节流仍然对齐，不表示输入可接受；认证和协议规则仍可能要求关闭连接。
type completeFrameError struct {
	err error
}

func (err *completeFrameError) Error() string {
	return "decode complete cluster frame: " + err.err.Error()
}

func (err *completeFrameError) Unwrap() error {
	return err.err
}

// readFrame 精确读取一个有长度上限的帧。连接管理组件负责截止时间与取消，
// 并判断错误是否需要关闭字节流；此处不管理 goroutine 或连接。
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
		// 完整帧头之后遇到 EOF 仍表示帧被截断，而不是正常流结束。
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

// writeFrame 要求连接管理组件串行执行写入。
// 部分写入失败直接返回，不重放，因为部分字节可能已经到达对端。
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
