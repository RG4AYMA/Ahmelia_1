package initiative

import (
	"errors"
	"time"
)

// B96.37.9 — User Availability.
//
// Availability is a policy-owned interpretation of already observed evidence.
// It does not create initiative decisions, select actions, or infer private
// facts. Explicit availability always outranks inference from activity.
type AvailabilityEvidence struct {
	Explicit          UserAvailability `json:"explicit"`
	LastUserActivity  time.Time        `json:"last_user_activity"`
	CurrentTurnActive bool             `json:"current_turn_active"`
	WaitingForUser    bool             `json:"waiting_for_user"`
	Now               time.Time        `json:"now"`
}

type AvailabilityAssessment struct {
	Availability UserAvailability `json:"availability"`
	Confidence   float64          `json:"confidence"`
	Inferred     bool             `json:"inferred"`
	Reason       string           `json:"reason"`
	ObservedAt   time.Time        `json:"observed_at"`
}

type AvailabilityPolicy struct {
	ActiveWindow time.Duration
	// Belief (opcional) estima alcanzabilidad cuando la actividad ya no es
	// reciente. Nil conserva el comportamiento histórico (UNKNOWN).
	Belief *PresenceBelief
}

func NewAvailabilityPolicy() *AvailabilityPolicy {
	return &AvailabilityPolicy{ActiveWindow: 2 * time.Minute, Belief: NewPresenceBelief()}
}

func (p *AvailabilityPolicy) Evaluate(e AvailabilityEvidence) AvailabilityAssessment {
	now := e.Now
	if now.IsZero() {
		now = time.Now()
	}
	window := 2 * time.Minute
	if p != nil && p.ActiveWindow > 0 {
		window = p.ActiveWindow
	}

	if e.Explicit != AvailabilityUnknown && e.Explicit != "" {
		return AvailabilityAssessment{Availability: e.Explicit, Confidence: 1, Reason: "explicit_availability", ObservedAt: now}
	}
	if e.CurrentTurnActive {
		return AvailabilityAssessment{Availability: AvailabilityAvailable, Confidence: .99, Inferred: true, Reason: "active_user_turn", ObservedAt: now}
	}
	if e.WaitingForUser {
		return AvailabilityAssessment{Availability: AvailabilityAvailable, Confidence: .9, Inferred: true, Reason: "execution_waiting_for_user", ObservedAt: now}
	}
	if !e.LastUserActivity.IsZero() {
		age := now.Sub(e.LastUserActivity)
		if age >= 0 && age <= window {
			return AvailabilityAssessment{Availability: AvailabilityAvailable, Confidence: .8, Inferred: true, Reason: "recent_user_activity", ObservedAt: now}
		}
	}
	if p != nil && p.Belief != nil {
		if a, ok := p.Belief.Assess(AvailabilityEvidence{LastUserActivity: e.LastUserActivity, Now: now}); ok {
			return a
		}
	}
	return AvailabilityAssessment{Availability: AvailabilityUnknown, Confidence: 0, Inferred: true, Reason: "availability_not_observed", ObservedAt: now}
}

func ValidateAvailabilityAssessment(a AvailabilityAssessment) error {
	switch a.Availability {
	case AvailabilityAvailable, AvailabilityBusy, AvailabilityDistracted, AvailabilityAway, AvailabilityUnknown, AvailabilityExplicitlyUnavailable:
	default:
		return errors.New("invalid user availability")
	}
	if a.Confidence < 0 || a.Confidence > 1 {
		return errors.New("availability confidence must be between 0 and 1")
	}
	if a.ObservedAt.IsZero() {
		return errors.New("availability assessment requires observation time")
	}
	if a.Reason == "" {
		return errors.New("availability assessment requires reason")
	}
	return nil
}
