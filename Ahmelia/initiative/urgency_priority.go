package initiative

import (
	"errors"
	"sort"
	"strings"
	"time"
)

// B96.37.10 — Urgency & Priority.
//
// This layer enriches an InitiativeOpportunity with two independent dimensions:
// priority answers "how important?" and urgency answers "how fast?".
// It never authorizes a conversational turn and therefore cannot produce an
// InitiativeDecision.
type PriorityLevel = Priority
type UrgencyLevel = Urgency

// Priority values are intentionally ordered only through the helpers below.
// Do not compare their string representation to infer urgency or authority.
func priorityRank(p Priority) int {
	switch normalizePriority(p) {
	case PriorityLow:
		return 1
	case PriorityMedium:
		return 2
	case PriorityHigh:
		return 3
	case PriorityCritical:
		return 4
	default:
		return 0
	}
}

func urgencyRank(u Urgency) int {
	switch normalizeUrgency(u) {
	case UrgencyNone:
		return 0
	case UrgencyLow:
		return 1
	case UrgencyMedium:
		return 2
	case UrgencyHigh:
		return 3
	case UrgencyCritical:
		return 4
	default:
		return -1
	}
}

func normalizePriority(p Priority) Priority {
	switch strings.ToUpper(strings.TrimSpace(string(p))) {
	case "LOW":
		return PriorityLow
	case "NORMAL", "MEDIUM":
		return PriorityMedium
	case "HIGH":
		return PriorityHigh
	case "CRITICAL", "URGENT":
		return PriorityCritical
	default:
		return ""
	}
}

func normalizeUrgency(u Urgency) Urgency {
	switch strings.ToUpper(strings.TrimSpace(string(u))) {
	case "", "NONE":
		return UrgencyNone
	case "LOW":
		return UrgencyLow
	case "MEDIUM":
		return UrgencyMedium
	case "HIGH":
		return UrgencyHigh
	case "CRITICAL", "URGENT":
		return UrgencyCritical
	default:
		return ""
	}
}

// PriorityUrgencyEvidence is a detached explanation for one enrichment input.
// It is evidence, not a decision.
type PriorityUrgencyEvidence struct {
	SignalID   string    `json:"signal_id,omitempty"`
	SignalType string    `json:"signal_type,omitempty"`
	Source     Source    `json:"source"`
	Reason     Reason    `json:"reason"`
	Priority   Priority  `json:"priority,omitempty"`
	Urgency    Urgency   `json:"urgency,omitempty"`
	Confidence float64   `json:"confidence"`
	ObservedAt time.Time `json:"observed_at"`
	Explicit   bool      `json:"explicit"`
	Rule       string    `json:"rule,omitempty"`
}

// PriorityUrgencyInteraction is descriptive only. It does not map to a
// decision or timing outcome.
type PriorityUrgencyInteraction string

const (
	InteractionLowPriorityLowUrgency           PriorityUrgencyInteraction = "LOW_PRIORITY_LOW_URGENCY"
	InteractionLowPriorityMediumUrgency        PriorityUrgencyInteraction = "LOW_PRIORITY_MEDIUM_URGENCY"
	InteractionLowPriorityHighUrgency          PriorityUrgencyInteraction = "LOW_PRIORITY_HIGH_URGENCY"
	InteractionLowPriorityCriticalUrgency      PriorityUrgencyInteraction = "LOW_PRIORITY_CRITICAL_URGENCY"
	InteractionMediumPriorityLowUrgency        PriorityUrgencyInteraction = "MEDIUM_PRIORITY_LOW_URGENCY"
	InteractionMediumPriorityMediumUrgency     PriorityUrgencyInteraction = "MEDIUM_PRIORITY_MEDIUM_URGENCY"
	InteractionMediumPriorityHighUrgency       PriorityUrgencyInteraction = "MEDIUM_PRIORITY_HIGH_URGENCY"
	InteractionMediumPriorityCriticalUrgency   PriorityUrgencyInteraction = "MEDIUM_PRIORITY_CRITICAL_URGENCY"
	InteractionHighPriorityLowUrgency          PriorityUrgencyInteraction = "HIGH_PRIORITY_LOW_URGENCY"
	InteractionHighPriorityMediumUrgency       PriorityUrgencyInteraction = "HIGH_PRIORITY_MEDIUM_URGENCY"
	InteractionHighPriorityHighUrgency         PriorityUrgencyInteraction = "HIGH_PRIORITY_HIGH_URGENCY"
	InteractionHighPriorityCriticalUrgency     PriorityUrgencyInteraction = "HIGH_PRIORITY_CRITICAL_URGENCY"
	InteractionCriticalPriorityLowUrgency      PriorityUrgencyInteraction = "CRITICAL_PRIORITY_LOW_URGENCY"
	InteractionCriticalPriorityMediumUrgency   PriorityUrgencyInteraction = "CRITICAL_PRIORITY_MEDIUM_URGENCY"
	InteractionCriticalPriorityHighUrgency     PriorityUrgencyInteraction = "CRITICAL_PRIORITY_HIGH_URGENCY"
	InteractionCriticalPriorityCriticalUrgency PriorityUrgencyInteraction = "CRITICAL_PRIORITY_CRITICAL_URGENCY"
)

