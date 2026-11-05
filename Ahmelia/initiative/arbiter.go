package initiative

import (
	"errors"
	"sort"
	"strings"
	"time"
)

// B96.37.17 — Initiative Arbitration.
//
// Arbitration answers one question only:
//   "Among the already-observed opportunities, which opportunity should be
//    evaluated first by B96.37.2?"
//
// It never returns Decision, never selects an Action, and never authorizes a
// conversational turn. B96.37.2 remains the sole INTERVENE/WAIT/DEFER authority.

type ArbitrationCandidate struct {
	ID            string
	Opportunity   InitiativeOpportunity
	DuplicateKey  string
	Compatible    bool
	ConflictsWith []string
	Score         float64
	Breakdown     ArbitrationScoreBreakdown
	Rank          int
}

type ArbitrationScoreBreakdown struct {
	Priority         float64
	Urgency          float64
	Relevance        float64
	UserContext      float64
	Timing           float64
	InterruptionCost float64
	ConversationLoad float64
	LearnedInfluence float64
	ConflictPenalty  float64
}

type ArbitrationResult struct {
	Selected   InitiativeCandidateRef
	Candidates []ArbitrationCandidate
	ComparedAt time.Time
	Provenance Provenance
}

type InitiativeCandidateRef struct {
	Valid       bool
	ID          string
	Opportunity InitiativeOpportunity
	Rank        int
}

type ArbitrationInput struct {
	Opportunities []InitiativeOpportunity
	Adaptations   []AdaptationSignal
	Timing        *TimingEvaluator
	Now           time.Time
}

// BuildCandidateOpportunities converts source evidence into independently
// comparable opportunities. One signal can create one candidate; equivalent
// candidates are merged by the arbiter. No decision is produced here.
func BuildCandidateOpportunities(signals []InitiativeSignal, in SignalInput) []InitiativeOpportunity {
	if in.Now.IsZero() {
		in.Now = time.Now()
	}
	normalized := NormalizeSignals(signals, in.Now)
	out := make([]InitiativeOpportunity, 0, len(normalized))
	for _, s := range normalized {
		if !isArbitrableSignal(s) {
			continue
		}
		if !s.Relevant && s.Confidence <= 0 {
			continue
		}
		out = append(out, opportunityFromSignal(s, in))
	}
	return out
}

func isArbitrableSignal(s InitiativeSignal) bool {
	// Availability, privacy, interruption, channel and load signals are
	// constraints/evidence consumed by their owners; they are not independent
	// initiatives. Likewise, enrichment projections must not manufacture a
	// candidate merely because an observation exists.
	switch s.Type {
	case "user_availability", "information_policy", "interruptibility", "channel",
		"enriched_user_availability", "enriched_interruptibility", "enriched_conversation_load",
		"enriched_information_policy", "enriched_safety_interruptibility":
		return false
	}
	return true
}
func opportunityFromSignal(s InitiativeSignal, in SignalInput) InitiativeOpportunity {
	return InitiativeOpportunity{
		Exists:             true,
		Source:             s.Source,
		Reason:             s.Reason,
		Relevance:          clamp(s.Confidence),
		Priority:           s.Priority,
		Urgency:            s.Urgency,
		UserAvailability:   in.UserAvailability,
		InteractionState:   in.RequestKind,
		Privacy:            in.Privacy,
		Channel:            in.Channel,
		Interruptibility:   in.Interruptibility,
		ConversationLoad:   in.ConversationLoad,
		Context:            in.ActiveTopic,
		Target:             s.Target,
		Signal:             s,
		Signals:            []InitiativeSignal{s},
		ExpiresAt:          s.ExpiresAt,
		RequiresDirectTurn: in.RequiresDirectTurn,
		UserNeedsFocus:     in.UserUrgency >= .65 || in.UserFrustration >= .65 || in.UserConfusion >= .65,
		CooldownActive:     in.CooldownActive,
	}
}

func NewArbitrationInput(opportunities []InitiativeOpportunity, adaptations []AdaptationSignal, timing *TimingEvaluator, now time.Time) ArbitrationInput {
	if now.IsZero() {
		now = time.Now()
	}
	return ArbitrationInput{Opportunities: append([]InitiativeOpportunity(nil), opportunities...), Adaptations: append([]AdaptationSignal(nil), adaptations...), Timing: timing, Now: now}
}

type InitiativeArbiter struct{}

func NewInitiativeArbiter() *InitiativeArbiter { return &InitiativeArbiter{} }

