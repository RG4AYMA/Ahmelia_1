package core

import (
	"sort"
	"strconv"
	"strings"
	"time"

	"ahmelia/initiative"
	"ahmelia/situation"
)

// Type aliases for initiative types
type (
	InterruptionPolicy = initiative.InterruptionPolicy
	Provenance         = initiative.Provenance
)

// FocusArbiter is the unified arbitration authority for ALL proposal sources.
// It receives ProposalContracts from interactive, initiative, and autonomous routes
// and selects which (if any) should proceed to execution.
// It does NOT execute — it only selects and returns the winning proposal with reasoning.
type FocusArbiter struct {
	interruptionPolicy *InterruptionPolicy
	arbitrationWeights ArbitrationWeights
}

// ArbitrationWeights configures how different factors contribute to the score.
type ArbitrationWeights struct {
	Priority           float64
	Urgency            float64
	EvidenceConfidence float64
	InterruptionCost   float64 // negative weight (higher cost = lower score)
	UserContext        float64
	Timing             float64
	ConflictPenalty    float64
}

// DefaultArbitrationWeights returns sensible defaults.
func DefaultArbitrationWeights() ArbitrationWeights {
	return ArbitrationWeights{
		Priority:           0.25,
		Urgency:            0.20,
		EvidenceConfidence: 0.20,
		InterruptionCost:   -0.25, // negative: higher cost reduces score
		UserContext:        0.05,
		Timing:             0.05,
		ConflictPenalty:    -0.15,
	}
}

// FocusArbitrationResult contains the arbitration outcome.
type FocusArbitrationResult struct {
	Selected   *ProposalContract
	Candidates []*ArbitrationCandidate
	ComparedAt time.Time
	Provenance Provenance
	Reasoning  string
}

// ArbitrationCandidate wraps a proposal with its score breakdown.
type ArbitrationCandidate struct {
	Proposal      *ProposalContract
	Score         float64
	Breakdown     FocusScoreBreakdown
	Rank          int
	ConflictsWith []string
}

// FocusScoreBreakdown details the scoring components.
type FocusScoreBreakdown struct {
	Priority           float64
	Urgency            float64
	EvidenceConfidence float64
	InterruptionCost   float64
	UserContext        float64
	Timing             float64
	ConflictPenalty    float64
	Total              float64
}

// NewFocusArbiter creates a new unified arbiter.
func NewFocusArbiter(interruptionPolicy *InterruptionPolicy) *FocusArbiter {
	if interruptionPolicy == nil {
		interruptionPolicy = initiative.NewInterruptionPolicy()
	}
	return &FocusArbiter{
		interruptionPolicy: interruptionPolicy,
		arbitrationWeights: DefaultArbitrationWeights(),
	}
}

// SetWeights allows customizing arbitration weights.
func (a *FocusArbiter) SetWeights(w ArbitrationWeights) {
	a.arbitrationWeights = w
}

// Arbitrate evaluates all proposals and selects the best one.
// Returns the selected proposal (or nil if none should proceed) and full details.
func (a *FocusArbiter) Arbitrate(proposals []*ProposalContract, now time.Time) (FocusArbitrationResult, error) {
	if now.IsZero() {
		now = time.Now()
	}

	if len(proposals) == 0 {
		return FocusArbitrationResult{
			ComparedAt: now,
			Provenance: Provenance{Component: "initiative.FocusArbiter", SignalType: "empty", CreatedAt: now},
			Reasoning:  "no proposals submitted",
		}, nil
	}

	// Convert to candidates
	candidates := make([]*ArbitrationCandidate, 0, len(proposals))
	for _, p := range proposals {
		if !a.isEligible(p, now) {
			continue
		}
		candidates = append(candidates, &ArbitrationCandidate{Proposal: p})
	}

	if len(candidates) == 0 {
		return FocusArbitrationResult{
			ComparedAt: now,
			Provenance: Provenance{Component: "initiative.FocusArbiter", SignalType: "none_eligible", CreatedAt: now},
			Reasoning:  "no eligible proposals after policy filtering",
		}, nil
	}

	// Score each candidate
	for _, c := range candidates {
		c.Breakdown = a.scoreCandidate(c.Proposal, now)
		c.Score = a.totalScore(c.Breakdown)
	}

	// Detect conflicts
	a.detectConflicts(candidates)
	for _, c := range candidates {
		c.Breakdown.ConflictPenalty = a.conflictPenalty(c, candidates)
		c.Score = a.totalScore(c.Breakdown)
	}

	// Sort by score (descending)
	sort.SliceStable(candidates, func(i, j int) bool {
		return candidates[i].Score > candidates[j].Score
	})

	// Assign ranks
	for i, c := range candidates {
		c.Rank = i + 1
	}

	selected := candidates[0].Proposal
	reasoning := a.buildReasoning(candidates)

	return FocusArbitrationResult{
		Selected:   selected,
		Candidates: candidates,
		ComparedAt: now,
		Provenance: Provenance{
			Component:  "initiative.FocusArbiter",
			Subsystem:  "focus_arbitration",
			SignalType: "proposal_selection",
			CreatedAt:  now,
		},
		Reasoning: reasoning,
	}, nil
}