func interactionFor(p Priority, u Urgency) PriorityUrgencyInteraction {
	p = normalizePriority(p)
	u = normalizeUrgency(u)
	return PriorityUrgencyInteraction(string(p) + "_PRIORITY_" + string(u) + "_URGENCY")
}

// TemporalEscalationRule is deliberately explicit. Age alone is never enough
// to escalate urgency. A rule must name the evidence it applies to, its
// starting urgency, and the elapsed threshold.
type TemporalEscalationRule struct {
	Name          string        `json:"name"`
	SignalType    string        `json:"signal_type"`
	After         time.Duration `json:"after"`
	From          Urgency       `json:"from"`
	To            Urgency       `json:"to"`
	MinConfidence float64       `json:"min_confidence"`
}

func (r TemporalEscalationRule) Valid() bool {
	return strings.TrimSpace(r.Name) != "" && strings.TrimSpace(r.SignalType) != "" && r.After > 0 && urgencyRank(r.From) >= 0 && urgencyRank(r.To) > urgencyRank(r.From) && r.MinConfidence >= 0 && r.MinConfidence <= 1
}

// PriorityUrgencyAssessment is the complete B96.37.10 output. Notice that it
// contains no Decision field by design.
type PriorityUrgencyAssessment struct {
	Priority           Priority                   `json:"priority"`
	Urgency            Urgency                    `json:"urgency"`
	Interaction        PriorityUrgencyInteraction `json:"interaction"`
	PriorityEvidence   []PriorityUrgencyEvidence  `json:"priority_evidence,omitempty"`
	UrgencyEvidence    []PriorityUrgencyEvidence  `json:"urgency_evidence,omitempty"`
	TemporalEscalated  bool                       `json:"temporal_escalated"`
	EscalationRule     string                     `json:"escalation_rule,omitempty"`
	Stale              bool                       `json:"stale"`
	Expired            bool                       `json:"expired"`
	EvaluatedAt        time.Time                  `json:"evaluated_at"`
	PriorityObservedAt time.Time                  `json:"priority_observed_at,omitempty"`
	UrgencyObservedAt  time.Time                  `json:"urgency_observed_at,omitempty"`
}

func (a PriorityUrgencyAssessment) Validate() error {
	if normalizePriority(a.Priority) == "" {
		return errors.New("invalid B96.37.10 priority")
	}
	if normalizeUrgency(a.Urgency) == "" {
		return errors.New("invalid B96.37.10 urgency")
	}
	if a.EvaluatedAt.IsZero() {
		return errors.New("B96.37.10 assessment requires evaluation time")
	}
	if a.TemporalEscalated && a.EscalationRule == "" {
		return errors.New("temporal escalation requires an explicit rule")
	}
	if a.Stale && !a.Expired && a.Urgency == UrgencyCritical {
		return errors.New("stale evidence cannot remain critical without a fresh supporting signal")
	}
	return nil
}

// PriorityUrgencyPolicy owns only the enrichment rules. Defaults are
// intentionally conservative and contain no age-based escalation rule.
type PriorityUrgencyPolicy struct {
	Version         string
	TemporalRules   []TemporalEscalationRule
	StaleGrace      time.Duration
	DefaultPriority Priority
	DefaultUrgency  Urgency
}

func NewPriorityUrgencyPolicy() *PriorityUrgencyPolicy {
	return &PriorityUrgencyPolicy{
		Version:         "B96.37.10",
		StaleGrace:      0,
		DefaultPriority: PriorityMedium,
		DefaultUrgency:  UrgencyNone,
	}
}

func (p *PriorityUrgencyPolicy) validDefaults() (Priority, Urgency) {
	priority, urgency := PriorityMedium, UrgencyNone
	if p != nil {
		if x := normalizePriority(p.DefaultPriority); x != "" {
			priority = x
		}
		if x := normalizeUrgency(p.DefaultUrgency); x != "" {
			urgency = x
		}
	}
	return priority, urgency
}

