package initiative

import (
	"errors"
	"strings"
	"time"
)

// B96.37.12 — Interruption Policy.
// This layer answers only whether an already-authorized opportunity is
// interruptible. It never creates an InitiativeDecision and never promotes
// WAIT/DEFER into INTERVENE.
type InterruptionLevel string

const (
	InterruptionNone      InterruptionLevel = "NONE"
	InterruptionSafe      InterruptionLevel = "SAFE"
	InterruptionLimited   InterruptionLevel = "LIMITED"
	InterruptionHigh      InterruptionLevel = "HIGH"
	InterruptionCritical  InterruptionLevel = "CRITICAL"
	InterruptionForbidden InterruptionLevel = "FORBIDDEN"
)

type InterruptionModel string

const (
	InterruptionModelNormal   InterruptionModel = "NORMAL"
	InterruptionModelWarn     InterruptionModel = "WARN"
	InterruptionModelCritical InterruptionModel = "CRITICAL"
)

type InterruptibilityAssessment struct {
	Allowed              bool               `json:"allowed"`
	Level                InterruptionLevel  `json:"level"`
	Model                InterruptionModel  `json:"model"`
	Reason               string             `json:"reason"`
	BlockingConstraints  []string           `json:"blocking_constraints,omitempty"`
	RequiredUrgency      Urgency            `json:"required_urgency"`
	RequiredAvailability []UserAvailability `json:"required_availability,omitempty"`
	ChannelConstraint    string             `json:"channel_constraint,omitempty"`
	Timing               Timing             `json:"timing"`
	CooldownActive       bool               `json:"cooldown_active"`
	EvaluatedAt          time.Time          `json:"evaluated_at"`
	ExpiresAt            time.Time          `json:"expires_at,omitempty"`
}

func (a InterruptibilityAssessment) Validate() error {
	switch a.Level {
	case InterruptionNone, InterruptionSafe, InterruptionLimited, InterruptionHigh, InterruptionCritical, InterruptionForbidden:
	default:
		return errors.New("invalid B96.37.12 interruption level")
	}
	if !a.Timing.Valid() {
		return errors.New("invalid B96.37.12 timing")
	}
	if a.EvaluatedAt.IsZero() {
		return errors.New("B96.37.12 assessment requires evaluation time")
	}
	if a.Allowed && a.Level == InterruptionForbidden {
		return errors.New("forbidden interruption cannot be allowed")
	}
	if a.Allowed && a.Timing != TimingInterrupt {
		return errors.New("allowed interruption must expose interrupt timing")
	}
	if a.Timing == TimingInterrupt && !a.Allowed {
		return errors.New("interrupt timing requires allowed interruption")
	}
	return nil
}

type InterruptionPolicy struct {
	Version               string
	Cooldown              time.Duration
	AssessmentTTL         time.Duration
	AllowSafeWhileBusy    bool
	AllowLimitedWhileBusy bool
}

func NewInterruptionPolicy() *InterruptionPolicy {
	return &InterruptionPolicy{Version: "B96.37.12", Cooldown: 30 * time.Second, AssessmentTTL: 30 * time.Second, AllowSafeWhileBusy: false, AllowLimitedWhileBusy: false}
}

func validInterruptionLevel(v InterruptionLevel) bool {
	switch v {
	case InterruptionNone, InterruptionSafe, InterruptionLimited, InterruptionHigh, InterruptionCritical, InterruptionForbidden:
		return true
	default:
		return false
	}
}

func interruptionLevel(v Interruptibility) InterruptionLevel {
	switch v {
	case InterruptSafe:
		return InterruptionSafe
	case InterruptLimited:
		return InterruptionLimited
	case InterruptNotRecommended:
		return InterruptionHigh
	case InterruptForbidden:
		return InterruptionForbidden
	default:
		return InterruptionNone
	}
}

func normalizeAvailability(v UserAvailability) UserAvailability {
	switch strings.ToUpper(strings.TrimSpace(string(v))) {
	case string(AvailabilityAvailable):
		return AvailabilityAvailable
	case string(AvailabilityBusy):
		return AvailabilityBusy
	case string(AvailabilityDistracted):
		return AvailabilityDistracted
	case string(AvailabilityAway):
		return AvailabilityAway
	case string(AvailabilityExplicitlyUnavailable):
		return AvailabilityExplicitlyUnavailable
	default:
		return AvailabilityUnknown
	}
}

