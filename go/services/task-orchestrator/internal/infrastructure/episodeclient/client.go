package episodeclient

import (
	"context"
	"errors"
	"time"

	"github.com/ecakeman/alert/services/platform/apierr"
	"github.com/ecakeman/alert/services/platform/httpx"

	"github.com/ecakeman/alert/services/contracts"
	portepisode "github.com/ecakeman/alert/services/task-orchestrator/internal/application/port/episode"
	"github.com/ecakeman/alert/services/task-orchestrator/internal/application/apperr"
)

// Client 是 episode 域的只读客户端（P5.3：对账快照 + 任务前置校验）。
type Client struct {
	http *httpx.Client
}

func New(baseURL string, timeout time.Duration) *Client {
	return &Client{http: httpx.NewClient(baseURL, timeout)}
}

// ListPendingDispatch 拉取待派发快照（低频对账）。
func (c *Client) ListPendingDispatch(ctx context.Context, limit int) ([]contracts.DispatchSnapshot, error) {
	var out []contracts.DispatchSnapshot
	if err := c.http.GetJSON(ctx, "/internal/episodes/pending-dispatch?limit="+itoa(limit), &out); err != nil {
		return nil, mapErr(err)
	}
	return out, nil
}

// GetEpisode 读取 episode 关键事实（retry 前置校验用）。
func (c *Client) GetEpisode(ctx context.Context, episodeID string) (portepisode.Episode, error) {
	var dto struct {
		EpisodeID    string `json:"episode_id"`
		ServiceID    string `json:"service_id"`
		Status       string `json:"status"`
		InputVersion int64  `json:"input_version"`
	}
	if err := c.http.GetJSON(ctx, "/internal/episodes/"+episodeID, &dto); err != nil {
		return portepisode.Episode{}, mapErr(err)
	}
	return portepisode.Episode{
		EpisodeID: dto.EpisodeID, ServiceID: dto.ServiceID, Status: dto.Status, InputVersion: dto.InputVersion,
	}, nil
}

// GetStatus 读取 episode 状态（retry 前置校验用）。
func (c *Client) GetStatus(ctx context.Context, episodeID string) (string, error) {
	var dto struct {
		Status string `json:"status"`
	}
	if err := c.http.GetJSON(ctx, "/internal/episodes/"+episodeID, &dto); err != nil {
		return "", mapErr(err)
	}
	return dto.Status, nil
}

func itoa(n int) string {
	if n <= 0 {
		n = 64
	}
	digits := ""
	for n > 0 {
		digits = string(rune('0'+n%10)) + digits
		n /= 10
	}
	return digits
}

func mapErr(err error) error {
	if err == nil {
		return nil
	}
	var ae *apierr.Error
	if errors.As(err, &ae) {
		switch ae.Code {
		case apierr.CodeNotFound:
			return apperr.NotFound(ae.Message)
		case apierr.CodeValidation:
			return apperr.Validation(ae.Message)
		case apierr.CodeDependencyUnavailable:
			return apperr.DependencyUnavailable(ae.Message)
		}
		return apperr.New(ae.Code, ae.Message)
	}
	return apperr.DependencyUnavailable("episode unavailable")
}