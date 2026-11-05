package naturaldialogue

import (
	"testing"

	"ahmelia/interaction"
)

func TestSelectMoveMapsCanonicalResponseIntent(t *testing.T) {
	req := interaction.SemanticRequest{
		Raw:               "¿Cuál es mi color favorito?",
		Normalized:        "cual es mi color favorito",
		CommunicativeActs: interaction.CommunicativeActs{Primary: interaction.ActQuestion, Confidence: 0.98},
	}
	got := SelectMove(req, interaction.ConversationSnapshot{}, interaction.ResponseIntent{Kind: interaction.ResponseIntentAnswer})
	if got != interaction.MoveAnswer {
		t.Fatalf("expected answer move, got %q", got)
	}
}

func TestSelectMoveTopicShiftUsesCanonicalAct(t *testing.T) {
	req := interaction.SemanticRequest{
		Raw: "cambiemos de tema", Normalized: "cambiemos de tema",
		CommunicativeActs: interaction.CommunicativeActs{Primary: interaction.ActTopicShift, Confidence: 0.86},
	}
	got := SelectMove(req, interaction.ConversationSnapshot{}, interaction.ResponseIntent{Kind: interaction.ResponseIntentRedirect})
	if got != interaction.MoveRedirect {
		t.Fatalf("expected redirect move, got %q", got)
	}
}

func TestSelectMoveNeverGeneratesTextOrCapability(t *testing.T) {
	req := interaction.SemanticRequest{
		Raw: "abrir chrome", Normalized: "abrir chrome",
		CommunicativeActs: interaction.CommunicativeActs{Primary: interaction.ActCommand, Confidence: 0.96},
	}
	got := SelectMove(req, interaction.ConversationSnapshot{}, interaction.ResponseIntent{Kind: interaction.ResponseIntentNone})
	if got != interaction.MoveWait {
		t.Fatalf("expected downstream execution boundary move, got %q", got)
	}
}
