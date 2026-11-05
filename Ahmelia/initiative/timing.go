package initiative

import (
	"errors"
	"time"
)

// B96.37.8 — Initiative Timing.
//
// Timing answers only when an already identified conversational opportunity is
// temporally admissible. It does not decide whether Ahmelia should intervene,
// select an action, generate content, or own user-availability policy.
// B96.37.9+ may enrich the signals used here; B96.37.2 remains the decision
// authority.
type TimingAssessment struct {
	Timing           Timing    `json:"timing"`
	Admissible       bool      `json:"admissible"`
	Interruptible    bool      `json:"interruptible"`
	RequiresDeferral bool      `json:"requires_deferral"`
	Reason           string    `json:"reason,omitempty"`
	EvaluatedAt      time.Time `json:"evaluated_at"`
}

// TimingEvaluator is intentionally stateless. The same opportunity produces
// the same timing assessment for a given evaluation instant.
type TimingEvaluator struct{}

func NewTimingEvaluator() *TimingEvaluator { return &TimingEvaluator{} }

func (e *TimingEvaluator) Evaluate(op InitiativeOpportunity, now time.Time) TimingAssessment {
	if now.IsZero() {
		now = time.Now()
	}
	a := TimingAssessment{Timing: TimingNow, Admissible: true, EvaluatedAt: now}

	// An active cooldown or high conversational load is a temporal block, not
	// an initiative decision. The policy layer decides DEFER afterwards.
	if op.CooldownActive {
		return deferTiming(a, "cooldown_active")
	}
	if op.ConversationLoad == LoadSaturated || op.ConversationLoad == LoadHigh {
		return deferTiming(a, "conversation_load_high")
	}

	switch op.UserAvailability {
	case AvailabilityExplicitlyUnavailable, AvailabilityAway:
		return deferTiming(a, "user_unavailable")
	case AvailabilityBusy, AvailabilityDistracted:
		if op.Urgency == UrgencyCritical && op.Interruptibility == InterruptSafe {
			a.Timing = TimingInterrupt
			a.Interruptible = true
			return a
		}
		return deferTiming(a, "user_not_available_for_noncritical_turn")
	}

	if op.Interruptibility == InterruptForbidden && op.Urgency == UrgencyCritical {
		// Criticality does not override a hard interruption prohibition.
		return deferTiming(a, "interrupt_forbidden")
	}

	return a
}

func deferTiming(a TimingAssessment, reason string) TimingAssessment {
	a.Timing = TimingDefer
	a.Admissible = false
	a.RequiresDeferral = true
	a.Reason = reason
	return a
}

func ValidateTimingAssessment(a TimingAssessment) error {
	if !a.Timing.Valid() {
		return errors.New("invalid timing assessment")
	}
	if a.EvaluatedAt.IsZero() {
		return errors.New("timing assessment requires evaluation time")
	}
	if a.Timing == TimingInterrupt {
		if !a.Admissible || !a.Interruptible {
			return errors.New("interrupt timing requires admissible interruptibility")
		}
	}
	if a.RequiresDeferral && a.Admissible {
		return errors.New("deferred timing cannot be admissible")
	}
	return nil
}
