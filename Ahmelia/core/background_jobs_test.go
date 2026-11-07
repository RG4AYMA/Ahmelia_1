package core

import (
	"context"
	"testing"
	"time"

	"ahmelia/interrupt"
	"ahmelia/production"
)

func waitForBackgroundJob(t *testing.T, e *Engine, id string, want BackgroundJobStatus) BackgroundJob {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		job, ok := e.BackgroundJob(id)
		if ok && job.Status == want {
			return job
		}
		time.Sleep(5 * time.Millisecond)
	}
	job, _ := e.BackgroundJob(id)
	t.Fatalf("job %s status = %q, quería %q: %+v", id, job.Status, want, job)
	return BackgroundJob{}
}

func TestBackgroundJobKeepsProcessResponsive(t *testing.T) {
	e := buildTestEngine(t)
	started := make(chan struct{})
	release := make(chan struct{})
	finished := make(chan struct{})

	id := e.StartBackgroundJob(context.Background(), "slow-task", func(ctx context.Context) (production.ActionResult, error) {
		defer close(finished)
		close(started)
		select {
		case <-release:
			return production.ActionResult{Success: true, Verified: true, OutcomeStatus: production.OutcomeSucceeded}, nil
		case <-ctx.Done():
			return production.ActionResult{}, ctx.Err()
		}
	})
	if id == "" {
		t.Fatal("background job was not created")
	}
	<-started

	done := make(chan struct{})
	go func() {
		e.Process("hola")
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("Process blocked while a background job was running")
	}

	job, ok := e.BackgroundJob(id)
	if !ok || job.Status != BackgroundJobRunning {
		t.Fatalf("job should still be running: %+v", job)
	}
	if !e.CancelBackgroundJob(id) {
		t.Fatal("cancel should succeed for a running job")
	}
	close(release)
	<-finished
	job = waitForBackgroundJob(t, e, id, BackgroundJobCanceled)
	if job.Result.Success || job.Result.Verified {
		t.Fatalf("late result was applied after cancel: %+v", job.Result)
	}
}

func TestBackgroundJobStoresCompletedResult(t *testing.T) {
	e := buildTestEngine(t)
	id := e.StartBackgroundJob(context.Background(), "quick-task", func(context.Context) (production.ActionResult, error) {
		return production.ActionResult{Success: true, Verified: true, OutcomeStatus: production.OutcomeSucceeded}, nil
	})
	if id == "" {
		t.Fatal("background job was not created")
	}
	job := waitForBackgroundJob(t, e, id, BackgroundJobCompleted)
	if !job.Result.Success || !job.Result.Verified {
		t.Fatalf("completed result not stored: %+v", job.Result)
	}
}

func TestCriticalEventWhileBackgroundJobRuns(t *testing.T) {
	e := buildTestEngine(t)
	bus := &fakeNoticeChannel{devices: []NoticeDevice{privateDevice()}}
	e.EnableInterruptionsAt("", bus)

	started := make(chan struct{})
	release := make(chan struct{})
	id := e.StartBackgroundJob(context.Background(), "slow-task", func(ctx context.Context) (production.ActionResult, error) {
		close(started)
		select {
		case <-release:
			return production.ActionResult{Success: true, Verified: true}, nil
		case <-ctx.Done():
			return production.ActionResult{}, ctx.Err()
		}
	})
	<-started
	defer func() {
		_ = e.CancelBackgroundJob(id)
		close(release)
	}()

	now := time.Now()
	event := interrupt.Event{
		ID:          "job-critical",
		Kind:        "test",
		Severity:    interrupt.SeverityCritical,
		Urgency:     5 * time.Minute,
		Consequence: interrupt.ConsequenceIrreversible,
		BlastRadius: interrupt.BlastProject,
		Privacy:     interrupt.PrivacyPrivate,
		Source:      "test",
		ObservedAt:  now,
		ExpiresAt:   now.Add(time.Hour),
		Evidence:    []string{"probe:job"},
	}
	notice, delivered, err := e.ObserveEvent(event)
	if err != nil {
		t.Fatalf("observe event: %v", err)
	}
	if !delivered {
		t.Fatal("critical event was not delivered while the background job was running")
	}
	if !notice.Complete() {
		t.Fatalf("notice is incomplete: %+v", notice)
	}
}
