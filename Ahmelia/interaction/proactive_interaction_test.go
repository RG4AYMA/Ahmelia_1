package interaction

import (
	"testing"
	"time"
)

func TestProactiveInteractionRequiresApprovedInitiative(t *testing.T) {
	got := DefaultProactiveInteractionPolicy().EvaluateProactiveInteraction(
		InitiativeDecision{ShouldInitiate: false, Reason: "no opportunity"},
		ConversationSnapshot{},
	)
	if got.Allowed {
		t.Fatal("proactivity must not bypass initiative policy")
	}
}

func TestProactiveInteractionRejectsInterruptions(t *testing.T) {
	got := DefaultProactiveInteractionPolicy().EvaluateProactiveInteraction(
		InitiativeDecision{
			ShouldInitiate: true,
			Candidate: InitiativeCandidate{
				Kind: MoveAsk, Question: "¿qué te parece?", Priority: .9,
				Interrupt: true, CreatedAt: time.Now(),
			},
		},
		ConversationSnapshot{},
	)
	if got.Allowed {
		t.Fatal("proactive interaction must not interrupt")
	}
}

func TestProactiveInteractionYieldsToUserNeed(t *testing.T) {
	got := DefaultProactiveInteractionPolicy().EvaluateProactiveInteraction(
		InitiativeDecision{
			ShouldInitiate: true,
			Candidate: InitiativeCandidate{
				Kind: MoveAsk, Question: "¿qué te gusta?", Priority: .9,
				CreatedAt: time.Now(),
			},
		},
		ConversationSnapshot{UserState: UserState{Urgency: .8}},
	)
	if got.Allowed {
		t.Fatal("proactivity must yield to urgent user state")
	}
}

func TestProactiveInteractionPreservesContentAndContext(t *testing.T) {
	now := time.Now()
	got := DefaultProactiveInteractionPolicy().EvaluateProactiveInteraction(
		InitiativeDecision{
			ShouldInitiate: true,
			Candidate: InitiativeCandidate{
				Kind: MoveAsk, Question: "  ¿Qué te gustaría explorar?  ",
				Reason: "useful opening", Priority: .8, CreatedAt: now,
			},
		},
		ConversationSnapshot{ActiveTopic: "nuestro proyecto"},
	)
	if !got.Allowed || got.Content != "¿Qué te gustaría explorar?" ||
		got.Topic != "nuestro proyecto" || got.Move != MoveAsk {
		t.Fatalf("unexpected proactive interaction: %+v", got)
	}
}

func TestProactiveInteractionExpiresStaleCandidate(t *testing.T) {
	p := DefaultProactiveInteractionPolicy()
	now := time.Now()
	stale := ProactiveInteraction{
		Allowed: true, Content: "¿seguimos?", CreatedAt: now.Add(-3 * time.Minute),
	}
	if p.ShouldSpeakNow(stale, now) {
		t.Fatal("stale proactive interaction should be regenerated")
	}
}
