package interaction

import "testing"

func TestInteractionContinuityProjectsCurrentInteractionAnchors(t *testing.T) {
	conv := NewConversationState()
	state := NewInteractionState(conv)

	req := SemanticRequest{
		Raw:        "quiero mejorar la escena",
		Normalized: "quiero mejorar la escena",
		Kind:       KindPlanning,
		Topic:      "escena",
		Objective:  Objective{Text: "mejorar la escena"},
		Language:   "es",
		Confidence: 0.95,
	}
	turnID := conv.RecordTurn(req, "Vamos a ello.")
	threadID := conv.OpenThread("escena", "mejorar la escena")
	conv.ActivateThread(threadID)
	conv.AddDecision("escena", "priorizar interacción", turnID)
	conv.AddCommitment("shared", "revisar la escena", turnID)
	state.Observe(req, WorkContext{}, false)

	got := state.Continuity()
	if !got.HasContinuity {
		t.Fatalf("expected continuity: %s", got.ContinuityReason)
	}
	if got.Topic != "escena" || got.Objective != "mejorar la escena" {
		t.Fatalf("unexpected focus: %+v", got)
	}
	if got.ThreadID != threadID || got.LastTurnID != turnID {
		t.Fatalf("continuity lost thread/turn anchor: %+v", got)
	}
	if len(got.RecentTurns) != 1 || len(got.RecentDecisions) != 1 || len(got.OpenCommitments) != 1 {
		t.Fatalf("expected recent semantic anchors, got turns=%d decisions=%d commitments=%d", len(got.RecentTurns), len(got.RecentDecisions), len(got.OpenCommitments))
	}
}

func TestInteractionContinuityKeepsHistoryDetached(t *testing.T) {
	conv := NewConversationState()
	state := NewInteractionState(conv)
	req := SemanticRequest{Raw: "hola", Kind: KindConversation, Topic: "social", Confidence: 1}
	conv.RecordTurn(req, "Hola.")
	state.Observe(req, WorkContext{}, false)

	got := state.Continuity()
	got.RecentTurns[0].UserText = "mutated locally"
	got.OpenCommitments = append(got.OpenCommitments, ConversationCommitment{Text: "local"})

	snap := conv.Snapshot()
	if snap.Turns[0].UserText != "hola" {
		t.Fatal("continuity projection must not expose mutable conversation history")
	}
	if len(snap.Commitments) != 0 {
		t.Fatal("continuity projection must not mutate authoritative commitments")
	}
}

func TestInteractionContinuityDoesNotCreateWorkOrChangeCognition(t *testing.T) {
	conv := NewConversationState()
	state := NewInteractionState(conv)
	req := SemanticRequest{Raw: "sigamos", Kind: KindContinuation, Topic: "escena", Objective: Objective{Text: "seguir"}, Confidence: 0.9}
	state.Observe(req, WorkContext{}, false)
	_ = state.Continuity()

	if conv.ActiveWork() != "" {
		t.Fatal("continuity must not create or activate work")
	}
}

func TestInteractionContinuityWithoutAnchorsIsNotResumable(t *testing.T) {
	state := NewInteractionState(NewConversationState())
	got := state.Continuity()
	if got.HasContinuity {
		t.Fatal("empty interaction should not claim continuity")
	}
	if got.ContinuityReason == "" {
		t.Fatal("expected a reason for missing continuity")
	}
}
