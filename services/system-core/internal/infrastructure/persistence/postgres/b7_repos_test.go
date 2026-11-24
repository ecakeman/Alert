package postgres

import (
	"context"
	"fmt"
	"testing"
	"time"

	portepisode "github.com/ecakeman/alert/services/system-core/internal/application/port/episode"
	portshared "github.com/ecakeman/alert/services/system-core/internal/application/port/shared"
	porttask "github.com/ecakeman/alert/services/system-core/internal/application/port/task"
)

func b7IDs() (serviceID, episodeID, taskID string) {
	suffix := fmt.Sprintf("%012d", time.Now().UnixNano()%1_000_000_000_000)
	return "11111111-1111-1111-1111-" + suffix,
		"22222222-2222-2222-2222-" + suffix,
		"33333333-3333-3333-3333-" + suffix
}

func TestB7TaskOwnershipArtifactQueries(t *testing.T) {
	pool := intPool(t)
	uow := NewUnitOfWork(pool)
	ctx := context.Background()
	serviceID, episodeID, taskID := b7IDs()
	now := time.Now().UTC()

	err := uow.WithinTx(ctx, func(ctx context.Context, tx portshared.Tx) error {
		if err := NewServiceRepo().Create(ctx, tx, sampleService(serviceID, "b7-"+serviceID)); err != nil {
			return err
		}
		episode := portepisode.Episode{
			EpisodeID: episodeID, ServiceID: serviceID, Status: "ACTIVE", StartedBy: "MONITOR",
			InputVersion: 1, ProcessedInputVersion: 0, StartedAt: now,
		}
		if err := NewEpisodeRepo().Create(ctx, tx, episode); err != nil {
			return err
		}
		return NewTaskRepo().Create(ctx, tx, porttask.AgentTask{
			TaskID: taskID, EpisodeID: episodeID, ServiceID: serviceID, TaskType: "DIAGNOSIS",
			Status: "PENDING", Generation: 1, MaxAttempts: 3,
			InputVersionFrom: 1, InputVersionTo: 1,
			DeadlineAt: now.Add(time.Hour), CreatedAt: now, UpdatedAt: now,
		})
	})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	err = uow.WithinTx(ctx, func(ctx context.Context, tx portshared.Tx) error {
		repo := NewTaskRepo()
		task, err := repo.GetForUpdate(ctx, tx, taskID)
		if err != nil {
			return err
		}
		if task.TaskID != taskID {
			return fmt.Errorf("GetForUpdate returned %s", task.TaskID)
		}
		lease := now.Add(time.Minute)
		dispatched := "DISPATCHED"
		updated, err := repo.UpdateCAS(ctx, tx, taskID, 1, []string{"PENDING"},
			porttask.TaskPatch{Status: &dispatched, LeaseUntil: &lease})
		if err != nil || updated != 1 {
			return fmt.Errorf("cas to dispatched: n=%d err=%v", updated, err)
		}
		stale, err := repo.ListStaleDispatched(ctx, tx, now.Add(time.Hour), 10)
		if err != nil {
			return err
		}
		found := false
		for _, item := range stale {
			if item.TaskID == taskID {
				found = true
			}
		}
		if !found {
			return fmt.Errorf("stale dispatched task not listed")
		}
		if _, err := repo.ListRunningLeaseExpired(ctx, tx, now.Add(time.Hour), 10); err != nil {
			return err
		}
		list, err := repo.ListByEpisode(ctx, tx, episodeID)
		if err != nil || len(list) == 0 {
			return fmt.Errorf("list by episode: n=%d err=%v", len(list), err)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("task queries: %v", err)
	}

	err = uow.WithinTx(ctx, func(ctx context.Context, tx portshared.Tx) error {
		repo := NewTaskRepo()
		retryWait := "RETRY_WAIT"
		nextRetry := now.Add(-time.Minute)
		updated, err := repo.UpdateCAS(ctx, tx, taskID, 1, []string{"DISPATCHED"},
			porttask.TaskPatch{Status: &retryWait, NextRetryAt: &nextRetry})
		if err != nil || updated != 1 {
			return fmt.Errorf("cas to retry wait: n=%d err=%v", updated, err)
		}
		updated, err = repo.PromoteRetry(ctx, tx, taskID, 1)
		if err != nil || updated != 1 {
			return fmt.Errorf("promote retry: n=%d err=%v", updated, err)
		}
		task, err := repo.Get(ctx, tx, taskID)
		if err != nil {
			return err
		}
		if task.Generation != 2 || task.Status != "DISPATCHED" {
			return fmt.Errorf("promote result: generation=%d status=%s", task.Generation, task.Status)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("promote retry: %v", err)
	}

	err = uow.WithinTx(ctx, func(ctx context.Context, tx portshared.Tx) error {
		repo := NewArtifactRepo()
		ref := porttask.ArtifactRef{
			ArtifactRefID: "44444444-4444-4444-4444-" + taskID[24:],
			EpisodeID:     episodeID, TaskID: taskID, Generation: 1,
			ArtifactType: "INVESTIGATION", ArtifactVersion: 1,
			ContentHash: "h1", ContentType: "text/plain", StorageKey: "file:investigation.txt",
			SourceRefs: []byte("[]"), RegisteredAt: now, UpdatedAt: now,
		}
		if err := repo.Insert(ctx, tx, ref); err != nil {
			return err
		}
		got, err := repo.GetByTypeVersion(ctx, tx, episodeID, "INVESTIGATION", 1)
		if err != nil {
			return err
		}
		if got.ContentHash != "h1" {
			return fmt.Errorf("artifact hash mismatch: %s", got.ContentHash)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("artifact queries: %v", err)
	}

	err = uow.WithinTx(ctx, func(ctx context.Context, tx portshared.Tx) error {
		repo := episodeRepo{}
		updated, err := repo.AdvanceProcessedVersion(ctx, tx, episodeID, 1)
		if err != nil || updated != 1 {
			return fmt.Errorf("advance: n=%d err=%v", updated, err)
		}
		updated, err = repo.AdvanceProcessedVersion(ctx, tx, episodeID, 1)
		if err != nil || updated != 0 {
			return fmt.Errorf("advance repeat: n=%d err=%v", updated, err)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("advance processed version: %v", err)
	}
}
