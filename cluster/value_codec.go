package cluster

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"reflect"
	"sort"
)

const (
	defaultMaxCodecDepth = 64
	maxCodecDepth        = 128
)

// ValueCodecConfig 配置默认值编解码器。
type ValueCodecConfig struct {
	// MaxDepth 限制列表与映射的嵌套深度，零值使用默认上限。
	MaxDepth int
	// MaxPayload 限制编码与解码的载荷字节数，零值使用默认上限。
	MaxPayload int
}

var (
	// ErrInvalidCodecConfig 表示 ValueCodec 配置无效。
	ErrInvalidCodecConfig = errors.New("cluster: invalid codec config")
	// ErrUnsupportedValue 表示值不在默认编解码器支持的范围内。
	ErrUnsupportedValue = errors.New("cluster: unsupported value")
	// ErrMalformedPayload 表示编码字节无法组成一个完整的值。
	ErrMalformedPayload = errors.New("cluster: malformed payload")
	// ErrMaxDepthExceeded 表示值超出配置的集合嵌套深度。
	ErrMaxDepthExceeded = errors.New("cluster: maximum value depth exceeded")
)

// ValueCodec 编码 Cluster 命令共享的有限、确定性值集合。
// 解码时，有符号整数统一为 int64，无符号整数统一为 uint64，浮点数统一为 float64。
type ValueCodec struct {
	maxDepth   int
	maxPayload int
}

// NewValueCodec 构造输入与输出均有上限的值编解码器。
func NewValueCodec(config ValueCodecConfig) (*ValueCodec, error) {
	if config.MaxDepth == 0 {
		config.MaxDepth = defaultMaxCodecDepth
	}
	if config.MaxPayload == 0 {
		config.MaxPayload = defaultMaxPayload
	}
	if config.MaxDepth < 1 || config.MaxDepth > maxCodecDepth {
		return nil, fmt.Errorf("%w: MaxDepth must be between 1 and %d", ErrInvalidCodecConfig, maxCodecDepth)
	}
	if config.MaxPayload < 1 {
		return nil, fmt.Errorf("%w: MaxPayload must be greater than zero", ErrInvalidCodecConfig)
	}
	return &ValueCodec{
		maxDepth:   config.MaxDepth,
		maxPayload: config.MaxPayload,
	}, nil
}

// Encode 将一个受支持的值转换为确定性的二进制载荷。
func (codec *ValueCodec) Encode(value any) ([]byte, error) {
	if codec == nil {
		return nil, fmt.Errorf("%w: codec is nil", ErrInvalidCodecConfig)
	}
	encoder := valueEncoder{
		codec: codec,
		data:  make([]byte, 0, 32),
	}
	if err := encoder.encode(reflect.ValueOf(value), 0); err != nil {
		return nil, err
	}
	return encoder.data, nil
}

// Decode 将一个完整二进制载荷转换为编解码器约定的规范 Go 值。
func (codec *ValueCodec) Decode(payload []byte) (any, error) {
	if codec == nil {
		return nil, fmt.Errorf("%w: codec is nil", ErrInvalidCodecConfig)
	}
	if len(payload) > codec.maxPayload {
		return nil, fmt.Errorf("%w: got %d, limit %d", ErrPayloadTooLarge, len(payload), codec.maxPayload)
	}
	decoder := valueDecoder{
		codec: codec,
		data:  payload,
	}
	value, err := decoder.decode(0)
	if err != nil {
		return nil, err
	}
	if decoder.offset != len(payload) {
		return nil, malformedPayload("trailing bytes")
	}
	return value, nil
}

type valueTag byte

const (
	valueTagNil valueTag = iota
	valueTagFalse
	valueTagTrue
	valueTagInt
	valueTagUint
	valueTagFloat
	valueTagString
	valueTagBytes
	valueTagList
	valueTagMap
)

type valueEncoder struct {
	codec *ValueCodec
	data  []byte
}

