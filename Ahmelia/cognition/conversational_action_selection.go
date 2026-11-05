package cognition

import "ahmelia/interaction"

// B96.33.9 — CONVERSATIONAL ACTION SELECTION
//
// Selects the next abstract conversational action after decision, uncertainty,
// and conflict have been evaluated. The selected action is a cognitive
// participation instruction, not a capability invocation, authorization, or
// execution command.

type ConversationalActionKind string

const (
	ActionUnknown          ConversationalActionKind = "unknown"
	ActionRespond          ConversationalActionKind = "respond"
	ActionClarify          ConversationalActionKind = "clarify"
	ActionDiscuss          ConversationalActionKind = "discuss"
	ActionPropose          ConversationalActionKind = "propose"
	ActionCorrect          ConversationalActionKind = "correct"
	ActionDisagree         ConversationalActionKind = "disagree"
	ActionConfirm          ConversationalActionKind = "confirm"
	ActionRequestExecution ConversationalActionKind = "request_execution"
	ActionWait             ConversationalActionKind = "wait"
	ActionContinueTopic    ConversationalActionKind = "continue_topic"
	ActionChangeTopic      ConversationalActionKind = "change_topic"
)

type ActionCandidate struct {
	Kind     ConversationalActionKind
	Score    float64
	Evidence []string
}

type ConversationalActionSelection struct {
	Version               string
	Primary               ConversationalActionKind
	Confidence            float64
	Ambiguous             bool
	NeedsClarification    bool
	Blocked               bool
	ExecutionIntent       bool
	CapabilityIndependent bool
	Candidates            []ActionCandidate
	Evidence              []string
	Reason                string
}

