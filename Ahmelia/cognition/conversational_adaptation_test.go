package cognition

import (
	"reflect"
	"testing"

	"ahmelia/interaction"
)

func buildB963311(req interaction.SemanticRequest, ctx interaction.ConversationSnapshot) (ConversationalCognitiveModel, CommunicativeIntentEvaluation, ConversationalGoalInference, ConversationalRelevance, ConversationalInterpretation, ConversationalDecision, ConversationalUncertainty, ConversationalConflict, ConversationalActionSelection, ConversationalResponseStrategy) {
	model := NewConversationalCognitiveModel(req, ctx)
	intent := EvaluateCommunicativeIntent(model)
	goal := InferConversationalGoal(model, intent)
	relevance := EvaluateConversationalRelevance(model, intent, goal)
	interpretation := InterpretConversation(model, intent, goal, relevance)
	decision := DecideConversationalParticipation(model, intent, goal, relevance, interpretation)
	uncertainty := EvaluateConversationalUncertainty(model, intent, goal, relevance, interpretation, decision)
	conflict := EvaluateConversationalConflict(model, intent, goal, relevance, interpretation, decision)
	action := SelectConversationalAction(model, intent, goal, relevance, interpretation, decision, uncertainty, conflict)
	strategy := SelectConversationalResponseStrategy(model, intent, goal, relevance, interpretation, decision, uncertainty, conflict, action)
	return model, intent, goal, relevance, interpretation, decision, uncertainty, conflict, action, strategy
}

func TestB963311_ActiveWorkActionAdaptsToTaskFocus(t *testing.T) {
	req := interaction.SemanticRequest{Raw: "cambia enemigos", Normalized: "cambia enemigos", Kind: interaction.KindAction, SpeechAct: interaction.SpeechCommand, RequestedAction: "cambiar enemigos", Objective: interaction.Objective{Text: "cambiar enemigos"}, Confidence: 0.95}
	m, i, g, r, in, d, u, c, a, s := buildB963311(req, interaction.ConversationSnapshot{ActiveWorkID: "w1", ActiveTopic: "enemigos"})
	ad := AdaptConversationalParticipation(m, i, g, r, in, d, u, c, a, s, interaction.ConversationSnapshot{ActiveWorkID: "w1", ActiveTopic: "enemigos"})
	if ad.Mode != AdaptationModeTaskFocused || ad.Detail != AdaptationDetailConcise || !ad.PreserveUserAgency {
		t.Fatalf("expected task-focused adaptation, got %+v", ad)
	}
}

func TestB963311_UncertaintyAdaptsToCaution(t *testing.T) {
	req := interaction.SemanticRequest{Raw: "hazlo con ese", Normalized: "hazlo con ese", Kind: interaction.KindAction, SpeechAct: interaction.SpeechCommand, RequestedAction: "hacerlo", Confidence: 0.95, References: []interaction.Reference{{Kind: interaction.ReferenceCurrentInput, Expression: "ese", Resolved: false}}}
	m, i, g, r, in, d, u, c, a, s := buildB963311(req, interaction.ConversationSnapshot{})
	ad := AdaptConversationalParticipation(m, i, g, r, in, d, u, c, a, s, interaction.ConversationSnapshot{})
	if !ad.Caution || ad.Mode != AdaptationModeCautious || ad.Pacing != AdaptationPacingCareful || ad.Directness != DirectnessCautious {
		t.Fatalf("expected cautious adaptation, got %+v", ad)
	}
}

func TestB963311_NewTopicResetsLocalAdaptation(t *testing.T) {
	req := interaction.SemanticRequest{Raw: "hablemos de música", Normalized: "hablemos de música", Kind: interaction.KindConversation, SpeechAct: interaction.SpeechInform, Confidence: 0.95, Topic: "música", ExplicitDomain: "music"}
	ctx := interaction.ConversationSnapshot{ActiveWorkID: "w1", ActiveTopic: "enemigos", ActiveObjective: "mejorar enemigos"}
	m, i, g, r, in, d, u, c, a, s := buildB963311(req, ctx)
	ad := AdaptConversationalParticipation(m, i, g, r, in, d, u, c, a, s, ctx)
	if ad.Mode != AdaptationModeResetContext || ad.Continuity {
		t.Fatalf("expected local adaptation reset, got %+v", ad)
	}
}

func TestB963311_ConfusionIncreasesDetailWithoutPersistentProfile(t *testing.T) {
	ctx := interaction.ConversationSnapshot{UserState: interaction.UserState{Confusion: 0.85, InteractionNeed: interaction.NeedExplanation}}
	req := interaction.SemanticRequest{Raw: "explícame esto", Normalized: "explicame esto", Kind: interaction.KindQuestion, SpeechAct: interaction.SpeechQuestion, Confidence: 0.90}
	m, i, g, r, in, d, u, c, a, s := buildB963311(req, ctx)
	ad := AdaptConversationalParticipation(m, i, g, r, in, d, u, c, a, s, ctx)
	if ad.Detail != AdaptationDetailDetailed || ad.Mode != AdaptationModeSupportive || !ad.Transient {
		t.Fatalf("expected transient supportive detail adaptation, got %+v", ad)
	}
	if !reflect.DeepEqual(ctx.UserState, interaction.UserState{Confusion: 0.85, InteractionNeed: interaction.NeedExplanation}) {
		t.Fatalf("adaptation must not mutate user state: %+v", ctx.UserState)
	}
}

func TestB963311_UrgencyMakesResponseConcise(t *testing.T) {
	ctx := interaction.ConversationSnapshot{UserState: interaction.UserState{Urgency: 0.80}}
	req := interaction.SemanticRequest{Raw: "¿qué es esto?", Normalized: "qué es esto", Kind: interaction.KindQuestion, SpeechAct: interaction.SpeechQuestion, Confidence: 0.95}
	m, i, g, r, in, d, u, c, a, s := buildB963311(req, ctx)
	ad := AdaptConversationalParticipation(m, i, g, r, in, d, u, c, a, s, ctx)
	if ad.Detail != AdaptationDetailConcise || ad.Pacing != AdaptationPacingFast {
		t.Fatalf("expected concise fast adaptation, got %+v", ad)
	}
}

func TestB963311_ContinuationPreservesContinuity(t *testing.T) {
	req := interaction.SemanticRequest{Raw: "continúa", Normalized: "continua", Kind: interaction.KindContinuation, SpeechAct: interaction.SpeechContinue, FollowUp: true, Continues: true, Confidence: 0.95}
	ctx := interaction.ConversationSnapshot{ActiveTopic: "enemigos", ActiveObjective: "mejorar enemigos"}
	m, i, g, r, in, d, u, c, a, s := buildB963311(req, ctx)
	ad := AdaptConversationalParticipation(m, i, g, r, in, d, u, c, a, s, ctx)
	if !ad.Continuity || !ad.Caution || ad.Mode != AdaptationModeCautious {
		t.Fatalf("expected continuity to be preserved while honoring upstream caution, got %+v", ad)
	}
}
