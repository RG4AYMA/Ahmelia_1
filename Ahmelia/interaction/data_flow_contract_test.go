package interaction

import "testing"

func TestReanalyzeReplacesInFlightTurnInsteadOfAppending(t *testing.T) {
	m := NewManager()
	first := m.Analyze("Mi color favorito es negro", Context{})
	if len(m.History()) != 1 {
		t.Fatalf("history after first analysis = %d, want 1", len(m.History()))
	}
	second := m.Reanalyze("Mi color favorito es azul", Context{})
	if len(m.History()) != 1 {
		t.Fatalf("reanalysis created a second turn: history=%d", len(m.History()))
	}
	if m.Last().Raw != second.Raw || m.Last().Raw == first.Raw {
		t.Fatalf("manager did not replace the in-flight semantic turn: first=%q last=%q", first.Raw, m.Last().Raw)
	}
}

func TestConversationStateSnapshotDoesNotShareNestedSemanticSlices(t *testing.T) {
	s := NewConversationState()
	req := Analyze("Sí, hazlo, pero primero cambia el nombre", Context{})
	s.RecordTurn(req, "Entendido.")

	snap := s.Snapshot()
	if len(snap.Turns) != 1 || len(snap.Turns[0].CommunicativeActs.Evidence) == 0 {
		t.Fatal("expected canonical acts and evidence in stored turn")
	}
	snap.Turns[0].CommunicativeActs.Secondary[0] = ActUnknown
	snap.Turns[0].CommunicativeActs.Evidence[0].Source = "mutated"
	if len(snap.Turns[0].References) == 0 {
		// References are optional; the test is specifically about nested act slices.
	}

	original := s.Snapshot().Turns[0]
	if original.CommunicativeActs.Secondary[0] == ActUnknown {
		t.Fatal("snapshot mutation leaked into ConversationState secondary acts")
	}
	if original.CommunicativeActs.Evidence[0].Source == "mutated" {
		t.Fatal("snapshot mutation leaked into ConversationState act evidence")
	}
}
