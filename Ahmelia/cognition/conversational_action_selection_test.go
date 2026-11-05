package cognition

import (
	"testing"

	"ahmelia/interaction"
)

func buildB96339(req interaction.SemanticRequest, ctx interaction.ConversationSnapshot) (ConversationalCognitiveModel, CommunicativeIntentEvaluation, ConversationalGoalInference, ConversationalRelevance, ConversationalInterpretation, ConversationalDecision, ConversationalUncertainty, ConversationalConflict) {
	model := NewConversationalCognitiveModel(req, ctx)
	intent := EvaluateCommunicativeIntent(model)
	goal := InferConversationalGoal(model, intent)
	relevance := EvaluateConversationalRelevance(model, intent, goal)
	interpretation := InterpretConversation(model, intent, goal, relevance)
	decision := DecideConversationalParticipation(model, intent, goal, relevance, interpretation)
	uncertainty := EvaluateConversationalUncertainty(model, intent, goal, relevance, interpretation, decision)
	conflict := EvaluateConversationalConflict(model, intent, goal, relevance, interpretation, decision)
	return model, intent, goal, relevance, interpretation, decision, uncertainty, conflict
}

func TestB96339_ActionRequestSelectsExecutionPostureWithoutExecution(t *testing.T) {
	req := interaction.SemanticRequest{
		Raw: "cambia enemigos", Normalized: "cambia enemigos",
		Kind: interaction.KindAction, SpeechAct: interaction.SpeechCommand,
		RequestedAction: "cambiar enemigos", Confidence: 0.96,
		Objective: interaction.Objective{Text: "cambiar enemigos"},
	}
	m, i, g, r, in, d, u, c := buildB96339(req, interaction.ConversationSnapshot{})
	a := SelectConversationalAction(m, i, g, r, in, d, u, c)
	if a.Primary != ActionRequestExecution || !a.ExecutionIntent {
		t.Fatalf("expected execution posture, got %+v", a)
	}
	if !a.CapabilityIndependent {
		t.Fatal("expected capability-independent selection")
	}
}

func TestB96339_UnresolvedReferenceForcesClarification(t *testing.T) {
	req := interaction.SemanticRequest{
		Raw: "hazlo con ese", Normalized: "hazlo con ese",
		Kind: interaction.KindAction, SpeechAct: interaction.SpeechCommand,
		RequestedAction: "hacerlo", Confidence: 0.96,
		References: []interaction.Reference{{Kind: interaction.ReferenceCurrentInput, Expression: "ese", Resolved: false}},
	}
	m, i, g, r, in, d, u, c := buildB96339(req, interaction.ConversationSnapshot{})
	a := SelectConversationalAction(m, i, g, r, in, d, u, c)
	if a.Primary != ActionClarify || !a.NeedsClarification || a.ExecutionIntent {
		t.Fatalf("expected clarification gate, got %+v", a)
	}
}

func TestB96339_ProposalDoesNotBecomeExecution(t *testing.T) {
	req := interaction.SemanticRequest{
		Raw: "podriamos mejorar enemigos", Normalized: "podriamos mejorar enemigos",
		Kind: interaction.KindConversation, SpeechAct: interaction.SpeechInform,
		Confidence: 0.93,
		Objective:  interaction.Objective{Text: "mejorar enemigos"},
	}
	m, i, g, r, in, d, u, c := buildB96339(req, interaction.ConversationSnapshot{ActiveTopic: "enemigos", ActiveObjective: "mejorar enemigos", ActiveWorkID: "w1"})
	a := SelectConversationalAction(m, i, g, r, in, d, u, c)
	if a.Primary == ActionRequestExecution || a.ExecutionIntent {
		t.Fatalf("proposal must not become execution: %+v", a)
	}
	if a.Primary != ActionPropose && a.Primary != ActionDiscuss {
		t.Fatalf("expected proposal/discussion, got %+v", a)
	}
}

func TestB96339_ConflictBlocksActionSelection(t *testing.T) {
	req := interaction.SemanticRequest{
		Raw: "hazlo con ese", Normalized: "hazlo con ese",
		Kind: interaction.KindAction, SpeechAct: interaction.SpeechCommand,
		RequestedAction: "hacerlo", Confidence: 0.95,
		References: []interaction.Reference{{Kind: interaction.ReferenceCurrentInput, Expression: "ese", Resolved: false}},
	}
	m, i, g, r, in, d, u, c := buildB96339(req, interaction.ConversationSnapshot{})
	if !c.BlocksAction && !u.BlocksAction {
		t.Fatal("expected at least one cognitive action blocker")
	}
	a := SelectConversationalAction(m, i, g, r, in, d, u, c)
	if a.Primary != ActionClarify || a.ExecutionIntent {
		t.Fatalf("expected clarification after conflict/uncertainty, got %+v", a)
	}
}

func TestB96339_NewTopicSelectsTopicChange(t *testing.T) {
	req := interaction.SemanticRequest{
		Raw: "hablemos de audio", Normalized: "hablemos de audio",
		Kind: interaction.KindConversation, SpeechAct: interaction.SpeechInform,
		Topic: "audio", Confidence: 0.94,
	}
	m, i, g, r, in, d, u, c := buildB96339(req, interaction.ConversationSnapshot{ActiveTopic: "enemigos", ActiveObjective: "mejorar enemigos", ActiveWorkID: "w1"})
	a := SelectConversationalAction(m, i, g, r, in, d, u, c)
	if a.Primary != ActionChangeTopic {
		t.Fatalf("expected topic change, got %+v", a)
	}
}

func TestB96339_DoesNotMutateContext(t *testing.T) {
	req := interaction.SemanticRequest{Raw: "continua", Normalized: "continua", Kind: interaction.KindConversation, SpeechAct: interaction.SpeechContinue, Continues: true, Confidence: 0.90}
	ctx := interaction.ConversationSnapshot{ActiveTopic: "enemigos", ActiveObjective: "mejorar IA", ActiveWorkID: "w1"}
	before := ctx
	m, i, g, r, in, d, u, c := buildB96339(req, ctx)
	_ = SelectConversationalAction(m, i, g, r, in, d, u, c)
	if ctx.ActiveTopic != before.ActiveTopic || ctx.ActiveObjective != before.ActiveObjective || ctx.ActiveWorkID != before.ActiveWorkID {
		t.Fatalf("selection mutated context: before=%+v after=%+v", before, ctx)
	}
}
