package cognition

import (
	"testing"

	"ahmelia/interaction"
)

func TestB96335_ProposalIsInterpretedWithoutExecution(t *testing.T) {
	req := interaction.SemanticRequest{
		Raw:       "Estoy pensando en cambiar el sistema de enemigos",
		Kind:      interaction.KindProposal,
		SpeechAct: interaction.SpeechPropose,
		Topic:     "sistema de enemigos",
		Objective: interaction.Objective{Text: "cambiar el sistema de enemigos"},
		Meaning:   interaction.SemanticMeaning{Primary: interaction.ConceptProposal, Concepts: []interaction.SemanticConcept{interaction.ConceptProposal}, Confidence: 0.94},
	}
	model := NewConversationalCognitiveModel(req, interaction.ConversationSnapshot{})
	intent := EvaluateCommunicativeIntent(model)
	goal := InferConversationalGoal(model, intent)
	rel := EvaluateConversationalRelevance(model, intent, goal)
	got := InterpretConversation(model, intent, goal, rel)

	if got.Intent != IntentShareIdea {
		t.Fatalf("intent = %q", got.Intent)
	}
	if got.ActionPosture == ActionPostureRequest {
		t.Fatalf("proposal became action request: %+v", got)
	}
	if got.Objective == "" {
		t.Fatal("expected objective")
	}
}

func TestB96335_CommandRemainsDownstreamActionRequest(t *testing.T) {
	req := interaction.SemanticRequest{
		Raw:             "Cambia el sistema de enemigos",
		Kind:            interaction.KindAction,
		SpeechAct:       interaction.SpeechCommand,
		Topic:           "sistema de enemigos",
		RequestedAction: "cambiar el sistema de enemigos",
		Objective:       interaction.Objective{Text: "cambiar el sistema de enemigos"},
		Meaning:         interaction.SemanticMeaning{Primary: interaction.ConceptCommand, Concepts: []interaction.SemanticConcept{interaction.ConceptCommand, interaction.ConceptRequest}, Confidence: 0.95},
	}
	model := NewConversationalCognitiveModel(req, interaction.ConversationSnapshot{})
	intent := EvaluateCommunicativeIntent(model)
	goal := InferConversationalGoal(model, intent)
	rel := EvaluateConversationalRelevance(model, intent, goal)
	got := InterpretConversation(model, intent, goal, rel)

	if got.Intent != IntentRequestAction {
		t.Fatalf("intent = %q", got.Intent)
	}
	if got.ActionPosture != ActionPostureRequest {
		t.Fatalf("posture = %q", got.ActionPosture)
	}
}

func TestB96335_UnresolvedReferenceRequiresClarification(t *testing.T) {
	req := interaction.SemanticRequest{Raw: "hazlo con ese", Kind: interaction.KindAction, RequestedAction: "hacerlo", References: []interaction.Reference{{Kind: interaction.ReferenceCurrentInput, Expression: "ese", Resolved: false}}}
	model := NewConversationalCognitiveModel(req, interaction.ConversationSnapshot{})
	intent := EvaluateCommunicativeIntent(model)
	goal := InferConversationalGoal(model, intent)
	rel := EvaluateConversationalRelevance(model, intent, goal)
	got := InterpretConversation(model, intent, goal, rel)

	if !got.HasUnresolvedRefs || !got.NeedsClarification {
		t.Fatalf("expected clarification: %+v", got)
	}
	if got.ActionPosture != ActionPostureClarify {
		t.Fatalf("posture = %q", got.ActionPosture)
	}
}

func TestB96335_NewTopicOverridesGenericWorkAttachment(t *testing.T) {
	req := interaction.SemanticRequest{Raw: "hablemos del equipo de sonido", Kind: interaction.KindConversation, SpeechAct: interaction.SpeechInform, Topic: "equipo de sonido"}
	ctx := interaction.ConversationSnapshot{ActiveTopic: "sistema de enemigos", ActiveObjective: "mejorar enemigos", ActiveWorkID: "work-1"}
	model := NewConversationalCognitiveModel(req, ctx)
	intent := EvaluateCommunicativeIntent(model)
	goal := InferConversationalGoal(model, intent)
	rel := EvaluateConversationalRelevance(model, intent, goal)
	got := InterpretConversation(model, intent, goal, rel)

	if got.Relevance != RelevanceNewTopic {
		t.Fatalf("relevance = %q", got.Relevance)
	}
	if got.Topic != "equipo de sonido" {
		t.Fatalf("topic = %q", got.Topic)
	}
}
