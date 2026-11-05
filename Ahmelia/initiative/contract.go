// Package initiative defines the canonical B96.37 conversational initiative
// contract. It decides whether a conversational opportunity may become a
// turn, but it does not generate language, execute work, or own privacy policy.
package initiative

import (
	"errors"
	"strings"
	"time"
)

type Decision string

const (
	Intervene Decision = "INTERVENE"
	Wait      Decision = "WAIT"
	Defer     Decision = "DEFER"
)

type Source string

const (
	SourceConversationalContext Source = "conversational_context"
	SourceOperationalWork       Source = "operational_work"
	SourceUserModel             Source = "user_model"
	SourceOpenCommitment        Source = "open_commitment"
	SourceAutonomousSignal      Source = "autonomous_system_signal"
	SourceInteractionContinuity Source = "interaction_continuity"
	SourceDeferredLinguistic    Source = "deferred_linguistic"
	SourceCognitionConflict     Source = "cognition_conflict"
)

type Reason string

const (
	ReasonClarificationNeeded              Reason = "clarification_needed"
	ReasonPendingTask                      Reason = "pending_task"
	ReasonUnfinishedCommitment             Reason = "unfinished_commitment"
	ReasonUsefulInformation                Reason = "useful_information"
	ReasonRelevantDiscovery                Reason = "relevant_discovery"
	ReasonPotentialProblem                 Reason = "potential_problem"
	ReasonWarning                          Reason = "warning"
	ReasonNextStepAvailable                Reason = "next_step_available"
	ReasonConversationContinue             Reason = "conversation_continuation"
	ReasonPausedThreadAvailable            Reason = "paused_thread_available"
	ReasonAlternativeAvailable             Reason = "alternative_available"
	ReasonNewPossibility                   Reason = "new_possibility"
	ReasonRelevantComment                  Reason = "relevant_comment"
	ReasonTopicRedirection                 Reason = "topic_redirection"
	ReasonContradictsAuthoritativeEvidence Reason = "contradicts_authoritative_evidence"
)

type Action string

const (
	ActionAsk                Action = "ASK"
	ActionSuggest            Action = "SUGGEST"
	ActionPropose            Action = "PROPOSE"
	ActionResume             Action = "RESUME"
	ActionComment            Action = "COMMENT"
	ActionWarn               Action = "WARN"
	ActionContribute         Action = "CONTRIBUTE"
	ActionRemind             Action = "REMIND"
	ActionShareDiscovery     Action = "SHARE_DISCOVERY"
	ActionPresentPossibility Action = "PRESENT_POSSIBILITY"
	ActionContinue           Action = "CONTINUE"
	ActionRedirect           Action = "REDIRECT"
	ActionDisagree           Action = "DISAGREE"
)

type Timing string

const (
	TimingNow       Timing = "NOW"
	TimingWait      Timing = "WAIT"
	TimingDefer     Timing = "DEFERRED"
	TimingInterrupt Timing = "INTERRUPT"
)

type Urgency string

const (
	UrgencyNone     Urgency = "NONE"
	UrgencyLow      Urgency = "LOW"
	UrgencyMedium   Urgency = "MEDIUM"
	UrgencyHigh     Urgency = "HIGH"
	UrgencyCritical Urgency = "CRITICAL"
)

type Priority string

const (
	PriorityLow      Priority = "LOW"
	PriorityMedium   Priority = "MEDIUM"
	PriorityNormal   Priority = PriorityMedium // deprecated: B96.37.10 canonicalizes NORMAL to MEDIUM
	PriorityHigh     Priority = "HIGH"
	PriorityCritical Priority = "CRITICAL"
)

type UserAvailability string

const (
	AvailabilityAvailable             UserAvailability = "AVAILABLE"
	AvailabilityBusy                  UserAvailability = "BUSY"
	AvailabilityDistracted            UserAvailability = "DISTRACTED"
	AvailabilityAway                  UserAvailability = "AWAY"
	AvailabilityUnknown               UserAvailability = "UNKNOWN"
	AvailabilityExplicitlyUnavailable UserAvailability = "EXPLICITLY_UNAVAILABLE"
)

type Interruptibility string

const (
	InterruptSafe           Interruptibility = "SAFE"
	InterruptLimited        Interruptibility = "LIMITED"
	InterruptNotRecommended Interruptibility = "NOT_RECOMMENDED"
	InterruptForbidden      Interruptibility = "FORBIDDEN"
)

type ConversationLoad string

const (
	LoadLow       ConversationLoad = "LOW"
	LoadNormal    ConversationLoad = "NORMAL"
	LoadHigh      ConversationLoad = "HIGH"
	LoadSaturated ConversationLoad = "SATURATED"
)

type InformationPolicy string

