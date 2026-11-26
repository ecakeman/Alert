package httpx

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/ecakeman/alert/services/platform/apierr"
)

// Client 是域间 HTTP/JSON 客户端：一次尝试、统一超时、trace 透传、错误解包。
type Client struct {
	BaseURL    string
	HTTP       *http.Client
	Timeout    time.Duration
	HeaderName string
}

// NewClient 创建客户端；默认 3s 超时并禁止自动重试。
func NewClient(baseURL string, timeout time.Duration) *Client {
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	return &Client{
		BaseURL:    strings.TrimRight(baseURL, "/"),
		HTTP:       &http.Client{Timeout: timeout},
		Timeout:    timeout,
		HeaderName: HeaderTraceID,
	}
}

// GetJSON 发起 GET 并解码响应。
func (c *Client) GetJSON(ctx context.Context, path string, out any) error {
	return c.doJSON(ctx, http.MethodGet, path, nil, out)
}

// PostJSON 发起 POST 并解码响应。
func (c *Client) PostJSON(ctx context.Context, path string, in, out any) error {
	return c.doJSON(ctx, http.MethodPost, path, in, out)
}

// PatchJSON 发起 PATCH 并解码响应。
func (c *Client) PatchJSON(ctx context.Context, path string, in, out any) error {
	return c.doJSON(ctx, http.MethodPatch, path, in, out)
}

// Delete 发起 DELETE；2xx 视为成功。
func (c *Client) Delete(ctx context.Context, path string) error {
	return c.doJSON(ctx, http.MethodDelete, path, nil, nil)
}

func (c *Client) doJSON(ctx context.Context, method, path string, in, out any) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.Timeout)
		defer cancel()
	}

	var body io.Reader
	if in != nil {
		payload, err := json.Marshal(in)
		if err != nil {
			return apierr.Validation("invalid request payload")
		}
		body = bytes.NewReader(payload)
	}

	url := c.BaseURL + "/" + strings.TrimLeft(path, "/")
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return apierr.Validation("invalid request url")
	}
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if traceID := TraceIDFrom(ctx); traceID != "" {
		req.Header.Set(c.HeaderName, traceID)
	}

	resp, err := c.HTTP.Do(req)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return apierr.DependencyUnavailable("upstream timeout")
		}
		return apierr.DependencyUnavailable(fmt.Sprintf("upstream unreachable: %s", req.URL.Host))
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return decodeError(resp)
	}
	if out == nil || resp.StatusCode == http.StatusNoContent {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return apierr.DependencyUnavailable("upstream response invalid")
	}
	return nil
}

func decodeError(resp *http.Response) error {
	var body apierr.Error
	if err := json.NewDecoder(resp.Body).Decode(&body); err == nil && body.Code != "" {
		return &body
	}
	if resp.StatusCode == http.StatusServiceUnavailable {
		return apierr.DependencyUnavailable("upstream unavailable")
	}
	return apierr.New("UPSTREAM_ERROR", fmt.Sprintf("upstream status %d", resp.StatusCode))
}
