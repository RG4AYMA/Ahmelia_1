//go:build liveprobe

package deliberation

// live_shadow_probe_test.go — sonda manual contra Ollama vivo.
//
// No forma parte de la suite normal: se ejecuta solo con
//   go test -tags liveprobe ./deliberation -run TestLiveShadowProposal -v
// Sirve para verificar que el prompt y el esquema producen una propuesta
// parseable con el modelo real, sin depender de dobles.

import (
	"context"
	"testing"
	"time"

	"ahmelia/linguistics"
)

func TestLiveShadowProposal(t *testing.T) {
	client := linguistics.NewOllamaClient()
	start := time.Now()
	if !client.Available() {
		t.Skip("Ollama no disponible")
	}
	availableAt := time.Now()
	t.Logf("available_after=%s", availableAt.Sub(start))

	var ws Workspace
	ws.Add("turno_actual", "abre eso", "interaction", 1)
	ws.Add("turno_anterior", "abre el proyecto de Unity", "conversation", 1)
	ws.Add("situacion", "proyecto está abierto Unity", "world", 0.9)

	harness := HarnessDecision{
		Action: "respond", Strategy: StrategyAnswer, Intent: "answer",
		Speak: true, Reason: "el turno actual pide continuar el trabajo previo",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Second)
	defer cancel()
	raw, err := client.GenerateShadowContext(ctx, BuildPrompt(&ws, harness), Schema(), 260)
	if err != nil {
		t.Fatalf("GenerateShadowContext: %v", err)
	}
	t.Logf("generation_after=%s", time.Since(availableAt))
	t.Logf("raw=%s", raw)
	p, err := ParseProposal(raw)
	if err != nil {
		t.Fatalf("ParseProposal: %v", err)
	}
	p.Bind(&ws)
	rec := Evaluate(harness, p, "abre eso", "live-1")
	t.Logf("outcome=%s strategy=%q grounded=%t evidence=%v rejected=%v confidence=%.2f",
		rec.Outcome, p.Strategy, p.Grounded, p.EvidenceIDs, p.RejectedEvidenceIDs, p.Confidence)
	if rec.Outcome == OutcomeInvalid || rec.Outcome == OutcomeUnavailable {
		t.Fatalf("el modelo real no produjo una propuesta válida: %+v", rec)
	}
}