// isEligible checks if a proposal passes basic policy gates.
func (a *FocusArbiter) isEligible(p *ProposalContract, now time.Time) bool {
	// Interruption policy governs unsolicited interventions. An interactive
	// proposal already belongs to a user turn in progress; treating it as an
	// interruption can reject every response (for example, Process() carries
	// ChannelUnknown). Autonomous proposals use their separate authorization.
	if a.interruptionPolicy != nil && p.Kind == ProposalKindInitiative {
		relevance := 0.0
		if len(p.Evidence) > 0 {
			relevance = p.Evidence[0].Confidence // first evidence confidence as relevance
		}
		// Availability comes from the proposal's own opportunity when it has one;
		// only a proposal with no payload falls back to the previous default.
		availability := initiative.AvailabilityAvailable
		if p.Initiative != nil && p.Initiative.Opportunity.UserAvailability != "" {
			availability = p.Initiative.Opportunity.UserAvailability
		}
		// Create a minimal InitiativeOpportunity from the proposal for policy evaluation
		opp := initiative.InitiativeOpportunity{
			Exists:           true,
			Source:           mapProposalKindToSource(p.Kind),
			Reason:           mapProposalKindToReason(p.Kind, p.Intention),
			Target:           p.IntentionDetail,
			Priority:         mapProposalPriorityToInitiative(p.Priority),
			Urgency:          mapProposalUrgencyToInitiative(p.Urgency),
			Relevance:        relevance,
			Channel:          string(p.Channel),
			Interruptibility: initiative.InterruptSafe,
		}

		// Build a minimal SignalInput
		input := initiative.SignalInput{
			Now:                now,
			ActiveTopic:        p.IntentionDetail,
			ActiveObjective:    p.IntentionDetail,
			ActiveWorkID:       p.Provenance.OriginID,        // Approximate
			DecisionEvidence:   situation.DecisionEvidence{}, // Empty for now
			Interruptibility:   initiative.InterruptSafe,
			UserUrgency:        p.InterruptionCost.UserUrgency,
			UserFrustration:    p.InterruptionCost.UserFrustration,
			UserConfusion:      p.InterruptionCost.UserConfusion,
			UserAvailability:   availability,
			Channel:            string(p.Channel),
			Privacy:            p.Privacy,
			ConversationLoad:   mapConversationLoad(p.InterruptionCost.ConversationLoad),
			RequiresDirectTurn: p.RequiresDirectTurn,
		}

		opp.UserAvailability = availability
		assessment := a.interruptionPolicy.Evaluate(opp, input, now)
		if !assessment.Allowed && !(proposalIsOpening(p, availability) && blockedOnlyByInterruptionUrgency(assessment)) {
			return false
		}
	}
	return true
}

// proposalIsOpening reports whether an initiative proposal opens a turn with a
// user who is free, as opposed to interrupting one who is occupied. The
// interruption policy exists to protect an occupied user: its rule that only
// critical urgency may interrupt is correct and stays untouched. But speaking
// to a user who is available, with no turn in progress and nothing in
// meeting/focus/rest, interrupts nothing, so that one rule does not apply.
// Channel, privacy, cooldown and availability gates still apply to openings.
func proposalIsOpening(p *ProposalContract, availability initiative.UserAvailability) bool {
	if p == nil || p.Initiative == nil || !p.Initiative.Idle {
		return false
	}
	if availability != initiative.AvailabilityAvailable {
		return false
	}
	c := p.InterruptionCost
	return !c.InMeeting && !c.InFocus && !c.IsResting
}

