package core

import (
	"strings"
	"testing"
)

// B96.40 behavioral gate: a real Engine turn must converge into one
// transversal Situation and then expose multiple specialist perspectives over
// that same situation. No source store is replaced or mutated by the stage.
func TestB9640E2E_EngineBuildsSharedSituation(t *testing.T) {
	e := newB963334AuditEngine(t)
	var responses []string
	e.SetResponseCallback(func(msg string) { responses = append(responses, msg) })

	e.Process("hola ahmelia como estas?")

	if e.lastSituation.ID == "" {
		t.Fatal("Engine did not build a Situation")
	}
	if e.lastSituation.Anchor.Description == "" {
		t.Fatal("Situation lost the real turn anchor")
	}
	if len(e.lastSituation.Evidence) == 0 || len(e.lastSituation.Relations) == 0 {
		t.Fatalf("Situation is not relational: evidence=%d relations=%d", len(e.lastSituation.Evidence), len(e.lastSituation.Relations))
	}
	// Fase 2 (2026-10-18): el saludo «¿cómo estás?» ahora enruta a subject=self
	// (ownership: second_person_verb/asks_about_state) y su respuesta entra
	// como una perspectiva adicional del especialista de conocimiento propio.
	// La invariante es múltiples perspectivas independientes sobre UNA misma
	// Situation, no un número fijo: conversation + operational + knowledge:self
	// es la composición esperada hoy.
	if len(e.lastSituationContributions) < 2 {
		t.Fatalf("expected multiple specialist perspectives over the same Situation, got %d", len(e.lastSituationContributions))
	}
	if e.lastSituationContributions[0].ID == e.lastSituationContributions[1].ID {
		t.Fatal("specialists did not produce independent contributions")
	}
	for _, c := range e.lastSituationContributions {
		if !strings.HasPrefix(c.ID, e.lastSituation.ID+":") {
			t.Fatalf("contribution %q is not attached to the shared Situation %q", c.ID, e.lastSituation.ID)
		}
	}
	if e.lastConversationalCognition.Valid == false {
		t.Fatalf("B96.33 stopped before/while consuming the integrated turn: %v", e.lastConversationalCognition.Errors)
	}
	if e.lastConversationalCognition.Situation.ID != e.lastSituation.ID {
		t.Fatalf("cognition did not consume the Engine Situation: cognition=%q engine=%q", e.lastConversationalCognition.Situation.ID, e.lastSituation.ID)
	}
	if len(e.lastConversationalCognition.Contributions) != len(e.lastSituationContributions) {
		t.Fatalf("cognition did not receive all Situation contributions: cognition=%d engine=%d", len(e.lastConversationalCognition.Contributions), len(e.lastSituationContributions))
	}
	if len(responses) == 0 {
		t.Fatal("real Engine E2E produced no response")
	}
}

func TestB9640SituationDoesNotBecomeConversationStateAuthority(t *testing.T) {
	e := newB963334AuditEngine(t)
	before := e.conversationState.Snapshot()
	e.Process("hola ahmelia")
	after := e.conversationState.Snapshot()
	if e.lastSituation.ID == "" {
		t.Fatal("Situation missing")
	}
	if before.ActiveThreadID == "" && after.ActiveThreadID == "" {
		// Fine: a greeting may not create a thread. The invariant is that the
		// Situation itself has no ConversationState mutation API.
		return
	}
	if after.SessionID == "" {
		t.Fatal("ConversationState was corrupted while building Situation")
	}
}
