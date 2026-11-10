package production

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestRetryBudgetProgressPreservesBestEvidence(t *testing.T) {
	b := NewRetryBudget(RetryPolicy{MaxAttempts: 10, NoProgressLimit: 3}, "acceptance-v1")
	check := func(id string, pass bool) ProgressCheck { return ProgressCheck{ID: id, Passed: pass} }
	if b.Observe([]ProgressCheck{check("build", false)}) {
		t.Fatal("first failure is baseline")
	}
	if !b.Observe([]ProgressCheck{check("build", true), check("test", false)}) {
		t.Fatal("verified new milestone must count")
	}
	if b.Observe([]ProgressCheck{check("build", false), check("test", true)}) {
		t.Fatal("regression must not count")
	}
	if b.Observe([]ProgressCheck{check("build", true), check("test", false)}) {
		t.Fatal("restoring an old pass must not count")
	}
	if b.Observe(nil) {
		t.Fatal("missing checks must not count")
	}
	if err := b.CanContinue(context.Background()); err == nil || !strings.Contains(err.Error(), "no_progress") {
		t.Fatalf("missing stop: %v", err)
	}
}

func TestRetryBudgetHardCapsAndImmutableContract(t *testing.T) {
	for _, tc := range []struct {
		name   string
		policy RetryPolicy
		run    func(*RetryBudget) error
		reason string
	}{
		{"attempts", RetryPolicy{MaxAttempts: 1}, func(b *RetryBudget) error {
			_ = b.BeginAttempt(context.Background(), "fixed")
			return b.BeginAttempt(context.Background(), "fixed")
		}, "max_attempts"},
		{"calls", RetryPolicy{MaxModelCalls: 1}, func(b *RetryBudget) error {
			_ = b.UseModelCall(context.Background())
			return b.UseModelCall(context.Background())
		}, "model_call"},
		{"contract", RetryPolicy{}, func(b *RetryBudget) error { return b.BeginAttempt(context.Background(), "simplified") }, "objective_contract_changed"},
		{"cancel", RetryPolicy{}, func(b *RetryBudget) error {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			return b.BeginAttempt(ctx, "fixed")
		}, "context canceled"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := NewRetryBudget(tc.policy, "fixed")
			if err := tc.run(b); err == nil || !strings.Contains(err.Error(), tc.reason) {
				t.Fatalf("got %v", err)
			}
		})
	}
}

func TestRetryBudgetRestoreKeepsCountersDeadlineAndEvidence(t *testing.T) {
	b := NewRetryBudget(RetryPolicy{MaxAttempts: 2}, "fixed")
	_ = b.BeginAttempt(context.Background(), "fixed")
	_ = b.UseModelCall(context.Background())
	b.Observe([]ProgressCheck{{ID: "build", Passed: true}})
	s := b.Snapshot()
	raw, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	var restored RetryState
	if err = json.Unmarshal(raw, &restored); err != nil {
		t.Fatal(err)
	}
	r := RestoreRetryBudget(restored)
	if !r.Snapshot().Deadline.Equal(s.Deadline) || r.Snapshot().ModelCalls != 1 {
		t.Fatal("budget reset")
	}
	copy := r.Snapshot()
	copy.BestPassed["invented"] = true
	if r.Snapshot().BestPassed["invented"] {
		t.Fatal("mutable snapshot")
	}
	if err = r.BeginAttempt(context.Background(), "fixed"); err != nil {
		t.Fatal(err)
	}
	if err = r.BeginAttempt(context.Background(), "fixed"); err == nil {
		t.Fatal("restart reset attempt cap")
	}
	restored.Deadline = time.Now().Add(-time.Second)
	if err = RestoreRetryBudget(restored).UseModelCall(context.Background()); err == nil {
		t.Fatal("expired deadline reset")
	}
}

func TestRetryBudgetConcurrentModelCallsRespectGlobalLimit(t *testing.T) {
	b := NewRetryBudget(RetryPolicy{MaxModelCalls: 3}, "fixed")
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _ = b.UseModelCall(context.Background()) }()
	}
	wg.Wait()
	if got := b.Snapshot().ModelCalls; got != 3 {
		t.Fatalf("calls=%d", got)
	}
}
