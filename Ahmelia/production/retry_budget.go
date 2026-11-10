package production

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// RetryPolicy bounds one automatic Work chain, including replans. Zero fields
// select conservative defaults; they never mean unlimited execution.
type RetryPolicy struct {
	MaxAttempts     int           `json:"max_attempts"`
	NoProgressLimit int           `json:"no_progress_limit"`
	MaxModelCalls   int           `json:"max_model_calls"`
	MaxDuration     time.Duration `json:"max_duration"`
}

func (p RetryPolicy) WithDefaults() RetryPolicy {
	if p.MaxAttempts <= 0 {
		p.MaxAttempts = 3
	}
	if p.NoProgressLimit <= 0 {
		p.NoProgressLimit = 2
	}
	if p.MaxModelCalls <= 0 {
		p.MaxModelCalls = 6
	}
	if p.MaxDuration <= 0 {
		p.MaxDuration = 15 * time.Minute
	}
	return p
}

// ProgressCheck is harness-observed evidence with a stable identity, never a
// model's claim or a diagnostic string. Missing checks cannot preserve a pass.
type ProgressCheck struct {
	ID     string `json:"id"`
	Passed bool   `json:"passed"`
}

// RetryState is an immutable snapshot committed with the Work attempt result.
type RetryState struct {
	Policy       RetryPolicy     `json:"policy"`
	Deadline     time.Time       `json:"deadline"`
	Contract     string          `json:"contract"`
	Attempts     int             `json:"attempts"`
	ModelCalls   int             `json:"model_calls"`
	NoProgress   int             `json:"no_progress"`
	Observations int             `json:"observations"`
	BestPassed   map[string]bool `json:"best_passed,omitempty"`
	StopReason   string          `json:"stop_reason,omitempty"`
}

func (s RetryState) Clone() RetryState {
	out := s
	out.BestPassed = make(map[string]bool, len(s.BestPassed))
	for k, v := range s.BestPassed {
		out.BestPassed[k] = v
	}
	return out
}

type RetryBudget struct {
	mu    sync.Mutex
	state RetryState
}

func NewRetryBudget(p RetryPolicy, contract string) *RetryBudget {
	p = p.WithDefaults()
	return &RetryBudget{state: RetryState{Policy: p, Contract: contract, Deadline: time.Now().Add(p.MaxDuration), BestPassed: map[string]bool{}}}
}

func RestoreRetryBudget(s RetryState) *RetryBudget {
	s = s.Clone()
	s.Policy = s.Policy.WithDefaults()
	return &RetryBudget{state: s}
}

func (b *RetryBudget) Snapshot() RetryState { b.mu.Lock(); defer b.mu.Unlock(); return b.state.Clone() }

func (b *RetryBudget) stop(reason string) error {
	if b.state.StopReason == "" {
		b.state.StopReason = reason
	}
	return fmt.Errorf("production retry stopped: %s", b.state.StopReason)
}

func (b *RetryBudget) check(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return b.stop(err.Error())
	}
	if b.state.StopReason != "" {
		return b.stop(b.state.StopReason)
	}
	if !time.Now().Before(b.state.Deadline) {
		return b.stop("time_budget_exhausted")
	}
	return nil
}

// CanContinue does not consume an attempt. It is also used before returning a
// retry decision, so the final failure explains why no further action occurs.
func (b *RetryBudget) CanContinue(ctx context.Context) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.canContinue(ctx)
}

func (b *RetryBudget) canContinue(ctx context.Context) error {
	if err := b.check(ctx); err != nil {
		return err
	}
	if b.state.Attempts >= b.state.Policy.MaxAttempts {
		return b.stop("max_attempts_reached")
	}
	if b.state.NoProgress >= b.state.Policy.NoProgressLimit {
		return b.stop("no_progress_limit_reached")
	}
	if b.state.ModelCalls >= b.state.Policy.MaxModelCalls {
		return b.stop("model_call_budget_exhausted")
	}
	return nil
}

func (b *RetryBudget) BeginAttempt(ctx context.Context, contract string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if err := b.canContinue(ctx); err != nil {
		return err
	}
	if contract != b.state.Contract {
		return b.stop("objective_contract_changed")
	}
	b.state.Attempts++
	return nil
}

func (b *RetryBudget) UseModelCall(ctx context.Context) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if err := b.canUseModelCall(ctx); err != nil {
		return err
	}
	b.state.ModelCalls++
	return nil
}

// Inner synthesis retries consume model calls, not new provider attempts.
// The last permitted provider attempt may still finish within its call cap.
func (b *RetryBudget) CanUseModelCall(ctx context.Context) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.canUseModelCall(ctx)
}

func (b *RetryBudget) canUseModelCall(ctx context.Context) error {
	if err := b.check(ctx); err != nil {
		return err
	}
	if b.state.ModelCalls >= b.state.Policy.MaxModelCalls {
		return b.stop("model_call_budget_exhausted")
	}
	return nil
}

// Observe establishes a baseline on the first failed attempt. Later progress
// requires a new verified pass and preservation of EVERY previous best pass.
// Regressing and restoring an old pass, or changing an error, buys no budget.
func (b *RetryBudget) Observe(checks []ProgressCheck) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	current := map[string]bool{}
	for _, c := range checks {
		if c.ID == "" {
			continue
		}
		if old, exists := current[c.ID]; exists {
			current[c.ID] = old && c.Passed
		} else {
			current[c.ID] = c.Passed
		}
	}
	preserved, added := true, false
	for id := range b.state.BestPassed {
		if !current[id] {
			preserved = false
		}
	}
	for id, passed := range current {
		if passed && !b.state.BestPassed[id] {
			added = true
		}
	}
	progress := b.state.Observations > 0 && preserved && added
	if b.state.Observations == 0 || progress {
		for id, passed := range current {
			if passed {
				b.state.BestPassed[id] = true
			}
		}
	}
	b.state.Observations++
	if progress {
		b.state.NoProgress = 0
	} else {
		b.state.NoProgress++
	}
	return progress
}

type retryBudgetContextKey struct{}

func (b *RetryBudget) Context(ctx context.Context) (context.Context, context.CancelFunc) {
	s := b.Snapshot()
	return context.WithDeadline(context.WithValue(ctx, retryBudgetContextKey{}, b), s.Deadline)
}

func RetryBudgetFromContext(ctx context.Context) *RetryBudget {
	if ctx == nil {
		return nil
	}
	b, _ := ctx.Value(retryBudgetContextKey{}).(*RetryBudget)
	return b
}
