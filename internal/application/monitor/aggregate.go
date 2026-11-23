package monitor

// nextStatus 按阈值聚合：失败达 failure 阈值 → ABNORMAL；成功达 recovery 阈值 → NORMAL；
// 未达阈值保持原状态（UNKNOWN 不会因单次成功变成 NORMAL）。
func nextStatus(current, checkStatus string, failures, successes, failureThreshold, recoveryThreshold int) (string, int, int) {
	if failureThreshold <= 0 {
		failureThreshold = 3
	}
	if recoveryThreshold <= 0 {
		recoveryThreshold = 2
	}
	if checkStatus == "SUCCESS" {
		successes++
		failures = 0
		if successes >= recoveryThreshold {
			return "NORMAL", failures, successes
		}
		return current, failures, successes
	}
	failures++
	successes = 0
	if failures >= failureThreshold {
		return "ABNORMAL", failures, successes
	}
	return current, failures, successes
}
