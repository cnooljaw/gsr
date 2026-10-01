package httpinbound

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/cnooljaw/gsr/runtime"
)

// newAdminMux 将 REST 路由映射到拥有业务状态的 Service，而不保存其实现指针。
func newAdminMux(local *runtime.Runtime, users, dashboard runtime.ServiceID) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /users/{id}", func(writer http.ResponseWriter, request *http.Request) {
		callAdminService(writer, request, local, users, userGetCommand)
	})
	mux.HandleFunc("POST /users/{id}/ban", func(writer http.ResponseWriter, request *http.Request) {
		callAdminService(writer, request, local, users, userBanCommand)
	})
	mux.HandleFunc("GET /dashboard/{id}", func(writer http.ResponseWriter, request *http.Request) {
		callAdminService(writer, request, local, dashboard, dashboardCommand)
	})
	return mux
}

func callAdminService(
	writer http.ResponseWriter,
	request *http.Request,
	local *runtime.Runtime,
	target runtime.ServiceID,
	command runtime.CommandID,
) {
	id, err := strconv.ParseUint(request.PathValue("id"), 10, 64)
	if err != nil || id == 0 {
		http.Error(writer, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(request.Context(), 2*time.Second)
	defer cancel()
	value, err := local.Call(ctx, target, command, id)
	if err != nil {
		status := http.StatusServiceUnavailable
		if errors.Is(err, context.DeadlineExceeded) {
			status = http.StatusGatewayTimeout
		}
		http.Error(writer, http.StatusText(status), status)
		return
	}
	body, err := json.Marshal(value)
	if err != nil {
		http.Error(writer, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	writer.Header().Set("Content-Type", "application/json")
	_, _ = writer.Write(append(body, '\n'))
}
