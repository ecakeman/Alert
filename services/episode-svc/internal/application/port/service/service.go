package service

import (
	"context"
)

// Service 是 episode 域对 registry 的最小只读视图（只做存在性与展示名校验）。
type Service struct {
	ServiceID  string
	ServiceKey string
	Name       string
	Enabled    bool
}

type SourceState struct {
	ServiceID  string
	SourceType string
	Status     string
	Reason     string
}

// Reader 是 episode 域对 registry 的只读端口；调用方必须在事务外使用。
type Reader interface {
	Get(ctx context.Context, serviceID string) (Service, error)
	ListSourceStates(ctx context.Context, serviceID string) ([]SourceState, error)
}