// blockedOnlyByInterruptionUrgency is true when the policy's sole objection is
// the urgency threshold for interrupting. Evaluate returns at the first failed
// channel/privacy/cooldown/availability gate, so urgency is the only constraint
// present exactly when every one of those gates passed.
func blockedOnlyByInterruptionUrgency(a initiative.InterruptibilityAssessment) bool {
	return len(a.BlockingConstraints) == 1 && a.BlockingConstraints[0] == "urgency_below_critical"
}

// Helper functions to map proposal types to initiative types
func mapProposalPriorityToInitiative(p ProposalPriority) initiative.Priority {
	switch p {
	case ProposalPriorityCritical:
		return initiative.PriorityCritical
	case ProposalPriorityHigh:
		return initiative.PriorityHigh
	case ProposalPriorityMedium:
		return initiative.PriorityMedium
	case ProposalPriorityLow:
		return initiative.PriorityLow
	default:
		return initiative.PriorityLow
	}
}

func mapProposalUrgencyToInitiative(u ProposalUrgency) initiative.Urgency {
	switch u {
	case ProposalUrgencyImmediate:
		return initiative.UrgencyCritical
	case ProposalUrgencyHigh:
		return initiative.UrgencyHigh
	case ProposalUrgencyMed:
		return initiative.UrgencyMedium
	case ProposalUrgencyLow:
		return initiative.UrgencyLow
	default:
		return initiative.UrgencyNone
	}
}

func mapConversationLoad(l ConversationLoadLevel) initiative.ConversationLoad {
	switch l {
	case ConversationLoadHigh:
		return initiative.LoadHigh
	case ConversationLoadMed:
		return initiative.LoadNormal
	case ConversationLoadLow:
		return initiative.LoadLow
	default:
		return initiative.LoadNormal
	}
}

func mapProposalKindToSource(k ProposalKind) initiative.Source {
	switch k {
	case ProposalKindInteractive:
		return initiative.SourceConversationalContext
	case ProposalKindInitiative:
		return initiative.SourceInteractionContinuity
	case ProposalKindAutonomous:
		return initiative.SourceAutonomousSignal
	default:
		return initiative.Source("unknown")
	}
}

func mapProposalKindToReason(k ProposalKind, intention string) initiative.Reason {
	switch k {
	case ProposalKindInteractive:
		return initiative.Reason(intention)
	case ProposalKindInitiative:
		return initiative.Reason(intention)
	case ProposalKindAutonomous:
		return initiative.Reason("autonomous_action")
	default:
		return initiative.Reason(intention)
	}
}

func (a *FocusArbiter) scoreCandidate(p *ProposalContract, now time.Time) FocusScoreBreakdown {
	breakdown := FocusScoreBreakdown{}

	// Priority score (0-1)
	breakdown.Priority = a.priorityScore(p.Priority)

	// Urgency score (0-1)
	breakdown.Urgency = a.urgencyScore(p.Urgency)

	// Evidence confidence (average of evidence confidences)
	breakdown.EvidenceConfidence = a.evidenceConfidence(p.Evidence)

	// Interruption cost (inverted: lower cost = higher score)
	breakdown.InterruptionCost = 1.0 - p.InterruptionCost.Total

	// User context (from evidence)
	breakdown.UserContext = a.userContextScore(p.Evidence)

	// Timing (how fresh/appropriate is this proposal)
	breakdown.Timing = a.timingScore(p, now)

	return breakdown
}

func (a *FocusArbiter) totalScore(b FocusScoreBreakdown) float64 {
	w := a.arbitrationWeights
	return w.Priority*b.Priority +
		w.Urgency*b.Urgency +
		w.EvidenceConfidence*b.EvidenceConfidence +
		w.InterruptionCost*b.InterruptionCost +
		w.UserContext*b.UserContext +
		w.Timing*b.Timing +
		w.ConflictPenalty*b.ConflictPenalty
}

