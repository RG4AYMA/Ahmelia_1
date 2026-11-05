package linguistics

import "testing"

func TestRouterDecide(t *testing.T) {
	router := NewRouter()

	tests := []struct {
		name       string
		input      string
		wantUseLLM bool
	}{
		{
			name:       "comando simple",
			input:      "abre Blender",
			wantUseLLM: false,
		},
		{
			name:       "pregunta simple",
			input:      "¿cómo funciona?",
			wantUseLLM: false,
		},
		{
			name:       "referencia contextual",
			input:      "continúa con lo que hacíamos ayer",
			wantUseLLM: true,
		},
		{
			name:       "restriccion",
			input:      "abre Unity pero no ejecutes el juego",
			wantUseLLM: true,
		},
		{
			name:       "condicion",
			input:      "hazlo sin borrar los archivos",
			wantUseLLM: true,
		},
		{
			name:       "entrada larga",
			input:      "abre el proyecto de Unity y revisa todos los archivos antes de continuar",
			wantUseLLM: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := router.Decide(tt.input)

			if result.UseLLM != tt.wantUseLLM {
				t.Errorf(
					"Decide(%q): UseLLM = %v, esperado %v. Razón: %s",
					tt.input,
					result.UseLLM,
					tt.wantUseLLM,
					result.Reason,
				)
			}

			t.Logf(
				"Entrada: %q | UseLLM: %v | Razón: %s",
				tt.input,
				result.UseLLM,
				result.Reason,
			)
		})
	}
}
