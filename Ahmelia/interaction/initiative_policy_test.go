package interaction

import (
	"testing"
	"time"
)

func TestInitiativePolicyDoesNotInterruptFocusedTurns(t *testing.T) {
	p := NewUserProfile()
	for _, kind := range []RequestKind{KindQuestion, KindAction, KindProduction, KindReasoning, KindCorrection, KindPause} {
		req := SemanticRequest{Kind: kind, Raw: "turno"}
		got := DefaultInitiativePolicy().EvaluateInitiative(req, ConversationSnapshot{}, p)
		if got.ShouldInitiate {
			t.Fatalf("kind %q should not trigger initiative", kind)
		}
	}
}

func TestInitiativePolicyRespectsUserNeed(t *testing.T) {
	p := NewUserProfile()
	ctx := ConversationSnapshot{UserState: UserState{Frustration: .9, Engagement: .9}}
	got := DefaultInitiativePolicy().EvaluateInitiative(
		SemanticRequest{Kind: KindConversation, Raw: "sigamos"},
		ctx, p,
	)
	if got.ShouldInitiate {
		t.Fatal("initiative should yield to high frustration")
	}
}

func TestInitiativePolicyCanInitiateFromUserModelGap(t *testing.T) {
	p := NewUserProfile()
	ctx := ConversationSnapshot{
		ActiveTopic: "nuestro proyecto",
		UserState:   UserState{Engagement: .8},
	}
	got := DefaultInitiativePolicy().EvaluateInitiative(
		SemanticRequest{Kind: KindConversation, Raw: "me gusta trabajar así"},
		ctx, p,
	)
	if !got.ShouldInitiate || got.Candidate.Kind != MoveAsk || got.Candidate.Question == "" {
		t.Fatalf("expected contextual initiative, got %+v", got)
	}
}

func TestInitiativePolicyCooldownAfterRecentAssistantQuestion(t *testing.T) {
	p := NewUserProfile()
	now := time.Now()
	ctx := ConversationSnapshot{
		Turns: []TurnRecord{
			{ID: 1, At: now.Add(-4 * time.Second), UserText: "hola", Assistant: "¿Qué te gustaría hacer?", Kind: KindConversation},
			{ID: 2, At: now.Add(-3 * time.Second), UserText: "seguir", Assistant: "Perfecto.", Kind: KindConversation},
		},
		UserState: UserState{Engagement: .8},
	}
	got := DefaultInitiativePolicy().EvaluateInitiative(
		SemanticRequest{Kind: KindConversation, Raw: "sigamos"},
		ctx, p,
	)
	if got.ShouldInitiate {
		t.Fatal("a recent assistant question should impose a cooldown")
	}
}

func TestInitiativePolicyIsDomainNeutral(t *testing.T) {
	p := NewUserProfile()
	req := SemanticRequest{Kind: KindConversation, Raw: "hablemos del proyecto"}
	got := DefaultInitiativePolicy().EvaluateInitiative(req, ConversationSnapshot{}, p)
	if got.ShouldInitiate && got.Candidate.Question == "" {
		t.Fatal("initiative candidate must carry a usable conversational move")
	}
}
