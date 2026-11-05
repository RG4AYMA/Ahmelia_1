package cognition

import "ahmelia/interaction"

// ConversationalBehaviorDecision is the single cognition-owned decision for
// the current conversational context. It separates answering the current
// speaker from starting an additional, unsolicited intervention.
//
// Initiative policy is an input about admissibility of the additional
// intervention; it cannot override the current-turn action. Language may
// express this decision but may not change it.
type ConversationalBehaviorDecision struct {
	Version              string
	Action               ConversationalActionKind
	SpeakToCurrentTurn   bool
	AdditionalInitiative InitiativeDisposition
	Strategy             ConversationalResponseStrategy
	Reason               string
	Evidence             []string
	Uncertainty          bool
}

type InitiativeDisposition string

const (
	InitiativeNotEvaluated InitiativeDisposition = "not_evaluated"
	InitiativeWait         InitiativeDisposition = "wait"
	InitiativeIntervene    InitiativeDisposition = "intervene"
	InitiativeDefer        InitiativeDisposition = "defer"
)

// ResolveConversationalBehavior consolidates cognition's selected action and
// expression strategy with the semantic response requirement. ResponseIntent
// supplies turn semantics; it is not a second behavioral authority.
func ResolveConversationalBehavior(action ConversationalActionSelection, strategy ConversationalResponseStrategy, intent interaction.ResponseIntent) ConversationalBehaviorDecision {
	decision := ConversationalBehaviorDecision{
		Version:              "B96.40.1",
		Action:               action.Primary,
		SpeakToCurrentTurn:   intent.ShouldSpeak && action.Primary != ActionWait,
		AdditionalInitiative: InitiativeNotEvaluated,
		Strategy:             strategy,
		Reason:               action.Reason,
		Evidence:             append([]string(nil), action.Evidence...),
		Uncertainty:          action.Ambiguous || action.NeedsClarification || strategy.Ambiguous,
	}
	decision.Strategy.ShouldGenerateLanguage = decision.SpeakToCurrentTurn
	if !intent.ShouldSpeak {
		decision.Evidence = append(decision.Evidence, "response_intent:no_current_turn_communication")
	}
	if action.Primary == ActionWait {
		decision.Evidence = append(decision.Evidence, "cognition:wait_suppresses_current_turn_language")
	}
	return decision
}

// RefineActionForAdaptation lets transiently observed interaction needs affect
// conduct, not only wording. This applies across supported assistance needs
// (for example frustration, confusion, or an expressed need for explanation),
// while preserving explicit clarification, execution, and control decisions.
func RefineActionForAdaptation(action ConversationalActionSelection, adaptation ConversationalAdaptation) ConversationalActionSelection {
	if adaptation.Mode != AdaptationModeSupportive || action.Primary != ActionRespond {
		return action
	}
	action.Primary = ActionDiscuss
	action.Candidates = append(append([]ActionCandidate(nil), action.Candidates...), ActionCandidate{
		Kind: ActionDiscuss, Score: action.Confidence, Evidence: []string{"adaptation:supportive_participation"},
	})
	action.Evidence = append(append([]string(nil), action.Evidence...), "adaptation:supportive_participation")
	action.Reason = "supportive participation selected from transient user-state evidence"
	return action
}

// WithInitiative records the policy result for additional initiative without
// changing the already-decided current-turn response.
func (d ConversationalBehaviorDecision) WithInitiative(disposition InitiativeDisposition) ConversationalBehaviorDecision {
	d.AdditionalInitiative = disposition
	d.Evidence = append(append([]string(nil), d.Evidence...), "initiative:additional_intervention_"+string(disposition))
	return d
}

// InitiativeBehaviorDecision converts an already-authorized additional
// intervention into the shared behavioral contract before language rendering.
func InitiativeBehaviorDecision(strategy ConversationalResponseStrategy, disposition InitiativeDisposition) ConversationalBehaviorDecision {
	action := ConversationalActionSelection{
		Version: "B96.33.9", Primary: ActionDiscuss, Confidence: strategy.Confidence,
		Evidence: []string{"initiative:authorized_additional_intervention"},
		Reason:   "additional initiative was permitted by the initiative policy",
	}
	d := ResolveConversationalBehavior(action, strategy, interaction.ResponseIntent{Kind: interaction.ResponseIntentInvite, ShouldSpeak: true})
	return d.WithInitiative(disposition)
}