func (a *FocusArbiter) priorityScore(p ProposalPriority) float64 {
	switch p {
	case ProposalPriorityCritical:
		return 1.0
	case ProposalPriorityHigh:
		return 0.8
	case ProposalPriorityMedium:
		return 0.6
	case ProposalPriorityLow:
		return 0.4
	default:
		return 0.1
	}
}

func (a *FocusArbiter) urgencyScore(u ProposalUrgency) float64 {
	switch u {
	case ProposalUrgencyImmediate:
		return 1.0
	case ProposalUrgencyHigh:
		return 0.8
	case ProposalUrgencyMed:
		return 0.6
	case ProposalUrgencyLow:
		return 0.3
	default:
		return 0.1
	}
}

func (a *FocusArbiter) evidenceConfidence(evidences []ProposalEvidence) float64 {
	if len(evidences) == 0 {
		return 0.3
	}
	sum := 0.0
	for _, e := range evidences {
		sum += e.Confidence
	}
	return sum / float64(len(evidences))
}

func (a *FocusArbiter) userContextScore(evidences []ProposalEvidence) float64 {
	// Look for user-state related evidence
	hasUserState := false
	for _, e := range evidences {
		if strings.HasPrefix(e.Kind, "user_") || e.Subject == "user" {
			hasUserState = true
			break
		}
	}
	if hasUserState {
		return 0.8
	}
	return 0.4
}

func (a *FocusArbiter) timingScore(p *ProposalContract, now time.Time) float64 {
	age := now.Sub(p.CreatedAt)
	if age < 30*time.Second {
		return 1.0
	}
	if age < 2*time.Minute {
		return 0.8
	}
	if age < 10*time.Minute {
		return 0.5
	}
	return 0.3
}

func (a *FocusArbiter) detectConflicts(candidates []*ArbitrationCandidate) {
	for i := range candidates {
		for j := i + 1; j < len(candidates); j++ {
			if a.conflict(candidates[i].Proposal, candidates[j].Proposal) {
				candidates[i].ConflictsWith = append(candidates[i].ConflictsWith, candidates[j].Proposal.ID)
				candidates[j].ConflictsWith = append(candidates[j].ConflictsWith, candidates[i].Proposal.ID)
			}
		}
	}
}

func (a *FocusArbiter) conflict(p1, p2 *ProposalContract) bool {
	// Same route, different intentions can compete
	if p1.Kind == p2.Kind && p1.Intention != p2.Intention {
		return true
	}
	// Different routes with same target can compete
	if strings.EqualFold(p1.IntentionDetail, p2.IntentionDetail) {
		return true
	}
	// Autonomous action vs interactive response on same work
	if p1.Kind == ProposalKindAutonomous && p2.Kind == ProposalKindInteractive {
		if p1.Action != nil && p2.Action != nil {
			if p1.Action.Target == p2.Action.Target {
				return true
			}
		}
	}
	return false
}

func (a *FocusArbiter) conflictPenalty(c *ArbitrationCandidate, all []*ArbitrationCandidate) float64 {
	if len(c.ConflictsWith) == 0 {
		return 0
	}
	// Penalty proportional to number and severity of conflicts
	penalty := float64(len(c.ConflictsWith)) * 0.15
	if penalty > 0.5 {
		penalty = 0.5
	}
	return -penalty
}

func (a *FocusArbiter) buildReasoning(candidates []*ArbitrationCandidate) string {
	if len(candidates) == 0 {
		return "no candidates"
	}
	if len(candidates) == 1 {
		return "single candidate: " + candidates[0].Proposal.Intention
	}
	top := candidates[0]
	second := candidates[1]
	return top.Proposal.Intention + " (score: " + formatFloat(top.Score) +
		") over " + second.Proposal.Intention + " (score: " + formatFloat(second.Score) + ")"
}

func formatFloat(f float64) string {
	return strings.TrimRight(strings.TrimRight(formatFloatPrec(f, 3), "0"), ".")
}

func formatFloatPrec(f float64, prec int) string {
	return strings.TrimRight(strings.TrimRight(strconv.FormatFloat(f, 'f', prec, 64), "0"), ".")
}
