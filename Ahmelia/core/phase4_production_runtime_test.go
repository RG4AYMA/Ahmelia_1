package core

import (
	"os"
	"strings"
	"testing"

	"ahmelia/interaction"
	"ahmelia/production"
)

func TestPhase4ProductionRuntimeHasSingleAuthorityCaller(t *testing.T) {
	turnSrc, err := os.ReadFile("turn_pipeline.go")
	if err != nil {
		t.Fatal(err)
	}
	engineSrc, err := os.ReadFile("engine.go")
	if err != nil {
		t.Fatal(err)
	}
	turnCode := string(turnSrc)
	engineCode := string(engineSrc)
	if strings.Contains(turnCode, "e.productionRuntime.ExecuteFromAuthority(") {
		t.Fatal("turn pipeline still owns the concrete production execution call")
	}
	if !strings.Contains(engineCode, "e.productionRuntime.ExecuteFromAuthority(") {
		t.Fatal("central authority no longer crosses the production adapter")
	}
	if strings.Contains(engineCode, "capabilityResolver.Resolve(raw)") || strings.Contains(engineCode, "capabilityResolver.Resolve(decision.Request.Parameters[\"raw\"])") {
		t.Fatal("authority path still resolves capability from raw text")
	}
	if !strings.Contains(engineCode, "e.capabilityResolver.Resolve(decision, request)") {
		t.Fatal("central authority no longer passes the accepted typed capability decision")
	}
	for _, required := range []string{
		"capabilityDecision := interaction.ResolveCapabilityInteraction(t.semanticReq)",
		"_ = e.authorityExecute(capabilityDecision, t.semanticReq)",
	} {
		if !strings.Contains(turnCode, required) {
			t.Fatalf("canonical capability -> authority boundary missing: %s", required)
		}
	}
}

func TestPhase4ProductionE2EExecutesThenConvergesToResponse(t *testing.T) {
	workDir := t.TempDir()
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(workDir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWD) })
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("AHMELIA_DISABLE_TTS", "1")
	t.Setenv("AHMELIA_SKIP_PROGRAM_INDEX_SCAN", "1")
	e := newInteractionE2EEngine(t)
	var responses []string
	e.SetResponseCallback(func(msg string) { responses = append(responses, msg) })

	project := "phase4_e2e_ahmelia"
	e.Process("Crea un pequeño programa Go llamado " + project + " que muestre Hola y añade un test.")

	if len(responses) == 0 || strings.TrimSpace(responses[len(responses)-1]) == "" {
		t.Fatal("production E2E did not produce a conversational result")
	}
	last, ok := e.productionRuntime.LastWork()
	if !ok {
		t.Fatal("production work was not persisted")
	}
	if last.Status != production.WorkCompleted {
		t.Fatalf("production status=%q, want completed; events=%v", last.Status, last.Events)
	}
	if e.lastResponseIntent.Kind != interaction.ResponseIntentAnswer {
		t.Fatalf("response intent=%q, want answer after production result", e.lastResponseIntent.Kind)
	}
	if e.lastNaturalDialogue.Move != interaction.MoveAnswer {
		t.Fatalf("dialogue move=%q, want answer after production result", e.lastNaturalDialogue.Move)
	}
	if e.lastResponseIntent.ShouldAct {
		t.Fatal("post-production response intent must not remain an action intent")
	}
	if e.flowTrace != nil {
		summary := e.flowTrace.Snapshot()
		joined := strings.Join(summary, "\n")
		if !strings.Contains(joined, "CAPABILITY DECISION") || !strings.Contains(joined, "PRODUCTION RESULT") {
			t.Fatalf("production authority trace incomplete:\n%s", joined)
		}
	}
}
