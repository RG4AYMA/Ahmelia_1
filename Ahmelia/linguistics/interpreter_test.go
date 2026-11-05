package linguistics

import (
	"os"
	"testing"

	"ahmelia/comprehension"
)

func TestInterpreterInterpret(t *testing.T) {
	client := &testLanguageClient{available: true}
	interpreter := NewInterpreter(client)

	tests := []struct {
		name           string
		input          string
		expectedIntent comprehension.IntentType
	}{
		{name: "comando directo", input: "abre Blender", expectedIntent: comprehension.IntentCommand},
		{name: "pregunta", input: "¿qué hora es?", expectedIntent: comprehension.IntentQuestion},
		{name: "referencia contextual", input: "continúa con lo que hacíamos ayer en Blender", expectedIntent: comprehension.IntentCommand},
		{name: "restriccion", input: "abre el proyecto de Unity pero no ejecutes el juego", expectedIntent: comprehension.IntentCommand},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := interpreter.Interpret(tt.input)
			if err != nil {
				t.Fatalf("Interpret() devolvió un error: %v", err)
			}

			if result.Intent != tt.expectedIntent {
				t.Errorf("Intent incorrecto: esperado %q, recibido %q", tt.expectedIntent, result.Intent)
			}
			if result.Confidence < 0 || result.Confidence > 1 {
				t.Errorf("Confidence fuera de rango: %f", result.Confidence)
			}
			if tt.name == "pregunta" && len(result.References) != 0 {
				t.Errorf("Una pregunta independiente no debe generar referencias contextuales: %v", result.References)
			}
			if tt.name == "referencia contextual" && len(result.Constraints) != 0 {
				t.Errorf("Una referencia contextual no debe convertirse en restricción: %v", result.Constraints)
			}
		})
	}

	if len(client.prompts) != len(tests) {
		t.Fatalf("se esperaba una consulta lingüística por caso: %d, recibido %d", len(tests), len(client.prompts))
	}
}

func TestInterpreterInterpretWithoutClient(t *testing.T) {
	interpreter := NewInterpreter(nil)
	if _, err := interpreter.Interpret("abre Blender"); err == nil {
		t.Fatal("se esperaba error cuando no existe cliente lingüístico")
	}
}

// Opt-in integration test. It is never required by the normal test suite.
// Run with AHMELIA_OLLAMA_INTEGRATION=1 when Ollama + phi4-mini are available.
func TestInterpreterInterpretWithOllama(t *testing.T) {
	if os.Getenv("AHMELIA_OLLAMA_INTEGRATION") != "1" {
		t.Skip("integración Ollama desactivada; usar AHMELIA_OLLAMA_INTEGRATION=1 para ejecutarla")
	}

	client := NewOllamaClient()
	if !client.Available() {
		t.Skip("Ollama no está disponible en este entorno")
	}

	result, err := NewInterpreter(client).Interpret("abre Blender")
	if err != nil {
		t.Fatalf("Interpret() con Ollama devolvió un error: %v", err)
	}
	if result.Intent != comprehension.IntentCommand {
		t.Fatalf("intent inesperado desde Ollama: %q", result.Intent)
	}
}
