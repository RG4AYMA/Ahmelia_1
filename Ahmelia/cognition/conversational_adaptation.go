package cognition

import "ahmelia/interaction"

// B96.33.11 — CONVERSATIONAL ADAPTATION
//
// Adapts an already-selected conversational response strategy to transient
// interaction signals. This is a turn/session-local cognitive adjustment; it
// does not create a persistent user model, infer stable traits, generate
// language, authorize work, or execute capabilities.

type ConversationalAdaptationMode string

const (
	AdaptationModeDefault      ConversationalAdaptationMode = "default"
	AdaptationModeCautious     ConversationalAdaptationMode = "cautious"
	AdaptationModeTaskFocused  ConversationalAdaptationMode = "task_focused"
	AdaptationModeExploratory  ConversationalAdaptationMode = "exploratory"
	AdaptationModeContinuity   ConversationalAdaptationMode = "continuity"
	AdaptationModeResetContext ConversationalAdaptationMode = "reset_context"
	AdaptationModeSupportive   ConversationalAdaptationMode = "supportive"
	AdaptationModeMinimal      ConversationalAdaptationMode = "minimal"
)

type AdaptationDetail string

const (
	AdaptationDetailDefault  AdaptationDetail = "default"
	AdaptationDetailConcise  AdaptationDetail = "concise"
	AdaptationDetailStandard AdaptationDetail = "standard"
	AdaptationDetailDetailed AdaptationDetail = "detailed"
)

type AdaptationPacing string

const (
	AdaptationPacingDefault AdaptationPacing = "default"
	AdaptationPacingNormal  AdaptationPacing = "normal"
	AdaptationPacingFast    AdaptationPacing = "fast"
	AdaptationPacingCareful AdaptationPacing = "careful"
)

type ConversationalAdaptation struct {
	Version            string
	Mode               ConversationalAdaptationMode
	Detail             AdaptationDetail
	Pacing             AdaptationPacing
	Directness         ResponseDirectness
	Initiative         ResponseInitiative
	Continuity         bool
	Caution            bool
	PreserveUserAgency bool
	Transient          bool
	Confidence         float64
	Evidence           []string
	Reason             string
}

