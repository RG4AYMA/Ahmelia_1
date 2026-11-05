package computeruse

import (
	"context"
	"fmt"
)

// ActionPlanner chooses the next physical action from the latest observation.
// Cognition remains above this boundary; the planner can be backed by a model,
// deterministic policy, or another agent component without changing the loop.
type ActionPlanner interface {
	NextAction(context.Context, Snapshot) (Action, bool, error)
}

// ProgressVerifier determines whether the goal is reached and, when it is not,
// whether the new observation represents meaningful progress. Keeping these
// decisions separate prevents a successful input dispatch from becoming a
// false completion signal.
type ProgressVerifier interface {
	Verify(context.Context, Snapshot) (done bool, reason string)
	Progress(context.Context, Snapshot, Snapshot) (progressed bool, reason string)
}

// LoopConfig bounds a computer-use session. The loop is deliberately finite:
// an external adapter, planner, or verifier must never be able to make
// Ahmelia spin forever on an unchanged screen.
type LoopConfig struct {
	MaxSteps int
}

// LoopResult is the complete observable execution trace of one bounded
// computer-use objective.
type LoopResult struct {
	Steps      []Step
	Completed  bool
	StopReason string
}

// ExecuteGoal runs the canonical closed loop:
// observe -> decide -> act -> observe -> verify -> repeat.
// It stops on success, planner exhaustion, context cancellation, lack of
// progress, or the configured step bound.
func ExecuteGoal(ctx context.Context, adapter Adapter, planner ActionPlanner, verifier ProgressVerifier, cfg LoopConfig) (LoopResult, error) {
	if adapter == nil || planner == nil || verifier == nil {
		return LoopResult{}, fmt.Errorf("computeruse: goal loop requires adapter, planner and verifier")
	}
	if cfg.MaxSteps <= 0 {
		return LoopResult{}, fmt.Errorf("computeruse: MaxSteps must be greater than zero")
	}

	result := LoopResult{Steps: make([]Step, 0, cfg.MaxSteps)}
	current, err := adapter.Observe(ctx)
	if err != nil {
		return result, fmt.Errorf("computeruse: initial observation: %w", err)
	}
	if done, reason := verifier.Verify(ctx, current); done {
		result.Completed = true
		result.StopReason = reason
		return result, nil
	}

	for i := 0; i < cfg.MaxSteps; i++ {
		if err := ctx.Err(); err != nil {
			result.StopReason = err.Error()
			return result, err
		}

		action, ok, err := planner.NextAction(ctx, current)
		if err != nil {
			return result, fmt.Errorf("computeruse: planner: %w", err)
		}
		if !ok {
			result.StopReason = "planner has no action"
			return result, nil
		}

		before := current
		physical, err := adapter.Act(ctx, action)
		if err != nil {
			result.Steps = append(result.Steps, Step{Action: action, Before: before, Result: physical})
			return result, fmt.Errorf("computeruse: action %q: %w", action.ID, err)
		}
		after, err := adapter.Observe(ctx)
		if err != nil {
			result.Steps = append(result.Steps, Step{Action: action, Before: before, Result: physical})
			return result, fmt.Errorf("computeruse: observation after action %q: %w", action.ID, err)
		}

		done, reason := verifier.Verify(ctx, after)
		progressed, progressReason := verifier.Progress(ctx, before, after)
		step := Step{Action: action, Before: before, Result: physical, After: after, Verified: done, Reason: reason}
		result.Steps = append(result.Steps, step)
		if done {
			result.Completed = true
			result.StopReason = reason
			return result, nil
		}
		if !progressed {
			result.StopReason = "no meaningful progress: " + progressReason
			return result, nil
		}
		current = after
	}

	result.StopReason = "maximum computer-use steps reached"
	return result, nil
}