// SelectConversationalAction converts the B96.33.6 participation decision into
// one abstract next conversational action. Uncertainty and conflict can block
// an action-oriented selection, in which case clarification wins. This layer
// never selects a provider/capability and never executes work.
func SelectConversationalAction(
	model ConversationalCognitiveModel,
	intent CommunicativeIntentEvaluation,
	goal ConversationalGoalInference,
	relevance ConversationalRelevance,
	interpretation ConversationalInterpretation,
	decision ConversationalDecision,
	uncertainty ConversationalUncertainty,
	conflict ConversationalConflict,
) ConversationalActionSelection {
	candidates := make([]ActionCandidate, 0, 8)
	add := func(kind ConversationalActionKind, score float64, evidence ...string) {
		if score <= 0 {
			return
		}
		candidates = append(candidates, ActionCandidate{Kind: kind, Score: clamp01(score), Evidence: uniqueStrings(evidence)})
	}

	switch decision.Primary {
	case DecisionAskClarify:
		add(ActionClarify, 0.99, "decision:ask_clarification")
	case DecisionRespond:
		add(ActionRespond, 0.90, "decision:respond")
	case DecisionDiscuss:
		add(ActionDiscuss, 0.90, "decision:discuss")
	case DecisionPropose:
		add(ActionPropose, 0.90, "decision:propose")
	case DecisionCorrect:
		add(ActionCorrect, 0.93, "decision:correct")
	case DecisionDisagree:
		add(ActionDisagree, 0.90, "decision:disagree")
	case DecisionConfirm:
		add(ActionConfirm, 0.90, "decision:confirm")
	case DecisionAct:
		add(ActionRequestExecution, 0.92, "decision:act", "boundary:execution_remains_downstream")
	case DecisionWait:
		add(ActionWait, 0.91, "decision:wait")
	case DecisionContinueTopic:
		add(ActionContinueTopic, 0.91, "decision:continue_topic")
	case DecisionChangeTopic:
		add(ActionChangeTopic, 0.94, "decision:change_topic")
	default:
		add(ActionUnknown, 0.20, "decision:unknown")
	}

	// Conversational proposals stay proposals/discussion; they must not be
	// promoted to an execution request merely because they concern active work.
	if intent.Primary == IntentShareIdea && !interpretation.ActionOriented {
		add(ActionPropose, 0.95, "intent:share_idea", "boundary:proposal_is_not_execution")
	}
	if interpretation.ActionPosture == ActionPostureConsider {
		add(ActionDiscuss, 0.94, "interpretation:consider_action")
	}
	if interpretation.ActionPosture == ActionPostureDecide {
		add(ActionDiscuss, 0.90, "interpretation:decision_participation")
	}
	if relevance.IsNewTopic && !decision.ActionCandidate {
		add(ActionChangeTopic, 0.96, "relevance:new_topic")
	}
	if goal.Primary == GoalStop {
		add(ActionWait, 0.97, "goal:stop")
	}

	// Hard cognitive blockers always take precedence over operational action.
	// A clarification action is itself a conversational action and is therefore
	// safe to select without granting execution authority.
	if decision.ActionCandidate && (uncertainty.RequiresClarificationBeforeAction(decision) || conflict.RequiresClarificationBeforeAction(decision)) {
		add(ActionClarify, 1.0, "gate:action_blocked_by_uncertainty_or_conflict")
	}
	if interpretation.HasUnresolvedRefs && decision.ActionCandidate {
		add(ActionClarify, 1.0, "gate:unresolved_reference")
	}

	candidates = aggregateActionCandidates(candidates)
	sortActionCandidates(candidates)
	out := ConversationalActionSelection{
		Version:               "B96.33.9",
		Candidates:            candidates,
		CapabilityIndependent: true,
		Confidence:            clamp01(minFloat(model.Signals.Confidence, intent.Confidence, goal.Confidence, relevance.Confidence, interpretation.Confidence, decision.Confidence)),
	}

	if len(candidates) == 0 {
		out.Primary = ActionUnknown
		out.NeedsClarification = true
		out.Reason = "no grounded conversational action selected"
		return out
	}

	winner := candidates[0]
	out.Primary = winner.Kind
	out.Confidence = winner.Score
	out.Evidence = append([]string(nil), winner.Evidence...)
	if len(candidates) > 1 {
		out.Ambiguous = winner.Score-candidates[1].Score < 0.12
	}

	out.NeedsClarification = interpretation.NeedsClarification || interpretation.HasUnresolvedRefs || uncertainty.NeedsClarification || conflict.NeedsClarification
	out.Blocked = uncertainty.RequiresClarificationBeforeAction(decision) || conflict.RequiresClarificationBeforeAction(decision)
	out.ExecutionIntent = out.Primary == ActionRequestExecution
	if out.ExecutionIntent && out.Blocked {
		// The selection must never leave an execution posture selected while a
		// cognitive blocker is active.
		out.Primary = ActionClarify
		out.ExecutionIntent = false
		out.NeedsClarification = true
		out.Evidence = uniqueStrings(append(out.Evidence, "gate:execution_action_removed_by_blocker"))
	}
	if out.Ambiguous && out.ExecutionIntent {
		out.Primary = ActionClarify
		out.ExecutionIntent = false
		out.NeedsClarification = true
		out.Blocked = true
		out.Evidence = uniqueStrings(append(out.Evidence, "gate:ambiguous_execution_selection"))
	}

	switch out.Primary {
	case ActionRequestExecution:
		out.Reason = "operational action was selected conceptually; authorization and execution remain downstream"
	case ActionClarify:
		out.Reason = "clarification selected because the conversational action is not sufficiently grounded for the current posture"
	case ActionPropose:
		out.Reason = "proposal selected as a conversational contribution, not as an execution command"
	default:
		out.Reason = "abstract conversational action selected from decision, interpretation, uncertainty and conflict"
	}
	return out
}

func aggregateActionCandidates(in []ActionCandidate) []ActionCandidate {
	out := make([]ActionCandidate, 0, len(in))
	for _, c := range in {
		found := false
		for i := range out {
			if out[i].Kind == c.Kind {
				if c.Score > out[i].Score {
					out[i].Score = c.Score
				}
				out[i].Evidence = uniqueStrings(append(out[i].Evidence, c.Evidence...))
				found = true
				break
			}
		}
		if !found {
			out = append(out, c)
		}
	}
	return out
}

func sortActionCandidates(in []ActionCandidate) {
	for i := 0; i < len(in); i++ {
		for j := i + 1; j < len(in); j++ {
			if in[j].Score > in[i].Score {
				in[i], in[j] = in[j], in[i]
			}
		}
	}
}

// Keep the shared interaction contract visible: this layer operates on
// established semantic cognition and does not introduce another parser.
var _ interaction.SemanticRequest
