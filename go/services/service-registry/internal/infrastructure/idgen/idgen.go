package idgen

import (
	"crypto/rand"
	"fmt"
)

// Generator 生成 UUIDv4 形状的 ID（数据库 uuid 列直接接受）。
type Generator struct{}

func (Generator) NewID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return ""
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
