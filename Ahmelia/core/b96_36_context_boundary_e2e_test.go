package core

import (
	"strings"
	"testing"
)

// B96.36 behavioral gate: an explicit conversational break must deactivate the
// old lane without deleting it, and an unrelated deterministic question must
// not inherit the old topic.
func TestB9636E2E_ExplicitBoundaryPreventsContextContamination(t *testing.T) {
	e := newInteractionE2EEngine(t)
	var responses []string
	e.SetResponseCallback(func(msg string) { responses = append(responses, msg) })

	e.Process("¿Qué habíamos decidido sobre Aurora?")
	before := e.interactionState.Snapshot()
	if before.Focus.Topic != "aurora" {
		t.Fatalf("expected Aurora focus before boundary, got %+v", before.Focus)
	}
	threadsBefore := len(before.Conversation.OpenThreads)

	e.Process("Eso ya no importa, hablemos de otra cosa")
	afterBoundary := e.interactionState.Snapshot()
	if afterBoundary.Focus.Topic != "" || afterBoundary.Focus.ThreadID != "" {
		t.Fatalf("old focus survived explicit boundary: %+v", afterBoundary.Focus)
	}
	if len(afterBoundary.Conversation.OpenThreads) != threadsBefore {
		t.Fatalf("boundary should preserve the old lane, threads before=%d after=%d", threadsBefore, len(afterBoundary.Conversation.OpenThreads))
	}

	e.Process("4 + 4")
	afterIndependent := e.interactionState.Snapshot()
	if afterIndependent.Focus.Topic != "" {
		t.Fatalf("unrelated arithmetic inherited stale topic: %+v", afterIndependent.Focus)
	}
	if len(responses) == 0 || !strings.Contains(responses[len(responses)-1], "8") {
		t.Fatalf("expected deterministic arithmetic response 8, responses=%v", responses)
	}
}
