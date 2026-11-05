package cognition

import (
	"testing"
	"time"

	"ahmelia/interaction"
	"ahmelia/situation"
)

func TestB96402_UnknownIntentConsultsSituationContribution(t *testing.T) {
	req := interaction.SemanticRequest{Raw: "future state", Normalized: "future state", SpeechAct: interaction.SpeechUnknown, Confidence: 0.20}
	ctx := interaction.ConversationSnapshot{}
	c := situation.Contribution{
		ID: "sit:test:conversation", Perspective: "conversation", Confidence: 0.88, CreatedAt: time.Now(),
		Relations: []situation.Relation{{Source: "user", Target: "future_state", Type: "proposes", Confidence: 0.88}},
	}
	out := RunConversationalCognitionE2E(req, ctx, SituationalCognitionContext{Situation: situation.Situation{ID: "sit:test"}, Contributions: []situation.Contribution{c}})
	if !out.SituationalResolution.Used {
		t.Fatal("expected Situation contribution to be consulted")
	}
	if out.Intent.Primary != IntentShareIdea {
		t.Fatalf("expected share_idea, got %s", out.Intent.Primary)
	}
	if out.Intent.NeedsClarification {
		t.Fatal("resolved structured contribution should clear clarification")
	}
}

func TestB96402_HighConfidenceDoesNotConsultSituation(t *testing.T) {
	req := interaction.SemanticRequest{Raw: "hello", Normalized: "hello", SpeechAct: interaction.SpeechSocial, Confidence: 0.98}
	c := situation.Contribution{ID: "sit:test:conversation", Perspective: "conversation", Confidence: 0.99, Relations: []situation.Relation{{Type: "proposes", Confidence: 0.99}}}
	out := RunConversationalCognitionE2E(req, interaction.ConversationSnapshot{}, SituationalCognitionContext{Situation: situation.Situation{ID: "sit:test"}, Contributions: []situation.Contribution{c}})
	if out.SituationalResolution.Used {
		t.Fatal("high-confidence structured cognition must not be overridden by Situation")
	}
}
