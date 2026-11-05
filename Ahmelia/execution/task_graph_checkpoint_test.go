package execution

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"ahmelia/planning"
)

type uncertainEffectError struct{ reason string }

func (e uncertainEffectError) Error() string              { return e.reason }
func (uncertainEffectError) Retryable() bool              { return false }
func (uncertainEffectError) RequiresReconciliation() bool { return true }

func TestTaskGraphResumesAfterProcessBoundary(t *testing.T) {
	dir := t.TempDir()
	store, err := NewFileTaskGraphCheckpointStore(filepath.Join(dir, "checkpoints"))
	if err != nil {
		t.Fatal(err)
	}

	a := planning.NewTask("g", "a", "", 1, time.Second)
	b := planning.NewTask("g", "b", "", 2, time.Second)
	b.Dependencies = []string{a.ID}

	// Simulate the durable checkpoint left by a previous process after task A
	// was already verified. The next process must not execute A again.
	if err := store.Save(TaskGraphCheckpoint{WorkID: "work-resume", Completed: []string{a.ID}}); err != nil {
		t.Fatal(err)
	}

	dispatcher, err := NewDispatcher(NewRegistry())
	if err != nil {
		t.Fatal(err)
	}
	worker, err := NewLocalWorker("local", WorkerCapabilities{})
	if err != nil {
		t.Fatal(err)
	}
	_ = dispatcher.RegisterWorker(worker)
	executor, err := NewTaskGraphExecutor(dispatcher, 0)
	if err != nil {
		t.Fatal(err)
	}

	calls := 0
	result, err := executor.RunResumable(context.Background(), "work-resume", []TaskExecutionSpec{{Task: a}, {Task: b}}, func(ctx context.Context, s TaskExecutionSpec) error {
		calls++
		if s.Task.ID == a.ID {
			t.Fatalf("completed task was re-executed")
		}
		return nil
	}, store)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("calls=%d, want 1 resumed task", calls)
	}
	if len(result.Completed) != 1 || result.Completed[0] != b.ID {
		t.Fatalf("completed=%v, want only resumed task %q", result.Completed, b.ID)
	}
	if _, ok, _ := store.Load("work-resume"); ok {
		t.Fatalf("completed graph checkpoint should be deleted")
	}
}

func TestTaskGraphRefusesBlindReplayOfUncertainRunningTask(t *testing.T) {
	dir := t.TempDir()
	store, err := NewFileTaskGraphCheckpointStore(filepath.Join(dir, "checkpoints"))
	if err != nil {
		t.Fatal(err)
	}
	task := planning.NewTask("g", "uncertain", "", 1, time.Second)
	if err := store.Save(TaskGraphCheckpoint{WorkID: "work-uncertain", Running: map[string]string{task.ID: "task-uncertain-attempt-1"}}); err != nil {
		t.Fatal(err)
	}
	d, err := NewDispatcher(NewRegistry())
	if err != nil {
		t.Fatal(err)
	}
	w, err := NewLocalWorker("local", WorkerCapabilities{})
	if err != nil {
		t.Fatal(err)
	}
	if err := d.RegisterWorker(w); err != nil {
		t.Fatal(err)
	}
	e, err := NewTaskGraphExecutor(d, 0)
	if err != nil {
		t.Fatal(err)
	}
	_, err = e.RunResumable(context.Background(), "work-uncertain", []TaskExecutionSpec{{Task: task}}, func(context.Context, TaskExecutionSpec) error {
		t.Fatal("uncertain task was replayed blindly")
		return nil
	}, store)
	if err == nil {
		t.Fatal("expected reconciliation requirement")
	}
}

func TestTaskGraphReconcilesCompletedRunningTaskWithoutReplay(t *testing.T) {
	dir := t.TempDir()
	store, err := NewFileTaskGraphCheckpointStore(filepath.Join(dir, "checkpoints"))
	if err != nil {
		t.Fatal(err)
	}
	task := planning.NewTask("g", "uncertain", "", 1, time.Second)
	if err := store.Save(TaskGraphCheckpoint{WorkID: "work-reconciled", Running: map[string]string{task.ID: "task-uncertain-attempt-1"}}); err != nil {
		t.Fatal(err)
	}
	d := graphWorker(t)
	e, err := NewTaskGraphExecutor(d, 0)
	if err != nil {
		t.Fatal(err)
	}
	e.SetExecutionReconciler(func(_ context.Context, s TaskExecutionSpec, jobID string) (ReconcileDecision, error) {
		if s.Task.ID != task.ID || jobID != "task-uncertain-attempt-1" {
			t.Fatalf("unexpected reconciliation input: task=%s job=%s", s.Task.ID, jobID)
		}
		return ReconcileCompleted, nil
	})
	calls := 0
	result, err := e.RunResumable(context.Background(), "work-reconciled", []TaskExecutionSpec{{Task: task}}, func(context.Context, TaskExecutionSpec) error {
		calls++
		return nil
	}, store)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 0 {
		t.Fatalf("reconciled completed task was re-executed: calls=%d", calls)
	}
	if len(result.Completed) != 1 || result.Completed[0] != task.ID {
		t.Fatalf("unexpected completed tasks: %#v", result.Completed)
	}
	if task.Status != planning.TaskCompleted {
		t.Fatalf("reconciled task status=%q, want completed", task.Status)
	}
}