const (
	InformationAllowed    InformationPolicy = "allowed"
	InformationRestricted InformationPolicy = "restricted"
	InformationPrivate    InformationPolicy = "private"
	InformationSensitive  InformationPolicy = "sensitive"
	InformationProhibited InformationPolicy = "prohibited"
)

type InitiativeConstraint struct {
	Code   string `json:"code"`
	Value  string `json:"value,omitempty"`
	Active bool   `json:"active"`
}

type Provenance struct {
	Component  string    `json:"component,omitempty"`
	Subsystem  string    `json:"subsystem,omitempty"`
	SignalType string    `json:"signal_type,omitempty"`
	OriginID   string    `json:"origin_id,omitempty"`
	CreatedAt  time.Time `json:"created_at,omitempty"`
}

type InitiativeSignal struct {
	ID          string     `json:"id,omitempty"`
	Type        string     `json:"type,omitempty"`
	Value       string     `json:"value,omitempty"`
	Confidence  float64    `json:"confidence"`
	ObservedAt  time.Time  `json:"observed_at,omitempty"`
	Source      Source     `json:"source"`
	Reason      Reason     `json:"reason"`
	Relevant    bool       `json:"relevant"`
	Priority    Priority   `json:"priority"`
	Urgency     Urgency    `json:"urgency"`
	Target      string     `json:"target,omitempty"`
	Provenance  Provenance `json:"provenance"`
	EvidenceIDs []string   `json:"evidence_ids,omitempty"`
	ExpiresAt   time.Time  `json:"expires_at,omitempty"`
}

type InitiativeOpportunity struct {
	ID                 string             `json:"id,omitempty"`
	Exists             bool               `json:"exists"`
	Source             Source             `json:"source"`
	Reason             Reason             `json:"reason"`
	Relevance          float64            `json:"relevance"`
	Priority           Priority           `json:"priority"`
	Urgency            Urgency            `json:"urgency"`
	UserAvailability   UserAvailability   `json:"user_availability"`
	InteractionState   string             `json:"interaction_state,omitempty"`
	Privacy            InformationPolicy  `json:"privacy"`
	Sensitivity        string             `json:"sensitivity,omitempty"`
	Channel            string             `json:"channel,omitempty"`
	Interruptibility   Interruptibility   `json:"interruptibility"`
	ConversationLoad   ConversationLoad   `json:"conversation_load"`
	Context            string             `json:"context,omitempty"`
	Target             string             `json:"target,omitempty"`
	Signal             InitiativeSignal   `json:"signal"`
	Signals            []InitiativeSignal `json:"signals,omitempty"`
	ExpiresAt          time.Time          `json:"expires_at,omitempty"`
	RequiresDirectTurn bool               `json:"requires_direct_turn"`
	UserNeedsFocus     bool               `json:"user_needs_focus"`
	CooldownActive     bool               `json:"cooldown_active"`
}

type InitiativeDecision struct {
	ID            string                 `json:"id"`
	Decision      Decision               `json:"decision"`
	Opportunity   InitiativeOpportunity  `json:"opportunity"`
	Action        Action                 `json:"action,omitempty"`
	Source        Source                 `json:"source"`
	Reason        Reason                 `json:"reason"`
	Target        string                 `json:"target,omitempty"`
	Timing        Timing                 `json:"timing"`
	Priority      Priority               `json:"priority"`
	Urgency       Urgency                `json:"urgency"`
	Constraints   []InitiativeConstraint `json:"constraints,omitempty"`
	Provenance    Provenance             `json:"provenance"`
	Confidence    float64                `json:"confidence"`
	CreatedAt     time.Time              `json:"created_at"`
	ExpiresAt     time.Time              `json:"expires_at,omitempty"`
	PolicyVersion string                 `json:"policy_version"`
}

func (d Decision) Valid() bool { return d == Intervene || d == Wait || d == Defer }
func (a Action) Valid() bool {
	switch a {
	case ActionAsk, ActionSuggest, ActionPropose, ActionResume, ActionComment, ActionWarn, ActionContribute, ActionRemind, ActionShareDiscovery, ActionPresentPossibility, ActionContinue, ActionRedirect, ActionDisagree:
		return true
	}
	return false
}
func (t Timing) Valid() bool {
	return t == TimingNow || t == TimingWait || t == TimingDefer || t == TimingInterrupt
}

func newID(now time.Time) string { return now.UTC().Format("20060102T150405.000000000Z07:00") }

// Policy evaluates an already-built opportunity. Signal collection and content
// generation remain outside this package's authority.
type Policy struct {
	Version          string
	MinimumRelevance float64
	Timing           *TimingEvaluator // B96.37.8 — temporal assessment only
}

func DefaultPolicy() Policy {
	return Policy{Version: "B96.37.2", MinimumRelevance: 0.45, Timing: NewTimingEvaluator()}
}

