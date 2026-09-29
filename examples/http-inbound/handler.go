package httpinbound

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/cnooljaw/gsr/runtime"
)

// roomCountHandler 是 HTTP 基础设施入口；它不持有或直接读取房间 Service 的状态。
func roomCountHandler(local *runtime.Runtime, roomID runtime.ServiceID) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		ctx, cancel := context.WithTimeout(request.Context(), time.Second)
		defer cancel()
		value, err := local.Call(ctx, roomID, roomCountCommand)
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				http.Error(writer, http.StatusText(http.StatusGatewayTimeout), http.StatusGatewayTimeout)
				return
			}
			http.Error(writer, http.StatusText(http.StatusServiceUnavailable), http.StatusServiceUnavailable)
			return
		}
		count, ok := value.(int)
		if !ok {
			http.Error(writer, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}
		body, err := json.Marshal(struct {
			Count int `json:"count"`
		}{Count: count})
		if err != nil {
			http.Error(writer, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		if _, err := writer.Write(append(body, '\n')); err != nil {
			return
		}
	})
}
