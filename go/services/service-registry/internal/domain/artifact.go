package domain

// ArtifactType 是正式诊断产物类型（与 DB 约束一致）。
type ArtifactType string

const (
	ArtifactInvestigation   ArtifactType = "INVESTIGATION"
	ArtifactDiagnosisReport ArtifactType = "DIAGNOSIS_REPORT"
)

// Valid 判断是否为正式产物类型；未知类型不得默认映射。
func (t ArtifactType) Valid() bool {
	return t == ArtifactInvestigation || t == ArtifactDiagnosisReport
}

// ArtifactRef 是产物登记引用，需可追溯到 task / generation / incident / 来源。
type ArtifactRef struct {
	Type        ArtifactType
	Version     int64
	ContentHash string
	TaskID      string
	Generation  int64
	IncidentID  string
	SourceRefs  []string
}

// ValidateArtifactRef 校验追溯字段、版本单调性、同版本 hash 一致、旧代次不覆盖。
func ValidateArtifactRef(ref ArtifactRef, latestVersion int64, latestHash string, currentGeneration int64) error {
	if !ref.Type.Valid() {
		return newError(CodeUnknownArtifact, "unknown artifact type")
	}
	if ref.TaskID == "" || ref.IncidentID == "" || ref.ContentHash == "" {
		return newError(CodeArtifactMissing, "missing trace field")
	}
	if ref.Version <= 0 {
		return newError(CodeInvalidRange, "artifact version must be > 0")
	}
	if ref.Version < latestVersion {
		return newError(CodeStateConflict, "artifact version must not go backwards")
	}
	if ref.Version == latestVersion && latestHash != "" && latestHash != ref.ContentHash {
		return newError(CodeHashMismatch, "same version with different content hash")
	}
	if currentGeneration > 0 && ref.Generation < currentGeneration {
		return newError(CodeStaleGeneration, "stale generation cannot overwrite artifact")
	}
	return nil
}

// ArtifactStatus 是归档时用于校验的产物快照。
type ArtifactStatus struct {
	Type           ArtifactType
	Version        int64
	ContentHash    string
	RegisteredHash string
	IncidentID     string
}

// CanArchive：必须已 RESOLVED，且两份正式产物存在、hash 与登记一致、归属同一 Incident。
func CanArchive(incidentStatus IncidentStatus, incidentID string, artifacts []ArtifactStatus) (bool, string) {
	if incidentStatus != IncidentResolved {
		return false, CodeNotResolved
	}
	seen := map[ArtifactType]bool{}
	for _, a := range artifacts {
		if a.IncidentID != incidentID || !a.Type.Valid() {
			continue
		}
		if a.Version <= 0 || a.ContentHash == "" || a.ContentHash != a.RegisteredHash {
			return false, CodeHashMismatch
		}
		seen[a.Type] = true
	}
	if !seen[ArtifactInvestigation] || !seen[ArtifactDiagnosisReport] {
		return false, CodeArtifactMissing
	}
	return true, ""
}
