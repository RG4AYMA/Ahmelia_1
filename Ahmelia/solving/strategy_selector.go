package solving

import (
	"ahmelia/interaction"
	"fmt"
	"strings"

	memoryintelligence "ahmelia/memory_intelligence"
)

// ============================================================================
// STRATEGY_SELECTOR.GO — Bloque 39, Fase 3 Profundizada (Nivel II)
// ============================================================================
//
// Hypothesis Comparator (Bloque 38) mira A vs B vs C, pero no decide —
// hoy quien "gana" sigue siendo, siempre, el de mayor Similarity
// (ClosestAction). Strategy Selector es el primero que puede elegir
// distinto: "A tiene mayor Similarity, pero B tiene mejor historial
// real → elegir B".
//
// MISMA ACLARACIÓN HONESTA QUE BLOQUE 38, aplicada de nuevo acá: el
// roadmap original hablaba de "B viola menos restricciones" — eso
// sigue sin ser posible (Requirement/ConstraintCheck no tienen
// TaskID, Bloque 38). La segunda señal real que SÍ existe, y que
// además ya usa Knowledge Mapper (Bloque 23) con el mismo criterio, es
// memory_intelligence.LearningState.ActionConfidence (Bloque 7/8):
// cuántas veces Ahmelia ya ejecutó esa Action y qué tan bien le fue.
// Una Action con Similarity alta pero sin historial, o con historial
// pobre, puede perder contra una con Similarity un poco menor pero
// track record real — "integrar, no crear", mismo criterio de toda
// Fase 3.
//
// Sin track record para NINGUNO de los candidatos (Action nueva para
// las tres, o LearningState no disponible), Strategy Selector no tiene
// ninguna señal adicional real y el resultado es idéntico al de
// simplemente tomar el de mayor Similarity — no inventa una preferencia
// donde no hay datos.

// trackRecordWeight y similarityWeight sostienen el mismo peso — ninguna
// señal vale más que la otra a propósito, mismo criterio que
// calibrateConfidence (Bloque 37) y Self Evaluation (Bloque 29).
const (
	similarityWeight  = 0.5
	trackRecordWeight = 0.5
)

// CandidateScore es UN candidato con las dos señales que lo evaluaron
// y el score combinado que decidió si ganaba o no.
type CandidateScore struct {
	Candidate   Candidate
	TrackRecord float64 // 0..1, ActionConfidence/100; -1 si no hay historial para esta Action
	Combined    float64 // score final usado para elegir
}

// HasTrackRecord es un atajo — evita comparar contra -1 a mano en
// cualquier lugar que consuma CandidateScore.
func (cs CandidateScore) HasTrackRecord() bool {
	return cs.TrackRecord >= 0
}

// Selection es el resultado de elegir estrategia para UN gap.
type Selection struct {
	TaskID             string
	TaskDesc           string
	Chosen             Candidate
	Scores             []CandidateScore // los candidatos evaluados, en el orden en que llegaron (elegido primero, después Alternatives)
	OverrodeSimilarity bool             // true si Chosen NO es el de mayor Similarity — el track record cambió el resultado
	Reason             string
}

// HasChoice es false si el gap no tenía ningún candidato para empezar
// (mismo caso que !Hypothesis.HasLead()).
func (s *Selection) HasChoice() bool {
	return s.Chosen.Action != ""
}

// Selector es el Strategy Selector en sí. A diferencia de la mayoría de
// Fase 3, SÍ tiene estado (una referencia, no estado propio) — necesita
// LearningState para la segunda señal, mismo criterio de dependencia
// real que ya estableció Knowledge Mapper (Bloque 23).
type Selector struct {
	Learning *memoryintelligence.LearningStore
}

// NewSelector arma el Selector. learning puede ser nil — sin él, todos
// los candidatos quedan sin track record y el resultado coincide con
// tomar directamente el de mayor Similarity.
func NewSelector(learning *memoryintelligence.LearningStore) *Selector {
	return &Selector{Learning: learning}
}

// trackRecordFor consulta ActionConfidence para una Action puntual.
// -1 si no hay Learning, o si esa Action nunca se ejecutó — mismo
// criterio de "sin dato, no un dato malo" que Knowledge Mapper.
func (s *Selector) trackRecordFor(action string) float64 {
	if s.Learning == nil {
		return -1
	}
	if s.Learning.HasConfidence(action) {
		return s.Learning.Confidence(action) / 100.0
	}
	return -1
}

