package linguistics

import (
	"fmt"
	"testing"
)

func TestPipelineProcessSimpleInput(t *testing.T) {
	router := NewRouter()

	pipeline := NewPipeline(
		router,
		nil,
		NewValidator(),
	)

	result, err := pipeline.Process("abre Blender")
	if err != nil {
		t.Fatalf("Process() devolvió un error: %v", err)
	}

	if result.UsedLLM {
		t.Error("La entrada simple no debería usar el LLM")
	}

	if result.Interpretation != nil {
		t.Errorf(
			"La entrada simple no debería producir interpretación: %v",
			result.Interpretation,
		)
	}

	if result.Decision.UseLLM {
		t.Errorf(
			"Router decidió usar LLM para una entrada simple: %s",
			result.Decision.Reason,
		)
	}

	t.Logf(
		"UsedLLM: %v | Reason: %s",
		result.UsedLLM,
		result.Decision.Reason,
	)
}

// ─────────────────────────────────────────────────────────────
// SEGUNDO TEST: ruta completa usando Phi-4 Mini
// ─────────────────────────────────────────────────────────────

func TestPipelineProcessWithLLM(t *testing.T) {
	client := &testLanguageClient{available: true}
	interpreter := NewInterpreter(client)

	pipeline := NewPipeline(
		NewRouter(),
		interpreter,
		NewValidator(),
	)

	input := "continúa con lo que hacíamos ayer"

	result, err := pipeline.Process(input)
	if err != nil {
		t.Fatalf("Process() devolvió un error: %v", err)
	}

	// El Router debe detectar que esta entrada requiere
	// análisis lingüístico adicional.
	if !result.Decision.UseLLM {
		t.Errorf(
			"El Router debería haber decidido usar el LLM. Razón: %s",
			result.Decision.Reason,
		)
	}

	// La ruta del LLM debe haber completado el flujo.
	if !result.UsedLLM {
		t.Error("Se esperaba UsedLLM=true después de procesar la entrada con el LLM")
	}

	// Debe existir una interpretación. El contenido exacto puede
	// variar entre inferencias del modelo.
	if result.Interpretation == nil {
		t.Fatal("Se esperaba una interpretación del LLM, pero se recibió nil")
	}

	t.Logf(
		"UsedLLM: %v | Reason: %s",
		result.UsedLLM,
		result.Decision.Reason,
	)

	t.Logf(
		"Intent: %s | Entities: %v | References: %v | Constraints: %v | Meaning: %q | Confidence: %.2f",
		result.Interpretation.Intent,
		result.Interpretation.Entities,
		result.Interpretation.References,
		result.Interpretation.Constraints,
		result.Interpretation.Meaning,
		result.Interpretation.Confidence,
	)

	if len(result.Warnings) > 0 {
		t.Logf("Warnings: %v", result.Warnings)
	}
}

func TestPipelineProcessWithLLMError(t *testing.T) {
	client := &testLanguageClient{
		available: true,
		err:       fmt.Errorf("servidor lingüístico no disponible"),
	}

	interpreter := NewInterpreter(client)

	pipeline := NewPipeline(
		NewRouter(),
		interpreter,
		NewValidator(),
	)

	// Esta entrada debe obligar al Router a usar el LLM.
	input := "continúa con lo que hacíamos ayer"

	// Si ocurre un panic, el defer lo detectará y hará fallar el test.
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Pipeline produjo un panic: %v", r)
		}
	}()

	result, err := pipeline.Process(input)

	if err == nil {
		t.Fatal("Se esperaba un error al conectar con Ollama en el puerto 11435, pero no se recibió ninguno")
	}

	if !result.Decision.UseLLM {
		t.Errorf(
			"El Router debería haber decidido usar el LLM. Razón: %s",
			result.Decision.Reason,
		)
	}

	if result.Interpretation != nil {
		t.Errorf(
			"Se esperaba Interpretation=nil después del error, recibido: %+v",
			result.Interpretation,
		)
	}

	if result.UsedLLM {
		t.Error("UsedLLM debería ser false cuando el Interpreter falla")
	}

	t.Logf(
		"Error manejado correctamente | UseLLM: %v | UsedLLM: %v | Error: %v",
		result.Decision.UseLLM,
		result.UsedLLM,
		err,
	)
}
