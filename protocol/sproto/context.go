package sproto

import (
	"errors"
	"reflect"
)

type requestState struct {
	active    bool
	responded bool
}

// Context 只在本次 Handler 中同步使用；Sproto session 与 Core/Cluster Session 分离。
type Context struct {
	session      int32
	state        *requestState
	responseType reflect.Type
	sendResponse func(int32, any) error
	sendPush     func(string, any) error
}

// Session 返回当前请求关联号；零表示不期待响应的 Notify。
func (request Context) Session() int32 {
	return request.session
}

// Response 按当前协议的 Response 类型编码并关联原 session；成功仅表示字节入口接纳。
func (request Context) Response(message any) error {
	if request.state == nil || !request.state.active {
		return ErrInvalidContext
	}
	if request.session == 0 || request.responseType == nil {
		return ErrResponseRequired
	}
	if request.state.responded {
		return ErrDuplicateResponse
	}
	if reflect.TypeOf(message) != request.responseType {
		return errors.New("sproto: response payload type does not match")
	}
	if err := request.sendResponse(request.session, message); err != nil {
		return err
	}
	request.state.responded = true
	return nil
}

// Push 在当前 Handler 中发送 S2C 主动消息，不登记等待响应。
func (request Context) Push(name string, message any) error {
	if request.state == nil || !request.state.active {
		return ErrInvalidContext
	}
	return request.sendPush(name, message)
}
