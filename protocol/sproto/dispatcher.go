package sproto

import (
	"errors"
	"fmt"
	"math"
	"reflect"

	codec "github.com/cnooljaw/gosproto"
)

var (
	// ErrInvalidFrame 表示包头、方向、关联号或载荷不符合当前协议。
	ErrInvalidFrame = errors.New("sproto: invalid client frame")
	// ErrUnknownProtocol 表示协议未定义或未绑定 Handler。
	ErrUnknownProtocol = errors.New("sproto: unsupported protocol")
	// ErrResponseRequired 表示请求缺少响应，或通知尝试生成响应。
	ErrResponseRequired = errors.New("sproto: response contract violated")
	// ErrDuplicateResponse 表示当前请求已经成功接纳一次响应。
	ErrDuplicateResponse = errors.New("sproto: duplicate response")
	// ErrInvalidContext 表示上下文已离开当前 Handler。
	ErrInvalidContext = errors.New("sproto: inactive request context")
)

// ClientSender 只接纳字节消息，协议不拥有连接的关闭责任。
type ClientSender interface {
	Send([]byte) error
}

// Config 使用原生 C2S/S2C 协议定义；首版 S2C 只允许无响应的 Push。
type Config struct {
	C2S    []*codec.Protocol
	S2C    []*codec.Protocol
	Limits codec.Limits
	// MaxPackedBytes 限制输入和输出的打包消息，零值使用 1 MiB。
	MaxPackedBytes int
}

type binding struct {
	protocol codec.Protocol
	handler  reflect.Value
}

// Dispatcher 归每连接 Agent 所有，构造后定义与 Handler 表只读，不启动 goroutine。
// Scope 只透传当前 Handler 上下文，不保存或解释 Service 身份。
type Dispatcher[Scope any] struct {
	config   Config
	sender   ClientSender
	requests map[int32]binding
	pushes   map[string]codec.Protocol
}

type packageHeader struct {
	Type    *int64 `sproto:"integer,0"`
	Session *int64 `sproto:"integer,1"`
}

// NewDispatcher 复制并检查协议和强类型 Handler 表，建立编解码和写回的关联。
// 有 Request 时 Handler 为 func(Scope, Context, *具体请求) error；无 Request 时省略最后参数。
func NewDispatcher[Scope any](config Config, sender ClientSender, handlers map[string]any) (*Dispatcher[Scope], error) {
	if nilSender(sender) {
		return nil, errors.New("sproto: sender is required")
	}
	if config.MaxPackedBytes == 0 {
		config.MaxPackedBytes = 1 << 20
	}
	if config.MaxPackedBytes < 1 {
		return nil, errors.New("sproto: invalid packed byte limit")
	}
	if err := config.Limits.Validate(); err != nil {
		return nil, err
	}
	requests, err := copyProtocols(config.C2S)
	if err != nil {
		return nil, err
	}
	pushes, err := copyProtocols(config.S2C)
	if err != nil {
		return nil, err
	}
	dispatcher := &Dispatcher[Scope]{
		config:   Config{Limits: config.Limits, MaxPackedBytes: config.MaxPackedBytes},
		sender:   sender,
		requests: make(map[int32]binding, len(requests)),
		pushes:   make(map[string]codec.Protocol, len(pushes)),
	}
	for name := range handlers {
		if _, exists := requests[name]; !exists {
			return nil, fmt.Errorf("%w: %s", ErrUnknownProtocol, name)
		}
	}
	for name, protocol := range requests {
		entry := binding{protocol: protocol}
		if handler, exists := handlers[name]; exists {
			entry.handler, err = compileHandler[Scope](protocol, handler)
			if err != nil {
				return nil, fmt.Errorf("protocol %s: %w", name, err)
			}
		}
		dispatcher.requests[protocol.Type] = entry
	}
	for name, protocol := range pushes {
		if protocol.HasResponse() {
			return nil, errors.New("sproto: server requests expecting client responses are unsupported")
		}
		dispatcher.pushes[name] = protocol
	}
	return dispatcher, nil
}

// MustDispatcher 用于静态接线，构造不满足契约时 panic。
func MustDispatcher[Scope any](config Config, sender ClientSender, handlers map[string]any) *Dispatcher[Scope] {
	dispatcher, err := NewDispatcher[Scope](config, sender, handlers)
	if err != nil {
		panic(err)
	}
	return dispatcher
}

func nilSender(sender ClientSender) bool {
	if sender == nil {
		return true
	}
	value := reflect.ValueOf(sender)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	}
	return false
}

func copyProtocols(definitions []*codec.Protocol) (map[string]codec.Protocol, error) {
	result := make(map[string]codec.Protocol, len(definitions))
	types := make(map[int32]bool, len(definitions))
	for _, definition := range definitions {
		if definition == nil || definition.Name == "" || definition.Type < 0 {
			return nil, errors.New("sproto: protocol requires a name and non-negative type")
		}
		if _, exists := result[definition.Name]; exists || types[definition.Type] {
			return nil, errors.New("sproto: duplicate protocol name or type")
		}
		for _, payload := range []reflect.Type{definition.Request, definition.Response} {
			if payload == nil {
				continue
			}
			if payload.Kind() != reflect.Pointer || payload.Elem().Kind() != reflect.Struct {
				return nil, errors.New("sproto: payload must be a pointer to struct")
			}
			if _, err := codec.GetSprotoType(payload.Elem()); err != nil {
				return nil, err
			}
		}
		result[definition.Name] = *definition
		types[definition.Type] = true
	}
	return result, nil
}

