package health

import "context"

// Checker 是就绪检查端口，由 infrastructure 实现；本阶段仅定义端口。
type Checker interface {
	Name() string
	Check(context.Context) error
}