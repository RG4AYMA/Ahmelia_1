package initiative

import (
	"sort"
	"strings"

	"ahmelia/situation"
	"time"
)

// SignalInput is the normalized observation boundary for B96.37.3. Upstream
// systems may provide rich domain objects; adapters reduce them to this input
// without transferring ownership of those systems to initiative.
type SignalInput struct {
	Now                 time.Time
	RequestKind         string
	ActiveTopic         string
	ActiveObjective     string
	ActiveWorkID        string
	LastUserInput       string
	LastAssistant       string
	TurnCount           int
	OpenThreadCount     int
	CommitmentCount     int
	CandidateExists     bool
	CandidateTopic      string
	CandidateText       string
	CandidateConfidence float64
	Channel             string
	HistoricalAvailable bool
	HistoricalRequired  bool
	UserUrgency         float64
	UserFrustration     float64
	UserConfusion       float64
	UserAvailability    UserAvailability
	Privacy             InformationPolicy
	Interruptibility    Interruptibility
	ConversationLoad    ConversationLoad
	RequiresDirectTurn  bool
	CooldownActive      bool
	// Enrichment is a read-only projection of upstream facts prepared by B96.37.5.
	// It never contains or authorizes an initiative decision.
	Enrichment         InitiativeSourceEnrichment
	DecisionEvidence   situation.DecisionEvidence
	DeferredLinguistic []DeferredLinguisticEvidence
}

// DeferredLinguisticEvidence is a completed result from the same linguistic
// interpreter used by the live interaction path. It is evidence for B96.37; it
// carries no authorization and cannot emit a conversational turn.
type DeferredLinguisticEvidence struct {
	JobID       string
	RawInput    string
	Meaning     string
	Confidence  float64
	CompletedAt time.Time
}

// SignalSource is an adapter boundary. It observes one concern and never
// authorizes an initiative decision.
type SignalSource interface {
	Name() string
	Collect(SignalInput) []InitiativeSignal
}

func signalID(now time.Time, source, typ string) string {
	return now.UTC().Format("20060102T150405.000000000Z07:00") + ":" + source + ":" + typ
}

func makeSignal(in SignalInput, source Source, reason Reason, typ, value, target string, relevant bool, confidence float64) InitiativeSignal {
	now := in.Now
	if now.IsZero() {
		now = time.Now()
	}
	return InitiativeSignal{
		ID: signalID(now, string(source), typ), Type: typ, Value: value,
		Source: source, Reason: reason, Relevant: relevant,
		Priority: PriorityNormal, Urgency: UrgencyNone, Target: target,
		Confidence: clamp(confidence), ObservedAt: now,
		Provenance: Provenance{Component: "initiative." + typ, Subsystem: "B96.37.3", SignalType: typ, CreatedAt: now},
	}
}

type CandidateSignalSource struct{}

func (CandidateSignalSource) Name() string { return "candidate" }
func (CandidateSignalSource) Collect(in SignalInput) []InitiativeSignal {
	if !in.CandidateExists {
		return nil
	}
	s := makeSignal(in, SourceConversationalContext, ReasonConversationContinue, "question_candidate", in.CandidateText, in.CandidateTopic, true, in.CandidateConfidence)
	s.Provenance.Component = "interaction.QuestionGenerator"
	s.Provenance.Subsystem = "B96.22.21"
	return []InitiativeSignal{s}
}

type ConversationSignalSource struct{}

func (ConversationSignalSource) Name() string { return "conversation" }
func (ConversationSignalSource) Collect(in SignalInput) []InitiativeSignal {
	var out []InitiativeSignal
	if strings.TrimSpace(in.ActiveTopic) != "" || strings.TrimSpace(in.ActiveObjective) != "" {
		target := in.ActiveTopic
		if target == "" {
			target = in.ActiveObjective
		}
		out = append(out, makeSignal(in, SourceConversationalContext, ReasonConversationContinue, "active_context", target, target, true, 0.7))
	}
	if in.OpenThreadCount > 0 {
		out = append(out, makeSignal(in, SourceInteractionContinuity, ReasonPausedThreadAvailable, "open_thread", "open_thread_available", in.ActiveTopic, true, 0.8))
	}
	if in.CommitmentCount > 0 {
		out = append(out, makeSignal(in, SourceOpenCommitment, ReasonUnfinishedCommitment, "open_commitment", "unfinished_commitment", in.ActiveTopic, true, 0.85))
	}
	return out
}

type WorkSignalSource struct{}

