package cluster

import "reflect"

// PayloadCodec 统一转换 Cluster 参数与回复，不按 CommandID 选择或限制命令。
// 默认使用 ValueCodec；自定义实现必须并发安全，两端使用相同编码约定。
type PayloadCodec interface {
	Encode(any) ([]byte, error)
	Decode([]byte) (any, error)
}

func isNilPayloadCodec(codec PayloadCodec) bool {
	if codec == nil {
		return true
	}
	value := reflect.ValueOf(codec)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}
