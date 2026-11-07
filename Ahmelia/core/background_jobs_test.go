package core

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ahmelia/interaction"
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

func waitForAnyBackgroundJobCompletion(t *testing.T, e *Engine) BackgroundJob {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		for _, job := range e.BackgroundJobs() {
			if job.Status == BackgroundJobCompleted || job.Status == BackgroundJobFailed || job.Status == BackgroundJobCanceled {
				return job
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("no background job reached a terminal state: %+v", e.BackgroundJobs())
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

func TestRecoverInterruptedWorkBlocksAndAnnounces(t *testing.T) {
	e := buildTestEngine(t)
	req := interaction.Analyze("crear un proyecto interrumpido", interaction.Context{})
	work, err := e.workStore.Begin(req)
	if err != nil {
		t.Fatalf("begin work: %v", err)
	}
	if _, err := e.workStore.BeginAttempt(work.ID, "programming.create_file", "produce", "programming"); err != nil {
		t.Fatalf("begin attempt: %v", err)
	}

	if recovered := e.RecoverInterruptedWork(); recovered != 1 {
		t.Fatalf("recovered = %d, quería 1", recovered)
	}
	persisted, ok := e.workStore.Get(work.ID)
	if !ok || persisted.Status != production.WorkBlocked {
		t.Fatalf("interrupted work was not blocked: ok=%t work=%+v", ok, persisted)
	}

	var responses []string
	e.SetResponseCallback(func(response string) { responses = append(responses, response) })
	e.Process("hola")
	joined := strings.Join(responses, " ")
	if !strings.Contains(joined, "interrumpido por un reinicio") {
		t.Fatalf("interrupted work was not announced: %q", joined)
	}

	// A second startup must not enqueue the same interrupted work again.
	if recovered := e.RecoverInterruptedWork(); recovered != 0 {
		t.Fatalf("second recovery = %d, quería 0", recovered)
	}
}

func TestRecoverInterruptedWorkSurvivesRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "production_work.json")
	e := buildTestEngine(t)
	e.workStore = NewPersistentWorkStore(production.NewWorkStore(path))
	req := interaction.Analyze("crear un proyecto que sobreviva al reinicio", interaction.Context{})
	work, err := e.workStore.Begin(req)
	if err != nil {
		t.Fatalf("begin work: %v", err)
	}
	if _, err := e.workStore.BeginAttempt(work.ID, "programming.create_file", "produce", "programming"); err != nil {
		t.Fatalf("begin attempt: %v", err)
	}

	restarted := buildTestEngine(t)
	restarted.workStore = NewPersistentWorkStore(production.NewWorkStore(path))
	if recovered := restarted.RecoverInterruptedWork(); recovered != 1 {
		t.Fatalf("recovered after restart = %d, quería 1", recovered)
	}
	persisted, ok := restarted.workStore.Get(work.ID)
	if !ok || persisted.Status != production.WorkBlocked {
		t.Fatalf("restarted work was not blocked: ok=%t work=%+v", ok, persisted)
	}
}
