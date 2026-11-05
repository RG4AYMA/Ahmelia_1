package naturaldialogue

import (
	"ahmelia/interaction"
	"testing"
)

func TestIsAnswerToPreviousSkipsPersistedCurrentTurn(t *testing.T) {
	snap := interaction.ConversationSnapshot{Turns: []interaction.TurnRecord{
		{ID: 1, UserText: "¿Cuál prefieres?", Kind: interaction.KindQuestion, SpeechAct: interaction.SpeechQuestion},
		{ID: 2, UserText: "la segunda", Kind: interaction.KindConversation, SpeechAct: interaction.SpeechInform},
	}}
	req := interaction.SemanticRequest{Raw: "la segunda", Kind: interaction.KindConversation, SpeechAct: interaction.SpeechInform}
	if !IsAnswerToPrevious(req, snap) {
		t.Fatal("expected current turn to resolve against preceding question")
	}
}
