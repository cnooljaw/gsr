package protocol

import "google.golang.org/protobuf/proto"

// Context 提供单次客户端消息处理期间的响应和主动推送能力。
type Context struct {
	session   *Session
	requestID uint32
	responded *bool
}

// RequestID 返回当前客户端请求编号；零表示客户端通知。
func (context Context) RequestID() uint32 {
	return context.requestID
}

// Response 使用当前 request_id 发送一个强类型响应，不要求业务重复处理 Envelope。
func (context Context) Response(msgID uint32, response proto.Message) error {
	if context.session == nil {
		return ErrUninitializedContext
	}
	if context.requestID == 0 {
		return ErrMissingRequestID
	}
	if context.responded != nil && *context.responded {
		return ErrDuplicateResponse
	}
	if err := context.session.sendResponse(context.requestID, msgID, response); err != nil {
		return err
	}
	if context.responded != nil {
		*context.responded = true
	}
	return nil
}

// Push 发送 request_id 为零的强类型服务端主动消息。
func (context Context) Push(msgID uint32, message proto.Message) error {
	if context.session == nil {
		return ErrUninitializedContext
	}
	return context.session.Push(msgID, message)
}