func (p *InterruptionPolicy) Evaluate(op InitiativeOpportunity, input SignalInput, now time.Time) InterruptibilityAssessment {
	if now.IsZero() {
		now = time.Now()
	}
	ttl := 30 * time.Second
	cooldown := 30 * time.Second
	allowSafeBusy, allowLimitedBusy := false, false
	if p != nil {
		if p.AssessmentTTL > 0 {
			ttl = p.AssessmentTTL
		}
		if p.Cooldown > 0 {
			cooldown = p.Cooldown
		}
		allowSafeBusy, allowLimitedBusy = p.AllowSafeWhileBusy, p.AllowLimitedWhileBusy
	}

	a := InterruptibilityAssessment{
		Level: interruptionLevel(op.Interruptibility), Model: InterruptionModelNormal,
		RequiredUrgency: UrgencyCritical, Timing: TimingDefer, EvaluatedAt: now, ExpiresAt: now.Add(ttl),
		ChannelConstraint: input.Channel,
	}
	a.CooldownActive = op.CooldownActive
	if a.CooldownActive {
		a.BlockingConstraints = append(a.BlockingConstraints, "cooldown_active")
		a.Reason = "cooldown_active"
		a.Level = InterruptionForbidden
		return a
	}

	if op.Interruptibility == InterruptForbidden {
		a.BlockingConstraints = append(a.BlockingConstraints, "explicit_interruption_forbidden")
		a.Reason = "explicit_interruption_forbidden"
		a.Level = InterruptionForbidden
		return a
	}
	if op.Privacy == InformationPrivate || op.Privacy == InformationSensitive || op.Privacy == InformationProhibited {
		a.BlockingConstraints = append(a.BlockingConstraints, "privacy_constraint")
		a.Reason = "privacy_constraint"
		a.Level = InterruptionForbidden
		return a
	}
	if input.Channel == "" || strings.EqualFold(input.Channel, "unknown") {
		a.BlockingConstraints = append(a.BlockingConstraints, "channel_unknown")
		a.Reason = "channel_unknown"
		a.Level = InterruptionForbidden
		return a
	}
	if input.Enrichment.Channel.Channel != "" && !input.Enrichment.Channel.Supported {
		a.BlockingConstraints = append(a.BlockingConstraints, "channel_unsupported")
		a.Reason = "channel_unsupported"
		a.Level = InterruptionForbidden
		return a
	}

	availability := normalizeAvailability(op.UserAvailability)
	switch availability {
	case AvailabilityExplicitlyUnavailable:
		a.BlockingConstraints = append(a.BlockingConstraints, "explicitly_unavailable")
	case AvailabilityAway:
		a.BlockingConstraints = append(a.BlockingConstraints, "user_away")
	case AvailabilityUnknown:
		a.BlockingConstraints = append(a.BlockingConstraints, "availability_unknown")
	case AvailabilityBusy:
		a.RequiredAvailability = []UserAvailability{AvailabilityAvailable, AvailabilityBusy}
	case AvailabilityDistracted:
		a.RequiredAvailability = []UserAvailability{AvailabilityAvailable}
	default:
		a.RequiredAvailability = []UserAvailability{AvailabilityAvailable}
	}
	if len(a.BlockingConstraints) > 0 {
		a.Reason = a.BlockingConstraints[0]
		return a
	}

	if op.Urgency != UrgencyCritical {
		a.Reason = "critical_urgency_required_for_interrupt"
		a.BlockingConstraints = append(a.BlockingConstraints, "urgency_below_critical")
		return a
	}

	switch availability {
	case AvailabilityBusy:
		if op.Interruptibility == InterruptLimited && !allowLimitedBusy {
			a.Reason = "busy_user_limited_interruption"
			a.BlockingConstraints = append(a.BlockingConstraints, "busy_user")
			return a
		}
		if op.Interruptibility == InterruptSafe && !allowSafeBusy {
			a.Reason = "busy_user_safe_interruption_disabled"
			a.BlockingConstraints = append(a.BlockingConstraints, "busy_user")
			return a
		}
	case AvailabilityDistracted:
		if op.Interruptibility != InterruptSafe {
			a.Reason = "distracted_user_requires_safe_interruption"
			a.BlockingConstraints = append(a.BlockingConstraints, "distracted_user")
			return a
		}
	}

	switch op.Interruptibility {
	case InterruptSafe:
		a.Level = InterruptionSafe
		a.Model = InterruptionModelNormal
	case InterruptLimited:
		a.Level = InterruptionLimited
		a.Model = InterruptionModelWarn
	case InterruptNotRecommended:
		a.Level = InterruptionHigh
		a.Model = InterruptionModelWarn
	default:
		a.Level = InterruptionSafe
	}
	if op.Urgency == UrgencyCritical && op.Reason == ReasonWarning {
		a.Model = InterruptionModelCritical
		a.Level = InterruptionCritical
	}
	a.Allowed = true
	a.Timing = TimingInterrupt
	a.Reason = "interruption_allowed_by_policy"
	a.ExpiresAt = now.Add(cooldown)
	return a
}

// EnrichOpportunity applies only interruption metadata. It does not create a decision.
func (p *InterruptionPolicy) EnrichOpportunity(op InitiativeOpportunity, input SignalInput, now time.Time) (InitiativeOpportunity, InterruptibilityAssessment, error) {
	a := p.Evaluate(op, input, now)
	if err := a.Validate(); err != nil {
		return op, a, err
	}
	if a.Allowed {
		switch a.Level {
		case InterruptionCritical, InterruptionHigh, InterruptionSafe:
			op.Interruptibility = InterruptSafe
		case InterruptionLimited:
			op.Interruptibility = InterruptLimited
		}
	} else if len(a.BlockingConstraints) > 0 {
		op.Interruptibility = InterruptForbidden
	}
	return op, a, nil
}

// LegacyInterruptionMigration maps legacy interruptibility values without changing decision authority.
func LegacyInterruptionMigration(v Interruptibility) (Interruptibility, bool) {
	switch strings.ToUpper(strings.TrimSpace(string(v))) {
	case "SAFE":
		return InterruptSafe, false
	case "LIMITED":
		return InterruptLimited, false
	case "NOT_RECOMMENDED", "NOT-RECOMMENDED":
		return InterruptNotRecommended, false
	case "FORBIDDEN":
		return InterruptForbidden, false
	case "ALLOWED":
		return InterruptSafe, true
	case "BLOCKED", "DENIED":
		return InterruptForbidden, true
	default:
		return InterruptNotRecommended, true
	}
}

// AssessInterruption is the public contract entry point.
func AssessInterruption(op InitiativeOpportunity, input SignalInput, now time.Time) (InterruptibilityAssessment, error) {
	return NewInterruptionPolicy().Evaluate(op, input, now), nil
}
