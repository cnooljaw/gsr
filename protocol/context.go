package protocol

import "google.golang.org/protobuf/proto"

// Context 提供单次客户端消息处理期间的响应和主动推送能力。
// 只在本次 Handler 中同步使用，不保存或用于异步任务；它不是取消/超时 context。
type Context struct {
	writer    *clientWriter
	requestID uint32
	responded *bool
}

// RequestID 返回当前客户端请求编号；零表示客户端通知。
func (context Context) RequestID() uint32 {
	return context.requestID
}

// Response 使用当前 request_id 发送强类型客户端响应，不是 Core Call 的 Reply。
// 成功仅表示编码后的字节被发送入口接纳，不保证客户端已经收到。
func (context Context) Response(msgID uint32, response proto.Message) error {
	if context.writer == nil {
		return ErrUninitializedContext
	}
	if context.requestID == 0 {
		return ErrMissingRequestID
	}
	if context.responded != nil && *context.responded {
		return ErrDuplicateResponse
	}
	if err := context.writer.sendResponse(context.requestID, msgID, response); err != nil {
		return err
	}
	if context.responded != nil {
		*context.responded = true
	}
	return nil
}

// Push 发送 request_id 为零的强类型服务端主动消息；成功仅表示发送入口接纳。
func (context Context) Push(msgID uint32, message proto.Message) error {
	if context.writer == nil {
		return ErrUninitializedContext
	}
	return context.writer.push(msgID, message)
}
