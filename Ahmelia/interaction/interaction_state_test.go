package interaction

import "testing"

func TestInteractionStateBuildsCrossModalView(t *testing.T) {
	conversation := NewConversationState()
	state := NewInteractionState(conversation)

	req := Analyze("mueve el objeto a la derecha", Context{})
	req.Channel = ChannelGesture
	state.Observe(req, WorkContext{}, false)

	snap := state.Snapshot()
	if snap.Metadata.Source != InteractionSourceGesture {
		t.Fatalf("expected gesture source, got %q", snap.Metadata.Source)
	}
	if snap.Metadata.Status != InteractionActive {
		t.Fatalf("expected active status, got %q", snap.Metadata.Status)
	}
	if snap.Semantic.Kind != req.Kind {
		t.Fatalf("semantic request was not preserved")
	}
}

func TestInteractionStateConversationIsSubsystemNotReplacement(t *testing.T) {
	conversation := NewConversationState()
	turn := Analyze("quiero mejorar el proyecto", Context{})
	conversation.RecordTurn(turn, "")

	state := NewInteractionState(conversation)
	state.Observe(turn, WorkContext{}, false)

	snap := state.Snapshot()
	if len(snap.Conversation.Turns) != 1 {
		t.Fatalf("expected conversation history to remain authoritative, got %d turns", len(snap.Conversation.Turns))
	}
	if snap.SessionID != conversation.Snapshot().SessionID {
		t.Fatal("interaction state session must come from ConversationState")
	}
}

func TestInteractionStateDoesNotOwnWorkLifecycle(t *testing.T) {
	conversation := NewConversationState()
	state := NewInteractionState(conversation)
	work := WorkContext{ID: "work-7", Status: "running", Objective: "hacer una escena"}

	req := SemanticRequest{Raw: "continúa", Kind: KindContinuation, Objective: Objective{Text: "hacer una escena"}, Channel: ChannelText}
	state.Observe(req, work, true)

	snap := state.Snapshot()
	if !snap.HasWork || snap.Work.ID != "work-7" {
		t.Fatalf("expected work snapshot, got %+v", snap.Work)
	}
	if snap.Focus.WorkID != "work-7" {
		t.Fatalf("expected work focus, got %q", snap.Focus.WorkID)
	}
	if conversation.Snapshot().ActiveWorkID != "" {
		t.Fatal("interaction state must not mutate ConversationState work ownership")
	}
}

func TestInteractionStateSupportsNonConversationInput(t *testing.T) {
	state := NewInteractionState(NewConversationState())
	req := SemanticRequest{Channel: ChannelVision, Kind: KindInform, Topic: "objeto detectado"}
	state.Observe(req, WorkContext{}, false)

	snap := state.Snapshot()
	if snap.Metadata.Source != InteractionSourceVision {
		t.Fatalf("expected vision source, got %q", snap.Metadata.Source)
	}
	if snap.Semantic.Kind != KindInform {
		t.Fatalf("expected semantic state for non-conversational input")
	}
}

func TestInteractionStateDoesNotDuplicateCognitionStatus(t *testing.T) {
	state := NewInteractionState(NewConversationState())
	state.SetStatus(InteractionWaiting)
	if got := state.Snapshot().Metadata.Status; got != InteractionWaiting {
		t.Fatalf("unexpected interaction status %q", got)
	}
}
