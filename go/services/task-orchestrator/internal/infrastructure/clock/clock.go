package clock

import "time"

// System 返回 UTC 当前时间。
type System struct{}

func (System) Now() time.Time { return time.Now().UTC() }