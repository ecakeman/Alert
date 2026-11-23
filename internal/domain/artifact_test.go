package domain

import "testing"

func fullArtifacts(inc string) []ArtifactStatus {
	return []ArtifactStatus{
		{Type: ArtifactInvestigation, Version: 1, ContentHash: "h1", RegisteredHash: "h1", IncidentID: inc},
		{Type: ArtifactDiagnosisReport, Version: 1, ContentHash: "h2", RegisteredHash: "h2", IncidentID: inc},
	}
}

// INV-10：先解除后归档；ACTIVE 直归档拒绝；缺产物/hash 不符拒绝。
func TestINV10_CanArchive(t *testing.T) {
	if ok, code := CanArchive(IncidentActive, "inc-1", fullArtifacts("inc-1")); ok || code != CodeNotResolved {
		t.Fatalf("ACTIVE archive must reject: ok=%v code=%s", ok, code)
	}
	if ok, code := CanArchive(IncidentResolved, "inc-1", fullArtifacts("inc-1")); !ok {
		t.Fatalf("RESOLVED with both artifacts should allow, code=%s", code)
	}

	one := []ArtifactStatus{
		{Type: ArtifactInvestigation, Version: 1, ContentHash: "h1", RegisteredHash: "h1", IncidentID: "inc-1"},
	}
	if ok, code := CanArchive(IncidentResolved, "inc-1", one); ok || code != CodeArtifactMissing {
		t.Fatalf("missing artifact should reject: ok=%v code=%s", ok, code)
	}

	bad := fullArtifacts("inc-1")
	bad[0].RegisteredHash = "other"
	if ok, code := CanArchive(IncidentResolved, "inc-1", bad); ok || code != CodeHashMismatch {
		t.Fatalf("hash mismatch should reject: ok=%v code=%s", ok, code)
	}
}

// INV-11：产物必须可追溯，版本单调、同版本 hash 一致、旧代次不覆盖。
func TestINV11_ArtifactRef(t *testing.T) {
	ref := ArtifactRef{
		Type: ArtifactInvestigation, Version: 2, ContentHash: "h",
		TaskID: "t1", Generation: 2, IncidentID: "inc-1",
	}
	if err := ValidateArtifactRef(ref, 1, "old", 2); err != nil {
		t.Fatalf("valid ref should pass, got %v", err)
	}

	missing := ref
	missing.TaskID = ""
	if err := ValidateArtifactRef(missing, 1, "", 2); !IsCode(err, CodeArtifactMissing) {
		t.Fatalf("missing trace field should be ARTIFACT_MISSING, got %v", err)
	}

	unknown := ref
	unknown.Type = "FOO"
	if err := ValidateArtifactRef(unknown, 1, "", 2); !IsCode(err, CodeUnknownArtifact) {
		t.Fatalf("unknown type should be UNKNOWN_ARTIFACT_TYPE, got %v", err)
	}

	if err := ValidateArtifactRef(ref, 3, "", 2); !IsCode(err, CodeStateConflict) {
		t.Fatalf("version backwards should be STATE_CONFLICT, got %v", err)
	}

	if err := ValidateArtifactRef(ref, 2, "different", 2); !IsCode(err, CodeHashMismatch) {
		t.Fatalf("same version different hash should be HASH_MISMATCH, got %v", err)
	}

	old := ref
	old.Generation = 1
	if err := ValidateArtifactRef(old, 1, "", 2); !IsCode(err, CodeStaleGeneration) {
		t.Fatalf("stale generation should be STALE_GENERATION, got %v", err)
	}
}