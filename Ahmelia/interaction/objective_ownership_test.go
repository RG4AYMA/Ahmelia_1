package interaction

import "testing"

func TestReconcileObjectiveOwnershipMovesLegacyPreambleToDraft(t *testing.T) {
	state := NewConversationState()
	state.ActiveObjective = "Quiero que implementes una pequeña utilidad de programación."

	state.ReconcileObjectiveOwnership(false)

	if state.ActiveObjective != "" {
		t.Fatalf("legacy incomplete objective remained active: %q", state.ActiveObjective)
	}
	draft, ok := state.PendingObjectiveDraft()
	if !ok || draft.Text == "" {
		t.Fatal("expected incomplete objective to become pending draft")
	}
}

func TestReconcileObjectiveOwnershipDoesNotRewriteOperationalWork(t *testing.T) {
	state := NewConversationState()
	state.ActiveObjective = "Quiero que implementes una pequeña utilidad de programación."

	state.ReconcileObjectiveOwnership(true)

	if state.ActiveObjective == "" {
		t.Fatal("operational objective was incorrectly rewritten")
	}
	if _, ok := state.PendingObjectiveDraft(); ok {
		t.Fatal("operational objective unexpectedly became conversational draft")
	}
}

func TestReconcileObjectiveOwnershipClearsDuplicatedActiveProjection(t *testing.T) {
	state := NewConversationState()
	state.ActiveObjective = "Quiero que implementes una pequeña utilidad de programación."
	state.BeginObjectiveDraft(state.ActiveObjective, 1)

	state.ReconcileObjectiveOwnership(false)

	if state.ActiveObjective != "" {
		t.Fatalf("duplicated active objective remained: %q", state.ActiveObjective)
	}
	if _, ok := state.PendingObjectiveDraft(); !ok {
		t.Fatal("pending draft disappeared")
	}
}
