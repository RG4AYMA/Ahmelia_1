package cognition

import (
	"strings"

	"ahmelia/interaction"
)

// B96.33.5 — CONVERSATIONAL INTERPRETATION
//
// Interpretation consolidates the semantic evidence produced by the shared
// interaction layer with B96.33 intent, goal and relevance assessments.
// It answers: "What does this turn mean in this conversation?"
//
// This is a cognitive interpretation contract, not a second language parser.
// It does not execute actions, authorize them, mutate conversation state, or
// select a provider/capability.

type InterpretationActionPosture string

const (
	ActionPostureNone     InterpretationActionPosture = "none"
	ActionPostureDiscuss  InterpretationActionPosture = "discuss"
	ActionPostureConsider InterpretationActionPosture = "consider_action"
	ActionPostureRequest  InterpretationActionPosture = "request_action"
	ActionPostureDecide   InterpretationActionPosture = "decide"
	ActionPostureClarify  InterpretationActionPosture = "clarify_before_action"
)

type InterpretationEvidence struct {
	Source string
	Value  string
	Weight float64
}

type ConversationalInterpretation struct {
	Version            string
	Meaning            string
	Intent             CommunicativeIntentKind
	Goal               ConversationalGoalKind
	Objective          string
	Topic              string
	Relevance          RelevanceKind
	Confidence         float64
	Ambiguous          bool
	NeedsClarification bool
	HasUnresolvedRefs  bool
	ActionOriented     bool
	ActionPosture      InterpretationActionPosture
	References         []interaction.Reference
	Entities           []string
	Constraints        []string
	Evidence           []InterpretationEvidence
	Reason             string
}

// InterpretConversation combines the outputs of B96.33.2, .3 and .4 into a
// single auditable meaning contract. The inputs remain authoritative in their
// respective layers; this function only derives a coherent projection.
func InterpretConversation(model ConversationalCognitiveModel, intent CommunicativeIntentEvaluation, goal ConversationalGoalInference, relevance ConversationalRelevance) ConversationalInterpretation {
	r := model.Request
	topic := strings.TrimSpace(firstNonEmpty(goal.Topic, r.Topic, model.Situation.Topic, model.Context.ActiveTopic))
	objective := strings.TrimSpace(firstNonEmpty(goal.Objective, r.Objective.Text))

	out := ConversationalInterpretation{
		Version:            "B96.33.5",
		Intent:             intent.Primary,
		Goal:               goal.Primary,
		Objective:          objective,
		Topic:              topic,
		Relevance:          relevance.Primary,
		Confidence:         clamp01(minFloat(intent.Confidence, goal.Confidence, relevance.Confidence)),
		Ambiguous:          intent.Ambiguous || goal.Ambiguous || relevance.Ambiguous || r.Meaning.Ambiguous,
		NeedsClarification: r.NeedsClarification || intent.NeedsClarification || goal.NeedsClarification || relevance.NeedsContext,
		HasUnresolvedRefs:  hasUnresolvedReference(r),
		ActionOriented:     intent.ActionOriented || (goal.ActionRelated && intent.Primary == IntentRequestAction),
		References:         cloneReferences(r.References),
		Entities:           append([]string(nil), r.Entities...),
		Constraints:        append([]string(nil), r.Objective.Constraints...),
	}

	// Preserve semantic evidence rather than reconstructing meaning from raw
	// language. This gives later decision layers a traceable interpretation.
	if r.Meaning.Primary != interaction.ConceptUnknown {
		out.Evidence = append(out.Evidence, InterpretationEvidence{Source: "semantic:meaning.primary", Value: string(r.Meaning.Primary), Weight: clamp01(r.Meaning.Confidence)})
	}
	if intent.Primary != IntentUnknown {
		out.Evidence = append(out.Evidence, InterpretationEvidence{Source: "cognition:intent", Value: string(intent.Primary), Weight: clamp01(intent.Confidence)})
	}
	if goal.Primary != GoalUnknown {
		out.Evidence = append(out.Evidence, InterpretationEvidence{Source: "cognition:goal", Value: string(goal.Primary), Weight: clamp01(goal.Confidence)})
	}
	if relevance.Primary != RelevanceUnknown {
		out.Evidence = append(out.Evidence, InterpretationEvidence{Source: "cognition:relevance", Value: string(relevance.Primary), Weight: clamp01(relevance.Confidence)})
	}
	for _, e := range r.Meaning.Evidence {
		out.Evidence = append(out.Evidence, InterpretationEvidence{Source: "semantic:evidence", Value: e, Weight: clamp01(r.Meaning.Confidence)})
	}

	out.ActionPosture = interpretationActionPosture(out, intent, goal, relevance)
	out.Meaning = composeInterpretationMeaning(out)
	out.Reason = interpretationReason(out)
	return out
}

func interpretationActionPosture(out ConversationalInterpretation, intent CommunicativeIntentEvaluation, goal ConversationalGoalInference, relevance ConversationalRelevance) InterpretationActionPosture {
	if out.NeedsClarification || out.HasUnresolvedRefs {
		if out.ActionOriented || intent.Primary == IntentRequestAction {
			return ActionPostureClarify
		}
	}
	switch intent.Primary {
	case IntentRequestAction:
		return ActionPostureRequest
	case IntentDecide:
		return ActionPostureDecide
	case IntentShareIdea, IntentDiscuss, IntentAskOpinion, IntentExplore:
		if goal.ActionRelated || relevance.RelatedToWork {
			return ActionPostureConsider
		}
		return ActionPostureDiscuss
	default:
		if out.ActionOriented {
			return ActionPostureConsider
		}
		return ActionPostureNone
	}
}

func composeInterpretationMeaning(out ConversationalInterpretation) string {
	parts := make([]string, 0, 4)
	if out.Intent != IntentUnknown {
		parts = append(parts, "intent="+string(out.Intent))
	}
	if out.Goal != GoalUnknown {
		parts = append(parts, "goal="+string(out.Goal))
	}
	if out.Topic != "" {
		parts = append(parts, "topic="+out.Topic)
	}
	if out.Objective != "" {
		parts = append(parts, "objective="+out.Objective)
	}
	return strings.Join(parts, "; ")
}

func interpretationReason(out ConversationalInterpretation) string {
	if out.NeedsClarification {
		return "interpretation remains incomplete because clarification/context is required"
	}
	if out.HasUnresolvedRefs {
		return "interpretation is constrained by unresolved references"
	}
	if out.ActionPosture == ActionPostureRequest {
		return "turn is interpreted as an action request; execution remains downstream"
	}
	if out.ActionPosture == ActionPostureConsider {
		return "turn is related to action/work but remains conversational rather than automatically executable"
	}
	if out.ActionPosture == ActionPostureDecide {
		return "turn is interpreted as participation in a decision"
	}
	if out.Relevance == RelevanceNewTopic {
		return "turn is interpreted as a new conversational topic"
	}
	return "turn interpreted from shared semantics, communicative intent, conversational goal and relevance"
}

func minFloat(values ...float64) float64 {
	if len(values) == 0 {
		return 0
	}
	out := values[0]
	for _, v := range values[1:] {
		if v < out {
			out = v
		}
	}
	return out
}

func cloneReferences(in []interaction.Reference) []interaction.Reference {
	return append([]interaction.Reference(nil), in...)
}