func (a *InitiativeArbiter) Arbitrate(in ArbitrationInput) (ArbitrationResult, error) {
	now := in.Now
	if now.IsZero() {
		now = time.Now()
	}
	candidates := deduplicateCandidates(in.Opportunities)
	if len(candidates) == 0 {
		return ArbitrationResult{
			ComparedAt: now,
			Provenance: Provenance{Component: "initiative.InitiativeArbiter", Subsystem: "B96.37.17", SignalType: "empty", CreatedAt: now},
		}, nil
	}
	for i := range candidates {
		candidates[i].Breakdown = scoreCandidate(candidates[i].Opportunity, in.Adaptations, in.Timing, now)
		candidates[i].Score = totalScore(candidates[i].Breakdown)
	}
	detectConflicts(candidates)
	for i := range candidates {
		candidates[i].Breakdown.ConflictPenalty = conflictPenalty(candidates[i], candidates)
		candidates[i].Score = totalScore(candidates[i].Breakdown)
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		return betterCandidate(candidates[i], candidates[j])
	})
	for i := range candidates {
		candidates[i].Rank = i + 1
	}
	selected := InitiativeCandidateRef{Valid: true, ID: candidates[0].ID, Opportunity: candidates[0].Opportunity, Rank: 1}
	return ArbitrationResult{
		Selected:   selected,
		Candidates: candidates,
		ComparedAt: now,
		Provenance: Provenance{Component: "initiative.InitiativeArbiter", Subsystem: "B96.37.17", SignalType: "candidate_comparison", CreatedAt: now},
	}, nil
}

func deduplicateCandidates(ops []InitiativeOpportunity) []ArbitrationCandidate {
	index := map[string]int{}
	out := make([]ArbitrationCandidate, 0, len(ops))
	for _, op := range ops {
		if !op.Exists {
			continue
		}
		key := candidateDuplicateKey(op)
		if i, ok := index[key]; ok {
			out[i].Opportunity.Relevance = maxFloat(out[i].Opportunity.Relevance, op.Relevance)
			if priorityRank(op.Priority) > priorityRank(out[i].Opportunity.Priority) {
				out[i].Opportunity.Priority = op.Priority
			}
			if urgencyRank(op.Urgency) > urgencyRank(out[i].Opportunity.Urgency) {
				out[i].Opportunity.Urgency = op.Urgency
			}
			out[i].Opportunity.Signals = append(out[i].Opportunity.Signals, op.Signals...)
			if out[i].Opportunity.Signal.Confidence < op.Signal.Confidence {
				out[i].Opportunity.Signal = op.Signal
			}
			continue
		}
		id := stableCandidateID(op, len(out))
		index[key] = len(out)
		out = append(out, ArbitrationCandidate{ID: id, Opportunity: op, DuplicateKey: key, Compatible: true})
	}
	return out
}

func candidateDuplicateKey(op InitiativeOpportunity) string {
	return strings.Join([]string{string(op.Source), string(op.Reason), strings.TrimSpace(op.Target), strings.TrimSpace(op.Context)}, "|")
}

func stableCandidateID(op InitiativeOpportunity, ordinal int) string {
	if op.Signal.ID != "" {
		return "arb-" + op.Signal.ID
	}
	return "arb-" + candidateDuplicateKey(op) + "-" + string(rune('a'+ordinal%26))
}

func detectConflicts(c []ArbitrationCandidate) {
	for i := range c {
		for j := i + 1; j < len(c); j++ {
			if conflict(c[i].Opportunity, c[j].Opportunity) {
				c[i].ConflictsWith = append(c[i].ConflictsWith, c[j].ID)
				c[j].ConflictsWith = append(c[j].ConflictsWith, c[i].ID)
			}
		}
	}
}

// Compatible reports whether two opportunities can coexist without an
// arbitration conflict. It is a comparison primitive, not an authorization
// policy and never produces a Decision.
func Compatible(a, b InitiativeOpportunity) bool { return !conflict(a, b) }

func conflict(a, b InitiativeOpportunity) bool {
	if a.Reason == b.Reason && strings.TrimSpace(a.Target) == strings.TrimSpace(b.Target) {
		return false
	}
	pair := func(x, y Reason) bool { return a.Reason == x && b.Reason == y || a.Reason == y && b.Reason == x }
	if pair(ReasonConversationContinue, ReasonTopicRedirection) ||
		pair(ReasonPausedThreadAvailable, ReasonNewPossibility) ||
		pair(ReasonPausedThreadAvailable, ReasonAlternativeAvailable) ||
		pair(ReasonWarning, ReasonRelevantComment) ||
		pair(ReasonPotentialProblem, ReasonRelevantComment) {
		return true
	}
	// Different targets from the same source can compete when one explicitly
	// redirects the current thread.
	return a.Source == b.Source &&
		(a.Reason == ReasonTopicRedirection || b.Reason == ReasonTopicRedirection) &&
		strings.TrimSpace(a.Target) != strings.TrimSpace(b.Target)
}

func scoreCandidate(op InitiativeOpportunity, adaptations []AdaptationSignal, timing *TimingEvaluator, now time.Time) ArbitrationScoreBreakdown {
	return ArbitrationScoreBreakdown{
		Priority:         float64(priorityRank(op.Priority)) / 4,
		Urgency:          float64(urgencyRank(op.Urgency)) / 4,
		Relevance:        clamp(op.Relevance),
		UserContext:      userContextScore(op),
		Timing:           timingScore(op, timing, now),
		InterruptionCost: interruptionCost(op),
		ConversationLoad: conversationLoadScore(op),
		LearnedInfluence: learnedInfluence(op, adaptations),
	}
}

