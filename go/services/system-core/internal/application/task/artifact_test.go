package task

import (
	"context"
	"testing"
	"time"

	"github.com/ecakeman/alert/services/system-core/internal/application/apperr"
	portdispatch "github.com/ecakeman/alert/services/system-core/internal/application/port/dispatch"
	portepisode "github.com/ecakeman/alert/services/system-core/internal/application/port/episode"
	portshared "github.com/ecakeman/alert/services/system-core/internal/application/port/shared"
	porttask "github.com/ecakeman/alert/services/system-core/internal/application/port/task"
	"github.com/ecakeman/alert/services/system-core/internal/domain"
)

type artifactStore struct {
	porttask.ArtifactRepo
	refs    []porttask.ArtifactRef
	inserts int
}

func (s *artifactStore) GetLatest(_ context.Context, _ portshared.Tx, episodeID, artifactType string) (porttask.ArtifactRef, error) {
	found := false
	var best porttask.ArtifactRef
	for _, ref := range s.refs {
		if ref.EpisodeID != episodeID || ref.ArtifactType != artifactType {
			continue
		}
		if !found || ref.ArtifactVersion > best.ArtifactVersion {
			best = ref
			found = true
		}
	}
	if !found {
		return porttask.ArtifactRef{}, apperr.NotFound("artifact not found")
	}
	return best, nil
}

func (s *artifactStore) GetByTypeVersion(_ context.Context, _ portshared.Tx, episodeID, artifactType string, version int) (porttask.ArtifactRef, error) {
	for _, ref := range s.refs {
		if ref.EpisodeID == episodeID && ref.ArtifactType == artifactType && ref.ArtifactVersion == version {
			return ref, nil
		}
	}
	return porttask.ArtifactRef{}, apperr.NotFound("artifact not found")
}

func (s *artifactStore) Insert(_ context.Context, _ portshared.Tx, ref porttask.ArtifactRef) error {
	for _, existing := range s.refs {
		if existing.EpisodeID == ref.EpisodeID && existing.ArtifactType == ref.ArtifactType && existing.ArtifactVersion == ref.ArtifactVersion {
			return apperr.AlreadyExists("artifact already exists")
		}
	}
	s.refs = append(s.refs, ref)
	s.inserts++
	return nil
}

type stubReader struct {
	text    string
	missing bool
}

func (r *stubReader) ReadArtifact(context.Context, porttask.ArtifactRef) (portdispatch.ArtifactContent, error) {
	if r.missing {
		return portdispatch.ArtifactContent{}, apperr.ArtifactNotFound("missing")
	}
	return portdispatch.ArtifactContent{Text: []byte(r.text), ContentType: "text/plain"}, nil
}

func TestArtifactRegisterIdempotencyConflictAndMissing(t *testing.T) {
	now := time.Date(2026, 11, 24, 10, 0, 0, 0, time.UTC)
	leaseUntil := now.Add(2 * time.Minute)
	task := porttask.AgentTask{
		TaskID: "t1", EpisodeID: "e1", ServiceID: "s1", TaskType: "DIAGNOSIS",
		Status: "RUNNING", Generation: 1, LeaseUntil: &leaseUntil,
	}
	tasks := &stubTasks{byID: map[string]porttask.AgentTask{"t1": task}}
	store := &artifactStore{}
	svc := &Service{deps: Deps{
		UOW: fakeUOW{}, Tasks: tasks, Artifacts: store,
		Episodes: &createEpisodes{episode: portepisode.Episode{EpisodeID: "e1", ServiceID: "s1"}},
		Clock:    fixedClock{now}, Lease: 30 * time.Second, IDs: &createIDs{},
	}}
	id := identityFor(task, leaseUntil)
	input := RegisterArtifactInput{
		Identity: id, ArtifactType: "INVESTIGATION", ArtifactVersion: 1,
		ContentHash: "h1", ContentType: "text/plain", StorageKey: "file:investigation.txt",
	}

	if err := svc.RegisterArtifact(context.Background(), input); err != nil {
		t.Fatalf("register should pass: %v", err)
	}
	if store.inserts != 1 {
		t.Fatalf("first register should insert once, got %d", store.inserts)
	}

	if err := svc.RegisterArtifact(context.Background(), input); err != nil {
		t.Fatalf("same version same hash should be idempotent: %v", err)
	}
	if store.inserts != 1 {
		t.Fatalf("idempotent register must not insert again, got %d", store.inserts)
	}

	conflict := input
	conflict.ContentHash = "h2"
	if code := apperr.Code(svc.RegisterArtifact(context.Background(), conflict)); code != domain.CodeHashMismatch {
		t.Fatalf("same version different hash should be HASH_MISMATCH, got %s", code)
	}

	if _, err := svc.ReadArtifact(context.Background(), "e1", "DIAGNOSIS_REPORT"); apperr.Code(err) != apperr.CodeArtifactNotFound {
		t.Fatalf("missing artifact should be ARTIFACT_NOT_FOUND, got %v", err)
	}

	svc.deps.Reader = &stubReader{text: "fixture-body"}
	view, err := svc.ReadArtifact(context.Background(), "e1", "INVESTIGATION")
	if err != nil || string(view.Content) != "fixture-body" {
		t.Fatalf("artifact content mismatch: view=%+v err=%v", view, err)
	}
}
