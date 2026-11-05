package computeruse

import (
	"context"
	"fmt"
	"time"
)

// Snapshot is a provider-neutral observation of an interactive digital environment.
// Concrete adapters may populate screenshots, active application/window, focused
// element, and structured UI metadata without forcing those details into the core.
type Snapshot struct {
	ID                string
	CapturedAt        time.Time
	ActiveApplication string
	ActiveWindow      string
	Screenshot        []byte
	UI                map[string]string

	// Fingerprint identifies the observable state, allowing closed-loop
	// controllers to distinguish a real state transition from a repeated
	// observation. It is evidence about the observation, never proof of goal
	// completion.
	Fingerprint string
	Source      string
}

// Action is an intentional computer interaction. Coordinates are optional:
// adapters may instead use semantic targets through Target.
type Action struct {
	ID       string
	Kind     ActionKind
	Target   string
	Text     string
	X, Y     int
	Button   string
	Key      string
	Duration time.Duration
}

type ActionKind string

const (
	Move        ActionKind = "move"
	Click       ActionKind = "click"
	DoubleClick ActionKind = "double_click"
	Type        ActionKind = "type"
	KeyPress    ActionKind = "key_press"
	Scroll      ActionKind = "scroll"
	Wait        ActionKind = "wait"
)

// Result is the physical outcome reported by the adapter. It is not proof of
// success; the controller must obtain a fresh observation and verify it.
type Result struct {
	ActionID string
	Success  bool
	Details  string
	Err      error
}

// Adapter is the boundary to an actual browser/desktop/mobile environment.
// It deliberately exposes observation separately from action execution.
type Adapter interface {
	Observe(context.Context) (Snapshot, error)
	Act(context.Context, Action) (Result, error)
}

// Verifier determines whether the observed state satisfies the requested
// condition. This keeps perception/evaluation above the physical adapter.
type Verifier interface {
	Verify(context.Context, Snapshot, Result) (bool, string)
}

// Controller performs the minimal closed loop:
// observe -> act -> observe -> verify.
// It never treats a successful input dispatch as proof that the goal happened.
type Controller struct {
	Adapter  Adapter
	Verifier Verifier
}

type Step struct {
	Action   Action
	Before   Snapshot
	Result   Result
	After    Snapshot
	Verified bool
	Reason   string
}

func (c *Controller) Execute(ctx context.Context, action Action) (Step, error) {
	if c == nil || c.Adapter == nil || c.Verifier == nil {
		return Step{}, fmt.Errorf("computeruse: controller requires adapter and verifier")
	}
	before, err := c.Adapter.Observe(ctx)
	if err != nil {
		return Step{}, fmt.Errorf("computeruse: pre-action observation: %w", err)
	}
	result, err := c.Adapter.Act(ctx, action)
	if err != nil {
		return Step{Action: action, Before: before, Result: result}, fmt.Errorf("computeruse: action %q: %w", action.ID, err)
	}
	after, err := c.Adapter.Observe(ctx)
	if err != nil {
		return Step{Action: action, Before: before, Result: result}, fmt.Errorf("computeruse: post-action observation: %w", err)
	}
	ok, reason := c.Verifier.Verify(ctx, after, result)
	return Step{Action: action, Before: before, Result: result, After: after, Verified: ok, Reason: reason}, nil
}
