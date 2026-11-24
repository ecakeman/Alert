package artifacts

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/ecakeman/alert/services/system-core/internal/application/apperr"
	porttask "github.com/ecakeman/alert/services/system-core/internal/application/port/task"
)

func TestReaderFixtureMissingAndTraversal(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "investigation.txt"), []byte("fixture-body"), 0o600); err != nil {
		t.Fatal(err)
	}
	reader := &Reader{root: root}

	content, err := reader.ReadArtifact(context.Background(), porttask.ArtifactRef{StorageKey: "file:investigation.txt"})
	if err != nil || string(content.Text) != "fixture-body" {
		t.Fatalf("fixture should be readable: content=%q err=%v", content.Text, err)
	}

	if _, err := reader.ReadArtifact(context.Background(), porttask.ArtifactRef{StorageKey: "file:missing.txt"}); apperr.Code(err) != apperr.CodeArtifactNotFound {
		t.Fatalf("missing fixture should be ARTIFACT_NOT_FOUND, got %v", err)
	}

	if _, err := reader.ReadArtifact(context.Background(), porttask.ArtifactRef{StorageKey: "file:../escape.txt"}); apperr.Code(err) != apperr.CodeArtifactNotFound {
		t.Fatalf("path traversal should be ARTIFACT_NOT_FOUND, got %v", err)
	}

	if _, err := reader.ReadArtifact(context.Background(), porttask.ArtifactRef{StorageKey: "cognitive://report"}); apperr.Code(err) != apperr.CodeArtifactNotFound {
		t.Fatalf("unsupported scheme should be ARTIFACT_NOT_FOUND, got %v", err)
	}
}
