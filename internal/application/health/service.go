package health

import "context"

// CheckResult 是单个就绪检查结果。
type CheckResult struct {
	Name   string `json:"name"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail,omitempty"`
}

// Status 是健康/就绪聚合结果。
type Status struct {
	Status string        `json:"status"`
	Checks []CheckResult `json:"checks"`
}

// Service 聚合多个 Checker，供 HTTP transport 调用。
type Service struct {
	checkers []Checker
}

func NewService(checkers ...Checker) *Service {
	return &Service{checkers: checkers}
}

// Live 表示进程存活，恒为 ok。
func (s *Service) Live() Status {
	return Status{Status: "ok", Checks: []CheckResult{}}
}

// Ready 执行全部 checker；全通过为 ready，任一失败为 not_ready。
func (s *Service) Ready(ctx context.Context) Status {
	results := make([]CheckResult, 0, len(s.checkers))
	failing := false
	for _, c := range s.checkers {
		cr := CheckResult{Name: c.Name(), OK: true}
		if err := c.Check(ctx); err != nil {
			cr.OK = false
			cr.Detail = err.Error()
			failing = true
		}
		results = append(results, cr)
	}
	status := "ready"
	if failing {
		status = "not_ready"
	}
	return Status{Status: status, Checks: results}
}