package interaction

import "strings"

// Polaridad del posterior de preferencias (P4/Paso 2, directiva DeepSeek
// 19/10): el posterior SOLO captura la familia explícita POSITIVA ("me gusta
// X", "mi favorito es X"). Las negaciones ("ya no me gusta X", "odio X") no
// se convierten en Outcome positivo: requieren su propio camino de admisión
// (R11/R12 — mecanismo reutilizable, no lista de frases).
//
// Mecanismo (regla léxico-estructural reutilizable, no whitelist por frase):
//  1. Cabeza de gusto: algún token de un léxico mínimo de preferencia
//     afirmativa en español (gusta/gustan, encanta/encantan, favorito/favorita
//     y flexiones de número/género, prefiero).
//  2. Ventana de negación: si un marcador de negación (no, nunca, jamás,
//     tampoco) aparece dentro de las 3 palabras anteriores a la cabeza, sin
//     cruzar frontera de contraste (pero/aunque/porque/pues), la expresión
//     es negada.
//
// POLÍTICA FAIL-CLOSED: si no hay cabeza de gusto afirmativa clara o hay
// ambigüedad, devuelve false (no se registra nada — honestidad antes que
// cobertura). Determinista, sin LLM.
func ExplicitPositivePreference(text string) bool {
	words := preferenceTokens(text)
	found := false
	for i, w := range words {
		if !preferenceHead(w) {
			continue
		}
		// Regla: TODA cabeza de gusto debe estar libre de negación ligada;
		// cualquier cabeza negada declara la expresión NO positiva (falla
		// cerrada — la corrección "pero ya no…" domina sobre lo anterior).
		// Ventana de 3 tokens hacia atrás ("ya no me gusta", "no me gusta").
		for j := i - 1; j >= 0 && i-j <= 3; j-- {
			if contrastStopped(words[j]) {
				break
			}
			if negationCue(words[j]) {
				return false
			}
		}
		// Ventana de 2 tokens hacia adelante ("prefiero no tomar gaseosa"):
		// el articulante "prefiero" puede anteceder la negación del objeto.
		for k := i + 1; k < len(words) && k-i <= 2; k++ {
			if contrastStopped(words[k]) {
				break
			}
			if negationCue(words[k]) {
				return false
			}
		}
		found = true // cabeza libre de negación ligada
	}
	return found // sin cabeza de gusto explícita: fail-closed
}

func preferenceTokens(text string) []string {
	replaced := strings.NewReplacer(
		"á", "a", "é", "e", "í", "i", "ó", "o", "ú", "u",
		"Á", "a", "É", "e", "Í", "i", "Ó", "o", "Ú", "u",
		".", " ", ",", " ", ";", " ", ":", " ", "!", " ", "¿", " ", "?", " ", "¡", " ",
	).Replace(strings.ToLower(strings.Join(strings.Fields(text), " ")))
	return strings.Fields(replaced)
}

// preferenceHead — léxico mínimo (mecanismo, R12): cabezas de gusto
// afirmativo. "gustaría" queda FUERA deliberadamente: es deseo/conjetura, no
// estado de preferencia; así la regla no inventa gustos desde preguntas de
// probemos.
func preferenceHead(w string) bool {
	switch w {
	case "gusta", "gustan", "encanta", "encantan", "encanto",
		"favorito", "favoritos", "favorita", "favoritas", "prefiero":
		return true
	}
	return false
}

func negationCue(w string) bool {
	switch w {
	case "no", "nunca", "jamas", "tampoco":
		return true
	}
	return false
}

func contrastStopped(w string) bool {
	switch w {
	case "pero", "aunque", "porque", "pues":
		return true
	}
	return false
}
