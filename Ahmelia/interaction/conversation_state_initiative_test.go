package interaction

import "testing"

func TestRecordAssistantInitiativeDoesNotFabricateUserTurn(t *testing.T) {
	s := NewConversationState()
	s.RecordTurn(SemanticRequest{Raw: "hola", Kind: KindConversation, SpeechAct: SpeechSocial}, "Hola.")
	id := s.RecordAssistantInitiative("¿Seguimos con el proyecto?", "proyecto")
	if id == 0 {
		t.Fatal("expected initiative turn id")
	}
	snap := s.Snapshot()
	if len(snap.Turns) != 2 {
		t.Fatalf("turns=%d, want 2", len(snap.Turns))
	}
	if snap.Turns[1].UserText != "" {
		t.Fatal("initiative turn fabricated user text")
	}
	if snap.Turns[1].Assistant == "" {
		t.Fatal("initiative response not recorded")
	}
}
