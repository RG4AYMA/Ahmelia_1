package interaction

import "testing"

// Polaridad del posterior de preferencias (P4/Paso 2, directiva DeepSeek
// 19/10). Mecanismo determinista de ventana de negación + léxico de cabeza de
// gusto — NO una lista de frases (R11); falla cerrada ante ambigüedad.
func TestExplicitPositivePreferencePolaridad(t *testing.T) {
	casos := map[string]bool{
		// Familia positiva explícita: SÍ entra.
		"me gusta la gaseosa":         true,
		"me gustan las películas":     true,
		"me encanta el café":          true,
		"me encantan los atardeceres": true,
		"mi favorito es el negro":     true,
		"mi favorita es la gaseosa":   true,
		"prefiero el café con leche":  true,
		"Pero me gusta la gaseosa":    true, // contraste antes del evaluador separado
		// Negación ligada: NO entra (requiere su propio camino de admisión).
		"ya no me gusta la gaseosa":      false,
		"no me gusta el café":            false,
		"nunca me gusta madrugar":        false,
		"jamás me gustan los lunes":      false,
		"tampoco me gusta el clima frío": false,
		"prefiero no tomar gaseosa":      false,
		"me gusta pero ya no me gusta":   false, // la segunda cabeza ligada gana por proximidad
		// Familia negativa directa: NO entra (no es +1).
		"odio el café":       false,
		"detesto el frío":    false,
		"aborrezco el ruido": false,
		// Malas señas / ambigüedad: falla cerrada, no inventa.
		"sí":                false,
		"probar la gaseosa": false,
		"la gaseosa":        false,
		"":                  false,
	}
	for text, want := range casos {
		if got := ExplicitPositivePreference(text); got != want {
			t.Fatalf("ExplicitPositivePreference(%q): got %v want %v", text, got, want)
		}
	}
}
