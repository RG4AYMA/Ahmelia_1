package cognition

import "ahmelia/interaction"

// B96.33.7 — CONVERSATIONAL UNCERTAINTY
//
// Makes uncertainty explicit after interpretation and decision. This layer
// does not reinterpret raw language. It measures where the current cognitive
// chain is weak, conflicting, underspecified, or dependent on unresolved
// context, and translates that into a safe conversational posture.
//
// Uncertainty is evidence about confidence, not a permission system. In
// particular, low confidence never grants execution; it only makes a decision
// less reliable and can require clarification.

type ConversationalUncertaintyKind string

const (
	UncertaintyNone           ConversationalUncertaintyKind = "none"
	UncertaintySemantic       ConversationalUncertaintyKind = "semantic"
	UncertaintyIntent         ConversationalUncertaintyKind = "intent"
	UncertaintyGoal           ConversationalUncertaintyKind = "goal"
	UncertaintyRelevance      ConversationalUncertaintyKind = "relevance"
	UncertaintyInterpretation ConversationalUncertaintyKind = "interpretation"
	UncertaintyDecision       ConversationalUncertaintyKind = "decision"
	UncertaintyReference      ConversationalUncertaintyKind = "reference"
	UncertaintyContext        ConversationalUncertaintyKind = "context"
)

type UncertaintyEvidence struct {
	Kind     ConversationalUncertaintyKind
	Score    float64
	Reason   string
	Critical bool
}

type ConversationalUncertainty struct {
	Version            string
	Primary            ConversationalUncertaintyKind
	Confidence         float64
	Uncertain          bool
	Ambiguous          bool
	NeedsClarification bool
	BlocksAction       bool
	Score              float64
	Evidence           []UncertaintyEvidence
	Reason             string
}

// EvaluateConversationalUncertainty exposes uncertainty already implied by
// the B96.33 pipeline. It is deliberately conservative: unresolved references,
// explicit ambiguity, and weak action decisions are treated as action blockers.
func EvaluateConversationalUncertainty(
	model ConversationalCognitiveModel,
	intent CommunicativeIntentEvaluation,
	goal ConversationalGoalInference,
	relevance ConversationalRelevance,
	interpretation ConversationalInterpretation,
	decision ConversationalDecision,
) ConversationalUncertainty {
	evidence := make([]UncertaintyEvidence, 0, 8)
	add := func(kind ConversationalUncertaintyKind, score float64, reason string, critical bool) {
		if score <= 0 {
			return
		}
		evidence = append(evidence, UncertaintyEvidence{Kind: kind, Score: clamp01(score), Reason: reason, Critical: critical})
	}

	if model.Request.Confidence > 0 && model.Request.Confidence < 0.60 {
		add(UncertaintySemantic, 1-model.Request.Confidence, "semantic:low_confidence", false)
	}
	if intent.Ambiguous || intent.NeedsClarification {
		add(UncertaintyIntent, 1-intent.Confidence, "intent:ambiguous_or_clarification", intent.ActionOriented)
	}
	if goal.Ambiguous || goal.NeedsClarification || goal.Primary == GoalUnknown {
		add(UncertaintyGoal, 1-goal.Confidence, "goal:ambiguous_or_unknown", false)
	}
	if relevance.Ambiguous || relevance.NeedsContext || relevance.Primary == RelevanceUnknown {
		add(UncertaintyRelevance, 1-relevance.Confidence, "relevance:ambiguous_or_missing_context", false)
	}
	if interpretation.Ambiguous || interpretation.NeedsClarification {
		add(UncertaintyInterpretation, 1-interpretation.Confidence, "interpretation:ambiguous_or_clarification", interpretation.ActionOriented)
	}
	if decision.Ambiguous || decision.Primary == DecisionUnknown {
		add(UncertaintyDecision, 1-decision.Confidence, "decision:ambiguous_or_unknown", decision.ActionCandidate)
	}
	if interpretation.HasUnresolvedRefs || model.Signals.HasUnresolvedRefs {
		add(UncertaintyReference, 0.99, "reference:unresolved", true)
	}
	if relevance.NeedsContext || (relevance.Primary == RelevanceUnknown && model.Signals.HasActiveContext) {
		add(UncertaintyContext, 0.88, "context:insufficient_for_relevance", true)
	}

	out := ConversationalUncertainty{
		Version:  "B96.33.7",
		Evidence: evidence,
		Confidence: clamp01(minFloat(
			model.Signals.Confidence,
			intent.Confidence,
			goal.Confidence,
			relevance.Confidence,
			interpretation.Confidence,
			decision.Confidence,
		)),
	}

	if len(evidence) == 0 {
		out.Primary = UncertaintyNone
		out.Score = 0
		out.Reason = "no material conversational uncertainty detected"
		return out
	}

	best := evidence[0]
	for _, e := range evidence[1:] {
		if e.Score > best.Score {
			best = e
		}
	}
	out.Primary = best.Kind
	out.Score = best.Score
	out.Ambiguous = intent.Ambiguous || goal.Ambiguous || relevance.Ambiguous || interpretation.Ambiguous || decision.Ambiguous
	out.NeedsClarification = interpretation.HasUnresolvedRefs || model.Signals.HasUnresolvedRefs
	if decision.ActionCandidate {
		out.NeedsClarification = interpretation.NeedsClarification || intent.NeedsClarification || goal.NeedsClarification || relevance.NeedsContext || decision.NeedsClarification || out.NeedsClarification
	}

	for _, e := range evidence {
		if e.Critical {
			out.BlocksAction = true
			break
		}
	}
	out.Uncertain = best.Score >= 0.25 && (decision.ActionCandidate || out.BlocksAction || out.NeedsClarification)
	if decision.ActionCandidate && out.Confidence < 0.70 {
		out.BlocksAction = true
		out.NeedsClarification = true
	}
	if out.Ambiguous && decision.ActionCandidate {
		out.BlocksAction = true
		out.NeedsClarification = true
	}

	if out.BlocksAction {
		out.Reason = "uncertainty prevents a sufficiently grounded action decision"
	} else if out.NeedsClarification {
		out.Reason = "uncertainty indicates that conversational clarification may improve the next turn"
	} else {
		out.Reason = "uncertainty is present but does not block the selected conversational participation"
	}
	return out
}

// RequiresClarificationBeforeAction is the explicit safety boundary consumed
// by later action/strategy layers. It does not authorize or execute anything.
func (u ConversationalUncertainty) RequiresClarificationBeforeAction(d ConversationalDecision) bool {
	return u.BlocksAction && (d.ActionCandidate || d.Primary == DecisionAskClarify)
}

// Keep the interaction package visible in the B96.33 chain: uncertainty is
// derived from the shared semantic contract, never from a second parser.
var _ interaction.SemanticRequest
