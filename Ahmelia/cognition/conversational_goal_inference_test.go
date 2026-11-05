package cognition

import (
	"ahmelia/interaction"
	"testing"
)

func TestB96333_ProposalInfersDesiredChangeWithoutExecution(t *testing.T) {
	r := interaction.SemanticRequest{Raw: "estoy pensando en cambiar el sistema de enemigos", Normalized: "estoy pensando en cambiar el sistema de enemigos", Kind: interaction.KindProposal, SpeechAct: interaction.SpeechPropose, Confidence: .9, Topic: "enemigos", Objective: interaction.Objective{Text: "cambiar el sistema de enemigos"}}
	m := NewConversationalCognitiveModel(r, interaction.ConversationSnapshot{})
	in := EvaluateCommunicativeIntent(m)
	g := InferConversationalGoal(m, in)
	if g.Primary != GoalChange {
		t.Fatalf("expected change goal, got %+v", g)
	}
	if g.Objective == "" || g.ActionRelated == false {
		t.Fatalf("expected concrete action-related goal, got %+v", g)
	}
}

func TestB96333_AskOpinionInfersOpinionGoal(t *testing.T) {
	r := interaction.SemanticRequest{Raw: "¿qué opinas del sistema?", Kind: interaction.KindQuestion, SpeechAct: interaction.SpeechQuestion, QuestionType: "opinion", Confidence: .9, Topic: "sistema", Meaning: interaction.SemanticMeaning{Concepts: []interaction.SemanticConcept{interaction.ConceptQuestion, interaction.ConceptOpinion}}}
	m := NewConversationalCognitiveModel(r, interaction.ConversationSnapshot{})
	g := InferConversationalGoal(m, EvaluateCommunicativeIntent(m))
	if g.Primary != GoalGetOpinion {
		t.Fatalf("expected get_opinion, got %+v", g)
	}
}

func TestB96333_FollowUpCarriesActiveObjective(t *testing.T) {
	ctx := interaction.ConversationSnapshot{ActiveObjective: "mejorar la IA de enemigos", ActiveTopic: "enemigos", ActiveThreadID: "thread-1"}
	r := interaction.SemanticRequest{Raw: "la segunda", Kind: interaction.KindContinuation, SpeechAct: interaction.SpeechContinue, FollowUp: true, Continues: true, AddressesCurrent: true, Confidence: .9}
	m := NewConversationalCognitiveModel(r, ctx)
	g := InferConversationalGoal(m, EvaluateCommunicativeIntent(m))
	if !g.CarriedFromContext || g.Objective != ctx.ActiveObjective {
		t.Fatalf("expected carried objective, got %+v", g)
	}
}

func TestB96333_UnknownGoalNeedsClarification(t *testing.T) {
	r := interaction.SemanticRequest{Raw: "no sé", Kind: interaction.KindUnknown}
	m := NewConversationalCognitiveModel(r, interaction.ConversationSnapshot{})
	g := InferConversationalGoal(m, EvaluateCommunicativeIntent(m))
	if g.Primary != GoalUnknown || !g.NeedsClarification {
		t.Fatalf("expected unknown + clarification, got %+v", g)
	}
}

func TestB96333_DecisionIsNotExecution(t *testing.T) {
	r := interaction.SemanticRequest{Raw: "decidamos cambiarlo", Kind: interaction.KindConversation, SpeechAct: interaction.SpeechInform, Confidence: .9, Meaning: interaction.SemanticMeaning{Concepts: []interaction.SemanticConcept{interaction.ConceptDecision}}, Objective: interaction.Objective{Text: "cambiarlo"}}
	m := NewConversationalCognitiveModel(r, interaction.ConversationSnapshot{})
	g := InferConversationalGoal(m, EvaluateCommunicativeIntent(m))
	if g.Primary != GoalDecide || !g.DecisionRelated {
		t.Fatalf("expected decide goal, got %+v", g)
	}
}
