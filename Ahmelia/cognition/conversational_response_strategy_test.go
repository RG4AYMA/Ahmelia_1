package cognition

import (
	"testing"

	"ahmelia/interaction"
)

func buildB963310(req interaction.SemanticRequest, ctx interaction.ConversationSnapshot) (ConversationalCognitiveModel, CommunicativeIntentEvaluation, ConversationalGoalInference, ConversationalRelevance, ConversationalInterpretation, ConversationalDecision, ConversationalUncertainty, ConversationalConflict, ConversationalActionSelection) {
	model := NewConversationalCognitiveModel(req, ctx)
	intent := EvaluateCommunicativeIntent(model)
	goal := InferConversationalGoal(model, intent)
	relevance := EvaluateConversationalRelevance(model, intent, goal)
	interpretation := InterpretConversation(model, intent, goal, relevance)
	decision := DecideConversationalParticipation(model, intent, goal, relevance, interpretation)
	uncertainty := EvaluateConversationalUncertainty(model, intent, goal, relevance, interpretation, decision)
	conflict := EvaluateConversationalConflict(model, intent, goal, relevance, interpretation, decision)
	action := SelectConversationalAction(model, intent, goal, relevance, interpretation, decision, uncertainty, conflict)
	return model, intent, goal, relevance, interpretation, decision, uncertainty, conflict, action
}

func TestB963310_AnswerStrategyForClearQuestion(t *testing.T) {
	req := interaction.SemanticRequest{
		Raw: "¿qué opinas del sistema?", Normalized: "qué opinas del sistema",
		Kind: interaction.KindQuestion, SpeechAct: interaction.SpeechQuestion,
		Confidence: 0.95, Topic: "sistema",
		Meaning: interaction.SemanticMeaning{Primary: interaction.ConceptUnknown, Confidence: 0.0},
	}
	m, i, g, r, in, d, u, c, a := buildB963310(req, interaction.ConversationSnapshot{})
	s := SelectConversationalResponseStrategy(m, i, g, r, in, d, u, c, a)
	if s.Mode != ResponseModeAnswer || s.Structure != StructureExplanation || s.NeedsClarification {
		t.Fatalf("expected direct answer strategy, got %+v", s)
	}
	if !s.ShouldGenerateLanguage || !s.PreserveUserAgency {
		t.Fatalf("expected language realization and agency preservation, got %+v", s)
	}
}

func TestB963310_ClarificationStrategyForUnresolvedReference(t *testing.T) {
	req := interaction.SemanticRequest{
		Raw: "hazlo con ese", Normalized: "hazlo con ese",
		Kind: interaction.KindAction, SpeechAct: interaction.SpeechCommand,
		RequestedAction: "hacerlo", Confidence: 0.96,
		References: []interaction.Reference{{Kind: interaction.ReferenceCurrentInput, Expression: "ese", Resolved: false}},
	}
	m, i, g, r, in, d, u, c, a := buildB963310(req, interaction.ConversationSnapshot{})
	s := SelectConversationalResponseStrategy(m, i, g, r, in, d, u, c, a)
	if s.Mode != ResponseModeClarify || s.Structure != StructureClarifying || !s.NeedsClarification {
		t.Fatalf("expected clarification strategy, got %+v", s)
	}
}

func TestB963310_ProposalStrategyNeverImpliesExecution(t *testing.T) {
	req := interaction.SemanticRequest{
		Raw: "podriamos mejorar enemigos", Normalized: "podriamos mejorar enemigos",
		Kind: interaction.KindConversation, SpeechAct: interaction.SpeechInform,
		Confidence: 0.93, Objective: interaction.Objective{Text: "mejorar enemigos"},
	}
	m, i, g, r, in, d, u, c, a := buildB963310(req, interaction.ConversationSnapshot{ActiveTopic: "enemigos", ActiveObjective: "mejorar enemigos", ActiveWorkID: "w1"})
	s := SelectConversationalResponseStrategy(m, i, g, r, in, d, u, c, a)
	if s.Mode == ResponseModeRequestExecution || a.Primary == ActionRequestExecution {
		t.Fatalf("proposal must not imply execution: strategy=%+v action=%+v", s, a)
	}
	if s.Mode != ResponseModePropose && s.Mode != ResponseModeDiscuss {
		t.Fatalf("expected proposal/discussion strategy, got %+v", s)
	}
	if !s.PreserveUserAgency || !s.ExecutionBoundary {
		t.Fatalf("expected explicit agency/execution boundaries, got %+v", s)
	}
}

func TestB963310_ClearActionGetsBoundaryStrategy(t *testing.T) {
	req := interaction.SemanticRequest{
		Raw: "cambia enemigos", Normalized: "cambia enemigos",
		Kind: interaction.KindAction, SpeechAct: interaction.SpeechCommand,
		RequestedAction: "cambiar enemigos", Confidence: 0.95,
		Objective: interaction.Objective{Text: "cambiar enemigos"},
	}
	m, i, g, r, in, d, u, c, a := buildB963310(req, interaction.ConversationSnapshot{})
	s := SelectConversationalResponseStrategy(m, i, g, r, in, d, u, c, a)
	if a.Primary != ActionRequestExecution {
		t.Fatalf("expected conceptual execution request, got %+v", a)
	}
	if s.Mode != ResponseModeRequestExecution || s.Structure != StructureActionBoundary {
		t.Fatalf("expected action-boundary strategy, got %+v", s)
	}
	if !s.ExecutionBoundary || !s.PreserveUserAgency {
		t.Fatalf("expected downstream execution boundary, got %+v", s)
	}
}

func TestB963310_BlockerOverridesExecutionStrategy(t *testing.T) {
	req := interaction.SemanticRequest{
		Raw: "hazlo con ese", Normalized: "hazlo con ese",
		Kind: interaction.KindAction, SpeechAct: interaction.SpeechCommand,
		RequestedAction: "hacerlo", Confidence: 0.95,
		References: []interaction.Reference{{Kind: interaction.ReferenceCurrentInput, Expression: "ese", Resolved: false}},
	}
	m, i, g, r, in, d, u, c, a := buildB963310(req, interaction.ConversationSnapshot{})
	s := SelectConversationalResponseStrategy(m, i, g, r, in, d, u, c, a)
	if s.Mode != ResponseModeClarify || s.Mode == ResponseModeRequestExecution {
		t.Fatalf("blocker must override execution strategy, got %+v", s)
	}
	if !s.ExecutionBoundary || !s.NeedsClarification {
		t.Fatalf("expected preserved boundary and clarification, got %+v", s)
	}
}

func TestB963310_StrategyDoesNotMutateContext(t *testing.T) {
	req := interaction.SemanticRequest{Raw: "continua", Normalized: "continua", Kind: interaction.KindConversation, SpeechAct: interaction.SpeechContinue, Continues: true, Confidence: 0.90}
	ctx := interaction.ConversationSnapshot{ActiveTopic: "enemigos", ActiveObjective: "mejorar IA", ActiveWorkID: "w1"}
	before := ctx
	m, i, g, r, in, d, u, c, a := buildB963310(req, ctx)
	_ = SelectConversationalResponseStrategy(m, i, g, r, in, d, u, c, a)
	if ctx.ActiveTopic != before.ActiveTopic || ctx.ActiveObjective != before.ActiveObjective || ctx.ActiveWorkID != before.ActiveWorkID {
		t.Fatalf("strategy mutated context: before=%+v after=%+v", before, ctx)
	}
}