// AdaptConversationalParticipation derives contextual adjustments from the
// established cognition chain and the current conversational state. UserState
// is consumed as an interaction signal only; it is not promoted into a stable
// user preference or personality model.
func AdaptConversationalParticipation(
	model ConversationalCognitiveModel,
	intent CommunicativeIntentEvaluation,
	goal ConversationalGoalInference,
	relevance ConversationalRelevance,
	interpretation ConversationalInterpretation,
	decision ConversationalDecision,
	uncertainty ConversationalUncertainty,
	conflict ConversationalConflict,
	action ConversationalActionSelection,
	strategy ConversationalResponseStrategy,
	ctx interaction.ConversationSnapshot,
) ConversationalAdaptation {
	out := ConversationalAdaptation{
		Version:            "B96.33.11",
		Mode:               AdaptationModeDefault,
		Detail:             AdaptationDetailDefault,
		Pacing:             AdaptationPacingDefault,
		Directness:         strategy.Directness,
		Initiative:         strategy.Initiative,
		Continuity:         relevance.ContinuesGoal || relevance.ContinuesTopic || model.Request.FollowUp || model.Request.Continues,
		Caution:            strategy.Directness == DirectnessCautious,
		PreserveUserAgency: strategy.PreserveUserAgency,
		Transient:          true,
		Confidence:         minFloat(model.Signals.Confidence, intent.Confidence, goal.Confidence, relevance.Confidence, interpretation.Confidence, decision.Confidence),
	}

	// Start from the response strategy and only apply evidence-backed deltas.
	switch strategy.Mode {
	case ResponseModeAnswer:
		out.Detail = AdaptationDetailStandard
	case ResponseModeDiscuss:
		out.Mode = AdaptationModeExploratory
		out.Detail = AdaptationDetailStandard
	case ResponseModePropose:
		out.Mode = AdaptationModeExploratory
		out.Initiative = maxInitiative(out.Initiative, InitiativeHelpful)
	case ResponseModeClarify:
		out.Mode = AdaptationModeCautious
		out.Caution = true
		out.Pacing = AdaptationPacingCareful
	case ResponseModeRequestExecution:
		out.Mode = AdaptationModeTaskFocused
		out.Detail = AdaptationDetailConcise
	case ResponseModeWait:
		out.Mode = AdaptationModeMinimal
		out.Initiative = InitiativeMinimal
	case ResponseModeContinue:
		out.Mode = AdaptationModeContinuity
	case ResponseModeChangeTopic:
		out.Mode = AdaptationModeResetContext
		out.Continuity = false
	}

	// Active work supports task focus, but only when the current turn is also
	// action-oriented. Mere presence of work must not hijack a new topic.
	if ctx.ActiveWorkID != "" && (action.Primary == ActionRequestExecution || intent.ActionOriented || goal.ActionRelated) && relevance.Primary != RelevanceNewTopic {
		out.Mode = AdaptationModeTaskFocused
		out.Detail = AdaptationDetailConcise
		out.Directness = maxDirectness(out.Directness, DirectnessDirect)
		out.Evidence = append(out.Evidence, "context:active_work")
	}

	// Follow-up/continuation preserves local conversational continuity.
	if out.Continuity && relevance.Primary != RelevanceNewTopic {
		if out.Mode == AdaptationModeDefault {
			out.Mode = AdaptationModeContinuity
		}
		out.Evidence = append(out.Evidence, "context:continuation")
	}

	// Explicit topic change resets task-specific adaptation rather than carrying
	// old work assumptions into the new topic.
	if relevance.Primary == RelevanceNewTopic || decision.Primary == DecisionChangeTopic {
		out.Mode = AdaptationModeResetContext
		out.Continuity = false
		out.Evidence = append(out.Evidence, "context:new_topic_resets_local_adaptation")
	}

	// Existing uncertainty/conflict and unresolved references increase caution;
	// they never become a reason to invent certainty or silently act.
	if interpretation.HasUnresolvedRefs || uncertainty.BlocksAction || conflict.BlocksAction || action.Blocked {
		out.Mode = AdaptationModeCautious
		out.Caution = true
		out.Pacing = AdaptationPacingCareful
		out.Directness = DirectnessCautious
		out.Initiative = InitiativeGuiding
		out.Evidence = append(out.Evidence, "cognitive_gate:caution")
	}

	// Current interaction signals can tune participation without creating a
	// durable user profile. These are observable communication signals only.
	u := ctx.UserState
	if u.Confusion >= 0.60 || u.InteractionNeed == interaction.NeedExplanation {
		out.Detail = AdaptationDetailDetailed
		out.Mode = AdaptationModeSupportive
		out.Initiative = InitiativeGuiding
		out.Evidence = append(out.Evidence, "user_signal:confusion_or_explanation_need")
	}
	if u.Frustration >= 0.60 || u.InteractionNeed == interaction.NeedAssistance {
		out.Detail = AdaptationDetailStandard
		out.Mode = AdaptationModeSupportive
		out.Initiative = InitiativeHelpful
		out.Evidence = append(out.Evidence, "user_signal:assistance_need")
	}
	if u.Urgency >= 0.60 {
		out.Detail = AdaptationDetailConcise
		out.Pacing = AdaptationPacingFast
		out.Evidence = append(out.Evidence, "user_signal:urgency")
	}
	if u.InteractionNeed == interaction.NeedCorrection || intent.Primary == IntentCorrect {
		out.Mode = AdaptationModeCautious
		out.Directness = DirectnessDirect
		out.Evidence = append(out.Evidence, "user_signal:correction")
	}

	// Cognitive safety gates outrank convenience adaptations. In particular, a
	// supportive/urgent adaptation cannot weaken an action boundary.
	if strategy.ExecutionBoundary || action.Primary == ActionRequestExecution {
		out.PreserveUserAgency = true
	}
	if out.Caution {
		out.Initiative = maxInitiative(out.Initiative, InitiativeGuiding)
	}

	out.Confidence = clamp01(out.Confidence)
	out.Evidence = uniqueStrings(out.Evidence)
	out.Reason = "transient conversational adaptation derived from established cognitive evidence and current interaction signals"
	return out
}

func maxInitiative(a, b ResponseInitiative) ResponseInitiative {
	rank := func(v ResponseInitiative) int {
		switch v {
		case InitiativeGuiding:
			return 3
		case InitiativeHelpful:
			return 2
		case InitiativeMinimal:
			return 1
		default:
			return 0
		}
	}
	if rank(b) > rank(a) {
		return b
	}
	return a
}

func maxDirectness(a, b ResponseDirectness) ResponseDirectness {
	rank := func(v ResponseDirectness) int {
		switch v {
		case DirectnessDirect:
			return 3
		case DirectnessGuided:
			return 2
		case DirectnessExploratory:
			return 1
		default:
			return 0
		}
	}
	if rank(b) > rank(a) {
		return b
	}
	return a
}