func (p Policy) Evaluate(op InitiativeOpportunity, now time.Time) InitiativeDecision {
	if now.IsZero() {
		now = time.Now()
	}
	if p.Version == "" {
		p.Version = "B96.37.2"
	}
	if p.MinimumRelevance <= 0 {
		p.MinimumRelevance = 0.45
	}
	d := InitiativeDecision{ID: newID(now), Opportunity: op, Source: op.Source, Reason: op.Reason, Target: op.Target, Priority: op.Priority, Urgency: op.Urgency, CreatedAt: now, ExpiresAt: op.ExpiresAt, PolicyVersion: p.Version, Confidence: clamp(op.Relevance)}

	if op.Reason == ReasonContradictsAuthoritativeEvidence {
		d.Provenance.Component = "initiative.B96.37.2"
		d.Provenance.Subsystem = "B96.37.2"
		d.Provenance.SignalType = op.Signal.Type
		d.Provenance.OriginID = strings.Join(op.Signal.EvidenceIDs, ",")
		d.Constraints = append(d.Constraints, InitiativeConstraint{Code: "disagreement_minimum_risk", Value: "medium", Active: true})
		// An already authorized/executable order owns the current turn. G2's
		// disagreement capability is deliberately fail-closed here rather than
		// trying to reopen a decision already accepted by the execution boundary.
		if op.InteractionState == "action" || op.InteractionState == "production" {
			d.Constraints = append(d.Constraints, InitiativeConstraint{Code: "disagreement_blocked_authorized_order", Value: op.InteractionState, Active: true})
			d.Decision, d.Timing = Wait, TimingWait
			return d
		}
	}
	if op.RequiresDirectTurn || op.UserNeedsFocus {
		d.Decision, d.Timing = Wait, TimingWait
		return d
	}
	if !op.Exists || !op.Signal.Relevant || op.Relevance < p.MinimumRelevance {
		d.Decision, d.Timing = Wait, TimingWait
		return d
	}
	if op.Privacy == InformationProhibited || op.Privacy == InformationPrivate {
		d.Decision, d.Timing = Defer, TimingDefer
		d.Constraints = []InitiativeConstraint{{Code: "disclosure_not_authorized", Active: true}}
		return d
	}
	if op.Privacy == InformationSensitive || op.Privacy == InformationRestricted {
		d.Constraints = append(d.Constraints, InitiativeConstraint{Code: "do_not_expose_sensitive_data", Active: true})
	}
	timingEvaluator := p.Timing
	if timingEvaluator == nil {
		timingEvaluator = NewTimingEvaluator()
	}
	timing := timingEvaluator.Evaluate(op, now)
	if err := ValidateTimingAssessment(timing); err != nil {
		d.Decision, d.Timing = Wait, TimingWait
		d.Constraints = append(d.Constraints, InitiativeConstraint{Code: "timing_invalid", Active: true})
		return d
	}
	if !timing.Admissible {
		d.Decision, d.Timing = Defer, timing.Timing
		return d
	}
	d.Decision, d.Timing = Intervene, timing.Timing
	return d
}

func clamp(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func ValidateDecision(d InitiativeDecision) error {
	if !d.Decision.Valid() {
		return errors.New("invalid initiative decision")
	}
	if d.Decision == Intervene && !d.Action.Valid() {
		return errors.New("intervention requires a valid action")
	}
	if d.Decision == Intervene && d.Action == ActionDisagree {
		if d.Reason != ReasonContradictsAuthoritativeEvidence || len(d.Opportunity.Signal.EvidenceIDs) < 2 {
			return errors.New("DISAGREE requires authoritative contradiction evidence ids")
		}
	}
	if d.Decision == Wait && d.Timing != TimingWait {
		return errors.New("WAIT requires WAIT timing")
	}
	if d.Decision == Defer && d.Timing != TimingDefer {
		return errors.New("DEFER requires DEFERRED timing")
	}
	if d.Timing == TimingInterrupt && d.Decision != Intervene {
		return errors.New("INTERRUPT requires INTERVENE")
	}
	if d.Timing == TimingInterrupt && d.Urgency != UrgencyCritical {
		return errors.New("INTERRUPT requires CRITICAL urgency")
	}
	if d.ExpiresAt.IsZero() == false && !d.ExpiresAt.After(d.CreatedAt) {
		return errors.New("expiration must be after creation")
	}
	return nil
}

func ReevaluateDeferred(d InitiativeDecision, op InitiativeOpportunity, now time.Time) InitiativeDecision {
	if d.Decision != Defer {
		return d
	}
	if !d.ExpiresAt.IsZero() && !now.Before(d.ExpiresAt) {
		return InitiativeDecision{ID: newID(now), Decision: Wait, Timing: TimingWait, Reason: ReasonRelevantComment, PolicyVersion: d.PolicyVersion, CreatedAt: now}
	}
	return DefaultPolicy().Evaluate(op, now)
}

func NormalizeText(s string) string { return strings.TrimSpace(s) }
