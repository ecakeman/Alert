package task

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/ecakeman/alert/services/system-core/internal/application/apperr"
	porttask "github.com/ecakeman/alert/services/system-core/internal/application/port/task"
	"github.com/ecakeman/alert/services/system-core/internal/domain"
)

func TestFenceRejectsFourMismatchesForAllActions(t *testing.T) {
	leaseUntil := time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)
	now := leaseUntil.Add(-time.Minute)
	base := porttask.AgentTask{
		TaskID: "t1", EpisodeID: "e1", TaskType: "DIAGNOSIS",
		Generation: 2, Status: "RUNNING", LeaseUntil: &leaseUntil,
	}
	valid := TaskIdentity{
		TaskID: "t1", EpisodeID: "e1", TaskType: "DIAGNOSIS",
		Generation: 2, LeaseToken: encodeLeaseToken("t1", 2, leaseUntil.Unix()),
	}

	actions := []TaskAction{ActionStarted, ActionHeartbeat, ActionRegisterArtifact, ActionReportResult}
	statusFor := map[TaskAction]string{
		ActionStarted:          "DISPATCHED",
		ActionHeartbeat:        "RUNNING",
		ActionRegisterArtifact: "RUNNING",
		ActionReportResult:     "RUNNING",
	}
	mutations := []struct {
		name   string
		mutate func(*porttask.AgentTask, *TaskIdentity)
		want   string
	}{
		{"episode", func(_ *porttask.AgentTask, id *TaskIdentity) { id.EpisodeID = "e2" }, domain.CodeStateConflict},
		{"type", func(_ *porttask.AgentTask, id *TaskIdentity) { id.TaskType = "ARCHIVE" }, domain.CodeTypeMismatch},
		{"generation", func(_ *porttask.AgentTask, id *TaskIdentity) { id.Generation = 1 }, domain.CodeStaleGeneration},
		{"lease", func(_ *porttask.AgentTask, id *TaskIdentity) {
			id.LeaseToken = encodeLeaseToken("t1", 2, leaseUntil.Unix()+3600)
		}, domain.CodeLeaseLost},
	}

	svc := &Service{}
	for _, act := range actions {
		for _, mut := range mutations {
			t.Run(string(act)+"/"+mut.name, func(t *testing.T) {
				task := base
				task.Status = statusFor[act]
				id := valid
				mut.mutate(&task, &id)
				snapshot := task

				err := svc.fence(task, id, act, now)
				if err == nil {
					t.Fatal("expected rejection")
				}
				var ae *apperr.Error
				if !errors.As(err, &ae) || ae.Code != mut.want {
					t.Fatalf("want code %s, got %v", mut.want, err)
				}
				if !reflect.DeepEqual(task, snapshot) {
					t.Fatal("rejected fence must not mutate the task")
				}
			})
		}
	}
}
