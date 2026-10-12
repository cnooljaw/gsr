package httpoutbound

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/cnooljaw/gsr/runtime"
)

const (
	refreshProfileCommand runtime.CommandID = iota + 1
	getProfileCommand
	maxProfileBodyBytes = 4 << 10
)

var (
	errProfileTimeout     = runtime.NewStatusError(runtime.ErrorClassDomain, "profile.timeout", "profile refresh timed out")
	errProfileUnavailable = runtime.NewStatusError(runtime.ErrorClassDomain, "profile.unavailable", "profile is unavailable")
)

// profileService 独占玩家显示名；HTTP Client 是应用注入的外部依赖，不是另一个 Service。
type profileService struct {
	client     *http.Client
	profileURL string
	name       string
}

var _ runtime.Service = (*profileService)(nil)

func newProfileService(client *http.Client, profileURL string) (*profileService, error) {
	if client == nil || client.Timeout <= 0 {
		return nil, errors.New("profile service requires an HTTP Client with finite timeout")
	}
	request, err := http.NewRequest(http.MethodGet, profileURL, nil)
	if err != nil {
		return nil, errors.New("profile URL must be an absolute HTTP(S) URL")
	}
	if request.URL.Host == "" || (request.URL.Scheme != "http" && request.URL.Scheme != "https") {
		return nil, errors.New("profile URL must be an absolute HTTP(S) URL")
	}
	return &profileService{client: client, profileURL: profileURL}, nil
}

func (service *profileService) OnStart(_ context.Context, _ runtime.ServiceContext) error {
	return nil
}

func (service *profileService) Handle(_ runtime.CommandContext, command runtime.CommandID, args ...any) (any, error) {
	switch command {
	case refreshProfileCommand:
		if len(args) != 0 {
			return nil, runtime.ErrInvalidCommandArguments
		}
		name, err := service.fetchProfile()
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				return nil, errProfileTimeout
			}
			return nil, errProfileUnavailable
		}
		service.name = name
		return nil, nil
	case getProfileCommand:
		if len(args) != 0 {
			return nil, runtime.ErrInvalidCommandArguments
		}
		return service.name, nil
	default:
		return nil, runtime.ErrUnknownCommand
	}
}

// OnStop 不关闭借用的 HTTP Client；创建它的应用负责 Transport 生命周期。
func (service *profileService) OnStop(_ context.Context) error {
	return nil
}

func (service *profileService) fetchProfile() (string, error) {
	request, err := http.NewRequest(http.MethodGet, service.profileURL, nil)
	if err != nil {
		return "", fmt.Errorf("build profile request: %w", err)
	}
	response, err := service.client.Do(request)
	if err != nil {
		return "", fmt.Errorf("fetch profile: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("fetch profile: HTTP status %d", response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxProfileBodyBytes+1))
	if err != nil {
		return "", fmt.Errorf("read profile response: %w", err)
	}
	if len(body) > maxProfileBodyBytes {
		return "", errors.New("profile response is too large")
	}
	var profile struct {
		DisplayName string `json:"display_name"`
	}
	if err := json.Unmarshal(body, &profile); err != nil {
		return "", fmt.Errorf("decode profile response: %w", err)
	}
	if profile.DisplayName == "" {
		return "", errors.New("profile response has no display name")
	}
	return profile.DisplayName, nil
}
