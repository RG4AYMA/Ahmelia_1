package interaction

import "testing"

func TestInteractionInterruptionResumeRestoresFocusAndPreservesNewTurns(t *testing.T) {
	conv := NewConversationState()
	state := NewInteractionState(conv)
	reqA := SemanticRequest{Raw: "mejorar enemigo", Normalized: "mejorar enemigo", Kind: KindPlanning, Topic: "enemigo", Objective: Objective{Text: "mejorar enemigo"}, Language: "es", Confidence: 0.95}
	turnA := conv.RecordTurn(reqA, "Vamos con ello.")
	thread := conv.OpenThread("enemigo", "mejorar enemigo")
	conv.ActivateThread(thread)
	state.Observe(reqA, WorkContext{ID: "work-1", Status: "active", Objective: "mejorar enemigo"}, true)
	coordinator := NewInteractionInterruptionCoordinator()
	interrupted := coordinator.Interrupt(state, "consulta puntual")
	if !interrupted.Success || interrupted.ResumeID == "" {
		t.Fatalf("interrupt failed: %+v", interrupted)
	}
	if state.Snapshot().Metadata.Status != InteractionInterrupted {
		t.Fatal("expected interrupted status")
	}
	reqB := SemanticRequest{Raw: "que hora es", Normalized: "que hora es", Kind: KindQuestion, Topic: "hora", Language: "es", Confidence: 1}
	conv.RecordTurn(reqB, "Es una pregunta independiente.")
	state.Observe(reqB, WorkContext{}, false)
	resumed := coordinator.Resume(state)
	if !resumed.Success || resumed.ResumeID != interrupted.ResumeID {
		t.Fatalf("resume failed: %+v", resumed)
	}
	snap := state.Snapshot()
	if snap.Metadata.Status != InteractionActive {
		t.Fatal("expected active status after resume")
	}
	if snap.Focus.ThreadID != thread || snap.Focus.Topic != "enemigo" || snap.Focus.Objective != "mejorar enemigo" {
		t.Fatalf("focus was not restored: %+v", snap.Focus)
	}
	if len(snap.Conversation.Turns) != 2 || snap.Conversation.Turns[0].ID != turnA {
		t.Fatalf("resume discarded interruption history")
	}
	if snap.Work.ID != "work-1" {
		t.Fatalf("resume lost observational work context")
	}
}

func TestInteractionInterruptionResumeSupportsNestedInterruptions(t *testing.T) {
	conv := NewConversationState()
	state := NewInteractionState(conv)
	req := SemanticRequest{Raw: "trabajar", Kind: KindPlanning, Topic: "trabajo", Objective: Objective{Text: "trabajar"}, Confidence: .9}
	conv.RecordTurn(req, "Seguimos.")
	thread := conv.OpenThread("trabajo", "trabajar")
	conv.ActivateThread(thread)
	state.Observe(req, WorkContext{}, false)
	first := state.Interrupt("uno")
	req2 := SemanticRequest{Raw: "otra cosa", Kind: KindConversation, Topic: "otro", Confidence: 1}
	conv.RecordTurn(req2, "Claro.")
	state.Observe(req2, WorkContext{}, false)
	second := state.Interrupt("dos")
	if len(state.PendingResumePoints()) != 2 {
		t.Fatal("expected nested interruption stack")
	}
	gotSecond, ok := state.Resume()
	if !ok || gotSecond.ID != second.ID {
		t.Fatalf("wrong nested resume")
	}
	gotFirst, ok := state.Resume()
	if !ok || gotFirst.ID != first.ID {
		t.Fatalf("wrong outer resume")
	}
	if len(state.PendingResumePoints()) != 0 {
		t.Fatal("resume stack should be empty")
	}
}

func TestInteractionResumeWithoutInterruptionIsNoop(t *testing.T) {
	state := NewInteractionState(NewConversationState())
	if _, ok := state.Resume(); ok {
		t.Fatal("resume without interruption should fail")
	}
}
