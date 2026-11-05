package cognition

import (
	"ahmelia/interaction"
	"ahmelia/situation"
	"testing"
)

func TestSituationalResolutionConsumesDecisionEvidence(t *testing.T) {
	req := interaction.SemanticRequest{Raw: "te doy feedback", Normalized: "te doy feedback", Confidence: .2, CommunicativeActs: interaction.CommunicativeActs{Primary: interaction.ActUnknown}}
	intent := EvaluateCommunicativeIntent(NewConversationalCognitiveModel(req, interaction.ConversationSnapshot{}))
	evidence := situation.DecisionEvidence{SituationID: "sit-test", Findings: []situation.DecisionFinding{{Kind: "gives_feedback_to", Perspective: "conversation", Confidence: .9}}}
	resolved, resolution := resolveFromSituation(intent, evidence)
	if !resolution.Used || resolved.Primary != IntentFeedback {
		t.Fatalf("resolved=%+v resolution=%+v", resolved, resolution)
	}
}

func TestB96404FutureRelationCanGroundSemanticIntent(t *testing.T) {
	req := interaction.SemanticRequest{
		Raw:               "haré todo lo posible para que puedas empezar a ver por ti misma",
		Normalized:        "haré todo lo posible para que puedas empezar a ver por ti misma",
		Confidence:        .20,
		CommunicativeActs: interaction.CommunicativeActs{Primary: interaction.ActUnknown, Confidence: .20},
		Relations:         []interaction.SemanticRelation{{Source: "user", Predicate: "anticipates", Target: "ahmelia:future_state", Confidence: .88}},
	}
	got := EvaluateCommunicativeIntent(NewConversationalCognitiveModel(req, interaction.ConversationSnapshot{}))
	if got.Primary != IntentShareIdea {
		t.Fatalf("future relation should ground semantic intent before situational override, got=%q", got.Primary)
	}
}
