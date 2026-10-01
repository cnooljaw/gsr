package httpinbound

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	gsr "github.com/cnooljaw/gsr"
	"github.com/cnooljaw/gsr/cluster"
	"github.com/cnooljaw/gsr/logging"
	"github.com/cnooljaw/gsr/runtime"
)

// 验证 MainService 开启的多条 REST 路由调用本地 Service，业务 Service 再汇总远端结果。
func TestAdminHTTPRoutesCallLocalAndRemoteServices(t *testing.T) {
	remote, err := runtime.NewRuntime(runtime.Config{})
	if err != nil {
		t.Fatalf("new remote Runtime: %v", err)
	}
	t.Cleanup(func() {
		if err := remote.Close(); err != nil {
			t.Errorf("remote Runtime.Close() error = %v", err)
		}
	})
	if err := remote.Start(); err != nil {
		t.Fatalf("start remote Runtime: %v", err)
	}
	roomID, err := remote.CreateService(context.Background(), &roomService{players: 12})
	if err != nil {
		t.Fatalf("create remote room Service: %v", err)
	}
	inbound, err := cluster.NewCluster(cluster.ClusterConfig{})
	if err != nil {
		t.Fatalf("new inbound Cluster: %v", err)
	}
	t.Cleanup(func() {
		inbound.Quiesce()
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := inbound.Close(ctx); err != nil {
			t.Errorf("inbound Cluster.Close() error = %v", err)
		}
	})
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

	reservation, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	httpAddress := reservation.Addr().String()
	if err := reservation.Close(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	started := make(chan error, 1)
	done := make(chan error, 1)
	go func() {
		done <- gsr.Run(ctx, gsr.Config{
			Logging: logging.Config{Writer: io.Discard},
			Cluster: cluster.ClusterConfig{Nodes: map[cluster.NodeID]string{"rooms-node": remoteAddress}},
		}, &adminMainService{address: httpAddress, started: started})
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Errorf("gsr.Run() error = %v", err)
			}
		case <-time.After(3 * time.Second):
			t.Error("gsr.Run() did not stop")
		}
	})
	select {
	case err := <-started:
		if err != nil {
			t.Fatalf("start HTTP application: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("HTTP application did not start")
	}

	client := &http.Client{Timeout: 3 * time.Second}
	baseURL := "http://" + httpAddress
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

type adminMainService struct {
	address string
	started chan<- error
}

func (service *adminMainService) OnStart(ctx context.Context, self gsr.ServiceContext) (result error) {
	defer func() { service.started <- result }()
	users, err := self.CreateService(ctx, newUserService())
	if err != nil {
		return err
	}
	if err := self.Register("users", users); err != nil {
		return err
	}
	dashboard, err := self.CreateService(ctx, newDashboardService("rooms-node"))
	if err != nil {
		return err
	}
	if err := self.Register("dashboard", dashboard); err != nil {
		return err
	}
	if err := self.OpenHTTP(service.address, func(in *gsr.HTTPIngress) http.Handler {
		return newAdminMux(in)
	}); err != nil {
		return err
	}
	self.Exit()
	return nil
}

func (service *adminMainService) Handle(_ gsr.CommandContext, _ runtime.CommandID, _ ...any) (any, error) {
	return nil, runtime.ErrUnknownCommand
}

func (service *adminMainService) OnStop(_ context.Context) error {
	return nil
}
