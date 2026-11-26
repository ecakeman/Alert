package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/ecakeman/alert/services/contracts"
	"github.com/ecakeman/alert/services/platform/db"

	"github.com/ecakeman/alert/services/task-orchestrator/internal/application/task"
	"github.com/ecakeman/alert/services/task-orchestrator/internal/config"
	"github.com/ecakeman/alert/services/task-orchestrator/internal/infrastructure/clock"
	"github.com/ecakeman/alert/services/task-orchestrator/internal/infrastructure/idgen"
	taskpostgres "github.com/ecakeman/alert/services/task-orchestrator/internal/infrastructure/persistence/postgres"
)

func main() {
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		log.Printf("config: %v", err)
		os.Exit(1)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := db.New(ctx, cfg.DBConfig())
	if err != nil {
		log.Printf("db: %v", err)
		os.Exit(1)
	}
	defer pool.Close()

	svc := task.New(task.Deps{
		UOW:         taskpostgres.NewUnitOfWork(pool.Raw()),
		Tasks:       taskpostgres.NewTaskRepo(),
		Ownership:   taskpostgres.NewOwnershipRepo(),
		Artifacts:   taskpostgres.NewArtifactRepo(),
		Outbox:      taskpostgres.NewOutboxRepo(),
		Idem:        taskpostgres.NewIdempotencyRepo(),
		Clock:       clock.System{},
		IDs:         idgen.Generator{},
		Lease:       30 * time.Second,
		MaxAttempts: 3,
		RetryDelay:  time.Second,
		Deadline:    2 * time.Minute,
	})

	snap := contracts.DispatchSnapshot{
		EpisodeID: os.Getenv("VERIFY_EPISODE_ID"), ServiceID: os.Getenv("VERIFY_SERVICE_ID"),
		EpisodeStatus: "ACTIVE", InputVersionFrom: 1, InputVersionTo: 1,
		MonitorWatermark: 1, AppEventWatermark: 1, FinalInputVersion: 1,
		ProcessedInputVersion: 0, Reason: "INPUT_APPENDED",
	}

	v1, r1, e1 := svc.Create(ctx, task.CreateInput{Snapshot: snap, TaskType: task.TaskTypeDiagnosis, Source: task.SourceInputAppended})
	fmt.Printf("create1 reused=%v err=%v task=%s status=%s from=%d to=%d\n", r1, e1, v1.TaskID, v1.Status, v1.InputVersionFrom, v1.InputVersionTo)

	v2, r2, e2 := svc.Create(ctx, task.CreateInput{Snapshot: snap, TaskType: task.TaskTypeDiagnosis, Source: task.SourceInputAppended})
	fmt.Printf("create2_same_snapshot reused=%v err=%v task=%q (静默幂等)\n", r2, e2, v2.TaskID)

	other := snap
	other.InputVersionTo = 2
	v3, r3, e3 := svc.Create(ctx, task.CreateInput{Snapshot: other, TaskType: task.TaskTypeDiagnosis, Source: task.SourceInputAppended})
	fmt.Printf("create3_new_version reused=%v err=%v same_task=%v\n", r3, e3, v3.TaskID == v1.TaskID)

	archiveSnap := snap
	archiveSnap.Reason = "EPISODE_RESOLVED"
	_, r4, e4 := svc.Create(ctx, task.CreateInput{Snapshot: archiveSnap, TaskType: task.TaskTypeArchive, Source: task.SourceEpisodeResolved})
	fmt.Printf("create4_other_type reused=%v err=%v\n", r4, e4)

	ok := e1 == nil && v1.Status == "PENDING" && v1.InputVersionFrom == 1 && v1.InputVersionTo == 1 &&
		e2 == nil && v2.TaskID == "" &&
		e3 == nil && r3 && v3.TaskID == v1.TaskID &&
		e4 != nil
	if !ok {
		fmt.Println("VERIFY_FAIL")
		os.Exit(1)
	}
	fmt.Println("VERIFY_PASS")
}