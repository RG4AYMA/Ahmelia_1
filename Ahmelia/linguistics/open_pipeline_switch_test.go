package linguistics

// Prueba pura del switch de comprensión abierta (hito "van juntos" 2026-10-17):
// el pipeline en modo abierto produce OpenMeaning y no interpretación cerrada,
// sin HTTP ni Ollama. El default de la capa sigue siendo el contrato cerrado
// (suites y fixtures no cambian).

import (
	"context"
	"testing"

	"ahmelia/interaction"
)

func TestPipelineOpenComprehensionSwitch(t *testing.T) {
	fake := fakeStructuredOpenClient{
		raw:       `{"purpose":"pregunta","owner_of_data":"user","topic":"eso","meaning":"Pregunta qué es eso","mentions":["eso"],"confidence":0.7}`,
		available: true,
	}
	pipeline := NewPipeline(NewRouter(), NewInterpreter(fake), NewValidator())
	input := "¿y eso qué es?"
	request := interaction.Analyze(input, interaction.Context{})

	decision := NewRouter().DecideRequest(request)
	if !decision.UseLLM {
		t.Fatalf("la entrada de prueba debe requerir profundidad lingüística, razón: %q", decision.Reason)
	}

	pipeline.SetOpenComprehension(true)
	result, err := pipeline.ProcessRequest(context.Background(), input, request)
	if err != nil {
		t.Fatalf("pipeline en modo abierto falló: %v", err)
	}
	if !result.UsedLLM {
		t.Fatal("modo abierto debe marcar UsedLLM")
	}
	if result.OpenMeaning == nil {
		t.Fatal("modo abierto debe producir OpenMeaning")
	}
	if result.Interpretation != nil {
		t.Fatal("modo abierto no debe producir interpretación cerrada")
	}
	if result.OpenMeaning.Purpose != OpenPurposeQuestion || result.OpenMeaning.Topic != "eso" {
		t.Fatalf("comprensión abierta inesperada: %+v", *result.OpenMeaning)
	}

	closed := NewPipeline(NewRouter(), NewInterpreter(fake), NewValidator())
	closedResult, closedErr := closed.ProcessRequest(context.Background(), input, request)
	if closedErr != nil {
		// El contrato cerrado no debe interpretar la salida abierta del fake:
		// es evidencia de que el default de la capa sigue siendo el cerrado.
		return
	}
	if closedResult.OpenMeaning != nil {
		t.Fatal("el modo cerrado no debe producir OpenMeaning")
	}
}
