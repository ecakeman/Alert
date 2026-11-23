package infrastructure

import "time"

// RealClock 提供 UTC 时间。
type RealClock struct{}

func (RealClock) Now() time.Time { return time.Now().UTC() }
