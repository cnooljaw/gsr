package sproto

import "unsafe"

// canDecodeIntegerRecords 只证明当前Decoder路由可达的非nil描述；不依赖当前公开Tag。
// 与Encode不同，负Tag、nil和短Fields不能被提前扫描改变语义，不能跨调用缓存此结果。
func canDecodeIntegerRecords(schema *SprotoType) bool {
	if !schema.int64Record {
		return false
	}
	for _, index := range schema.tagMap {
		if uint(index) >= uint(len(schema.Fields)) {
			return false
		}
		field := schema.Fields[index]
		if field != nil && !field.nativeInteger(schema) {
			return false
		}
	}
	return true
}

// decodeIntegerRecord只接收本次数组证明覆盖的字段与reflect.New(schema.typ)创建的独占对象。
func decodeIntegerRecord(chunk []byte, schema *SprotoType, pointer decodePointer, budget *decodeBudget) (int, error) {
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
		field := schema.FieldByTag(tag)
		tag++
		var bits uint64
		if entry != 0 {
			if field == nil {
				continue
			}
			bits = uint64(entry/2 - 1)
		} else {
			data := chunk[total:]
			if len(data) < 4 {
				return 0, ErrDecode
			}
			size := uint64(readUint32(data))
			if size > uint64(len(data)-4) {
				return 0, ErrDecode
			}
			total += 4 + int(size)
			if field == nil {
				continue
			}
			switch size {
			case 4:
				bits = uint64(int64(int32(readUint32(data[4:8]))))
			case 8:
				bits = readUint64(data[4:12])
			default:
				return 0, ErrDecode
			}
		}
		result := new(int64)
		*result = int64(bits)
		// 数组入口已核对当前可达字段与实际子类型；Go指针写入保留屏障。
		*(**int64)(unsafe.Add(pointer.value, field.encOffset)) = result
	}
	return total, nil
}