func evidenceForSignal(s InitiativeSignal) (PriorityUrgencyEvidence, PriorityUrgencyEvidence, bool, bool) {
	p := normalizePriority(s.Priority)
	u := normalizeUrgency(s.Urgency)
	base := PriorityUrgencyEvidence{SignalID: s.ID, SignalType: s.Type, Source: s.Source, Reason: s.Reason, Confidence: clamp(s.Confidence), ObservedAt: s.ObservedAt}
	pe, ue := base, base
	pe.Priority, ue.Urgency = p, u
	return pe, ue, p != "", u != "" && u != UrgencyNone
}

func bestEvidence(e []PriorityUrgencyEvidence, priority bool) (PriorityUrgencyEvidence, bool) {
	var best PriorityUrgencyEvidence
	found := false
	for _, x := range e {
		rank := 0
		if priority {
			rank = priorityRank(x.Priority)
		} else {
			rank = urgencyRank(x.Urgency)
		}
		if rank <= 0 {
			continue
		}
		if !found || rank > func() int {
			if priority {
				return priorityRank(best.Priority)
			}
			return urgencyRank(best.Urgency)
		}() || (rank == func() int {
			if priority {
				return priorityRank(best.Priority)
			}
			return urgencyRank(best.Urgency)
		}() && x.Confidence > best.Confidence) || (rank == func() int {
			if priority {
				return priorityRank(best.Priority)
			}
			return urgencyRank(best.Urgency)
		}() && x.Confidence == best.Confidence && x.ObservedAt.After(best.ObservedAt)) {
			best, found = x, true
		}
	}
	return best, found
}

func (p *PriorityUrgencyPolicy) AggregatePriority(op InitiativeOpportunity, input SignalInput) (Priority, []PriorityUrgencyEvidence) {
	defaultPriority, _ := p.validDefaults()
	evidence := make([]PriorityUrgencyEvidence, 0, len(op.Signals)+2)
	for _, s := range op.Signals {
		pe, _, ok, _ := evidenceForSignal(s)
		if ok {
			evidence = append(evidence, pe)
		}
	}
	if pe, _, ok, _ := evidenceForSignal(op.Signal); ok {
		pe.Explicit = true
		evidence = append(evidence, pe)
	}
	// Strong user-focus observations are important but do not imply urgency.
	if input.UserFrustration >= .90 || input.UserConfusion >= .90 {
		evidence = append(evidence, PriorityUrgencyEvidence{SignalType: "user_focus", Source: SourceInteractionContinuity, Reason: ReasonClarificationNeeded, Priority: PriorityHigh, Confidence: maxFloat(input.UserFrustration, input.UserConfusion), ObservedAt: input.Now, Explicit: false, Rule: "strong_user_focus"})
	}
	if best, ok := bestEvidence(evidence, true); ok {
		return normalizePriority(best.Priority), evidence
	}
	return defaultPriority, evidence
}

func (p *PriorityUrgencyPolicy) AggregateUrgency(op InitiativeOpportunity, input SignalInput) (Urgency, []PriorityUrgencyEvidence) {
	_, defaultUrgency := p.validDefaults()
	evidence := make([]PriorityUrgencyEvidence, 0, len(op.Signals)+3)
	for _, s := range op.Signals {
		_, ue, _, ok := evidenceForSignal(s)
		if ok {
			evidence = append(evidence, ue)
		}
	}
	if _, ue, _, ok := evidenceForSignal(op.Signal); ok {
		ue.Explicit = true
		evidence = append(evidence, ue)
	}
	metrics := []struct {
		value float64
		name  string
	}{{input.UserUrgency, "user_urgency"}, {input.UserFrustration, "user_frustration"}, {input.UserConfusion, "user_confusion"}}
	max := 0.0
	name := ""
	for _, m := range metrics {
		if m.value > max {
			max, name = m.value, m.name
		}
	}
	switch {
	case max >= .90:
		evidence = append(evidence, PriorityUrgencyEvidence{SignalType: name, Source: SourceInteractionContinuity, Reason: ReasonClarificationNeeded, Urgency: UrgencyCritical, Confidence: max, ObservedAt: input.Now, Rule: "user_metric_critical"})
	case max >= .75:
		evidence = append(evidence, PriorityUrgencyEvidence{SignalType: name, Source: SourceInteractionContinuity, Reason: ReasonClarificationNeeded, Urgency: UrgencyHigh, Confidence: max, ObservedAt: input.Now, Rule: "user_metric_high"})
	case max >= .60:
		evidence = append(evidence, PriorityUrgencyEvidence{SignalType: name, Source: SourceInteractionContinuity, Reason: ReasonClarificationNeeded, Urgency: UrgencyMedium, Confidence: max, ObservedAt: input.Now, Rule: "user_metric_medium"})
	}
	if best, ok := bestEvidence(evidence, false); ok {
		return normalizeUrgency(best.Urgency), evidence
	}
	return defaultUrgency, evidence
}

