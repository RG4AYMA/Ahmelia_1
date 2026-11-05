package main

import "testing"

// TestStripWakeWord_SacaLaWakeWordSiEstaPresente (Integration Fix)
// reproduce el bug real: "amelia como instalo go" tipeado por texto se
// buscaba como Skill literal "Amelia como instalo go" — la voz nunca
// lo sufre porque voice_google_backup.py ya hace este mismo strip en
// Python antes de mandar el comando a Go.
func TestStripWakeWord_SacaLaWakeWordSiEstaPresente(t *testing.T) {
	casos := map[string]string{
		"amelia como instalo go":   "como instalo go",
		"Ahmelia como instalo go":  "como instalo go",
		"A MELIA como instalo go":  "como instalo go",
		"jamelia sabes usar go":    "sabes usar go",
		"hamelia que sabes sobre X": "que sabes sobre X",
	}
	for input, want := range casos {
		if got := stripWakeWord(input); got != want {
			t.Errorf("stripWakeWord(%q) = %q, esperaba %q", input, got, want)
		}
	}
}

// TestStripWakeWord_SinWakeWordDejaElTextoIgual confirma que el texto
// sin wake word (el caso normal, ya que el texto NO la necesita) no se
// toca — no hay ningún strip parcial ni falso positivo.
func TestStripWakeWord_SinWakeWordDejaElTextoIgual(t *testing.T) {
	casos := []string{
		"como instalo go",
		"sabes usar go",
		"amelio como instalo go",   // parecido pero no es un wake word real
		"tengo una amelia en casa", // "amelia" NO al principio — no debe tocarse
	}
	for _, c := range casos {
		if got := stripWakeWord(c); got != c {
			t.Errorf("stripWakeWord(%q) = %q, esperaba que quedara igual", c, got)
		}
	}
}

// TestStripWakeWord_SoloLaWakeWordDevuelveVacio confirma el caso
// límite: si el usuario escribe solo la wake word sola, sin comando,
// el resultado es string vacío, no un espacio suelto.
func TestStripWakeWord_SoloLaWakeWordDevuelveVacio(t *testing.T) {
	if got := stripWakeWord("amelia"); got != "" {
		t.Errorf("stripWakeWord(\"amelia\") = %q, esperaba string vacío", got)
	}
}
