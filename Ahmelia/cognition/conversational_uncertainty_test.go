package cognition

import (
	"testing"

	"ahmelia/interaction"
)

func buildB96337(req interaction.SemanticRequest, ctx interaction.ConversationSnapshot) (ConversationalCognitiveModel, CommunicativeIntentEvaluation, ConversationalGoalInference, ConversationalRelevance, ConversationalInterpretation, ConversationalDecision) {
	model := NewConversationalCognitiveModel(req, ctx)
	intent := EvaluateCommunicativeIntent(model)
	goal := InferConversationalGoal(model, intent)
	relevance := EvaluateConversationalRelevance(model, intent, goal)
	interpretation := InterpretConversation(model, intent, goal, relevance)
	decision := DecideConversationalParticipation(model, intent, goal, relevance, interpretation)
	return model, intent, goal, relevance, interpretation, decision
}

func TestB96337_ClearConversationHasNoMaterialUncertainty(t *testing.T) {
	req := interaction.SemanticRequest{
		Raw: "¿qué opinas del sistema?", Normalized: "qué opinas del sistema",
		Kind: interaction.KindQuestion, SpeechAct: interaction.SpeechQuestion,
		QuestionType: "opinion", Confidence: 0.95, Topic: "sistema",
		Meaning: interaction.SemanticMeaning{Primary: interaction.ConceptOpinion, Confidence: 0.95},
	}
	m, i, g, r, in, d := buildB96337(req, interaction.ConversationSnapshot{})
	u := EvaluateConversationalUncertainty(m, i, g, r, in, d)
	if u.BlocksAction || u.NeedsClarification || u.Uncertain {
		t.Fatalf("expected no material uncertainty, got %+v", u)
	}
}

func TestB96337_UnresolvedReferenceBlocksAction(t *testing.T) {
	req := interaction.SemanticRequest{
		Raw: "hazlo con ese", Normalized: "hazlo con ese",
		Kind: interaction.KindAction, SpeechAct: interaction.SpeechCommand,
		RequestedAction: "hacerlo", Confidence: 0.95,
		References: []interaction.Reference{{Kind: interaction.ReferenceCurrentInput, Expression: "ese", Resolved: false}},
	}
	m, i, g, r, in, d := buildB96337(req, interaction.ConversationSnapshot{})
	u := EvaluateConversationalUncertainty(m, i, g, r, in, d)
	if u.Primary != UncertaintyReference || !u.BlocksAction || !u.NeedsClarification {
		t.Fatalf("expected reference uncertainty to block action, got %+v", u)
	}
	if !u.RequiresClarificationBeforeAction(d) {
		t.Fatal("expected clarification before action")
	}
}

func TestB96337_AmbiguousActionBlocksWithoutExecuting(t *testing.T) {
	req := interaction.SemanticRequest{
		Raw: "hazlo", Normalized: "hazlo",
		Kind: interaction.KindAction, SpeechAct: interaction.SpeechCommand,
		RequestedAction: "hacerlo", Confidence: 0.45,
		References: []interaction.Reference{{Kind: interaction.ReferenceCurrentInput, Expression: "lo", Resolved: true, Value: "tarea"}},
	}
	m, i, g, r, in, d := buildB96337(req, interaction.ConversationSnapshot{})
	u := EvaluateConversationalUncertainty(m, i, g, r, in, d)
	if d.Primary != DecisionAskClarify && !u.BlocksAction {
		t.Fatalf("expected ambiguous action to be blocked, decision=%+v uncertainty=%+v", d, u)
	}
	if d.ExecutionRequested && !u.BlocksAction {
		t.Fatal("low confidence must not leave an unblocked execution request")
	}
}

func TestB96337_UncertaintyDoesNotAuthorizeExecution(t *testing.T) {
	req := interaction.SemanticRequest{
		Raw: "cambia enemigos", Normalized: "cambia enemigos",
		Kind: interaction.KindAction, SpeechAct: interaction.SpeechCommand,
		RequestedAction: "cambiar enemigos", Confidence: 0.95,
	}
	m, i, g, r, in, d := buildB96337(req, interaction.ConversationSnapshot{})
	u := EvaluateConversationalUncertainty(m, i, g, r, in, d)
	if d.Primary != DecisionAct || !d.ExecutionRequested {
		t.Fatalf("expected action request to remain represented, got %+v", d)
	}
	if u.RequiresClarificationBeforeAction(d) {
		t.Fatal("clear action request should not be blocked by uncertainty")
	}
	// There is deliberately no authorization or execution operation in this layer.
}

func TestB96337_CurrentTurnProductionDoesNotSelfConflictInRelevance(t *testing.T) {
	req := interaction.Analyze("Crea un pequeño programa Go llamado demo_fase4 que muestre Hola y añade un test.", interaction.Context{
		CurrentTopic:    "test",
		ActiveObjective: "Crea un pequeño programa Go llamado demo_fase4 que muestre Hola y añade un test.",
		Language:        "es",
	})
	m, i, g, r, in, d := buildB96337(req, interaction.ConversationSnapshot{
		ActiveTopic:     req.Topic,
		ActiveObjective: req.Objective.Text,
	})
	u := EvaluateConversationalUncertainty(m, i, g, r, in, d)
	if u.BlocksAction || u.NeedsClarification {
		t.Fatalf("same-turn goal/topic overlap must not block an explicit production request: relevance=%+v uncertainty=%+v decision=%+v", r, u, d)
	}
	if d.Primary != DecisionAct || !d.ExecutionRequested {
		t.Fatalf("expected action decision for grounded production request: decision=%+v", d)
	}
}
