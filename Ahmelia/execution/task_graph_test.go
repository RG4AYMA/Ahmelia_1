package execution

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"ahmelia/planning"
)

func graphWorker(t *testing.T) *Dispatcher {
	t.Helper()
	r := NewRegistry()
	w, err := NewLocalWorker("graph-local", WorkerCapabilities{CPU: 8, MemoryMB: 16384, Domains: map[string]bool{"test": true}, Capabilities: map[string]bool{"run": true}, ConcurrentSlots: 8})
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Register(w); err != nil {
		t.Fatal(err)
	}
	d, err := NewDispatcher(r)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestTaskGraphExecutesIndependentTasksConcurrently(t *testing.T) {
	d := graphWorker(t)
	e, err := NewTaskGraphExecutor(d, 0)
	if err != nil {
		t.Fatal(err)
	}
	a := planning.NewTask("g", "a", "", 1, 0)
	b := planning.NewTask("g", "b", "", 2, 0)
	c := planning.NewTask("g", "c", "", 3, 0)
	c.Dependencies = []string{a.ID, b.ID}
	var mu sync.Mutex
	active, maxActive := 0, 0
	_, err = e.Run(context.Background(), []TaskExecutionSpec{{Task: a}, {Task: b}, {Task: c}}, func(ctx context.Context, s TaskExecutionSpec) error {
		mu.Lock()
		active++
		if active > maxActive {
			maxActive = active
		}
		mu.Unlock()
		defer func() { mu.Lock(); active--; mu.Unlock() }()
		time.Sleep(40 * time.Millisecond)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if maxActive < 2 {
		t.Fatalf("expected independent tasks to overlap, maxActive=%d", maxActive)
	}
	if !c.IsCompleted() || !a.IsCompleted() || !b.IsCompleted() {
		t.Fatal("expected all tasks completed")
	}
}

func TestTaskGraphBlocksDependentsAfterFailure(t *testing.T) {
	d := graphWorker(t)
	e, _ := NewTaskGraphExecutor(d, 0)
	a := planning.NewTask("g", "a", "", 1, 0)
	b := planning.NewTask("g", "b", "", 2, 0)
	b.Dependencies = []string{a.ID}
	calledB := false
	res, err := e.Run(context.Background(), []TaskExecutionSpec{{Task: a}, {Task: b}}, func(ctx context.Context, s TaskExecutionSpec) error {
		if s.Task.ID == b.ID {
			calledB = true
		}
		return errors.New("boom")
	})
	if err != nil {
		t.Fatal(err)
	}
	if calledB {
		t.Fatal("dependent task should not execute")
	}
	if len(res.Failed) != 2 {
		t.Fatalf("expected failed root and blocked dependent, got %d", len(res.Failed))
	}
}

func TestTaskGraphRejectsCyclesBeforeExecution(t *testing.T) {
	d := graphWorker(t)
	e, _ := NewTaskGraphExecutor(d, 0)
	a := planning.NewTask("g", "a", "", 1, 0)
	b := planning.NewTask("g", "b", "", 2, 0)
	a.Dependencies = []string{b.ID}
	b.Dependencies = []string{a.ID}
	called := false
	_, err := e.Run(context.Background(), []TaskExecutionSpec{{Task: a}, {Task: b}}, func(context.Context, TaskExecutionSpec) error { called = true; return nil })
	if err == nil {
		t.Fatal("expected cycle validation error")
	}
	if called {
		t.Fatal("graph must validate before executing")
	}
}

func TestTaskGraphRetriesWithinExplicitBound(t *testing.T) {
	d := graphWorker(t)
	e, _ := NewTaskGraphExecutor(d, 1)
	task := planning.NewTask("g", "retry", "", 1, 0)
	calls := 0
	res, err := e.Run(context.Background(), []TaskExecutionSpec{{Task: task}}, func(context.Context, TaskExecutionSpec) error {
		calls++
		if calls == 1 {
			return errors.New("transient")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("expected exactly 2 attempts, got %d", calls)
	}
	if len(res.Attempts) != 2 {
		t.Fatalf("expected 2 recorded attempts, got %d", len(res.Attempts))
	}
	if !task.IsCompleted() {
		t.Fatal("task should complete after bounded retry")
	}
}

type noRetryTestError struct{ reason string }

func (e noRetryTestError) Error() string { return e.reason }
func (noRetryTestError) Retryable() bool { return false }

func TestTaskGraphDoesNotBlindlyRetryUnverifiedEffectsAcrossVariants(t *testing.T) {
	// Three known failure classes and five unseen/adversarial variants of the
	// same operation contract: once the callback reports an unverified effect,
	// retrying it must be explicitly authorized elsewhere after reconciliation.
	variants := []string{
		"known: filesystem result not observed",
		"known: process result not verified",
		"known: authority reports executed but not verified",
		"new: timeout after external side effect",
		"new: partial output with insufficient evidence",
		"new: contradictory observation",
		"new: cancellation after operation dispatch",
		"new: response identity mismatch",
	}
	for _, variant := range variants {
		t.Run(variant, func(t *testing.T) {
			d := graphWorker(t)
			e, err := NewTaskGraphExecutor(d, 3)
			if err != nil {
				t.Fatal(err)
			}
			task := planning.NewTask("g", "same operation", "", 1, 0)
			calls := 0
			result, err := e.Run(context.Background(), []TaskExecutionSpec{{Task: task}}, func(context.Context, TaskExecutionSpec) error {
				calls++
				return noRetryTestError{reason: variant}
			})
			if err != nil {
				t.Fatalf("unexpected graph error: %v", err)
			}
			if calls != 1 {
				t.Fatalf("unsafe repeated operation: calls=%d, want 1", calls)
			}
			if len(result.Failed) != 1 {
				t.Fatalf("failed tasks=%d, want 1", len(result.Failed))
			}
			if task.Status != planning.TaskBlocked {
				t.Fatalf("task status=%v, want blocked", task.Status)
			}
		})
	}
}
