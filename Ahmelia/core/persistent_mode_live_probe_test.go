//go:build liveprobe

package core

// Sonda manual del tramo 3.4 contra Ollama vivo.
//
// No forma parte de la suite normal:
//   go test -tags liveprobe ./core -run TestLivePersistentModeSuggestionNeverApplied -v -timeout 400s
//
// Verifica con el modelo real que el espacio de trabajo incluye el modo
// persistente y que una sugerencia del modelo no cambia el estado del arnés.
// Usa un presupuesto mayor que el de producción (90 s) a propósito: mide la
// conducta del modelo, no el presupuesto de la sombra.

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ahmelia/agency"
	"ahmelia/deliberation"
	"ahmelia/linguistics"
)

func TestLivePersistentModeSuggestionNeverApplied(t *testing.T) {
	dir := t.TempDir()
	e := &Engine{}
	e.enablePersistentModeAt(filepath.Join(dir, "persistent_mode.json"))
	if err := e.LastPersistentModeError(); err != "" {
		t.Fatalf("enable: %s", err)
	}
	if _, err := e.SetPersistentMode(agency.ModeFocus, "owner"); err != nil {
		t.Fatalf("set focus: %v", err)
	}

	client := linguistics.NewOllamaClient()
	if !client.Available() {
		t.Skip("Ollama no disponible")
	}
	e.deliberationClient = client
	e.deliberationShadow = deliberation.NewShadowLog(filepath.Join(dir, "deliberation_shadow.jsonl"))

	input := "abre eso"
	ws := e.buildDeliberationWorkspace(&processTurn{input: input})
	if ws.Empty() {
		t.Fatal("el espacio de trabajo quedó vacío")
	}
	rendered := ws.Render()
	if !strings.Contains(rendered, "modo_persistente") {
		t.Fatalf("el espacio de trabajo no incluye el modo persistente:\n%s", rendered)
	}
	for _, line := range strings.Split(rendered, "\n") {
		if strings.Contains(line, "modo_persistente") {
			t.Logf("workspace=%s", line)
		}
	}

	harness := deliberation.HarnessDecision{
		Action: "respond", Strategy: deliberation.StrategyAnswer, Intent: "answer",
		Speak: true, Reason: "el turno pide conversación, no acción",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()
	start := time.Now()
	raw, err := client.GenerateShadowContext(ctx, deliberation.BuildPrompt(ws, harness), deliberation.Schema(), 400)
	if err != nil {
		t.Fatalf("GenerateShadowContext: %v", err)
	}
	t.Logf("elapsed=%s raw_len=%d raw=%s", time.Since(start), len(raw), raw)
	proposal, err := deliberation.ParseProposal(raw)
	if err != nil {
		t.Fatalf("ParseProposal: %v", err)
	}
	proposal.Bind(ws)
	rec := deliberation.Evaluate(harness, proposal, input, "live-mode-2")
	t.Logf("outcome=%s reasons=%v", rec.Outcome, rec.Reasons)
	t.Logf("reading=%q critique=%q suggestion=%q rejected=%q",
		rec.Proposal.Reading, rec.Proposal.Critique, rec.Proposal.ModeSuggestion, rec.Proposal.ModeSuggestionRejected)

	state, ok := e.PersistentModeState()
	if !ok || state.Mode != agency.ModeFocus {
		t.Fatalf("la sugerencia del modelo cambió el modo: %+v ok=%t", state, ok)
	}
}
