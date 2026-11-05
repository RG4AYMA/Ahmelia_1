package solving

import (
	"ahmelia/interaction"
	"fmt"
	"strings"
)

// ============================================================================
// REFLECTION_LOOP.GO — Bloque 41, Fase 3 Profundizada (Nivel II)
// ============================================================================
//
// El propio roadmap lo dijo antes de que este bloque existiera:
// Reflection ANTES de Bloque 38 habría sido "elegí A... creo que elegí
// bien" — sin nada real sobre lo cual reflexionar. Recién ahora hay
// material de verdad:
//
//   - Comparator (Bloque 38): Comparison.HasCloseCall() — ¿alguna
//     alternativa quedó cerca de la elegida?
//   - Strategy Selector (Bloque 39): Selection.OverrodeSimilarity —
//     ¿el historial real cambió lo que hubiera elegido solo por
//     parecido de palabras?
//   - Adaptive Confidence (Bloque 40): AdaptiveAction — ¿qué tan
//     sólida era la Decision de fondo?
//
// Reflection Loop cruza las tres. "Integrar, no crear": no calcula
// ninguna señal nueva — solo decide, con lo que YA existe, si hay algo
// concreto que valga la pena preguntarse, y responde con honestidad.
//
// Igual que DescribeAll (38) y DescribeSelections (39) omiten los gaps
// sin nada interesante, Reflection Loop hace lo mismo pero a un nivel
// más estricto: un gap SIN close call y SIN override no genera ninguna
// Reflection — no hay pregunta real que hacerse ahí ("¿por qué elegí
// la única opción que había?" no es una reflexión, es una tautología).

// Reflection es una pregunta real, con respuesta real, sobre UNA
// elección puntual.
type Reflection struct {
	TaskID           string
	TaskDesc         string
	Chosen           string // Label del candidato elegido
	Question         string
	Answer           string
	ShouldReconsider bool // true solo cuando hubo un close call real y NADA (ni historial) lo respaldó
}

// ReflectionSet es el resultado de reflexionar sobre TODOS los gaps de
// un plan, más una nota de cierre a nivel plan (Bloque 40, opcional).
type ReflectionSet struct {
	Goal        string
	Reflections []Reflection
	Confidence  *AdaptiveAction // Bloque 40 — nil si no se pasó
}

// NeedsReconsideration devuelve solo las Reflection donde
// ShouldReconsider es true — el subconjunto que de verdad importa si
// alguien pregunta "¿deberías haber hecho algo distinto?".
func (rs *ReflectionSet) NeedsReconsideration() []Reflection {
	var out []Reflection
	for _, r := range rs.Reflections {
		if r.ShouldReconsider {
			out = append(out, r)
		}
	}
	return out
}

// Reflector es el Reflection Loop en sí. Sin estado propio, mismo
// criterio que el resto de Fase 3.
type Reflector struct{}

// NewReflector arma el Reflector.
func NewReflector() *Reflector {
	return &Reflector{}
}

// Reflect cruza comparisons (Bloque 38) y selections (Bloque 39),
// emparejados por TaskID, y arma una Reflection por cada gap que tenga
// algo real que decir. confidence (Bloque 40) es opcional — puede ser
// nil si no se calculó para esta consulta puntual.
func (r *Reflector) Reflect(comparisons []*Comparison, selections []*Selection, confidence *AdaptiveAction) *ReflectionSet {
	rs := &ReflectionSet{Confidence: confidence}

	selByTask := make(map[string]*Selection, len(selections))
	for _, sel := range selections {
		if sel != nil {
			selByTask[sel.TaskID] = sel
		}
	}

	for _, comp := range comparisons {
		if comp == nil {
			continue
		}
		sel, ok := selByTask[comp.TaskID]
		if !ok || !sel.HasChoice() {
			continue
		}

		hadCloseCall := comp.HasCloseCall()
		if !hadCloseCall && !sel.OverrodeSimilarity {
			continue // nada real que reflexionar en este gap
		}

		ref := Reflection{TaskID: comp.TaskID, TaskDesc: comp.TaskDesc, Chosen: sel.Chosen.Label}

		switch {
		case sel.OverrodeSimilarity:
			// El historial real ya justificó apartarse del mayor
			// parecido de palabras — haya habido o no un close call de
			// por medio, la razón de Strategy Selector (39) ya es la
			// respuesta honesta; no hace falta reconsiderar algo que
			// ya se decidió con evidencia real.
			ref.Question = fmt.Sprintf("¿Por qué elegí %s en vez de la opción más parecida por palabras?", ref.Chosen)
			ref.Answer = sel.Reason
			ref.ShouldReconsider = false

		default: // hadCloseCall && !OverrodeSimilarity
			ref.Question = "¿Había una alternativa real que casi elijo?"
			ref.Answer = fmt.Sprintf(
				"Sí — %s. Ganó %s por mayor parecido de palabras, sin ningún historial real que desempatara; la diferencia era chica, vale la pena tenerla presente si esta elección no funciona.",
				closeCallText(comp), ref.Chosen,
			)
			ref.ShouldReconsider = true
		}

		rs.Reflections = append(rs.Reflections, ref)
	}

	return rs
}

// closeCallText arma "Unity (65%)" a partir de la primera alternativa
// marcada WasClose en un Comparison — evita que Reflect() tenga que
// conocer la forma interna de ComparisonNote.
func closeCallText(comp *Comparison) string {
	for _, n := range comp.Notes {
		if n.Rank > 1 && n.WasClose {
			return fmt.Sprintf("%s (%.0f%%) estuvo cerca", n.Candidate.Label, n.Candidate.Similarity*100)
		}
	}
	return "una alternativa estuvo cerca"
}

// Describe arma la narrativa completa de la reflexión, cerrando con la
// nota de Adaptive Confidence (Bloque 40) si se pasó una.
func (rs *ReflectionSet) Describe() string {
	var b strings.Builder

	if len(rs.Reflections) == 0 {
		b.WriteString("No encontré nada concreto sobre lo cual reflexionar en este plan — ninguna elección estuvo cerca de otra, ni el historial cambió ningún resultado.")
	} else {
		parts := make([]string, len(rs.Reflections))
		for i, ref := range rs.Reflections {
			parts[i] = fmt.Sprintf("%s: %s %s", ref.TaskID, ref.Question, ref.Answer)
		}
		b.WriteString(strings.Join(parts, " "))
	}

	if rs.Confidence != nil {
		b.WriteString(" ")
		b.WriteString(rs.Confidence.Describe())
	}

	return b.String()
}

// ── Trigger de consulta ──────────────────────────────────────────────

var reflectionTriggers = []string{
	"deberias reconsiderar algo",
	"hiciste bien en elegir eso",
	"te arrepientes de alguna eleccion",
	"reflexiona sobre tu decision",
}

// ParseReflectionQuery detecta si el usuario pide que Ahmelia
// reflexione sobre sus propias elecciones — distinta de
// ParseComparisonQuery (Bloque 38, que solo describe qué tan cerca
// estuvieron) y de ParseStrategyQuery (Bloque 39, que pregunta qué
// elegiría): esta pregunta si, mirando hacia atrás, la elección fue
// sólida.
func ParseReflectionQuery(req interaction.SemanticRequest) bool {
	return interaction.HasSemanticQuery(req, interaction.QuerySolvingReflection)
}
