package postgres

import "time"

// utc 统一时间归一化为 UTC（timestamptz 读写）。
func utc(t time.Time) time.Time { return t.UTC() }

func utcPtr(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	u := t.UTC()
	return &u
}