package naturaldialogue

import (
	"ahmelia/interaction"
	"testing"
)

func TestDirectQuestionBlocksProactiveFollowUp(t *testing.T) {
	req := interaction.SemanticRequest{Raw: "¿Cuál es mi horario habitual para avanzar en el proyecto?", Kind: interaction.KindQuestion, SpeechAct: interaction.SpeechQuestion}
	d := NewIntegration().EvaluateRequest(req, interaction.ConversationSnapshot{ActiveTopic: "proyecto"}, WaitingForUser)
	if !d.RequiresDirectTurn {
		t.Fatal("direct question must require direct turn")
	}
	if d.AllowProactiveFollowUp {
		t.Fatal("direct question cannot authorize proactive follow-up")
	}
}

func TestAnswerToOpenQuestionRemainsOptionalContinuation(t *testing.T) {
	req := interaction.SemanticRequest{Raw: "El negro", Kind: interaction.KindInform}
	snap := interaction.ConversationSnapshot{Turns: []interaction.TurnRecord{{Kind: interaction.KindQuestion, SpeechAct: interaction.SpeechQuestion}}}
	d := NewIntegration().EvaluateRequest(req, snap, WaitingForUser)
	if !d.IsAnswerToPreviousTurn {
		t.Fatal("turn should be recognized as answer to previous question")
	}
	if d.RequiresDirectTurn {
		t.Fatal("ordinary answer should not be treated as a direct capability request")
	}
	if !d.AllowProactiveFollowUp {
		t.Fatal("optional continuation should remain structurally admissible")
	}
}
