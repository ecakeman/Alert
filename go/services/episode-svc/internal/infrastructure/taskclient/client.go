package taskclient

import (
	"context"
	"errors"
	"time"

	"github.com/ecakeman/alert/services/platform/apierr"
	"github.com/ecakeman/alert/services/platform/httpx"

	"github.com/ecakeman/alert/services/episode-svc/internal/application/apperr"
	porttask "github.com/ecakeman/alert/services/episode-svc/internal/application/port/task"
)

// Client 是 task 域的域间 HTTP 客户端（P4 过渡期指向单体 /internal 端点）。
type Client struct {
	http *httpx.Client
}

func New(baseURL string, timeout time.Duration) *Client {
	return &Client{http: httpx.NewClient(baseURL, timeout)}
}

type activeTaskDTO struct {
	TaskID     string `json:"task_id"`
	EpisodeID  string `json:"episode_id"`
	ServiceID  string `json:"service_id"`
	TaskType   string `json:"task_type"`
	Status     string `json:"status"`
	Generation int64  `json:"generation"`
}

// GetActive 返回活动任务；404 视为“无活动任务”(nil, nil)。
func (c *Client) GetActive(ctx context.Context, episodeID string) (*porttask.ActiveTask, error) {
	var dto activeTaskDTO
	if err := c.http.GetJSON(ctx, "/internal/episodes/"+episodeID+"/active-task", &dto); err != nil {
		var ae *apierr.Error
		if errors.As(err, &ae) && ae.Code == apierr.CodeNotFound {
			return nil, nil
		}
		return nil, mapErr(err)
	}
	return &porttask.ActiveTask{
		TaskID: dto.TaskID, EpisodeID: dto.EpisodeID, ServiceID: dto.ServiceID,
		TaskType: dto.TaskType, Status: dto.Status, Generation: dto.Generation,
	}, nil
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
	return apperr.DependencyUnavailable("task unavailable")
}