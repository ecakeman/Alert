package domain

import "testing"

// INV-02：UNKNOWN 永远不等于 NORMAL；UNKNOWN 组合不构成“可恢复”。
func TestINV02_SourceStatus(t *testing.T) {
	if IsNormal(ParseSourceStatus("UNKNOWN")) {
		t.Fatal("UNKNOWN must not be normal")
	}
	if IsNormal(ParseSourceStatus("")) {
		t.Fatal("empty must be UNKNOWN")
	}
	if IsNormal(ParseSourceStatus("garbage")) {
		t.Fatal("illegal must be UNKNOWN")
	}
	if !IsNormal(ParseSourceStatus("NORMAL")) {
		t.Fatal("NORMAL should be normal")
	}
	if IsNormal(ParseSourceStatus("ABNORMAL")) {
		t.Fatal("ABNORMAL must not be normal")
	}

	// 双来源组合：任一方 UNKNOWN / ABNORMAL 都不算可恢复。
	if IsNormal(ParseSourceStatus("UNKNOWN")) && IsNormal(ParseSourceStatus("NORMAL")) {
		t.Fatal("UNKNOWN+NORMAL must not be recoverable")
	}
	if IsNormal(ParseSourceStatus("ABNORMAL")) && IsNormal(ParseSourceStatus("NORMAL")) {
		t.Fatal("ABNORMAL+NORMAL must not be recoverable")
	}
	if !(IsNormal(ParseSourceStatus("NORMAL")) && IsNormal(ParseSourceStatus("NORMAL"))) {
		t.Fatal("NORMAL+NORMAL should be recoverable")
	}
}