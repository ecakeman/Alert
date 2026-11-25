package httpx

import (
	"context"
	"net/http"
	"strings"
	"time"
)

// Run 执行镜像内自检：请求 http://<addr>/readyz，2s 超时，200 → 0，其余 → 1。
// 供各服务 main 以 -healthcheck 子命令调用，镜像内无需 curl/wget。
func Run(addr string) int {
	if !strings.Contains(addr, "://") {
		addr = "http://" + addr
	}
	url := strings.TrimRight(addr, "/") + "/readyz"
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 1
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 1
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		return 0
	}
	return 1
}