func compileHandler[Scope any](protocol codec.Protocol, handler any) (reflect.Value, error) {
	if handler == nil {
		return reflect.Value{}, errors.New("sproto: handler is required")
	}
	function := reflect.ValueOf(handler)
	signature := function.Type()
	if signature.Kind() != reflect.Func || function.IsNil() || signature.IsVariadic() {
		return reflect.Value{}, errors.New("sproto: handler must be a non-nil function")
	}
	count := 2
	if protocol.HasRequest() {
		count++
	}
	if signature.NumIn() != count || signature.NumOut() != 1 {
		return reflect.Value{}, errors.New("sproto: handler arity does not match protocol")
	}
	if signature.In(0) != reflect.TypeOf((*Scope)(nil)).Elem() || signature.In(1) != reflect.TypeOf(Context{}) || signature.Out(0) != reflect.TypeOf((*error)(nil)).Elem() {
		return reflect.Value{}, errors.New("sproto: handler context or result type does not match")
	}
	if protocol.HasRequest() && signature.In(2) != protocol.Request {
		return reflect.Value{}, errors.New("sproto: handler payload type does not match")
	}
	return function, nil
}

// HandleFrame 解包原生 package 与请求并透传 Scope；失败只返回诊断，不生成其它协议的错误包。
// Agent 应关闭不能获得合法结果的连接，使客户端终止等待；业务失败使用自身 Response。
func (dispatcher *Dispatcher[Scope]) HandleFrame(self Scope, frame []byte) error {
	if len(frame) > dispatcher.config.MaxPackedBytes {
		return codec.ErrLimit
	}
	maxBytes := dispatcher.config.Limits.MaxBytes
	if maxBytes == 0 {
		maxBytes = 8 << 20
	}
	unpacked, err := codec.UnpackWithLimit(frame, maxBytes)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidFrame, err)
	}
	var header packageHeader
	used, err := codec.DecodeWithLimits(unpacked, &header, dispatcher.config.Limits)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidFrame, err)
	}
	if header.Type == nil || *header.Type < 0 || *header.Type > math.MaxInt32 {
		return ErrInvalidFrame
	}
	entry, exists := dispatcher.requests[int32(*header.Type)]
	if !exists || !entry.handler.IsValid() {
		return ErrUnknownProtocol
	}
	if entry.protocol.HasResponse() {
		if header.Session == nil || *header.Session <= 0 || *header.Session > math.MaxInt32 {
			return ErrInvalidFrame
		}
	} else if header.Session != nil {
		return ErrInvalidFrame
	}
	arguments := []reflect.Value{reflect.ValueOf(&self).Elem()}
	state := &requestState{active: true}
	defer func() { state.active = false }()
	request := Context{
		state:        state,
		responseType: entry.protocol.Response,
		sendResponse: func(session int32, response any) error {
			current := int64(session)
			return dispatcher.send(packageHeader{Session: &current}, response)
		},
		sendPush: dispatcher.Push,
	}
	if header.Session != nil {
		request.session = int32(*header.Session)
	}
	arguments = append(arguments, reflect.ValueOf(request))
	if entry.protocol.HasRequest() {
		payload := reflect.New(entry.protocol.Request.Elem())
		consumed, err := codec.DecodeWithLimits(unpacked[used:], payload.Interface(), dispatcher.config.Limits)
		if err != nil {
			return fmt.Errorf("%w: %w", ErrInvalidFrame, err)
		}
		used += consumed
		arguments = append(arguments, payload)
	}
	// pack 为完整消息补齐最多七个零字节；其它尾部内容不是合法载荷。
	if len(unpacked)-used > 7 {
		return ErrInvalidFrame
	}
	for _, extra := range unpacked[used:] {
		if extra != 0 {
			return ErrInvalidFrame
		}
	}
	returned := entry.handler.Call(arguments)
	if !returned[0].IsNil() {
		return returned[0].Interface().(error)
	}
	if entry.protocol.HasResponse() && !state.responded {
		return ErrResponseRequired
	}
	return nil
}

// Push 编码 S2C 中不期待响应的消息，必须由所属 Agent 串行调用。
func (dispatcher *Dispatcher[Scope]) Push(name string, message any) error {
	protocol, exists := dispatcher.pushes[name]
	if !exists {
		return ErrUnknownProtocol
	}
	if reflect.TypeOf(message) != protocol.Request {
		return errors.New("sproto: push payload type does not match")
	}
	typ := int64(protocol.Type)
	return dispatcher.send(packageHeader{Type: &typ}, message)
}

func (dispatcher *Dispatcher[Scope]) send(header packageHeader, message any) error {
	encoded, err := codec.EncodeWithLimits(&header, dispatcher.config.Limits)
	if err != nil {
		return err
	}
	if message != nil {
		payload, err := codec.EncodeWithLimits(message, dispatcher.config.Limits)
		if err != nil {
			return err
		}
		encoded = append(encoded, payload...)
	}
	maxBytes := dispatcher.config.Limits.MaxBytes
	if maxBytes == 0 {
		maxBytes = 8 << 20
	}
	if len(encoded) > maxBytes {
		return codec.ErrLimit
	}
	packed := codec.Pack(encoded)
	if len(packed) > dispatcher.config.MaxPackedBytes {
		return codec.ErrLimit
	}
	return dispatcher.sender.Send(packed)
}
