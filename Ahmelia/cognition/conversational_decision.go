package cognition

import "ahmelia/interaction"

// B96.33.6 — CONVERSATIONAL DECISION
//
// Selects the conversational participation mode for the current turn.
// This is a decision about how Ahmelia should participate, not permission to
// execute a capability. Authorization and execution remain downstream.

type ConversationalDecisionKind string

const (
	DecisionUnknown       ConversationalDecisionKind = "unknown"
	DecisionRespond       ConversationalDecisionKind = "respond"
	DecisionAskClarify    ConversationalDecisionKind = "ask_clarification"
	DecisionDiscuss       ConversationalDecisionKind = "discuss"
	DecisionPropose       ConversationalDecisionKind = "propose"
	DecisionCorrect       ConversationalDecisionKind = "correct"
	DecisionDisagree      ConversationalDecisionKind = "disagree"
	DecisionConfirm       ConversationalDecisionKind = "confirm"
	DecisionAct           ConversationalDecisionKind = "act"
	DecisionWait          ConversationalDecisionKind = "wait"
	DecisionContinueTopic ConversationalDecisionKind = "continue_topic"
	DecisionChangeTopic   ConversationalDecisionKind = "change_topic"
)

type DecisionCandidate struct {
	Kind     ConversationalDecisionKind
	Score    float64
	Evidence []string
}

type ConversationalDecision struct {
	Version            string
	Primary            ConversationalDecisionKind
	Confidence         float64
	Ambiguous          bool
	NeedsClarification bool
	ActionCandidate    bool
	ExecutionRequested bool
	Candidates         []DecisionCandidate
	Evidence           []string
	Reason             string
}

// DecideConversationalParticipation maps established interpretation to a
// participation decision. It never authorizes or executes work.
func DecideConversationalParticipation(model ConversationalCognitiveModel, intent CommunicativeIntentEvaluation, goal ConversationalGoalInference, relevance ConversationalRelevance, interpretation ConversationalInterpretation) ConversationalDecision {
	candidates := make([]DecisionCandidate, 0, 8)
	add := func(kind ConversationalDecisionKind, score float64, evidence ...string) {
		if score > 0 {
			candidates = append(candidates, DecisionCandidate{Kind: kind, Score: clamp01(score), Evidence: uniqueStrings(evidence)})
		}
	}

	if interpretation.HasUnresolvedRefs {
		add(DecisionAskClarify, 0.99, "interpretation:unresolved_reference")
	} else if relevance.Primary == RelevanceNewTopic {
		add(DecisionChangeTopic, 0.94, "relevance:new_topic")
	} else if interpretation.NeedsClarification {
		add(DecisionAskClarify, 0.97, "interpretation:clarification_required")
	} else {
		switch intent.Primary {
		case IntentRequestAction:
			add(DecisionAct, 0.92, "intent:request_action")
		case IntentAskOpinion, IntentQuestion, IntentInform, IntentSocial:
			add(DecisionRespond, 0.86, "intent:requires_response")
		case IntentShareIdea:
			add(DecisionDiscuss, 0.86, "intent:share_idea")
		case IntentDiscuss, IntentExplore:
			add(DecisionDiscuss, 0.90, "intent:discussion_or_exploration")
		case IntentCorrect:
			add(DecisionCorrect, 0.92, "intent:correction")
		case IntentReject:
			add(DecisionDisagree, 0.90, "intent:rejection")
		case IntentConfirm:
			add(DecisionConfirm, 0.90, "intent:confirmation")
		case IntentDecide:
			add(DecisionDiscuss, 0.84, "intent:decision_discussion")
		case IntentContinue:
			add(DecisionContinueTopic, 0.91, "intent:continue")
		case IntentControl:
			add(DecisionWait, 0.82, "intent:control")
		default:
			add(DecisionRespond, 0.58, "fallback:respond")
		}
	}

	// A conversational proposal about active work should be discussed or
	// evaluated before any action is considered. A request_action remains an
	// action candidate, but this layer does not grant execution authority.
	if intent.Primary == IntentShareIdea && relevance.RelatedToWork {
		add(DecisionDiscuss, 0.93, "boundary:proposal_about_active_work")
	}
	if interpretation.ActionPosture == ActionPostureConsider {
		add(DecisionDiscuss, 0.88, "interpretation:consider_action")
	}
	if interpretation.ActionPosture == ActionPostureDecide {
		add(DecisionDiscuss, 0.86, "interpretation:decision_participation")
	}
	if goal.Primary == GoalStop {
		add(DecisionWait, 0.96, "goal:stop")
	}

	candidates = aggregateDecisionCandidates(candidates)
	sortDecisionCandidates(candidates)
	out := ConversationalDecision{Version: "B96.33.6", Candidates: candidates}
	if len(candidates) == 0 {
		out.Primary = DecisionUnknown
		out.NeedsClarification = true
		out.Reason = "no grounded conversational participation decision"
		return out
	}

	winner := candidates[0]
	out.Primary, out.Confidence = winner.Kind, winner.Score
	out.Evidence = append([]string(nil), winner.Evidence...)
	out.NeedsClarification = interpretation.NeedsClarification || interpretation.HasUnresolvedRefs
	out.ActionCandidate = winner.Kind == DecisionAct
	out.ExecutionRequested = out.ActionCandidate && intent.Primary == IntentRequestAction
	if len(candidates) > 1 {
		out.Ambiguous = winner.Score-candidates[1].Score < 0.12
	}
	if out.Ambiguous && out.ActionCandidate {
		out.NeedsClarification = true
		out.Primary = DecisionAskClarify
	}
	if out.Primary == DecisionAct {
		out.Reason = "user request indicates action; authorization and execution remain downstream"
	} else if out.Primary == DecisionAskClarify {
		out.Reason = "clarification is required before confident conversational participation"
	} else if out.Primary == DecisionChangeTopic {
		out.Reason = "explicit relevance boundary indicates a topic change"
	} else {
		out.Reason = "participation decision derived from interpretation, intent, goal and relevance"
	}
	return out
}

func aggregateDecisionCandidates(in []DecisionCandidate) []DecisionCandidate {
	out := make([]DecisionCandidate, 0, len(in))
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

func sortDecisionCandidates(in []DecisionCandidate) {
	for i := 0; i < len(in); i++ {
		for j := i + 1; j < len(in); j++ {
			if in[j].Score > in[i].Score {
				in[i], in[j] = in[j], in[i]
			}
		}
	}
}

// Keep interaction imported as an architectural assertion: this decision is
// explicitly downstream of the shared interaction contracts.
var _ interaction.SemanticRequest
