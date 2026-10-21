package sproto

import (
	"encoding/binary"
	"math"
	"reflect"
	"unsafe"
)

type Tag struct {
	Tag uint16
	Val *uint16
}

func readUint16(buffer []byte) uint16 {
	return binary.LittleEndian.Uint16(buffer)
}

func readUint32(buffer []byte) uint32 {
	return binary.LittleEndian.Uint32(buffer)
}

func readUint64(buffer []byte) uint64 {
	return binary.LittleEndian.Uint64(buffer)
}

func readChunk(chunk []byte) (int, []byte, error) {
	if len(chunk) < 4 {
		return 0, nil, ErrDecode
	}
	size := uint64(readUint32(chunk))
	if size > uint64(len(chunk)-4) {
		return 0, nil, ErrDecode
	}
	end := 4 + int(size)
	return end, chunk[4:end], nil
}

func targetValue(field *SprotoField, value reflect.Value) reflect.Value {
	if field.pointer {
		value.Set(reflect.New(field.valueType))
		return value.Elem()
	}
	return value
}

func setInteger(value reflect.Value, bits uint64) error {
	switch value.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		signed := int64(bits)
		if value.OverflowInt(signed) {
			return ErrDecode
		}
		value.SetInt(signed)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		if int64(bits) < 0 || value.OverflowUint(bits) {
			return ErrDecode
		}
		value.SetUint(bits)
	default:
		return ErrDecode
	}
	return nil
}

func decodeBool(inline uint16, hasInline bool, _ []byte, field *SprotoField, value reflect.Value, _ *decodeBudget) error {
	if !hasInline {
		return ErrDecode
	}
	targetValue(field, value).SetBool(inline != 0)
	return nil
}

// scalarBits 解释 inline 或 4/8 字节数值，所有赋值路径共用相同的边界校验。
func scalarBits(inline uint16, hasInline bool, data []byte) (uint64, error) {
	if hasInline {
		return uint64(inline), nil
	}
	switch len(data) {
	case 4:
		return uint64(int64(int32(readUint32(data)))), nil
	case 8:
		return readUint64(data), nil
	default:
		return 0, ErrDecode
	}
}

func decodeInt(inline uint16, hasInline bool, data []byte, field *SprotoField, value reflect.Value, _ *decodeBudget) error {
	bits, err := scalarBits(inline, hasInline, data)
	if err != nil {
		return err
	}
	return setInteger(targetValue(field, value), bits)
}

// decodeInt64 无需重复查询目标位宽；wire 已保证全部 int64 位模式可表示。
func decodeInt64(inline uint16, hasInline bool, data []byte, field *SprotoField, value reflect.Value, _ *decodeBudget) error {
	bits, err := scalarBits(inline, hasInline, data)
	if err != nil {
		return err
	}
	targetValue(field, value).SetInt(int64(bits))
	return nil
}

// decodeInt64Pointer 由 schema 仅绑定到原生 *int64，命名类型继续使用反射兼容路径。
func decodeInt64Pointer(inline uint16, hasInline bool, data []byte, _ *SprotoField, value reflect.Value, _ *decodeBudget) error {
	bits, err := scalarBits(inline, hasInline, data)
	if err != nil {
		return err
	}
	result := new(int64)
	*result = int64(bits)
	value.Set(reflect.ValueOf(result))
	return nil
}

func decodeDouble(inline uint16, hasInline bool, data []byte, field *SprotoField, value reflect.Value, _ *decodeBudget) error {
	bits, err := scalarBits(inline, hasInline, data)
	if err != nil {
		return err
	}
	targetValue(field, value).SetFloat(math.Float64frombits(bits))
	return nil
}

func decodeString(_ uint16, _ bool, data []byte, field *SprotoField, value reflect.Value, _ *decodeBudget) error {
	targetValue(field, value).SetString(string(data))
	return nil
}

// decodeStringPointer 对原生 *string 直接构造独占值，命名类型仍由通用路径处理。
func decodeStringPointer(_ uint16, _ bool, data []byte, _ *SprotoField, value reflect.Value, _ *decodeBudget) error {
	result := new(string)
	*result = string(data)
	value.Set(reflect.ValueOf(result))
	return nil
}

func decodeBytes(_ uint16, _ bool, data []byte, _ *SprotoField, value reflect.Value, _ *decodeBudget) error {
	value.SetBytes(append([]byte{}, data...))
	return nil
}

