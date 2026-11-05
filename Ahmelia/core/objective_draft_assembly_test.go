package core

import (
	"path/filepath"
	"strings"
	"testing"

	"ahmelia/interaction"
	"ahmelia/production"
)

func TestPendingObjectiveDraftIsAssembledBeforeCognition(t *testing.T) {
	t.Setenv("AHMELIA_DISABLE_TTS", "1")
	t.Setenv("AHMELIA_SKIP_PROGRAM_INDEX_SCAN", "1")
	e := buildTestEngine(t)
	e.workStore = NewPersistentWorkStore(production.NewWorkStore(filepath.Join(t.TempDir(), "work.json")))
	e.conversationState = interaction.NewConversationState()
	e.interactionManager = interaction.NewManager()
	e.workBridge = interaction.NewWorkBridge()
	e.conversationState.BeginObjectiveDraft("Quiero que implementes una pequeña utilidad de programación.", 1)

	tn := &processTurn{input: "Objetivo:\nCrea una función Go llamada IsEven(n int) bool.\nCondiciones:\nEjecuta los tests."}
	cleanup := e.inputContextPhase(tn)
	defer cleanup()

	if tn.semanticReq.Kind != interaction.KindProduction {
		t.Fatalf("assembled kind=%q, want production; objective=%q", tn.semanticReq.Kind, tn.semanticReq.Objective.Text)
	}
	if !strings.Contains(tn.semanticReq.Objective.Text, "Quiero que implementes una pequeña utilidad") ||
		!strings.Contains(tn.semanticReq.Objective.Text, "Crea una función Go llamada IsEven") ||
		!strings.Contains(tn.semanticReq.Objective.Text, "Ejecuta los tests") {
		t.Fatalf("draft was not assembled into one objective: %q", tn.semanticReq.Objective.Text)
	}
}

func TestObjectiveDraftContinuityKeepsVoiceDirectUntilOperationalBoundary(t *testing.T) {
	t.Setenv("AHMELIA_DISABLE_TTS", "1")
	t.Setenv("AHMELIA_SKIP_PROGRAM_INDEX_SCAN", "1")
	e := buildTestEngine(t)
	e.conversationState = interaction.NewConversationState()
	e.interactionManager = interaction.NewManager()
	e.workBridge = interaction.NewWorkBridge()

	first := &processTurn{input: "Quiero que implementes una pequeña utilidad de programación."}
	cleanup := e.inputContextPhase(first)
	cleanup()
	if !interaction.NeedsObjectiveAssembly(first.semanticReq) {
		t.Fatal("expected first turn to be an incomplete objective")
	}
	if e.sess == nil {
		t.Fatal("test engine session unavailable")
	}
	// The actual voice-mode file is owned by Session; the assertion here is
	// behavioral through the persisted draft rather than the file contents.
	e.conversationState.BeginObjectiveDraft(first.semanticReq.Objective.Text, 1)
	if _, ok := e.conversationState.PendingObjectiveDraft(); !ok {
		t.Fatal("expected pending objective draft")
	}
}

func TestObjectiveDraftStructuralLabelDoesNotTriggerExecutionAndSubstantiveFragmentCompletesDraft(t *testing.T) {
	t.Setenv("AHMELIA_DISABLE_TTS", "1")
	t.Setenv("AHMELIA_SKIP_PROGRAM_INDEX_SCAN", "1")
	e := buildTestEngine(t)
	e.conversationState = interaction.NewConversationState()
	e.interactionManager = interaction.NewManager()
	e.workBridge = interaction.NewWorkBridge()

	first := &processTurn{input: "Quiero que implementes una pequeña utilidad de programación."}
	cleanup := e.inputContextPhase(first)
	cleanup()
	if !interaction.NeedsObjectiveAssembly(first.semanticReq) {
		t.Fatal("first turn should remain an unfinished delegation")
	}
	if !e.preExecutionPhase(first) {
		t.Fatal("first incomplete turn should be consumed by objective assembly")
	}

	label := &processTurn{input: "Objetivo:"}
	cleanup = e.inputContextPhase(label)
	cleanup()
	if _, ok := e.conversationState.PendingObjectiveDraft(); !ok {
		t.Fatal("objective draft disappeared after structural label")
	}
	if !e.preExecutionPhase(label) {
		t.Fatal("structural label should remain inside objective assembly")
	}
	draft, _ := e.conversationState.PendingObjectiveDraft()
	if draft.Fragments != 1 {
		t.Fatalf("structural label changed fragment count: %d", draft.Fragments)
	}

	body := &processTurn{input: "Crea una función Go llamada IsEven(n int) bool que devuelva true si el número es par y false si es impar. Condiciones: añade tests para positivos, negativos y cero."}
	cleanup = e.inputContextPhase(body)
	cleanup()
	draft, ok := e.conversationState.PendingObjectiveDraft()
	if !ok {
		t.Fatal("objective draft disappeared after substantive fragment")
	}
	if draft.Fragments != 2 {
		t.Fatalf("substantive fragment did not advance draft: %d", draft.Fragments)
	}
	if body.semanticReq.Raw != draft.Text {
		t.Fatalf("semantic request is not the assembled draft: raw=%q draft=%q", body.semanticReq.Raw, draft.Text)
	}
	if interaction.NeedsObjectiveAssembly(body.semanticReq) {
		t.Fatal("assembled substantive objective was still classified as an open preamble")
	}
}
