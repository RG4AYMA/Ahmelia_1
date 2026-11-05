package core

import "testing"

func TestB9640AutonomousInitiativeDoesNotRepeatPreviousAssistantResponse(t *testing.T) {
	if !isDuplicateInitiativeResponse("  misma respuesta  ", "misma respuesta") {
		t.Fatal("expected exact repeated response to be suppressed")
	}
	if isDuplicateInitiativeResponse("respuesta nueva", "respuesta anterior") {
		t.Fatal("different autonomous response was incorrectly suppressed")
	}
	if isDuplicateInitiativeResponse("", "respuesta anterior") {
		t.Fatal("empty candidate must not be treated as a duplicate initiative")
	}
}