func totalScore(s ArbitrationScoreBreakdown) float64 {
	// Ranking weights are deliberately subordinate to B96.37.2: they order
	// candidates but cannot produce a decision.
	return .24*s.Priority + .18*s.Urgency + .22*s.Relevance + .12*s.UserContext +
		.10*s.Timing + .06*s.InterruptionCost + .05*s.ConversationLoad +
		.03*s.LearnedInfluence - s.ConflictPenalty
}

func betterCandidate(a, b ArbitrationCandidate) bool {
	if a.Score != b.Score {
		return a.Score > b.Score
	}
	// Deterministic tie-break hierarchy: policy-relevant evidence first,
	// then recency, then stable identity. None of these is a decision.
	if priorityRank(a.Opportunity.Priority) != priorityRank(b.Opportunity.Priority) {
		return priorityRank(a.Opportunity.Priority) > priorityRank(b.Opportunity.Priority)
	}
	if urgencyRank(a.Opportunity.Urgency) != urgencyRank(b.Opportunity.Urgency) {
		return urgencyRank(a.Opportunity.Urgency) > urgencyRank(b.Opportunity.Urgency)
	}
	at, bt := a.Opportunity.Signal.ObservedAt, b.Opportunity.Signal.ObservedAt
	if !at.Equal(bt) {
		return at.After(bt)
	}
	return a.ID < b.ID
}

func userContextScore(op InitiativeOpportunity) float64 {
	v := clamp(op.Relevance)
	if strings.TrimSpace(op.Context) != "" && strings.TrimSpace(op.Target) != "" {
		v += .10
	}
	if op.Source == SourceConversationalContext || op.Source == SourceInteractionContinuity {
		v += .05
	}
	return clamp(v)
}

func timingScore(op InitiativeOpportunity, evaluator *TimingEvaluator, now time.Time) float64 {
	if evaluator != nil {
		assessment := evaluator.Evaluate(op, now)
		if !assessment.Admissible {
			return .05
		}
		switch assessment.Timing {
		case TimingInterrupt:
			return .65
		case TimingNow:
			return .95
		case TimingDefer:
			return .15
		default:
			return .25
		}
	}
	if op.CooldownActive {
		return .05
	}
	switch op.UserAvailability {
	case AvailabilityExplicitlyUnavailable, AvailabilityAway:
		return .05
	case AvailabilityBusy, AvailabilityDistracted:
		return .35
	}
	switch op.ConversationLoad {
	case LoadSaturated:
		return .10
	case LoadHigh:
		return .35
	case LoadNormal:
		return .70
	default:
		return .85
	}
}

func interruptionCost(op InitiativeOpportunity) float64 {
	switch op.Interruptibility {
	case InterruptForbidden:
		return .05
	case InterruptNotRecommended:
		return .25
	case InterruptLimited:
		return .60
	case InterruptSafe:
		return .90
	default:
		return .40
	}
}

func conversationLoadScore(op InitiativeOpportunity) float64 {
	switch op.ConversationLoad {
	case LoadSaturated:
		return .05
	case LoadHigh:
		return .30
	case LoadNormal:
		return .70
	case LoadLow:
		return .90
	default:
		return .50
	}
}

func learnedInfluence(op InitiativeOpportunity, adaptations []AdaptationSignal) float64 {
	best := 0.0
	for _, a := range adaptations {
		if a.Scope == ScopeGlobal && a.Signal == "favorable" {
			best = maxFloat(best, .5*a.Weight*a.Confidence)
		}
		if a.Key == string(op.Reason) && a.Signal == "favorable" {
			best = maxFloat(best, a.Weight*a.Confidence)
		}
		if a.Key == string(op.Reason) && a.Signal == "caution" {
			best = minFloat(best, -.75*a.Weight*a.Confidence)
		}
	}
	return clampSigned(best)
}

func conflictPenalty(c ArbitrationCandidate, all []ArbitrationCandidate) float64 {
	if len(c.ConflictsWith) == 0 {
		return 0
	}
	// A candidate with weaker evidence yields to a stronger conflicting
	// candidate. This is ordering, not authorization.
	penalty := 0.0
	for _, id := range c.ConflictsWith {
		for _, other := range all {
			if other.ID == id && other.Score > c.Score {
				penalty += .08
			}
		}
	}
	if penalty > .24 {
		penalty = .24
	}
	return penalty
}

func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func clampSigned(v float64) float64 {
	if v < -.75 {
		return -.75
	}
	if v > .75 {
		return .75
	}
	return v
}

func (r ArbitrationResult) Validate() error {
	if r.Selected.Valid {
		if r.Selected.Rank != 1 {
			return errors.New("selected arbitration candidate must be rank 1")
		}
		if len(r.Candidates) == 0 {
			return errors.New("selected candidate requires candidate set")
		}
	}
	for i, c := range r.Candidates {
		if c.Rank != i+1 {
			return errors.New("arbitration ranks must be contiguous")
		}
		if c.ID == "" {
			return errors.New("arbitration candidate requires id")
		}
	}
	return nil
}
