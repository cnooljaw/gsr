package httpinbound

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/cnooljaw/gsr/cluster"
	"github.com/cnooljaw/gsr/httpserver"
	"github.com/cnooljaw/gsr/runtime"
)

// TestAdminHTTPRoutesCallLocalAndRemoteServices 验证多条 REST 路由能修改本地状态，并由业务 Service 汇总本地与远端结果。
func TestAdminHTTPRoutesCallLocalAndRemoteServices(t *testing.T) {
	var local, remote *runtime.Runtime
	var outbound, inbound *cluster.Cluster
	var server *httpserver.Server
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if server != nil {
			if err := server.Close(ctx); err != nil {
				t.Errorf("HTTP Close() error = %v", err)
			}
		}
		if outbound != nil {
			outbound.Quiesce()
		}
		if local != nil {
			if err := local.Close(); err != nil {
				t.Errorf("local Runtime.Close() error = %v", err)
			}
		}
		if outbound != nil {
			if err := outbound.Close(ctx); err != nil {
				t.Errorf("outbound Cluster.Close() error = %v", err)
			}
		}
		if inbound != nil {
			inbound.Quiesce()
		}
		if remote != nil {
			if err := remote.Close(); err != nil {
				t.Errorf("remote Runtime.Close() error = %v", err)
			}
		}
		if inbound != nil {
			if err := inbound.Close(ctx); err != nil {
				t.Errorf("inbound Cluster.Close() error = %v", err)
			}
		}
	})

	var err error
	remote, err = runtime.NewRuntime(runtime.Config{})
	if err != nil {
		t.Fatalf("new remote Runtime: %v", err)
	}
	if err := remote.Start(); err != nil {
		t.Fatalf("start remote Runtime: %v", err)
	}
	roomID, err := remote.CreateService(context.Background(), &roomService{players: 12})
	if err != nil {
		t.Fatalf("create remote room Service: %v", err)
	}
	inbound, err = cluster.NewCluster(cluster.ClusterConfig{})
	if err != nil {
		t.Fatalf("new inbound Cluster: %v", err)
	}
	remoteAddress, err := inbound.Open(remote, cluster.ListenConfig{
		Address: "127.0.0.1:0",
		ResolveService: func(name cluster.ServiceName) (runtime.ServiceID, error) {
			if name == "rooms" {
				return roomID, nil
			}
			return 0, runtime.ErrServiceNotFound
		},
	})
	if err != nil {
		t.Fatalf("open remote Cluster: %v", err)
	}
	outbound, err = cluster.NewCluster(cluster.ClusterConfig{
		Nodes: map[cluster.NodeID]string{"rooms-node": remoteAddress},
	})
	if err != nil {
		t.Fatalf("new outbound Cluster: %v", err)
	}
	local, err = runtime.NewRuntime(runtime.Config{})
	if err != nil {
		t.Fatalf("new local Runtime: %v", err)
	}
	if err := local.Start(); err != nil {
		t.Fatalf("start local Runtime: %v", err)
	}
	userID, err := local.CreateService(context.Background(), &userService{})
	if err != nil {
		t.Fatalf("create user Service: %v", err)
	}
	dashboardID, err := local.CreateService(context.Background(), &dashboardService{
		users: userID, network: outbound, remoteNode: "rooms-node",
	})
	if err != nil {
		t.Fatalf("create dashboard Service: %v", err)
	}
	server, err = httpserver.New(httpserver.Config{
		Address: "127.0.0.1:0", Handler: newAdminMux(local, userID, dashboardID),
	})
	if err != nil {
		t.Fatalf("new HTTP Server: %v", err)
	}
	if err := server.Start(); err != nil {
		t.Fatalf("start HTTP Server: %v", err)
	}

	client := &http.Client{Timeout: 3 * time.Second}
	baseURL := "http://" + server.Addr().String()
	for _, step := range []struct {
		method, path string
		banned       bool
		roomPlayers  int
	}{
		{method: http.MethodGet, path: "/users/7"},
		{method: http.MethodPost, path: "/users/7/ban", banned: true},
		{method: http.MethodGet, path: "/dashboard/7", banned: true, roomPlayers: 12},
	} {
		request, err := http.NewRequest(step.method, baseURL+step.path, nil)
		if err != nil {
			t.Fatalf("new request %s %s: %v", step.method, step.path, err)
		}
		response, err := client.Do(request)
		if err != nil {
			t.Fatalf("request %s %s: %v", step.method, step.path, err)
		}
		var body struct {
			ID          uint64     `json:"id"`
			Banned      bool       `json:"banned"`
			User        userRecord `json:"user"`
			RoomPlayers int        `json:"room_players"`
		}
		decodeErr := json.NewDecoder(response.Body).Decode(&body)
		response.Body.Close()
		if response.StatusCode != http.StatusOK || decodeErr != nil {
			t.Fatalf("%s %s: status = %d, decode = %v", step.method, step.path, response.StatusCode, decodeErr)
		}
		if step.roomPlayers > 0 {
			if body.User.ID != 7 || body.User.Banned != step.banned || body.RoomPlayers != step.roomPlayers {
				t.Fatalf("%s response = %+v", step.path, body)
			}
		} else if body.ID != 7 || body.Banned != step.banned {
			t.Fatalf("%s response = %+v", step.path, body)
		}
	}
}
