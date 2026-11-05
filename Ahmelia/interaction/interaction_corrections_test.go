package interaction

import "testing"

func TestActiveInteractionContextProjectsCorrection(t *testing.T) {
	state := NewInteractionState(NewConversationState())
	req := Analyze("no, quería decir la interacción del personaje", Context{})
	state.Observe(req, WorkContext{}, false)

	ctx := state.ActiveContext()
	if !ctx.Correction.IsCorrection {
		t.Fatal("expected active correction")
	}
	if ctx.Correction.Replacement != "la interacción del personaje" {
		t.Fatalf("unexpected replacement: %q", ctx.Correction.Replacement)
	}
	if !ctx.Correction.Applied {
		t.Fatal("expected correction with a replacement to be marked applied")
	}
}

func TestActiveCorrectionDoesNotCreateHistory(t *testing.T) {
	conversation := NewConversationState()
	state := NewInteractionState(conversation)
	state.Observe(Analyze("no, quería decir la escena", Context{}), WorkContext{}, false)

	if len(conversation.Snapshot().Turns) != 0 {
		t.Fatal("interaction correction projection must not create conversation history")
	}
	correction := state.ActiveCorrection()
	correction.Replacement = "mutated locally"
	if state.ActiveCorrection().Replacement == correction.Replacement {
		t.Fatal("active correction must be returned as a detached value")
	}
}

func TestActiveCorrectionKeepsExplicitTarget(t *testing.T) {
	state := NewInteractionState(NewConversationState())
	req := Analyze("no el proyecto viejo, sino el nuevo", Context{})
	state.Observe(req, WorkContext{}, false)

	c := state.ActiveCorrection()
	if c.Target != "el proyecto viejo" || c.Replacement != "el nuevo" {
		t.Fatalf("unexpected correction projection: %#v", c)
	}
}
