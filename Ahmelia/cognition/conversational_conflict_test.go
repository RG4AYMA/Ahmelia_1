package cognition

import (
	"testing"

	"ahmelia/interaction"
)

func buildB96338(req interaction.SemanticRequest, ctx interaction.ConversationSnapshot) (ConversationalCognitiveModel, CommunicativeIntentEvaluation, ConversationalGoalInference, ConversationalRelevance, ConversationalInterpretation, ConversationalDecision) {
	model := NewConversationalCognitiveModel(req, ctx)
	intent := EvaluateCommunicativeIntent(model)
	goal := InferConversationalGoal(model, intent)
	relevance := EvaluateConversationalRelevance(model, intent, goal)
	interpretation := InterpretConversation(model, intent, goal, relevance)
	decision := DecideConversationalParticipation(model, intent, goal, relevance, interpretation)
	return model, intent, goal, relevance, interpretation, decision
}

func TestB96338_ClearTurnHasNoConflict(t *testing.T) {
	req := interaction.SemanticRequest{
		Raw: "cambia enemigos", Normalized: "cambia enemigos",
		Kind: interaction.KindAction, SpeechAct: interaction.SpeechCommand,
		RequestedAction: "cambiar enemigos", Confidence: 0.95,
		Objective: interaction.Objective{Text: "cambiar enemigos"},
	}
	m, i, g, r, in, d := buildB96338(req, interaction.ConversationSnapshot{})
	c := EvaluateConversationalConflict(m, i, g, r, in, d)
	if c.HasConflict || c.BlocksAction || c.NeedsClarification {
		t.Fatalf("expected coherent turn, got %+v", c)
	}
}

func TestB96338_UnresolvedReferenceVsActionBlocks(t *testing.T) {
	req := interaction.SemanticRequest{
		Raw: "hazlo con ese", Normalized: "hazlo con ese",
		Kind: interaction.KindAction, SpeechAct: interaction.SpeechCommand,
		RequestedAction: "hacerlo", Confidence: 0.95,
		References: []interaction.Reference{{Kind: interaction.ReferenceCurrentInput, Expression: "ese", Resolved: false}},
	}
	m, i, g, r, in, d := buildB96338(req, interaction.ConversationSnapshot{})
	c := EvaluateConversationalConflict(m, i, g, r, in, d)
	if c.Primary != ConflictInterpretDecision || !c.BlocksAction {
		t.Fatalf("expected unresolved-reference conflict, got %+v", c)
	}
	if !c.RequiresClarificationBeforeAction(d) {
		t.Fatal("expected conflict to require clarification before action")
	}
}

func TestB96338_CorrectionCannotSilentlyExecute(t *testing.T) {
	req := interaction.SemanticRequest{
		Raw: "no, queria decir crear enemigos", Normalized: "no, queria decir crear enemigos",
		Kind: interaction.KindCorrection, SpeechAct: interaction.SpeechCorrect,
		RequestedAction: "crear enemigos", Confidence: 0.96,
		Correction: interaction.CorrectionInfo{IsCorrection: true, IsNegation: true, Replacement: "crear enemigos", Confidence: 0.97},
		Objective:  interaction.Objective{Text: "crear enemigos"},
	}
	m, i, g, r, in, d := buildB96338(req, interaction.ConversationSnapshot{})
	c := EvaluateConversationalConflict(m, i, g, r, in, d)
	if c.Primary != ConflictCorrectionAction || !c.BlocksAction {
		t.Fatalf("expected correction/action conflict, got %+v", c)
	}
}

func TestB96338_NewTopicAndContinuationConflict(t *testing.T) {
	req := interaction.SemanticRequest{
		Raw: "hablemos de audio", Normalized: "hablemos de audio",
		Kind: interaction.KindConversation, SpeechAct: interaction.SpeechInform,
		Topic: "audio", Continues: true, Confidence: 0.94,
	}
	ctx := interaction.ConversationSnapshot{ActiveTopic: "enemigos", ActiveObjective: "mejorar enemigos", ActiveWorkID: "w1"}
	m, i, g, r, in, d := buildB96338(req, ctx)
	c := EvaluateConversationalConflict(m, i, g, r, in, d)
	if c.Primary != ConflictTopicContinuity || !c.BlocksAction {
		t.Fatalf("expected topic/continuity conflict, got %+v", c)
	}
	_ = d
}

func TestB96338_DoesNotMutateConversationState(t *testing.T) {
	req := interaction.SemanticRequest{Raw: "cambia enemigos", Normalized: "cambia enemigos", Kind: interaction.KindAction, SpeechAct: interaction.SpeechCommand, RequestedAction: "cambiar enemigos", Confidence: 0.95}
	ctx := interaction.ConversationSnapshot{ActiveTopic: "enemigos", ActiveObjective: "mejorar IA", ActiveWorkID: "w1"}
	before := ctx
	m, i, g, r, in, d := buildB96338(req, ctx)
	_ = EvaluateConversationalConflict(m, i, g, r, in, d)
	if ctx.ActiveTopic != before.ActiveTopic || ctx.ActiveObjective != before.ActiveObjective || ctx.ActiveWorkID != before.ActiveWorkID {
		t.Fatalf("conflict evaluation mutated context: before=%+v after=%+v", before, ctx)
	}
}