func (encoder *valueEncoder) encode(value reflect.Value, depth int) error {
	if !value.IsValid() {
		return encoder.writeByte(valueTagNil)
	}
	if value.Kind() == reflect.Interface {
		if value.IsNil() {
			return encoder.writeByte(valueTagNil)
		}
		return encoder.encode(value.Elem(), depth)
	}

	switch value.Kind() {
	case reflect.Bool:
		if value.Bool() {
			return encoder.writeByte(valueTagTrue)
		}
		return encoder.writeByte(valueTagFalse)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if err := encoder.writeByte(valueTagInt); err != nil {
			return err
		}
		return encoder.writeUint64(uint64(value.Int()))
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		if err := encoder.writeByte(valueTagUint); err != nil {
			return err
		}
		return encoder.writeUint64(value.Uint())
	case reflect.Float32, reflect.Float64:
		floatValue := value.Float()
		if math.IsNaN(floatValue) || math.IsInf(floatValue, 0) {
			return unsupportedValue(value, "non-finite float")
		}
		if err := encoder.writeByte(valueTagFloat); err != nil {
			return err
		}
		return encoder.writeUint64(math.Float64bits(floatValue))
	case reflect.String:
		if err := encoder.writeByte(valueTagString); err != nil {
			return err
		}
		return encoder.writeBytes([]byte(value.String()))
	case reflect.Slice:
		if value.Type().Elem().Kind() == reflect.Uint8 {
			if err := encoder.writeByte(valueTagBytes); err != nil {
				return err
			}
			return encoder.writeBytes(value.Bytes())
		}
		return encoder.encodeList(value, depth)
	case reflect.Array:
		return encoder.encodeList(value, depth)
	case reflect.Map:
		return encoder.encodeMap(value, depth)
	default:
		return unsupportedValue(value, "kind is not encodable")
	}
}

func (encoder *valueEncoder) encodeList(value reflect.Value, depth int) error {
	if depth >= encoder.codec.maxDepth {
		return fmt.Errorf("%w: depth %d", ErrMaxDepthExceeded, depth+1)
	}
	if err := encoder.writeByte(valueTagList); err != nil {
		return err
	}
	if err := encoder.writeCount(value.Len()); err != nil {
		return err
	}
	for index := 0; index < value.Len(); index++ {
		if err := encoder.encode(value.Index(index), depth+1); err != nil {
			return err
		}
	}
	return nil
}

func (encoder *valueEncoder) encodeMap(value reflect.Value, depth int) error {
	if value.Type().Key().Kind() != reflect.String {
		return unsupportedValue(value, "map key must be string")
	}
	if depth >= encoder.codec.maxDepth {
		return fmt.Errorf("%w: depth %d", ErrMaxDepthExceeded, depth+1)
	}
	keys := value.MapKeys()
	sort.Slice(keys, func(left, right int) bool {
		return keys[left].String() < keys[right].String()
	})
	if err := encoder.writeByte(valueTagMap); err != nil {
		return err
	}
	if err := encoder.writeCount(len(keys)); err != nil {
		return err
	}
	for _, key := range keys {
		if err := encoder.writeBytes([]byte(key.String())); err != nil {
			return err
		}
		if err := encoder.encode(value.MapIndex(key), depth+1); err != nil {
			return err
		}
	}
	return nil
}

func (encoder *valueEncoder) writeByte(tag valueTag) error {
	return encoder.write([]byte{byte(tag)})
}

func (encoder *valueEncoder) writeUint64(value uint64) error {
	var bytes [8]byte
	binary.LittleEndian.PutUint64(bytes[:], value)
	return encoder.write(bytes[:])
}

func (encoder *valueEncoder) writeCount(count int) error {
	if uint64(count) > math.MaxUint32 {
		return fmt.Errorf("%w: collection has %d items", ErrPayloadTooLarge, count)
	}
	var bytes [4]byte
	binary.LittleEndian.PutUint32(bytes[:], uint32(count))
	return encoder.write(bytes[:])
}

func (encoder *valueEncoder) writeBytes(value []byte) error {
	if uint64(len(value)) > math.MaxUint32 {
		return fmt.Errorf("%w: value has %d bytes", ErrPayloadTooLarge, len(value))
	}
	var length [4]byte
	binary.LittleEndian.PutUint32(length[:], uint32(len(value)))
	if err := encoder.write(length[:]); err != nil {
		return err
	}
	return encoder.write(value)
}

func (encoder *valueEncoder) write(value []byte) error {
	if len(value) > encoder.codec.maxPayload-len(encoder.data) {
		return fmt.Errorf("%w: limit %d", ErrPayloadTooLarge, encoder.codec.maxPayload)
	}
	encoder.data = append(encoder.data, value...)
	return nil
}

type valueDecoder struct {
	codec  *ValueCodec
	data   []byte
	offset int
}

