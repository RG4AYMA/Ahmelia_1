package cognition

import (
	"ahmelia/interaction"
	"testing"
)

func TestB96336_ProposalDoesNotBecomeAction(t *testing.T) {
	req := interaction.SemanticRequest{Raw: "estoy pensando en cambiar enemigos", Kind: interaction.KindProposal, SpeechAct: interaction.SpeechPropose, Topic: "enemigos", Objective: interaction.Objective{Text: "cambiar enemigos"}}
	model := NewConversationalCognitiveModel(req, interaction.ConversationSnapshot{ActiveTopic: "enemigos", ActiveObjective: "mejorar enemigos", ActiveWorkID: "w1"})
	i := EvaluateCommunicativeIntent(model)
	g := InferConversationalGoal(model, i)
	r := EvaluateConversationalRelevance(model, i, g)
	in := InterpretConversation(model, i, g, r)
	d := DecideConversationalParticipation(model, i, g, r, in)
	if d.Primary != DecisionDiscuss {
		t.Fatalf("expected discuss, got %+v", d)
	}
	if d.ActionCandidate || d.ExecutionRequested {
		t.Fatalf("proposal became action: %+v", d)
	}
}

func TestB96336_ActionRequestIsNotAuthorization(t *testing.T) {
	req := interaction.SemanticRequest{Raw: "cambia enemigos", Kind: interaction.KindAction, SpeechAct: interaction.SpeechCommand, RequestedAction: "cambiar enemigos", Objective: interaction.Objective{Text: "cambiar enemigos"}}
	model := NewConversationalCognitiveModel(req, interaction.ConversationSnapshot{})
	i := EvaluateCommunicativeIntent(model)
	g := InferConversationalGoal(model, i)
	r := EvaluateConversationalRelevance(model, i, g)
	in := InterpretConversation(model, i, g, r)
	d := DecideConversationalParticipation(model, i, g, r, in)
	if d.Primary != DecisionAct {
		t.Fatalf("expected act candidate, got %+v", d)
	}
	if !d.ExecutionRequested {
		t.Fatal("expected request to be marked as execution-requested")
	}
	// This package has no authorization/execution side effect; the decision is only a downstream candidate.
}

func TestB96336_UnresolvedReferenceForcesClarification(t *testing.T) {
	req := interaction.SemanticRequest{Raw: "hazlo con ese", Kind: interaction.KindAction, SpeechAct: interaction.SpeechCommand, RequestedAction: "hacerlo", References: []interaction.Reference{{Kind: interaction.ReferenceCurrentInput, Expression: "ese", Resolved: false}}}
	model := NewConversationalCognitiveModel(req, interaction.ConversationSnapshot{})
	i := EvaluateCommunicativeIntent(model)
	g := InferConversationalGoal(model, i)
	r := EvaluateConversationalRelevance(model, i, g)
	in := InterpretConversation(model, i, g, r)
	d := DecideConversationalParticipation(model, i, g, r, in)
	if d.Primary != DecisionAskClarify || !d.NeedsClarification {
		t.Fatalf("expected clarification, got %+v", d)
	}
}

func TestB96336_ExplicitNewTopicWins(t *testing.T) {
	req := interaction.SemanticRequest{Raw: "hablemos de audio", Kind: interaction.KindConversation, SpeechAct: interaction.SpeechInform, Topic: "audio"}
	ctx := interaction.ConversationSnapshot{ActiveTopic: "enemigos", ActiveObjective: "mejorar IA", ActiveWorkID: "w1"}
	model := NewConversationalCognitiveModel(req, ctx)
	i := EvaluateCommunicativeIntent(model)
	g := InferConversationalGoal(model, i)
	r := EvaluateConversationalRelevance(model, i, g)
	in := InterpretConversation(model, i, g, r)
	d := DecideConversationalParticipation(model, i, g, r, in)
	if r.Primary != RelevanceNewTopic || d.Primary != DecisionChangeTopic {
		t.Fatalf("expected topic change, relevance=%+v decision=%+v", r, d)
	}
}
