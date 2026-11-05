package memory_intelligence

import (
	"ahmelia/interaction"
	"fmt"
	"sort"
	"strings"
)

// ============================================================================
// PATTERNS.GO — Fase 3 (Patrones) de Pattern Learning
// ============================================================================
//
// Un patrón es una Chain (sequence.go) que cruzó un umbral mínimo de
// repetición: deja de ser "pasó un par de veces, puede ser casualidad" para
// ser "esto es un flujo reconocible". DefaultPatternThreshold es
// explícitamente PROVISIONAL — no hay todavía datos de uso real (semanas de
// sesiones) para justificar un número "correcto"; 3 es un piso razonable
// para no reaccionar a ruido de una sola sesión, pero se espera ajustarlo
// con evidencia real más adelante (Fase 4 — Hábitos ya va a necesitar un
// criterio más estricto que este, con consistencia temporal y no solo
// conteo).
const DefaultPatternThreshold = 3

// Pattern es una Chain promovida: ya se considera un flujo repetido, no una
// coincidencia. Steps puede ser de cualquier largo 2..MaxChainLength — no
// solo pares (eso era la versión anterior, basada en SequenceLink).
type Pattern struct {
	Steps []string
	Count int
}

// DetectPatterns filtra las Chain que alcanzan minCount repeticiones y las
// devuelve ordenadas de más a menos frecuente.
func DetectPatterns(chains []Chain, minCount int) []Pattern {
	patterns := []Pattern{}
	for _, c := range chains {
		if c.Count >= minCount {
			patterns = append(patterns, Pattern{Steps: c.Steps, Count: c.Count})
		}
	}
	sort.Slice(patterns, func(i, j int) bool { return patterns[i].Count > patterns[j].Count })
	return patterns
}

// DescribePatterns arma la narrativa en español de los patrones detectados.
// No inventa nada que no esté en el conteo: si no hay patrones todavía, lo
// dice tal cual — no rellena con algo genérico.
func DescribePatterns(patterns []Pattern) string {
	if len(patterns) == 0 {
		return "Todavía no he detectado ningún patrón que se repita lo suficiente."
	}
	shown := patterns
	if len(shown) > 5 {
		shown = shown[:5]
	}
	lines := make([]string, 0, len(shown))
	for _, p := range shown {
		lines = append(lines, fmt.Sprintf("%s (%d veces)", strings.Join(p.Steps, " → "), p.Count))
	}
	return "He notado esto: " + strings.Join(lines, "; ")
}

// patternTriggers son las frases que disparan la consulta de patrones.
// Deliberadamente separadas de relationTriggers (relations.go) aunque el
// tema sea parecido — "qué patrones" pregunta por flujos SECUENCIALES
// detectados solos, no por sinónimos de un comando puntual.
var patternTriggers = []string{
	"que patrones has detectado",
	"que patrones detectaste",
	"que sueles hacer",
	"que patron sigues",
}

// ParsePatternsQuery detecta si la pregunta pide un resumen de patrones
// detectados. A diferencia de ParseRelationsQuery, esto no recorta un
// sujeto — hoy responde con el resumen general (top 5 patrones), no un
// patrón puntual sobre un comando específico. Filtrar por sujeto ("qué
// sueles hacer después de abrir Unity") queda pendiente para cuando haga
// falta de verdad, no se arma la extracción de sujeto sin un caso real que
// la use.
func ParsePatternsQuery(req interaction.SemanticRequest) bool {
	return interaction.HasSemanticQuery(req, interaction.QueryHistoryPatterns)
}