func (WorkSignalSource) Name() string { return "work" }
func (WorkSignalSource) Collect(in SignalInput) []InitiativeSignal {
	if strings.TrimSpace(in.ActiveWorkID) == "" {
		return nil
	}
	return []InitiativeSignal{makeSignal(in, SourceOperationalWork, ReasonNextStepAvailable, "active_work", in.ActiveWorkID, in.ActiveTopic, true, 0.75)}
}

type UserStateSignalSource struct{}

func (UserStateSignalSource) Name() string { return "user_state" }
func (UserStateSignalSource) Collect(in SignalInput) []InitiativeSignal {
	max := in.UserUrgency
	typ, value := "user_state", "normal"
	if in.UserUrgency > 0.65 {
		typ, value = "user_focus_needed", "urgent"
		max = in.UserUrgency
	}
	if in.UserFrustration > max {
		max = in.UserFrustration
		typ, value = "user_focus_needed", "frustrated"
	}
	if in.UserConfusion > max {
		max = in.UserConfusion
		typ, value = "user_focus_needed", "confused"
	}
	if max <= 0.65 {
		return nil
	}
	s := makeSignal(in, SourceInteractionContinuity, ReasonClarificationNeeded, typ, value, in.ActiveTopic, true, max)
	s.Priority = PriorityHigh
	if max >= 0.9 {
		s.Urgency = UrgencyHigh
	}
	return []InitiativeSignal{s}
}

// DefaultSignalSources deliberately contains adapters only. New domain
// signals should be added here through adapters rather than by expanding the
// Engine's orchestration logic.
func DefaultSignalSources() []SignalSource {
	return []SignalSource{CandidateSignalSource{}, ConversationSignalSource{}, WorkSignalSource{}, UserStateSignalSource{}, DeferredLinguisticSignalSource{}}
}

type DeferredLinguisticSignalSource struct{}

func (DeferredLinguisticSignalSource) Name() string { return "deferred_linguistic" }
func (DeferredLinguisticSignalSource) Collect(in SignalInput) []InitiativeSignal {
	var out []InitiativeSignal
	for _, item := range in.DeferredLinguistic {
		if strings.TrimSpace(item.JobID) == "" {
			continue
		}
		value := "resolution_ready"
		if strings.TrimSpace(item.Meaning) != "" {
			value = strings.TrimSpace(item.Meaning)
		}
		s := makeSignal(in, SourceDeferredLinguistic, ReasonUsefulInformation, "deferred_linguistic_resolution", value, item.JobID, true, item.Confidence)
		s.Priority = PriorityHigh
		s.ObservedAt = item.CompletedAt
		s.Provenance.Component = "linguistics.DeferredResolver"
		s.Provenance.Subsystem = "B96.39.35"
		s.Provenance.OriginID = item.JobID
		out = append(out, s)
	}
	return out
}

// NormalizeSignals removes unusable observations, clamps confidence, and
// copies provenance metadata needed for auditability. It does not choose a
// decision or discard meaningful negative signals.
func NormalizeSignals(signals []InitiativeSignal, now time.Time) []InitiativeSignal {
	if now.IsZero() {
		now = time.Now()
	}
	out := make([]InitiativeSignal, 0, len(signals))
	for _, s := range signals {
		if s.Source == "" || s.Reason == "" {
			continue
		}
		s.Confidence = clamp(s.Confidence)
		if s.ObservedAt.IsZero() {
			s.ObservedAt = now
		}
		if s.Provenance.CreatedAt.IsZero() {
			s.Provenance.CreatedAt = s.ObservedAt
		}
		if s.Provenance.SignalType == "" {
			s.Provenance.SignalType = s.Type
		}
		out = append(out, s)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].ObservedAt.Before(out[j].ObservedAt) })
	return out
}

// AggregateSignals keeps all evidence while selecting one primary signal for
// the legacy Opportunity.Signal field. No decision semantics live here.
func AggregateSignals(signals []InitiativeSignal) []InitiativeSignal {
	out := append([]InitiativeSignal(nil), signals...)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Relevant != out[j].Relevant {
			return out[i].Relevant
		}
		return out[i].Confidence > out[j].Confidence
	})
	return out
}