func (p *PriorityUrgencyPolicy) applyTemporalEscalation(urgency Urgency, evidence []PriorityUrgencyEvidence, now time.Time) (Urgency, bool, string) {
	if p == nil || len(p.TemporalRules) == 0 {
		return urgency, false, ""
	}
	best := urgency
	bestName := ""
	for _, rule := range p.TemporalRules {
		if !rule.Valid() {
			continue
		}
		for _, e := range evidence {
			if e.SignalType != rule.SignalType || e.ObservedAt.IsZero() || e.Confidence < rule.MinConfidence {
				continue
			}
			if now.Sub(e.ObservedAt) < rule.After {
				continue
			}
			if urgencyRank(urgency) != urgencyRank(rule.From) {
				continue
			}
			if urgencyRank(rule.To) > urgencyRank(best) {
				best, bestName = normalizeUrgency(rule.To), rule.Name
			}
		}
	}
	return best, bestName != "", bestName
}

func signalFresh(s InitiativeSignal, now time.Time) bool {
	if s.ExpiresAt.IsZero() {
		return true
	}
	return now.Before(s.ExpiresAt)
}

// Enrich is the sole public B96.37.10 operation. It mutates only the
// opportunity's priority/urgency metadata and related audit evidence. It does
// not set Exists, Relevance, Signal.Relevant, Timing, or any decision value.
func (p *PriorityUrgencyPolicy) Enrich(op InitiativeOpportunity, input SignalInput, now time.Time) (InitiativeOpportunity, PriorityUrgencyAssessment, error) {
	if now.IsZero() {
		now = time.Now()
	}
	priority, pe := p.AggregatePriority(op, input)
	urgency, ue := p.AggregateUrgency(op, input)
	urgency, escalated, rule := p.applyTemporalEscalation(urgency, ue, now)

	fresh := 0
	for _, s := range op.Signals {
		if signalFresh(s, now) {
			fresh++
		}
	}
	stale := len(op.Signals) > 0 && fresh == 0
	expired := !op.ExpiresAt.IsZero() && !now.Before(op.ExpiresAt)
	if stale {
		// Staleness removes urgency unless a fresh input metric has independently
		// established it. Priority remains useful as historical importance.
		freshMetric := input.UserUrgency >= .60 || input.UserFrustration >= .60 || input.UserConfusion >= .60
		if !freshMetric {
			urgency = UrgencyNone
		}
	}

	assessment := PriorityUrgencyAssessment{
		Priority: priority, Urgency: urgency, Interaction: interactionFor(priority, urgency),
		PriorityEvidence: pe, UrgencyEvidence: ue, TemporalEscalated: escalated,
		EscalationRule: rule, Stale: stale, Expired: expired, EvaluatedAt: now,
	}
	if best, ok := bestEvidence(pe, true); ok {
		assessment.PriorityObservedAt = best.ObservedAt
	}
	if best, ok := bestEvidence(ue, false); ok {
		assessment.UrgencyObservedAt = best.ObservedAt
	}
	if err := assessment.Validate(); err != nil {
		return op, assessment, err
	}

	op.Priority, op.Urgency = priority, urgency
	op.Signal.Priority, op.Signal.Urgency = priority, urgency
	op.Signals = append([]InitiativeSignal(nil), op.Signals...)
	if len(op.Signals) > 0 {
		for i := range op.Signals {
			op.Signals[i].Priority = normalizePriority(op.Signals[i].Priority)
			op.Signals[i].Urgency = normalizeUrgency(op.Signals[i].Urgency)
		}
	}
	return op, assessment, nil
}

// SortEvidence makes audit output deterministic without changing semantic
// authority.
func SortEvidence(e []PriorityUrgencyEvidence) {
	sort.SliceStable(e, func(i, j int) bool {
		if e[i].ObservedAt.Equal(e[j].ObservedAt) {
			return e[i].Confidence > e[j].Confidence
		}
		return e[i].ObservedAt.Before(e[j].ObservedAt)
	})
}

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

// NormalizePriority is the legacy migration boundary for external callers.
func NormalizePriority(p Priority) Priority { return normalizePriority(p) }

// NormalizeUrgency is the legacy migration boundary for external callers.
func NormalizeUrgency(u Urgency) Urgency { return normalizeUrgency(u) }
