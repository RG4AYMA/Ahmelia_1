package tools

import (
	"context"
	"fmt"
)

// Failure is the minimal, tool-layer description of a failed verified
// execution. It deliberately does not duplicate core's historical failure
// model: the tools package only knows what Execute -> Observe -> Verify
// actually established.
type Failure struct {
	ActionID string
	Reason   string
	Attempts int
}

// RecoveryAction is the next action requested by the recovery layer.
type RecoveryAction string

const (
	RecoveryRetry   RecoveryAction = "retry"
	RecoveryReplan  RecoveryAction = "replan"
	RecoveryBlock   RecoveryAction = "block"
	RecoveryAskUser RecoveryAction = "ask_user"
)

func (a RecoveryAction) Valid() bool {
	switch a {
	case RecoveryRetry, RecoveryReplan, RecoveryBlock, RecoveryAskUser:
		return true
	default:
		return false
	}
}

// FailureDiagnoser classifies a failed verified execution from evidence
// already produced by B94.5. It never executes or changes an Action.
type FailureDiagnoser interface {
	Diagnose(context.Context, VerifiedResult) (Failure, error)
}

// RecoveryDecider chooses what to do after diagnosis. It is policy only;
// applying Retry/Replan/Block belongs to the caller that owns the plan/task.
type RecoveryDecider interface {
	Decide(context.Context, Failure) RecoveryAction
}

// RecoveryDecision is the auditable output of B94.7.
type RecoveryDecision struct {
	Action RecoveryAction
	Reason string
}

// RecoveryCoordinator connects B94.5 evidence to a recovery decision without
// becoming a second executor or planner.
type RecoveryCoordinator struct {
	diagnoser FailureDiagnoser
	decider   RecoveryDecider
}

func NewRecoveryCoordinator(diagnoser FailureDiagnoser, decider RecoveryDecider) *RecoveryCoordinator {
	return &RecoveryCoordinator{diagnoser: diagnoser, decider: decider}
}

// Recover diagnoses a failed VerifiedResult and asks the recovery policy for
// exactly one next step. A successful VerifiedResult is never treated as a
// failure. Missing collaborators and invalid decisions fail closed.
func (c *RecoveryCoordinator) Recover(ctx context.Context, result VerifiedResult) (RecoveryDecision, error) {
	if err := ctx.Err(); err != nil {
		return RecoveryDecision{}, err
	}
	if c == nil || c.diagnoser == nil || c.decider == nil {
		return RecoveryDecision{}, fmt.Errorf("recovery coordinator: diagnoser/decider no configurados")
	}
	if result.Success {
		return RecoveryDecision{}, fmt.Errorf("recovery coordinator: no se puede recuperar una ejecución verificada como exitosa")
	}

	failure, err := c.diagnoser.Diagnose(ctx, result)
	if err != nil {
		return RecoveryDecision{}, fmt.Errorf("diagnóstico de fallo: %w", err)
	}
	if failure.ActionID == "" {
		failure.ActionID = result.ActionID
	}
	if failure.ActionID != result.ActionID {
		return RecoveryDecision{}, fmt.Errorf("diagnóstico: ActionID inconsistente: %q != %q", failure.ActionID, result.ActionID)
	}
	if failure.Attempts <= 0 {
		failure.Attempts = len(result.Attempts)
	}
	if failure.Reason == "" {
		failure.Reason = result.Reason
	}

	action := c.decider.Decide(ctx, failure)
	if !action.Valid() {
		return RecoveryDecision{}, fmt.Errorf("recovery coordinator: decisión inválida %q", action)
	}

	return RecoveryDecision{Action: action, Reason: failure.Reason}, nil
}

// DefaultFailureDiagnoser preserves evidence rather than guessing a cause.
// Cause classification can be supplied later by the B92 diagnostician.
type DefaultFailureDiagnoser struct{}

func (DefaultFailureDiagnoser) Diagnose(_ context.Context, result VerifiedResult) (Failure, error) {
	if result.Success {
		return Failure{}, fmt.Errorf("resultado exitoso")
	}
	attempts := 0
	if len(result.Attempts) > 0 {
		// Attempts is a history, not a count. A resumed/recovered work may have
		// one persisted latest attempt whose Number is greater than one.
		for _, attempt := range result.Attempts {
			if attempt.Number > attempts {
				attempts = attempt.Number
			}
		}
	}
	if attempts == 0 {
		attempts = len(result.Attempts)
	}
	return Failure{
		ActionID: result.ActionID,
		Reason:   result.Reason,
		Attempts: attempts,
	}, nil
}

// RetryThenReplan is a small policy adapter for callers that need a bounded
// fallback before escalating to the planner. It does not execute anything.
type RetryThenReplan struct {
	MaxRetries int
}

func (p RetryThenReplan) Decide(_ context.Context, failure Failure) RecoveryAction {
	if p.MaxRetries > 0 && failure.Attempts > 0 && failure.Attempts <= p.MaxRetries {
		return RecoveryRetry
	}
	return RecoveryReplan
}