func decodeBoolSlice(_ uint16, _ bool, data []byte, _ *SprotoField, value reflect.Value, budget *decodeBudget) error {
	if err := budget.add(len(data)); err != nil {
		return err
	}
	result := reflect.MakeSlice(value.Type(), len(data), len(data))
	for i, b := range data {
		result.Index(i).SetBool(b != 0)
	}
	value.Set(result)
	return nil
}

func decodeIntSlice(_ uint16, _ bool, data []byte, field *SprotoField, value reflect.Value, budget *decodeBudget) error {
	// 官方接受空内容或仅有宽度前缀的空数组；非空数组必须先校验宽度。
	if len(data) <= 1 {
		if field.int64Slice {
			*value.Addr().Interface().(*[]int64) = []int64{}
			return nil
		}
		value.Set(reflect.MakeSlice(value.Type(), 0, 0))
		return nil
	}
	width := int(data[0])
	data = data[1:]
	if (width != 4 && width != 8) || len(data)%width != 0 {
		return ErrDecode
	}
	count := len(data) / width
	if err := budget.add(count); err != nil {
		return err
	}
	if field.int64Slice {
		result := make([]int64, count)
		if width == 4 {
			for index := range result {
				result[index] = int64(int32(readUint32(data[index*4 : index*4+4 : index*4+4])))
			}
		} else {
			for index := range result {
				result[index] = int64(readUint64(data[index*8 : index*8+8 : index*8+8]))
			}
		}
		*value.Addr().Interface().(*[]int64) = result
		return nil
	}
	result := reflect.MakeSlice(value.Type(), count, count)
	for i := 0; i < count; i++ {
		var bits uint64
		if width == 4 {
			bits = uint64(int64(int32(readUint32(data[i*width:]))))
		} else {
			bits = readUint64(data[i*width:])
		}
		if field.Wire == WireDoubleName {
			result.Index(i).SetFloat(math.Float64frombits(bits))
		} else if err := setInteger(result.Index(i), bits); err != nil {
			return err
		}
	}
	value.Set(result)
	return nil
}

// countChunks 在任何数组分配前检查完整边界和元素预算。
func countChunks(data []byte, budget *decodeBudget) (int, error) {
	count := 0
	for len(data) > 0 {
		used, _, err := readChunk(data)
		if err != nil {
			return 0, err
		}
		if err := budget.add(1); err != nil {
			return 0, err
		}
		count++
		data = data[used:]
	}
	return count, nil
}

func decodeStringSlice(_ uint16, _ bool, data []byte, _ *SprotoField, value reflect.Value, budget *decodeBudget) error {
	count, err := countChunks(data, budget)
	if err != nil {
		return err
	}
	result := reflect.MakeSlice(value.Type(), count, count)
	// 只借用本次新切片；实际元素须恰为string，命名元素保留反射路径。
	var slots []string
	if count > 0 && value.Type().Elem() == reflect.TypeFor[string]() {
		slots = unsafe.Slice((*string)(result.UnsafePointer()), count)
	}
	for index := 0; index < count; index++ {
		used, content, err := readChunk(data)
		if err != nil {
			return err
		}
		if slots != nil {
			slots[index] = string(content)
		} else {
			result.Index(index).SetString(string(content))
		}
		data = data[used:]
	}
	value.Set(result)
	return nil
}

func decodeStruct(_ uint16, _ bool, data []byte, field *SprotoField, value reflect.Value, budget *decodeBudget) error {
	result := reflect.New(field.valueType)
	used, err := decodeMessage(data, field.st, result, budget)
	if err != nil {
		return err
	}
	if used != len(data) {
		return ErrDecode
	}
	value.Set(result)
	return nil
}

