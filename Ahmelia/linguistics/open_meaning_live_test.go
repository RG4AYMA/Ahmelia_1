//go:build ahmelia_live

package linguistics

// Prueba VIVA manual del contrato de comprensión abierta contra el modelo real
// de producción (no forma parte de las suites regulares: requiere el build tag
// ahmelia_live). Ejecutar explícitamente:
//
//	go test -tags ahmelia_live -run TestOpenMeaningLive ./linguistics -count=1 -v
//
// PB 2-4 concesiones en CPU: cada llamada ≈25-50 s con qwen3:4b.

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestOpenMeaningLive(t *testing.T) {
	if os.Getenv("AHMELIA_LIVE_LLM") != "1" {
		t.Skip("prueba solo con AHMELIA_LIVE_LLM=1")
	}
	client := NewOllamaClient()
	if !client.Available() {
		t.Skip("ollama no disponible")
	}
	interp := NewInterpreter(client)

	// Los 3 casos del rediseño: preferencia, corrección, social.
	inputs := []string{
		"mi bebida favorita es el café con leche",
		"no, me refería a la temperatura de la ducha",
		"hola, ¿cómo estás hoy?",
	}
	for _, in := range inputs {
		start := time.Now()
		m, err := interp.InterpretOpenContext(context.Background(), in)
		if err != nil {
			t.Errorf("entrada %q falló: %v", in, err)
			continue
		}
		t.Logf("%-50s (%.0fs) purpose=%q owner=%q topic=%q conf=%.2f mentions=%v meaning=%q",
			in, time.Since(start).Seconds(), m.Purpose, m.OwnerOfData, m.Topic, m.Confidence, m.Mentions, m.Meaning)
	}
}
