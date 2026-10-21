package sproto

import (
	"reflect"
	"unsafe"
)

// decodePointer 只在当前同步 Decode 内借用临时结果；不缓存地址或跨调用保留 uintptr。
// wire 与预算仍由通用 parser 校验，这里只负责已确认布局的原生 *int64 / *string 写入。
type decodePointer struct {
	value unsafe.Pointer
}

// exactNative 必须由调用点基于同一 value 即时核对原生能力及实际 Go 类型。
func messageDecodePointer(value reflect.Value, exactNative bool) decodePointer {
	if !exactNative {
		return decodePointer{}
	}
	return decodePointer{value: value.UnsafePointer()}
}

// 每次写入都核对公开字段描述；外来 owner 或变化的形状必须保留反射索引语义。
func (pointer decodePointer) setInteger(schema *SprotoType, field *SprotoField, value *int64) bool {
	if pointer.value == nil || !field.nativeInteger(schema) {
		return false
	}
	// 使用 Go 类型化指针赋值，写屏障由编译器生成，不能用字节复制替代。
	*(**int64)(unsafe.Add(pointer.value, field.encOffset)) = value
	return true
}

// setString只在本次临时结果类型及字段原始布局仍匹配时写入，外来描述保持反射索引语义。
func (pointer decodePointer) setString(schema *SprotoType, field *SprotoField, value *string) bool {
	if pointer.value == nil || !field.nativeStringPointer || field.encOwner != schema || field.Wire != WireBytesName || field.Array {
		return false
	}
	// 类型化Go指针赋值保留写屏障，字符串仍由调用点复制，不借用输入缓冲。
	*(**string)(unsafe.Add(pointer.value, field.encOffset)) = value
	return true
}
