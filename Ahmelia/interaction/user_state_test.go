package interaction

import "testing"

func TestAnalyzeUserStateDetectsCommunicationSignals(t *testing.T) {
	r := SemanticRequest{Raw: "Llevo una hora y sigue sin funcionar 😤", Kind: KindConversation, Confidence: 0.8}
	a := AnalyzeUserState(r, ConversationSnapshot{})
	if a.Current.Frustration < 0.7 {
		t.Fatalf("expected frustration signal, got %v", a.Current.Frustration)
	}
	if a.Current.InteractionNeed != NeedAssistance {
		t.Fatalf("expected assistance need, got %q", a.Current.InteractionNeed)
	}
	if len(a.Evidence) == 0 {
		t.Fatal("expected evidence")
	}
}

func TestAnalyzeUserStateTracksDelta(t *testing.T) {
	ctx := ConversationSnapshot{UserState: UserState{Frustration: 0.1, Engagement: 0.5}}
	r := SemanticRequest{Raw: "no funciona otra vez", Kind: KindConversation, Confidence: 0.7}
	a := AnalyzeUserState(r, ctx)
	if a.Delta.Frustration <= 0 {
		t.Fatalf("expected frustration increase, got %v", a.Delta.Frustration)
	}
	if a.Delta.Engagement <= 0 {
		t.Fatalf("expected engagement increase, got %v", a.Delta.Engagement)
	}
}

func TestConversationStatePersistsUserStateAcrossTurns(t *testing.T) {
	s := NewConversationState()
	r := SemanticRequest{Raw: "esto no funciona", Kind: KindConversation, Confidence: 0.7}
	s.RecordTurn(r, "Entiendo.")
	if s.Snapshot().UserState.Frustration < 0.6 {
		t.Fatal("expected state to be stored in conversation state")
	}
}