func TestTaskGraphPersistsUnverifiedEffectsForReconciliationAcrossVariants(t *testing.T) {
	// Three known cases plus five unseen/adversarial variants of the same
	// contract: once the callback says an effect may have happened but cannot
	// verify it, a later process must not dispatch it again before reconciliation.
	cases := []struct {
		name        string
		description string
		err         error
	}{
		{name: "known_create", description: "create folder alpha", err: uncertainEffectError{"folder may exist"}},
		{name: "known_volume", description: "set system volume", err: uncertainEffectError{"volume state unknown"}},
		{name: "known_file", description: "write project file", err: uncertainEffectError{"write outcome unknown"}},
		{name: "variant_ui_timeout", description: "toggle application setting", err: uncertainEffectError{"UI timed out after click"}},
		{name: "variant_network_lost", description: "submit remote job", err: uncertainEffectError{"connection lost after submit"}},
		{name: "variant_process_exit", description: "launch local process", err: uncertainEffectError{"process exited before result capture"}},
		{name: "variant_partial_artifact", description: "generate model artifact", err: uncertainEffectError{"partial artifact observed"}},
		{name: "variant_context_cancel", description: "apply project migration", err: uncertainEffectError{"context ended after dispatch"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			store, err := NewFileTaskGraphCheckpointStore(filepath.Join(dir, "checkpoints"))
			if err != nil {
				t.Fatal(err)
			}
			task := planning.NewTask("goal-uncertain", tc.name, tc.description, 1, time.Second)
			executor, err := NewTaskGraphExecutor(graphWorker(t), 2)
			if err != nil {
				t.Fatal(err)
			}
			calls := 0
			result, err := executor.RunResumable(context.Background(), "work-"+tc.name, []TaskExecutionSpec{{Task: task, ExecutionFingerprint: tc.name}}, func(context.Context, TaskExecutionSpec) error {
				calls++
				return tc.err
			}, store)
			if err != nil {
				t.Fatal(err)
			}
			if calls != 1 {
				t.Fatalf("first run calls=%d, want 1", calls)
			}
			if len(result.NeedsReconciliation) != 1 || result.NeedsReconciliation[0] != task.ID {
				t.Fatalf("reconciliation state=%v, want task %q", result.NeedsReconciliation, task.ID)
			}
			checkpoint, ok, err := store.Load("work-" + tc.name)
			if err != nil || !ok {
				t.Fatalf("checkpoint missing: ok=%v err=%v", ok, err)
			}
			if checkpoint.Running[task.ID] == "" {
				t.Fatal("uncertain task lost durable running identity")
			}

			// Simulate a new process without a reconciler: it must fail closed
			// before dispatch, not treat the uncertainty as an ordinary failure.
			_, err = executor.RunResumable(context.Background(), "work-"+tc.name, []TaskExecutionSpec{{Task: task, ExecutionFingerprint: tc.name}}, func(context.Context, TaskExecutionSpec) error {
				calls++
				return nil
			}, store)
			if err == nil {
				t.Fatal("expected reconciliation-required error")
			}
			if calls != 1 {
				t.Fatalf("uncertain operation replayed %d times", calls-1)
			}
		})
	}
}

func TestTaskGraphPersistsReconciledBlockedOutcomeWithoutReplay(t *testing.T) {
	dir := t.TempDir()
	store, err := NewFileTaskGraphCheckpointStore(filepath.Join(dir, "checkpoints"))
	if err != nil {
		t.Fatal(err)
	}
	task := planning.NewTask("g", "uncertain", "unchanged operation", 1, time.Second)
	if err := store.Save(TaskGraphCheckpoint{WorkID: "work-blocked", Running: map[string]string{task.ID: "task-uncertain-attempt-1"}}); err != nil {
		t.Fatal(err)
	}
	d := graphWorker(t)
	e, err := NewTaskGraphExecutor(d, 0)
	if err != nil {
		t.Fatal(err)
	}
	e.SetExecutionReconciler(func(context.Context, TaskExecutionSpec, string) (ReconcileDecision, error) {
		return ReconcileBlocked, nil
	})
	calls := 0
	first, err := e.RunResumable(context.Background(), "work-blocked", []TaskExecutionSpec{{Task: task}}, func(context.Context, TaskExecutionSpec) error {
		calls++
		return nil
	}, store)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 0 || !task.IsBlocked() || len(first.Failed) != 1 {
		t.Fatalf("blocked reconciliation should stop without dispatch: calls=%d status=%q failed=%v", calls, task.Status, first.Failed)
	}
	checkpoint, ok, err := store.Load("work-blocked")
	if err != nil || !ok {
		t.Fatalf("load blocked checkpoint: ok=%v err=%v", ok, err)
	}
	if len(checkpoint.Blocked) != 1 || checkpoint.Blocked[0] != task.ID {
		t.Fatalf("blocked outcome was not durable: %#v", checkpoint.Blocked)
	}

	// A later invocation of the same unchanged graph must retain the block,
	// rather than interpreting the removed Running entry as permission to run.
	e.SetExecutionReconciler(nil)
	second, err := e.RunResumable(context.Background(), "work-blocked", []TaskExecutionSpec{{Task: task}}, func(context.Context, TaskExecutionSpec) error {
		calls++
		return nil
	}, store)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 0 || !task.IsBlocked() || len(second.Failed) != 1 {
		t.Fatalf("later invocation replayed reconciled-blocked operation: calls=%d status=%q failed=%v", calls, task.Status, second.Failed)
	}
}