// ResolveSignalConflicts handles observation-level conflicts only. For the
// same signal type, the freshest observation wins; different types remain
// simultaneously visible. This never resolves WAIT/DEFER/INTERVENE.
func ResolveSignalConflicts(signals []InitiativeSignal) []InitiativeSignal {
	latest := map[string]InitiativeSignal{}
	order := make([]string, 0, len(signals))
	for _, s := range signals {
		key := s.Type
		if key == "" {
			key = string(s.Source) + ":" + string(s.Reason)
		}
		old, ok := latest[key]
		if !ok {
			order = append(order, key)
			latest[key] = s
			continue
		}
		if s.ObservedAt.After(old.ObservedAt) || (s.ObservedAt.Equal(old.ObservedAt) && s.Confidence >= old.Confidence) {
			latest[key] = s
		}
	}
	out := make([]InitiativeSignal, 0, len(latest))
	for _, key := range order {
		out = append(out, latest[key])
	}
	return out
}

// BuildOpportunity constructs evidence into an opportunity. It is a derived
// representation, not an authorization decision.
func BuildOpportunity(signals []InitiativeSignal, in SignalInput) InitiativeOpportunity {
	signals = ResolveSignalConflicts(AggregateSignals(NormalizeSignals(signals, in.Now)))
	var primary InitiativeSignal
	relevance := 0.0
	for _, s := range signals {
		if s.Relevant && s.Confidence > relevance {
			relevance = s.Confidence
			primary = s
		}
	}
	if primary.Source == "" && len(signals) > 0 {
		primary = signals[0]
	}
	source, reason, target := primary.Source, primary.Reason, primary.Target
	if source == "" {
		source = SourceConversationalContext
	}
	if reason == "" {
		reason = ReasonRelevantComment
	}
	id := ""
	if primary.ID != "" {
		id = "opportunity-" + primary.ID
	}
	return InitiativeOpportunity{
		ID: id, Exists: len(signals) > 0 && relevance > 0,
		Source: source, Reason: reason, Relevance: relevance,
		Priority: primary.Priority, Urgency: primary.Urgency, Context: in.ActiveTopic,
		Target: target, Signal: primary, Signals: signals,
		InteractionState: in.RequestKind,
		UserNeedsFocus:   in.UserUrgency >= 0.65 || in.UserFrustration >= 0.65 || in.UserConfusion >= 0.65,
		UserAvailability: in.UserAvailability, Privacy: in.Privacy, Interruptibility: in.Interruptibility,
		ConversationLoad: in.ConversationLoad, RequiresDirectTurn: in.RequiresDirectTurn, CooldownActive: in.CooldownActive,
	}
}

// SignalEngine is the B96.37.3 orchestration component. It gathers signals,
// normalizes evidence, and constructs an opportunity. Policy evaluation stays
// in B96.37.2.
type SignalEngine struct{ Sources []SignalSource }

func NewSignalEngine(sources ...SignalSource) *SignalEngine {
	if len(sources) == 0 {
		sources = DefaultSignalSources()
	}
	return &SignalEngine{Sources: append([]SignalSource(nil), sources...)}
}
func (e *SignalEngine) Collect(in SignalInput) []InitiativeSignal {
	if e == nil {
		return nil
	}
	var out []InitiativeSignal
	for _, source := range e.Sources {
		if source != nil {
			out = append(out, source.Collect(in)...)
		}
	}
	// Negative observations are first-class evidence. They inhibit or defer
	// through B96.37.2; this layer never turns them into a decision.
	if in.UserAvailability != AvailabilityUnknown {
		s := makeSignal(in, SourceInteractionContinuity, ReasonConversationContinue, "user_availability", string(in.UserAvailability), "", true, 0.95)
		out = append(out, s)
	}
	if in.Privacy != InformationAllowed {
		s := makeSignal(in, SourceUserModel, ReasonPotentialProblem, "information_policy", string(in.Privacy), "", true, 0.95)
		out = append(out, s)
	}
	if in.Interruptibility != InterruptSafe {
		s := makeSignal(in, SourceInteractionContinuity, ReasonPotentialProblem, "interruptibility", string(in.Interruptibility), "", true, 0.9)
		out = append(out, s)
	}
	if in.Channel != "" {
		out = append(out, makeSignal(in, SourceInteractionContinuity, ReasonConversationContinue, "channel", in.Channel, "", true, 0.8))
	}
	if in.HistoricalRequired || in.HistoricalAvailable {
		typ := "historical_context_available"
		value := "available"
		if in.HistoricalRequired && !in.HistoricalAvailable {
			typ, value = "historical_context_required", "required"
		}
		out = append(out, makeSignal(in, SourceConversationalContext, ReasonUsefulInformation, typ, value, in.ActiveTopic, in.HistoricalAvailable || in.HistoricalRequired, 0.85))
	}
	return NormalizeSignals(out, in.Now)
}
func (e *SignalEngine) Opportunity(in SignalInput) InitiativeOpportunity {
	return BuildOpportunity(e.Collect(in), in)
}
