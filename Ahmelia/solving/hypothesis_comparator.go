package solving

import (
	"ahmelia/interaction"
	"fmt"
	"strings"
)

// ============================================================================
// HYPOTHESIS_COMPARATOR.GO — Bloque 38, Fase 3 Profundizada (Nivel II)
// ============================================================================
//
// Multi-Hypothesis Engine (Bloque 36) dejó de descartar el 2do y 3er
// mejor candidato de cada gap — pero nadie los miraba después:
// Decision Engine (Bloque 27) y Confidence Calibration (Bloque 37)
// siguen leyendo solo el mejor (ClosestAction/Similarity). Es como
// pensar tres posibilidades y decir "perfecto, ignoro dos". Hypothesis
// Comparator es el primero que efectivamente mira Alternatives.
//
// No decide — eso es Strategy Selector, el bloque siguiente. Esto solo
// analiza y explica qué tan cerca estuvo el candidato descartado del
// elegido. Sin esto, un futuro Reflection Loop no tendría material real
// sobre qué reflexionar ("elegí A" a secas, sin saber si B estuvo cerca,
// es una reflexión vacía).
//
// ACLARACIÓN IMPORTANTE, decidida antes de escribir esto: la
// comparación es ENTRE candidatos del MISMO gap (el elegido vs. sus
// Alternatives) — no entre distintos gaps, y no contra ConstraintReport.
// Requirement/ConstraintCheck (Bloque 26) viven a nivel del
// GoalAnalysis completo, sin TaskID — no existe ningún dato hoy que
// vincule "este riesgo específico" con "esta Action específica".
// Comparar "Hipótesis A: 2 riesgos" tal como lo sugería el roadmap
// original no sería honesto con los datos reales — se deja fuera de
// este bloque a propósito, no es un olvido.

// CloseCallThreshold: si la diferencia de Similarity entre el elegido y
// una alternativa es menor a esto, se considera que estuvo cerca —
// vale la pena decirlo, no es un descarte automático sin más.
const CloseCallThreshold = 0.15

// ComparisonNote es UN candidato dentro de la comparación, con su
// posición y qué tan lejos quedó del elegido.
type ComparisonNote struct {
	Candidate Candidate
	Rank      int     // 1 = el elegido (antes ClosestAction), 2/3 = Alternatives en orden
	Gap       float64 // Similarity del elegido menos la de este candidato; 0 para el elegido mismo
	WasClose  bool    // Gap < CloseCallThreshold — no aplica al elegido (Rank==1)
}

// Comparison es el resultado de comparar los candidatos de UN gap.
type Comparison struct {
	TaskID   string
	TaskDesc string
	Notes    []ComparisonNote
}

// HasCloseCall es true si alguna alternativa quedó cerca del elegido —
// el dato concreto que un futuro Reflection Loop necesitaría para
// preguntarse "¿debí explorar más?".
func (c *Comparison) HasCloseCall() bool {
	for _, n := range c.Notes {
		if n.Rank > 1 && n.WasClose {
			return true
		}
	}
	return false
}

// Describe arma la narrativa de esta comparación puntual.
func (c *Comparison) Describe() string {
	if len(c.Notes) == 0 {
		return fmt.Sprintf("%s (%s): sin ningún candidato, nada que comparar.", c.TaskID, c.TaskDesc)
	}
	if len(c.Notes) == 1 {
		return fmt.Sprintf("%s (%s): un solo candidato (%s), sin alternativas con las que compararlo.",
			c.TaskID, c.TaskDesc, c.Notes[0].Candidate.Label)
	}

	var parts []string
	for _, n := range c.Notes {
		if n.Rank == 1 {
			parts = append(parts, fmt.Sprintf("elegí %s (%.0f%%)", n.Candidate.Label, n.Candidate.Similarity*100))
			continue
		}
		closeness := "quedó bastante atrás"
		if n.WasClose {
			closeness = "estuvo cerca, casi la elijo"
		}
		parts = append(parts, fmt.Sprintf("%s (%.0f%%, %s)", n.Candidate.Label, n.Candidate.Similarity*100, closeness))
	}
	return fmt.Sprintf("%s (%s): %s.", c.TaskID, c.TaskDesc, strings.Join(parts, "; "))
}

// Comparator es el Hypothesis Comparator en sí. Sin estado propio,
// mismo criterio que el resto de Fase 3.
type Comparator struct{}

// NewComparator arma el Comparator.
func NewComparator() *Comparator {
	return &Comparator{}
}

// Compare arma la comparación de UN Hypothesis: el candidato elegido
// (ClosestAction/ClosestLabel/Similarity/Reason, aplanados de vuelta a
// un Candidate) más sus Alternatives (Bloque 36), en orden.
func (cmp *Comparator) Compare(h Hypothesis) *Comparison {
	c := &Comparison{TaskID: h.TaskID, TaskDesc: h.TaskDesc}
	if !h.HasLead() {
		return c // sin candidato, nada que comparar — Notes queda vacío, no inventado
	}

	best := Candidate{Action: h.ClosestAction, Label: h.ClosestLabel, Similarity: h.Similarity, Reason: h.Reason}
	c.Notes = append(c.Notes, ComparisonNote{Candidate: best, Rank: 1})

	for i, alt := range h.Alternatives {
		gap := best.Similarity - alt.Similarity
		c.Notes = append(c.Notes, ComparisonNote{
			Candidate: alt,
			Rank:      i + 2,
			Gap:       gap,
			WasClose:  gap < CloseCallThreshold,
		})
	}
	return c
}

// CompareAll arma una Comparison por cada Hypothesis del set. hs puede
// ser nil — mismo criterio defensivo de todo Fase 3 — devuelve nil.
func (cmp *Comparator) CompareAll(hs *HypothesisSet) []*Comparison {
	if hs == nil {
		return nil
	}
	out := make([]*Comparison, 0, len(hs.Hypotheses))
	for _, h := range hs.Hypotheses {
		out = append(out, cmp.Compare(h))
	}
	return out
}

// DescribeAll arma la narrativa conjunta de varias Comparison —
// omitiendo las que no tienen nada que comparar (un solo candidato o
// ninguno), para no llenar la respuesta de líneas vacías de "nada que
// comparar" cuando la mayoría de los gaps de un plan chico no tienen
// alternativas reales.
func DescribeAll(comparisons []*Comparison) string {
	var useful []string
	for _, c := range comparisons {
		if len(c.Notes) > 1 {
			useful = append(useful, c.Describe())
		}
	}
	if len(useful) == 0 {
		return "Ninguno de los gaps tuvo más de un candidato real — nada que comparar todavía."
	}
	return strings.Join(useful, " ")
}

// ── Trigger de consulta ──────────────────────────────────────────────

var comparisonTriggers = []string{
	"compara las hipotesis",
	"que tan cerca estuvo la otra opcion",
	"pudiste haber elegido otra cosa",
	"que otras opciones considerarte",
}

// ParseComparisonQuery detecta si el usuario pregunta por la
// comparación entre candidatos — distinta de ParseHypothesisQuery
// (Bloque 25, que solo pide CUÁL es la mejor pista): esta pregunta
// específicamente si hubo otras opciones cerca de ganar.
func ParseComparisonQuery(req interaction.SemanticRequest) bool {
	return interaction.HasSemanticQuery(req, interaction.QuerySolvingComparison)
}