func TestTaskGraphReconcilesRetryOnlyWhenExplicitlySafe(t *testing.T) {
	dir := t.TempDir()
	store, err := NewFileTaskGraphCheckpointStore(filepath.Join(dir, "checkpoints"))
	if err != nil {
		t.Fatal(err)
	}
	task := planning.NewTask("g", "uncertain", "", 1, time.Second)
	if err := store.Save(TaskGraphCheckpoint{WorkID: "work-retry", Running: map[string]string{task.ID: "task-uncertain-attempt-1"}}); err != nil {
		t.Fatal(err)
	}
	d := graphWorker(t)
	e, err := NewTaskGraphExecutor(d, 0)
	if err != nil {
		t.Fatal(err)
	}
	e.SetExecutionReconciler(func(context.Context, TaskExecutionSpec, string) (ReconcileDecision, error) {
		return ReconcileRetry, nil
	})
	calls := 0
	result, err := e.RunResumable(context.Background(), "work-retry", []TaskExecutionSpec{{Task: task}}, func(context.Context, TaskExecutionSpec) error {
		calls++
		return nil
	}, store)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("expected one explicitly authorized replay, got %d", calls)
	}
	if len(result.Completed) != 1 || result.Completed[0] != task.ID {
		t.Fatalf("unexpected completed tasks: %#v", result.Completed)
	}
}

func TestTaskGraphRejectsCheckpointWhenPlannedOperationChanges(t *testing.T) {
	mutations := []struct {
		name   string
		mutate func(*TaskExecutionSpec)
	}{
		{name: "mapped operation fingerprint", mutate: func(s *TaskExecutionSpec) { s.ExecutionFingerprint = "operation-beta" }},
		{name: "task description", mutate: func(s *TaskExecutionSpec) { s.Task.Description = "create folder beta" }},
		{name: "capability", mutate: func(s *TaskExecutionSpec) { s.CapabilityID = "delete_folder" }},
		{name: "operation", mutate: func(s *TaskExecutionSpec) { s.Operation = "delete_folder" }},
		{name: "dependency graph", mutate: func(s *TaskExecutionSpec) { s.Task.Dependencies = []string{"upstream-task"} }},
	}
	for _, mutation := range mutations {
		t.Run(mutation.name, func(t *testing.T) {
			dir := t.TempDir()
			store, err := NewFileTaskGraphCheckpointStore(filepath.Join(dir, "checkpoints"))
			if err != nil {
				t.Fatal(err)
			}
			d := graphWorker(t)
			executor, err := NewTaskGraphExecutor(d, 0)
			if err != nil {
				t.Fatal(err)
			}

			task := planning.NewTask("goal-fingerprint", "create folder", "create folder alpha", 1, time.Second)
			spec := TaskExecutionSpec{Task: task, CapabilityID: "create_folder", Operation: "create_folder", ExecutionFingerprint: "operation-alpha"}
			_, err = executor.RunResumable(context.Background(), "work-fingerprint", []TaskExecutionSpec{spec}, func(context.Context, TaskExecutionSpec) error {
				return fmt.Errorf("simulated interruption after planned attempt")
			}, store)
			if err != nil {
				t.Fatalf("initial interrupted run returned unexpected error: %v", err)
			}
			checkpoint, ok, err := store.Load("work-fingerprint")
			if err != nil || !ok {
				t.Fatalf("expected durable checkpoint, ok=%v err=%v", ok, err)
			}
			if checkpoint.GraphFingerprint == "" {
				t.Fatal("planned workflow checkpoint did not persist its graph fingerprint")
			}

			changedTask := planning.NewTask("goal-fingerprint", "create folder", "create folder alpha", 1, time.Second)
			changedTask.ID = task.ID // Same task identity, potentially different requested effect.
			changed := TaskExecutionSpec{Task: changedTask, CapabilityID: "create_folder", Operation: "create_folder", ExecutionFingerprint: "operation-alpha"}
			mutation.mutate(&changed)
			calls := 0
			_, err = executor.RunResumable(context.Background(), "work-fingerprint", []TaskExecutionSpec{changed}, func(context.Context, TaskExecutionSpec) error {
				calls++
				return nil
			}, store)
			if err == nil {
				t.Fatal("expected stale checkpoint to be rejected when the planned operation changed")
			}
			if calls != 0 {
				t.Fatalf("changed plan executed %d times before stale checkpoint rejection", calls)
			}
		})
	}
}
