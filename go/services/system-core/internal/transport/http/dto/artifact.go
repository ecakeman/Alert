package dto

// ArtifactList 是路由 16 不带 type 时的元数据列表壳。
type ArtifactList struct {
	Artifacts []ArtifactRef `json:"artifacts"`
}
