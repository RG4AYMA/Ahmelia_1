package interaction

import "testing"

func TestDetectDecisionDoesNotMutateConversationState(t *testing.T) {
	state := NewConversationState()
	req := Analyze("vamos a usar C#", Context{})
	c := DetectDecision(req, 7)
	if !c.IsDecision {
		t.Fatal("expected decision candidate")
	}
	if len(state.Snapshot().Decisions) != 0 {
		t.Fatal("detection must not mutate durable decision history")
	}
}

func TestApplyDecisionPersistsAcceptedDecision(t *testing.T) {
	state := NewConversationState()
	c := DecisionCandidate{
		IsDecision: true, Topic: "implementación",
		Decision: "usar C#", Confidence: 0.95, SourceTurn: 4,
	}
	if !ApplyDecision(state, c) {
		t.Fatal("expected decision to be applied")
	}
	got := state.Snapshot().Decisions
	if len(got) != 1 || got[0].Decision != "usar C#" || got[0].SourceTurn != 4 {
		t.Fatalf("unexpected durable decision: %#v", got)
	}
}

func TestActiveDecisionProjectsDurableDecision(t *testing.T) {
	conversation := NewConversationState()
	conversation.AddDecision("implementación", "usar C#", 4)
	state := NewInteractionState(conversation)
	state.Observe(Analyze("seguimos", Context{}), WorkContext{}, false)

	d := state.ActiveDecision()
	if d.Decision != "usar C#" || d.Topic != "implementación" || d.SourceTurn != 4 {
		t.Fatalf("unexpected active decision: %#v", d)
	}
}

func TestActiveDecisionPrefersActiveTopic(t *testing.T) {
	conversation := NewConversationState()
	conversation.AddDecision("Unity", "usar C#", 2)
	conversation.AddDecision("Blender", "usar Python", 3)

	state := NewInteractionState(conversation)
	state.Observe(Analyze("sigamos con Unity", Context{}), WorkContext{}, false)

	d := state.ActiveDecision()
	if d.Topic != "Unity" || d.Decision != "usar C#" {
		t.Fatalf("expected active-topic decision, got %#v", d)
	}
}

func TestDecisionRejectsEmptyCandidate(t *testing.T) {
	state := NewConversationState()
	if ApplyDecision(state, DecisionCandidate{IsDecision: true}) {
		t.Fatal("empty decision must not be persisted")
	}
}

func TestDecisionDetectionSupportsEnglish(t *testing.T) {
	req := Analyze("we decided to use C#", Context{})
	c := DetectDecision(req, 9)
	if !c.IsDecision {
		t.Fatal("expected English decision candidate")
	}
}
