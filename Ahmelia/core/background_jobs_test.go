package core

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ahmelia/interaction"
	"ahmelia/interrupt"
	"ahmelia/production"
	"ahmelia/programming"
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

func TestBackgroundResultsSurviveRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "background_results.json")
	first := buildTestEngine(t)
	if err := first.EnableBackgroundResultPersistence(path); err != nil {
		t.Fatalf("enable persistence: %v", err)
	}
	first.backgroundResults.enqueue(backgroundResult{
		JobID: "job-persisted", WorkID: "work-persisted", Recovered: true,
		Result: production.ActionResult{Success: true, Verified: true, Summary: "resultado persistido"},
	})
	first.backgroundResults.enqueue(backgroundResult{
		JobID: "job-canceled", WorkID: "work-canceled", Err: errBackgroundJobCanceled, Canceled: true,
	})

	restarted := buildTestEngine(t)
	if err := restarted.EnableBackgroundResultPersistence(path); err != nil {
		t.Fatalf("reload persistence: %v", err)
	}
	var responses []string
	restarted.SetResponseCallback(func(response string) { responses = append(responses, response) })
	restarted.Process("hola")
	if joined := strings.Join(responses, " "); !strings.Contains(joined, "Reconcilié el trabajo interrumpido: resultado persistido") {
		t.Fatalf("persisted result was not announced: %q", joined)
	}
	if joined := strings.Join(responses, " "); !strings.Contains(joined, "Cancelé el trabajo en segundo plano") {
		t.Fatalf("persisted cancellation was not announced: %q", joined)
	}

	again := buildTestEngine(t)
	if err := again.EnableBackgroundResultPersistence(path); err != nil {
		t.Fatalf("reload after take: %v", err)
	}
	var second []string
	again.SetResponseCallback(func(response string) { second = append(second, response) })
	again.Process("hola")
	if joined := strings.Join(second, " "); strings.Contains(joined, "Reconcilié el trabajo interrumpido") {
		t.Fatalf("persisted result was announced twice: %q", joined)
	}
}

func TestReconcileInterruptedProgrammingWorkResolvesFromDurablePlan(t *testing.T) {
	root := t.TempDir()
	mainContent := "package main\n\nfunc Add(a, b int) int { return a + b }\n\nfunc main() {}\n"
	testContent := "package main\n\nimport \"testing\"\n\nfunc TestAdd(t *testing.T) { if Add(2, 3) != 5 { t.Fatal(\"bad sum\") } }\n"
	for name, content := range map[string]string{
		"go.mod":       "module reconcile\n\ngo 1.20\n",
		"main.go":      mainContent,
		"main_test.go": testContent,
	} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	plan := programming.MultiFilePlan{
		ID: "plan-reconcile",
		Project: programming.CodeProject{
			ID: "reconcile", Name: "reconcile", Root: root, Language: "Go",
			BuildCommand: "go build ./...", TestCommand: "go test ./...",
		},
		Changes: []programming.MultiFileChange{
			{ID: "main", Operation: programming.SourceCreate, File: programming.SourceFile{Path: "main.go", Language: "Go", Role: programming.RoleSource, ProjectID: "reconcile"}, Content: mainContent},
			{ID: "test", Operation: programming.SourceCreate, File: programming.SourceFile{Path: "main_test.go", Language: "Go", Role: programming.RoleTest, ProjectID: "reconcile"}, Content: testContent},
		},
	}
	spec, err := json.Marshal(plan)
	if err != nil {
		t.Fatalf("marshal plan: %v", err)
	}

	path := filepath.Join(t.TempDir(), "production_work.json")
	e := buildTestEngine(t)
	e.workStore = NewPersistentWorkStore(production.NewWorkStore(path))
	req := interaction.Analyze("crear un proyecto que será reconciliado", interaction.Context{})
	work, err := e.workStore.Begin(req)
	if err != nil {
		t.Fatalf("begin work: %v", err)
	}
	attempt, err := e.workStore.BeginAttempt(work.ID, "programming.create_file", "produce", "programming")
	if err != nil {
		t.Fatalf("begin attempt: %v", err)
	}
	if err := e.workStore.SetAttemptEffectSpecification(work.ID, attempt, spec); err != nil {
		t.Fatalf("set effect spec: %v", err)
	}

	restarted := buildTestEngine(t)
	restarted.workStore = NewPersistentWorkStore(production.NewWorkStore(path))
	if reconciled := restarted.ReconcileInterruptedProgrammingWork(context.Background()); reconciled != 1 {
		t.Fatalf("reconciled = %d, quería 1", reconciled)
	}
	persisted, ok := restarted.workStore.Get(work.ID)
	if !ok || persisted.Status != production.WorkCompleted || persisted.LastActionResult == nil || !persisted.LastActionResult.Success {
		t.Fatalf("reconciled work is not completed: ok=%t work=%+v", ok, persisted)
	}
	if len(persisted.Attempts) != 1 || persisted.Attempts[0].Status != "succeeded" {
		t.Fatalf("reconciled attempt is not succeeded: %+v", persisted.Attempts)
	}

	var responses []string
	restarted.SetResponseCallback(func(response string) { responses = append(responses, response) })
	restarted.Process("hola")
	if joined := strings.Join(responses, " "); !strings.Contains(joined, "Reconcilié el trabajo interrumpido") {
		t.Fatalf("reconciled work was not announced: %q", joined)
	}
}
