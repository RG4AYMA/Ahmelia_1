package cognition

import (
	"ahmelia/interaction"
	"testing"
)

func TestB96334_ActiveGoalIsHighlyRelevant(t *testing.T) {
	ctx := interaction.ConversationSnapshot{ActiveTopic: "enemigos", ActiveObjective: "mejorar la IA de enemigos", ActiveWorkID: "work-1"}
	r := interaction.SemanticRequest{Raw: "quiero mejorar la IA de enemigos", Kind: interaction.KindConversation, SpeechAct: interaction.SpeechInform, Topic: "enemigos", Objective: interaction.Objective{Text: "mejorar la IA de enemigos"}}
	m := NewConversationalCognitiveModel(r, ctx)
	in := EvaluateCommunicativeIntent(m)
	g := InferConversationalGoal(m, in)
	rel := EvaluateConversationalRelevance(m, in, g)
	if !rel.ContinuesGoal || rel.Score < .85 {
		t.Fatalf("expected strong goal relevance, got %+v", rel)
	}
}

func TestB96334_NewTopicIsNotForcedIntoCurrentWork(t *testing.T) {
	ctx := interaction.ConversationSnapshot{ActiveTopic: "enemigos", ActiveObjective: "mejorar la IA de enemigos", ActiveWorkID: "work-1"}
	r := interaction.SemanticRequest{Raw: "hablemos del sistema de sonido", Kind: interaction.KindConversation, SpeechAct: interaction.SpeechInform, Topic: "sonido"}
	m := NewConversationalCognitiveModel(r, ctx)
	in := EvaluateCommunicativeIntent(m)
	g := InferConversationalGoal(m, in)
	rel := EvaluateConversationalRelevance(m, in, g)
	if !rel.IsNewTopic || rel.Primary != RelevanceNewTopic {
		t.Fatalf("expected new topic, got %+v", rel)
	}
}

func TestB96334_ResolvedReferenceLinksCurrentWork(t *testing.T) {
	ctx := interaction.ConversationSnapshot{ActiveWorkID: "work-1", ActiveTopic: "enemigos"}
	r := interaction.SemanticRequest{Raw: "continúa con ese archivo", Kind: interaction.KindContinuation, SpeechAct: interaction.SpeechContinue, FollowUp: true, Continues: true, References: []interaction.Reference{{Kind: interaction.ReferenceRecentWork, Expression: "ese archivo", Resolved: true, Value: "work-1"}}}
	m := NewConversationalCognitiveModel(r, ctx)
	in := EvaluateCommunicativeIntent(m)
	g := InferConversationalGoal(m, in)
	rel := EvaluateConversationalRelevance(m, in, g)
	if !rel.RelatedToWork {
		t.Fatalf("expected work relevance, got %+v", rel)
	}
}

func TestB96334_SocialTurnCanBeIndependent(t *testing.T) {
	r := interaction.SemanticRequest{Raw: "hola, ¿cómo estás?", Kind: interaction.KindConversation, SpeechAct: interaction.SpeechSocial}
	ctx := interaction.ConversationSnapshot{ActiveObjective: "mejorar la IA", ActiveWorkID: "work-1"}
	m := NewConversationalCognitiveModel(r, ctx)
	in := EvaluateCommunicativeIntent(m)
	g := InferConversationalGoal(m, in)
	rel := EvaluateConversationalRelevance(m, in, g)
	if !rel.IsIndependent {
		t.Fatalf("expected independent social relevance, got %+v", rel)
	}
}

func TestB96334_RelevanceDoesNotAuthorizeAction(t *testing.T) {
	ctx := interaction.ConversationSnapshot{ActiveObjective: "mejorar la IA de enemigos", ActiveWorkID: "work-1"}
	r := interaction.SemanticRequest{Raw: "cambiemos los enemigos", Kind: interaction.KindProposal, SpeechAct: interaction.SpeechPropose, Objective: interaction.Objective{Text: "cambiar los enemigos"}}
	m := NewConversationalCognitiveModel(r, ctx)
	in := EvaluateCommunicativeIntent(m)
	g := InferConversationalGoal(m, in)
	rel := EvaluateConversationalRelevance(m, in, g)
	if rel.RelatedToWork && in.ActionOriented && r.SpeechAct != interaction.SpeechCommand {
		t.Fatalf("relevance must not turn proposal into command: %+v", rel)
	}
}
