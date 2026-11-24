package artifacts

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/ecakeman/alert/services/system-core/internal/application/apperr"
	portdispatch "github.com/ecakeman/alert/services/system-core/internal/application/port/dispatch"
	porttask "github.com/ecakeman/alert/services/system-core/internal/application/port/task"
)

// fixtureRoot 是仓库内固定夹具根（常量，不是配置项）。
const fixtureRoot = "testdata/artifacts"

// Reader 是 B8 前的受限正文读取：只读 file: 夹具，其余一律不存在。
type Reader struct{ root string }

var _ portdispatch.ArtifactContentReader = (*Reader)(nil)

func NewReader() *Reader { return &Reader{root: fixtureRoot} }

func (r *Reader) ReadArtifact(_ context.Context, ref porttask.ArtifactRef) (portdispatch.ArtifactContent, error) {
	if !strings.HasPrefix(ref.StorageKey, "file:") {
		return portdispatch.ArtifactContent{}, apperr.ArtifactNotFound("artifact content is unavailable")
	}
	relative := strings.TrimPrefix(ref.StorageKey, "file:")
	relative = filepath.Clean(relative)
	if relative == "." || relative == ".." || filepath.IsAbs(relative) ||
		strings.HasPrefix(relative, ".."+string(os.PathSeparator)) {
		return portdispatch.ArtifactContent{}, apperr.ArtifactNotFound("artifact content is unavailable")
	}

	root, err := filepath.Abs(r.root)
	if err != nil {
		return portdispatch.ArtifactContent{}, apperr.ArtifactNotFound("artifact content is unavailable")
	}
	full, err := filepath.Abs(filepath.Join(root, relative))
	if err != nil || (full != root && !strings.HasPrefix(full, root+string(os.PathSeparator))) {
		return portdispatch.ArtifactContent{}, apperr.ArtifactNotFound("artifact content is unavailable")
	}
	data, err := os.ReadFile(full)
	if err != nil {
		return portdispatch.ArtifactContent{}, apperr.ArtifactNotFound("artifact content is unavailable")
	}

	contentType := "text/plain"
	if strings.EqualFold(filepath.Ext(full), ".json") {
		contentType = "application/json"
	}
	log.Printf("artifact fixture read storage_key=%s", ref.StorageKey)
	return portdispatch.ArtifactContent{Text: data, ContentType: contentType}, nil
}