func decodeStructSlice(_ uint16, _ bool, data []byte, field *SprotoField, value reflect.Value, budget *decodeBudget) error {
	count, err := countChunks(data, budget)
	if err != nil {
		return err
	}
	result := reflect.MakeSlice(value.Type(), count, count)
	var slots []unsafe.Pointer
	nativeRecords := false
	if count > 0 && field.Array && field.Wire == WireStructName && field.st != nil &&
		value.Type().Elem() == reflect.PointerTo(field.st.typ) {
		// 同步借用新切片的指针槽；实际元素类型已核对，不缓存地址。
		slots = unsafe.Slice((*unsafe.Pointer)(result.UnsafePointer()), count)
		nativeRecords = count >= 8 && canDecodeIntegerRecords(field.st)
	}
	for index := 0; index < count; index++ {
		used, content, err := readChunk(data)
		if err != nil {
			return err
		}
		current := reflect.New(field.st.typ)
		currentPointer := current.UnsafePointer()
		var consumed int
		if nativeRecords {
			consumed, err = decodeIntegerRecord(content, field.st, decodePointer{value: currentPointer}, budget)
		} else {
			consumed, err = decodeMessage(content, field.st, current, budget)
		}
		if err != nil {
			return err
		}
		if consumed != len(content) {
			return ErrDecode
		}
		if slots != nil {
			// Go 指针赋值保留 GC 写屏障，不能改成字节复制或 uintptr 存储。
			slots[index] = currentPointer
		} else {
			result.Index(index).Set(current)
		}
		data = data[used:]
	}
	value.Set(result)
	return nil
}

func decodeMessage(chunk []byte, schema *SprotoType, value reflect.Value, budget *decodeBudget) (int, error) {
	budget.depth++
	defer func() { budget.depth-- }()
	if budget.depth > budget.limits.MaxDepth {
		return 0, ErrLimit
	}
	if len(chunk) < 2 {
		return 0, ErrDecode
	}
	count := int(readUint16(chunk))
	total := 2 + count*2
	if total > len(chunk) {
		return 0, ErrDecode
	}
	if err := budget.add(count); err != nil {
		return 0, err
	}
	record := value.Elem()
	exactNative := (schema.nativeInt64Pointer || schema.nativeStringPointer) && record.Type() == schema.typ
	pointer := messageDecodePointer(value, exactNative)
	tag := 0
	for index := 0; index < count; index++ {
		entry := int(readUint16(chunk[2+index*2 : 4+index*2 : 4+index*2]))
		if entry%2 == 1 {
			tag += (entry + 1) / 2
			continue
		}
		if tag > 65535 {
			return 0, ErrDecode
		}
		hasInline := entry != 0
		inline := uint16(entry/2 - 1)
		var content []byte
		if !hasInline {
			used, data, err := readChunk(chunk[total:])
			if err != nil {
				return 0, err
			}
			total += used
			content = data
		}
		field := schema.FieldByTag(tag)
		tag++
		if field == nil {
			continue
		}
		if hasInline && (field.Array || field.Wire == WireBytesName || field.Wire == WireStructName) {
			return 0, ErrDecode
		}
		if field.nativeInt64Pointer {
			bits, err := scalarBits(inline, hasInline, content)
			if err != nil {
				return 0, err
			}
			result := new(int64)
			*result = int64(bits)
			if !pointer.setInteger(schema, field, result) {
				record.Field(field.index[0]).Set(reflect.ValueOf(result))
			}
			continue
		}
		if field.nativeStringPointer {
			result := new(string)
			*result = string(content)
			if !pointer.setString(schema, field, result) {
				record.Field(field.index[0]).Set(reflect.ValueOf(result))
			}
			continue
		}
		if err := field.dec(inline, hasInline, content, field, record.Field(field.index[0]), budget); err != nil {
			return 0, err
		}
	}
	return total, nil
}

// Decode 解码一个结构体，使用默认有限预算，返回已消费的字节数。
func Decode(data []byte, message interface{}) (int, error) {
	return DecodeWithLimits(data, message, Limits{})
}

// DecodeWithLimits 在递归与分配前检查预算；失败时保留调用方原值。
func DecodeWithLimits(data []byte, message interface{}, limits Limits) (int, error) {
	limits, err := normalizeLimits(limits)
	if err != nil {
		return 0, err
	}
	if len(data) > limits.MaxBytes {
		return 0, ErrLimit
	}
	typ, value, err := getbase(message)
	if err != nil {
		return 0, err
	}
	schema, err := GetSprotoType(typ.Elem())
	if err != nil {
		return 0, err
	}
	result := reflect.New(typ.Elem())
	used, err := decodeMessage(data, schema, result, &decodeBudget{limits: limits})
	if err != nil {
		return 0, err
	}
	value.Elem().Set(result.Elem())
	return used, nil
}

func MustDecode(data []byte, message interface{}) int {
	used, err := Decode(data, message)
	if err != nil {
		panic(err)
	}
	return used
}
