package core

import (
	"os"
	"strings"
	"testing"

	"ahmelia/interaction"
	"ahmelia/memory"
	"ahmelia/selfknowledge"
)

func newFlowAuditEngine(t *testing.T) *Engine {
	isolateTestWorkspace(t)
	t.Helper()
	old := os.Getenv("AHMELIA_DISABLE_TTS")
	_ = os.Setenv("AHMELIA_DISABLE_TTS", "1")
	t.Cleanup(func() { _ = os.Setenv("AHMELIA_DISABLE_TTS", old) })
	sess := memory.NewSession()
	long := memory.LoadLongMemory()
	short := LoadShortMemory()
	history := selfknowledge.NewActionHistory(20)
	selfQuery := selfknowledge.NewSelfQuery(history, selfknowledge.NewModuleRegistry(), "v3.1")
	e := NewEngine(sess, long, &short, history, selfQuery)
	e.conversationState = interaction.NewConversationState()
	e.interactionManager = interaction.NewManager()
	installF9LanguageStubs(t, e)
	e.SetResponseCallback(func(string) {})
	return e
}

func TestDataStateFlow_SelfQuestionReachesGroundedResponseAndClosesTurn(t *testing.T) {
	e := newFlowAuditEngine(t)
	e.Process("hola ahmelia como estas?")

	snap := e.conversationState.Snapshot()
	if len(snap.Turns) != 1 {
		t.Fatalf("expected one persisted human turn, got %d", len(snap.Turns))
	}
	reply := strings.TrimSpace(snap.Turns[0].Assistant)
	if reply == "" {
		t.Fatal("final turn lost its assistant response")
	}
	if strings.Contains(strings.ToLower(reply), "fuente de conocimiento suficiente") {
		t.Fatalf("self-state question leaked into generic knowledge fallback: %q", reply)
	}
	if e.internalState.Current() != "Esperando respuesta" {
		t.Fatalf("internal state did not close the turn: %q", e.internalState.Current())
	}
	if e.conversationManager.TurnState() != "AHMELIA_IDLE" {
		t.Fatalf("conversation turn ownership did not close: %q", e.conversationManager.TurnState())
	}
	if e.autonomousState.Execution() != "Idle" {
		t.Fatalf("autonomous execution state did not close: %q", e.autonomousState.Execution())
	}
}

func TestDataStateFlow_ProductionSemanticContractIsNotLostAcrossRuntimeBoundary(t *testing.T) {
	cases := []struct {
		input  string
		action string
	}{
		{"volumen al 40", "volumen_set"},
		{"mueve archivo a documentos", "mover_archivo"},
	}
	for _, tc := range cases {
		req := interaction.Analyze(tc.input, interaction.Context{})
		if req.Action.Name != tc.action {
			t.Fatalf("%q: semantic action=%q want %q", tc.input, req.Action.Name, tc.action)
		}
		if !req.CommunicativeActs.Has(interaction.ActCommand) {
			t.Fatalf("%q: command act disappeared before runtime boundary: %+v", tc.input, req.CommunicativeActs)
		}
	}
}

func TestDataStateFlow_SelfQuestionDoesNotReuseUserPreference(t *testing.T) {
	e := newFlowAuditEngine(t)
	e.Process("¿Tú tienes color favorito?")
	snap := e.conversationState.Snapshot()
	if len(snap.Turns) != 1 {
		t.Fatalf("expected one turn, got %d", len(snap.Turns))
	}
	reply := strings.ToLower(strings.TrimSpace(snap.Turns[0].Assistant))
	if strings.Contains(reply, "tu color favorito es negro") || strings.Contains(reply, "el negro es el color favorito que mencionaste") {
		t.Fatalf("self question incorrectly answered with user-owned preference: %q", snap.Turns[0].Assistant)
	}
}

func TestDataStateFlow_SelfKnowledgeQuestionAboutSelf(t *testing.T) {
	e := newFlowAuditEngine(t)
	e.Process("que sabes sobre ti misma")
	snap := e.conversationState.Snapshot()
	if len(snap.Turns) != 1 {
		t.Fatalf("expected one turn, got %d", len(snap.Turns))
	}
	reply := strings.TrimSpace(snap.Turns[0].Assistant)
	if reply == "" {
		t.Fatal("self-knowledge question got empty response")
	}
	// Should answer about self, not ask back or fall into generic knowledge
	lower := strings.ToLower(reply)
	if strings.Contains(lower, "qué sabes") || strings.Contains(lower, "que sabes") {
		t.Fatalf("self-knowledge question bounced back as question: %q", reply)
	}
	if strings.Contains(lower, "fuente de conocimiento suficiente") {
		t.Fatalf("self-knowledge question leaked into generic knowledge fallback: %q", reply)
	}
	// Should mention capabilities or identity
	if !strings.Contains(lower, "puedo") && !strings.Contains(lower, "asistente") && !strings.Contains(lower, "ahm") && !strings.Contains(lower, "capacidad") {
		t.Logf("self-knowledge reply (may be incomplete but not wrong): %q", reply)
	}
}
