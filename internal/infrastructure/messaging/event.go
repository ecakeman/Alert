package messaging

import (
	"encoding/json"

	"github.com/ecakeman/alert/internal/contracts"
)

// Encode 把契约事件编码为线上 JSON（唯一信封）。
func Encode(e contracts.Event) ([]byte, error) { return json.Marshal(e) }

// Decode 解析线上 JSON 为契约事件（版本校验语义由 S1.3 收口）。
func Decode(body []byte) (contracts.Event, error) {
	var e contracts.Event
	if err := json.Unmarshal(body, &e); err != nil {
		return contracts.Event{}, err
	}
	if err := e.Validate(); err != nil {
		return contracts.Event{}, err
	}
	return e, nil
}