func (decoder *valueDecoder) decode(depth int) (any, error) {
	tag, err := decoder.readByte()
	if err != nil {
		return nil, err
	}
	switch valueTag(tag) {
	case valueTagNil:
		return nil, nil
	case valueTagFalse:
		return false, nil
	case valueTagTrue:
		return true, nil
	case valueTagInt:
		value, err := decoder.readUint64()
		return int64(value), err
	case valueTagUint:
		return decoder.readUint64()
	case valueTagFloat:
		bits, err := decoder.readUint64()
		if err != nil {
			return nil, err
		}
		value := math.Float64frombits(bits)
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return nil, malformedPayload("non-finite float")
		}
		return value, nil
	case valueTagString:
		return decoder.readString()
	case valueTagBytes:
		value, err := decoder.readBytes()
		if err != nil {
			return nil, err
		}
		return append([]byte(nil), value...), nil
	case valueTagList:
		return decoder.decodeList(depth)
	case valueTagMap:
		return decoder.decodeMap(depth)
	default:
		return nil, malformedPayload("unknown value tag %d", tag)
	}
}

func (decoder *valueDecoder) decodeList(depth int) (any, error) {
	if depth >= decoder.codec.maxDepth {
		return nil, fmt.Errorf("%w: depth %d", ErrMaxDepthExceeded, depth+1)
	}
	count, err := decoder.readCount()
	if err != nil {
		return nil, err
	}
	if uint64(count) > uint64(decoder.codec.maxPayload) {
		return nil, fmt.Errorf("%w: list has %d items", ErrPayloadTooLarge, count)
	}
	if count > len(decoder.data)-decoder.offset {
		return nil, malformedPayload("list count exceeds remaining payload")
	}
	values := make([]any, count)
	for index := range values {
		values[index], err = decoder.decode(depth + 1)
		if err != nil {
			return nil, err
		}
	}
	return values, nil
}

func (decoder *valueDecoder) decodeMap(depth int) (any, error) {
	if depth >= decoder.codec.maxDepth {
		return nil, fmt.Errorf("%w: depth %d", ErrMaxDepthExceeded, depth+1)
	}
	count, err := decoder.readCount()
	if err != nil {
		return nil, err
	}
	if uint64(count) > uint64(decoder.codec.maxPayload) {
		return nil, fmt.Errorf("%w: map has %d items", ErrPayloadTooLarge, count)
	}
	if count > len(decoder.data)-decoder.offset {
		return nil, malformedPayload("map count exceeds remaining payload")
	}
	values := make(map[string]any, count)
	for range count {
		key, err := decoder.readString()
		if err != nil {
			return nil, err
		}
		if _, exists := values[key]; exists {
			return nil, malformedPayload("duplicate map key %q", key)
		}
		value, err := decoder.decode(depth + 1)
		if err != nil {
			return nil, err
		}
		values[key] = value
	}
	return values, nil
}

func (decoder *valueDecoder) readByte() (byte, error) {
	if decoder.offset >= len(decoder.data) {
		return 0, malformedPayload("unexpected end of payload")
	}
	value := decoder.data[decoder.offset]
	decoder.offset++
	return value, nil
}

func (decoder *valueDecoder) readUint64() (uint64, error) {
	bytes, err := decoder.read(8)
	if err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint64(bytes), nil
}

func (decoder *valueDecoder) readCount() (int, error) {
	bytes, err := decoder.read(4)
	if err != nil {
		return 0, err
	}
	count := binary.LittleEndian.Uint32(bytes)
	if uint64(count) > uint64(maxInt()) {
		return 0, malformedPayload("collection count overflows int")
	}
	return int(count), nil
}

func (decoder *valueDecoder) readString() (string, error) {
	value, err := decoder.readBytes()
	if err != nil {
		return "", err
	}
	return string(value), nil
}

func (decoder *valueDecoder) readBytes() ([]byte, error) {
	bytes, err := decoder.read(4)
	if err != nil {
		return nil, err
	}
	length := binary.LittleEndian.Uint32(bytes)
	if uint64(length) > uint64(decoder.codec.maxPayload) {
		return nil, fmt.Errorf("%w: value has %d bytes", ErrPayloadTooLarge, length)
	}
	return decoder.read(int(length))
}

func (decoder *valueDecoder) read(length int) ([]byte, error) {
	if length < 0 || length > len(decoder.data)-decoder.offset {
		return nil, malformedPayload("truncated payload")
	}
	value := decoder.data[decoder.offset : decoder.offset+length]
	decoder.offset += length
	return value, nil
}

func unsupportedValue(value reflect.Value, detail string) error {
	return fmt.Errorf("%w: %s (%s)", ErrUnsupportedValue, detail, value.Type())
}

func malformedPayload(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrMalformedPayload, fmt.Sprintf(format, args...))
}

func maxInt() int {
	return int(^uint(0) >> 1)
}
