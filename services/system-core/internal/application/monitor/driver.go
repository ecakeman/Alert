package monitor

import "context"

// EpisodeDriver 是 monitor 对“故障开启/复用”与“解除”的最小依赖面；
// 由 bootstrap 注入 episode 用例的转调实现，使 monitor 不依赖 episode 包的具体类型。
type EpisodeDriver interface {
	OpenOrReuse(ctx context.Context, serviceID string) (created bool, err error)
	TryResolve(ctx context.Context, episodeID string) (resolved bool, missing []string, err error)
}
