package core

import (
	"os"
	"strings"
	"testing"

	"ahmelia/interaction"
)

// B96.32.14 — Interaction E2E. These tests intentionally cross the real
// Engine.Process boundary and then inspect the shared interaction projection.
// The goal is not to retest individual interaction helpers, but to prove that
// references, continuity, correction, switching, interruption/resume and
// persistence remain coherent when they travel through the complete runtime.
func TestB963214E2E_InteractionLifecycleAcrossTurns(t *testing.T) {
	e := newInteractionE2EEngine(t)
	var responses []string
	e.SetResponseCallback(func(msg string) { responses = append(responses, msg) })

	e.Process("Quiero mejorar la escena del menú")
	first := e.interactionState.Snapshot()
	if first.Metadata.TurnID == 0 {
		t.Fatal("expected the first real Engine.Process turn to update interaction state")
	}
	if first.Focus.Topic != "escena" {
		t.Fatalf("expected scene focus after first turn, got %+v", first.Focus)
	}
	if first.Focus.Objective == "" {
		t.Fatal("expected the semantic objective to survive the Engine.Process boundary")
	}
	if len(first.Conversation.Turns) != 1 {
		t.Fatalf("expected one authoritative conversation turn, got %d", len(first.Conversation.Turns))
	}

	e.Process("cambiemos de tema, hablemos de audio")
	switched := e.interactionState.Snapshot()
	if switched.Focus.Topic != "audio" {
		t.Fatalf("expected audio to become the active topic, got %+v", switched.Focus)
	}
	if len(switched.Conversation.OpenThreads) < 2 {
		t.Fatalf("expected the topic switch to preserve both interaction lanes, got %d threads", len(switched.Conversation.OpenThreads))
	}
	if len(switched.Conversation.Turns) != 2 {
		t.Fatalf("expected two authoritative turns after switch, got %d", len(switched.Conversation.Turns))
	}

	e.Process("volvamos a la escena")
	resumed := e.interactionState.Snapshot()
	if resumed.Focus.Topic != "escena" {
		t.Fatalf("expected scene lane to resume, got %+v", resumed.Focus)
	}
	if resumed.Focus.ThreadID == "" {
		t.Fatal("expected resumed interaction to keep a concrete thread anchor")
	}
	if len(resumed.Conversation.OpenThreads) < 2 {
		t.Fatal("resuming a topic must not discard the switched-away lane")
	}

	e.Process("No, me refería a la escena principal")
	corrected := e.interactionState.Snapshot()
	if !corrected.Semantic.Correction.IsCorrection {
		t.Fatalf("expected correction semantics in the live interaction projection, got %+v", corrected.Semantic.Correction)
	}
	if corrected.Semantic.Correction.Replacement == "" {
		t.Fatal("expected correction replacement to survive semantic repair")
	}
	if len(corrected.Conversation.Turns) != 4 {
		t.Fatalf("expected four authoritative turns, got %d", len(corrected.Conversation.Turns))
	}
	last := corrected.Conversation.Turns[len(corrected.Conversation.Turns)-1]
	if last.Relation != "correction" && last.Kind != "correction" {
		t.Fatalf("expected the correction relation to be persisted, got relation=%q kind=%q", last.Relation, last.Kind)
	}

	if len(responses) == 0 {
		t.Fatal("expected Engine.Process to traverse the response boundary during the lifecycle")
	}
}

func TestB963214E2E_InterruptionResumeAndPersistenceKeepTheSameInteractionAnchor(t *testing.T) {
	e := newInteractionE2EEngine(t)
	e.Process("Quiero trabajar en la escena principal")
	before := e.interactionState.Snapshot()
	if before.Focus.ThreadID == "" || before.Focus.Topic == "" {
		t.Fatalf("expected an active interaction anchor before interruption, got %+v", before.Focus)
	}

	point := e.interactionState.Interrupt("consulta puntual")
	if point.ID == "" {
		t.Fatal("expected interruption to create a resumable interaction anchor")
	}
	if got := e.interactionState.Snapshot().Metadata.Status; got != interaction.InteractionInterrupted {
		t.Fatalf("expected interrupted status, got %q", got)
	}

	e.Process("¿qué hacemos ahora?")
	during := e.interactionState.Snapshot()
	if during.Metadata.Status != interaction.InteractionActive {
		t.Fatalf("a new real turn should reactivate the interaction projection, got %q", during.Metadata.Status)
	}
	if len(during.Conversation.Turns) != 2 {
		t.Fatalf("expected the independent turn to remain in history, got %d turns", len(during.Conversation.Turns))
	}

	resumedPoint, ok := e.interactionState.Resume()
	if !ok || resumedPoint.ID != point.ID {
		t.Fatalf("expected the original interruption anchor to resume, got ok=%v point=%q want=%q", ok, resumedPoint.ID, point.ID)
	}
	afterResume := e.interactionState.Snapshot()
	if afterResume.Focus.ThreadID != before.Focus.ThreadID || afterResume.Focus.Topic != before.Focus.Topic {
		t.Fatalf("resume changed the active interaction anchor: before=%+v after=%+v", before.Focus, afterResume.Focus)
	}
	if len(afterResume.Conversation.Turns) != 2 {
		t.Fatal("resume must preserve the independent turn recorded during the interruption")
	}

	path := "memory/b96_32_14_e2e_interaction_state.json"
	defer os.Remove(path)
	if err := e.interactionState.Save(path); err != nil {
		t.Fatalf("save interaction state: %v", err)
	}
	loaded := interaction.LoadInteractionState(path)
	loadedSnap := loaded.Snapshot()
	if loadedSnap.Focus.ThreadID != afterResume.Focus.ThreadID || loadedSnap.Focus.Topic != afterResume.Focus.Topic {
		t.Fatalf("persistence lost interaction focus: saved=%+v loaded=%+v", afterResume.Focus, loadedSnap.Focus)
	}
	if len(loadedSnap.Conversation.Turns) != len(afterResume.Conversation.Turns) {
		t.Fatalf("persistence changed conversation history: saved=%d loaded=%d", len(afterResume.Conversation.Turns), len(loadedSnap.Conversation.Turns))
	}
	if strings.TrimSpace(loadedSnap.Semantic.Raw) == "" {
		t.Fatal("persistence lost the latest semantic interaction request")
	}
}