// Select evalúa los candidatos de UN Hypothesis (el elegido por
// Similarity más sus Alternatives, Bloque 36) y decide cuál conviene
// de verdad, combinando Similarity y track record real.
func (s *Selector) Select(h Hypothesis) *Selection {
	sel := &Selection{TaskID: h.TaskID, TaskDesc: h.TaskDesc}
	if !h.HasLead() {
		return sel // sin candidatos, nada que seleccionar
	}

	candidates := make([]Candidate, 0, 1+len(h.Alternatives))
	candidates = append(candidates, Candidate{Action: h.ClosestAction, Label: h.ClosestLabel, Similarity: h.Similarity, Reason: h.Reason})
	candidates = append(candidates, h.Alternatives...)

	best := -1
	for i, c := range candidates {
		tr := s.trackRecordFor(c.Action)
		combined := c.Similarity
		if tr >= 0 {
			combined = c.Similarity*similarityWeight + tr*trackRecordWeight
		}
		sel.Scores = append(sel.Scores, CandidateScore{Candidate: c, TrackRecord: tr, Combined: combined})
		if best == -1 || combined > sel.Scores[best].Combined {
			best = i
		}
	}

	sel.Chosen = sel.Scores[best].Candidate
	sel.OverrodeSimilarity = sel.Chosen.Action != h.ClosestAction

	if sel.OverrodeSimilarity {
		original := sel.Scores[0]
		chosen := sel.Scores[best]
		sel.Reason = fmt.Sprintf(
			"%s tenía mayor similitud (%.0f%%), pero %s tiene historial real de %.0f%% de éxito — elijo %s por track record, no solo por parecido de palabras.",
			original.Candidate.Label, original.Candidate.Similarity*100,
			chosen.Candidate.Label, chosen.TrackRecord*100, chosen.Candidate.Label,
		)
	} else if sel.Scores[best].HasTrackRecord() {
		sel.Reason = fmt.Sprintf("%s tiene la mejor combinación de similitud (%.0f%%) e historial real (%.0f%%).",
			sel.Chosen.Label, sel.Chosen.Similarity*100, sel.Scores[best].TrackRecord*100)
	} else {
		sel.Reason = fmt.Sprintf("%s es la más parecida (%.0f%%); sin historial de ejecución todavía para ningún candidato.",
			sel.Chosen.Label, sel.Chosen.Similarity*100)
	}

	return sel
}

// SelectAll arma una Selection por cada Hypothesis del set. hs puede
// ser nil — devuelve nil, mismo criterio defensivo de todo Fase 3.
func (s *Selector) SelectAll(hs *HypothesisSet) []*Selection {
	if hs == nil {
		return nil
	}
	out := make([]*Selection, 0, len(hs.Hypotheses))
	for _, h := range hs.Hypotheses {
		out = append(out, s.Select(h))
	}
	return out
}

// DescribeSelections arma la narrativa conjunta — solo de las
// Selection con algo elegido; omite los gaps sin candidatos (mismo
// criterio de no llenar la respuesta de líneas vacías que ya usaba
// DescribeAll en Bloque 38).
func DescribeSelections(selections []*Selection) string {
	var useful []string
	for _, sel := range selections {
		if !sel.HasChoice() {
			continue
		}
		useful = append(useful, fmt.Sprintf("%s (%s): %s", sel.TaskID, sel.TaskDesc, sel.Reason))
	}
	if len(useful) == 0 {
		return "Ningún gap tenía candidatos entre los que elegir todavía."
	}
	return strings.Join(useful, " ")
}

// ── Trigger de consulta ──────────────────────────────────────────────

var strategyTriggers = []string{
	"que estrategia elegirias",
	"con cual te quedarias",
	"cual conviene mas de verdad",
}

// ParseStrategyQuery detecta si el usuario pregunta por la decisión
// final entre candidatos — distinta de ParseComparisonQuery (Bloque
// 38, que solo describe qué tan cerca estuvieron, sin decidir).
func ParseStrategyQuery(req interaction.SemanticRequest) bool {
	return interaction.HasSemanticQuery(req, interaction.QuerySolvingStrategy)
}
