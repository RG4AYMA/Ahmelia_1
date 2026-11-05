package solving

import (
	"ahmelia/interaction"
	"fmt"
	"strings"
)

// ============================================================================
// COUNTERFACTUAL_EVALUATION.GO — Bloque 42, Fase 3 Profundizada (Nivel II)
// ============================================================================
//
// Reflection Loop (Bloque 41) ya filtra: solo llegan acá los gaps con
// un close call real (Comparator, 38) o un override real (Strategy
// Selector, 39) — "nada real que reflexionar" ya quedó descartado
// antes. Counterfactual Evaluation responde la pregunta que Reflection
// deja abierta: "¿qué habría pasado si...?" — no con una alternativa
// cualquiera al azar, sino con dos escenarios concretos que Strategy
// Selector (39) ya sabe calcular pero nunca mostró por separado: "si
// SOLO hubiera importado el parecido de palabras" y "si SOLO hubiera
// importado el historial real".
//
// similarityWeight/trackRecordWeight (Bloque 39, 0.5/0.5) son el peso
// que Strategy Selector usa hoy para el resultado COMBINADO. Este
// bloque no inventa una tercera fórmula de scoring — reusa
// exactamente los mismos dos números que ya trae cada CandidateScore
// (Similarity, TrackRecord), llevados a sus extremos (mirar solo uno,
// mirar solo el otro), para ver si el ganador cambia en cada extremo.
// "Integrar, no crear".

// Counterfactual es el resultado de evaluar los dos escenarios
// extremos para UN gap concreto.
type Counterfactual struct {
	TaskID   string
	TaskDesc string
	Chosen   Candidate // lo que efectivamente se eligió (score combinado, Bloque 39)

	WinnerBySimilarityOnly  Candidate
	ChangedBySimilarityOnly bool // true si, mirando solo Similarity, el ganador es distinto del elegido

	HasTrackRecordData       bool // false si ningún candidato tenía historial — ese escenario no se puede evaluar, no se inventa
	WinnerByTrackRecordOnly  Candidate
	ChangedByTrackRecordOnly bool

	Narrative string
}

// CounterfactualEvaluator es el módulo en sí. Sin estado propio, mismo
// criterio que el resto de Fase 3.
type CounterfactualEvaluator struct{}

// NewCounterfactualEvaluator arma el evaluador.
func NewCounterfactualEvaluator() *CounterfactualEvaluator {
	return &CounterfactualEvaluator{}
}

// Evaluate arma el Counterfactual de UNA Selection (Bloque 39). nil si
// no hay al menos dos candidatos reales evaluados — sin eso no hay
// ningún "qué habría pasado si elegía otra cosa" que tenga sentido.
func (ce *CounterfactualEvaluator) Evaluate(sel *Selection) *Counterfactual {
	if sel == nil || !sel.HasChoice() || len(sel.Scores) < 2 {
		return nil
	}

	cf := &Counterfactual{TaskID: sel.TaskID, TaskDesc: sel.TaskDesc, Chosen: sel.Chosen}

	bestSimIdx := 0
	for i, sc := range sel.Scores {
		if sc.Candidate.Similarity > sel.Scores[bestSimIdx].Candidate.Similarity {
			bestSimIdx = i
		}
	}
	cf.WinnerBySimilarityOnly = sel.Scores[bestSimIdx].Candidate
	cf.ChangedBySimilarityOnly = cf.WinnerBySimilarityOnly.Action != sel.Chosen.Action

	bestTrIdx := -1
	for i, sc := range sel.Scores {
		if !sc.HasTrackRecord() {
			continue
		}
		if bestTrIdx == -1 || sc.TrackRecord > sel.Scores[bestTrIdx].TrackRecord {
			bestTrIdx = i
		}
	}
	if bestTrIdx != -1 {
		cf.HasTrackRecordData = true
		cf.WinnerByTrackRecordOnly = sel.Scores[bestTrIdx].Candidate
		cf.ChangedByTrackRecordOnly = cf.WinnerByTrackRecordOnly.Action != sel.Chosen.Action
	}

	cf.Narrative = describeCounterfactual(cf)
	return cf
}

// describeCounterfactual arma la narrativa de los dos escenarios.
func describeCounterfactual(cf *Counterfactual) string {
	var parts []string

	if cf.ChangedBySimilarityOnly {
		parts = append(parts, fmt.Sprintf(
			"si solo hubiera importado el parecido de palabras, habría elegido %s en vez de %s",
			cf.WinnerBySimilarityOnly.Label, cf.Chosen.Label,
		))
	} else {
		parts = append(parts, fmt.Sprintf("mirando solo el parecido de palabras, la elección no cambia: sigue siendo %s", cf.Chosen.Label))
	}

	switch {
	case !cf.HasTrackRecordData:
		parts = append(parts, "sin datos de historial para ningún candidato, no se puede evaluar el escenario de \"solo historial\"")
	case cf.ChangedByTrackRecordOnly:
		parts = append(parts, fmt.Sprintf(
			"si solo hubiera importado el historial real, habría elegido %s en vez de %s",
			cf.WinnerByTrackRecordOnly.Label, cf.Chosen.Label,
		))
	default:
		parts = append(parts, fmt.Sprintf("mirando solo el historial real, la elección tampoco cambia: sigue siendo %s", cf.Chosen.Label))
	}

	return strings.Join(parts, "; ") + "."
}

// EvaluateReflections arma un Counterfactual por cada Reflection
// (Bloque 41) que tenga una Selection correspondiente con material
// suficiente — reusa el filtro que Reflection Loop YA aplicó (nada de
// interesante → ninguna Reflection → nada que evaluar acá) en vez de
// repetir el criterio de "¿vale la pena mirar este gap?" con otra
// lógica. reflections/selections emparejados por TaskID, mismo criterio
// que Reflect() en Bloque 41.
func (ce *CounterfactualEvaluator) EvaluateReflections(reflections []Reflection, selections []*Selection) []*Counterfactual {
	selByTask := make(map[string]*Selection, len(selections))
	for _, sel := range selections {
		if sel != nil {
			selByTask[sel.TaskID] = sel
		}
	}

	var out []*Counterfactual
	for _, ref := range reflections {
		sel, ok := selByTask[ref.TaskID]
		if !ok {
			continue
		}
		if cf := ce.Evaluate(sel); cf != nil {
			out = append(out, cf)
		}
	}
	return out
}

// DescribeCounterfactuals arma la narrativa conjunta de varios
// Counterfactual — mismo criterio que DescribeAll (38) y
// DescribeSelections (39): un mensaje honesto en vez de una lista
// vacía cuando no hay nada que evaluar.
func DescribeCounterfactuals(cfs []*Counterfactual) string {
	if len(cfs) == 0 {
		return "No hay ninguna elección con material suficiente para evaluar un escenario contrafáctico todavía."
	}
	parts := make([]string, len(cfs))
	for i, cf := range cfs {
		parts[i] = fmt.Sprintf("%s (%s): %s", cf.TaskID, cf.TaskDesc, cf.Narrative)
	}
	return strings.Join(parts, " ")
}

// ── Trigger de consulta ──────────────────────────────────────────────

var counterfactualTriggers = []string{
	"que hubiera pasado si elegias otra cosa",
	"que pasaria si eligieras diferente",
	"y si hubieras elegido la otra opcion",
}

// ParseCounterfactualQuery detecta si el usuario pregunta por un
// escenario alternativo — distinta de ParseReflectionQuery (Bloque 41,
// que pregunta si la elección fue sólida, sin comparar escenarios
// explícitos).
func ParseCounterfactualQuery(req interaction.SemanticRequest) bool {
	return interaction.HasSemanticQuery(req, interaction.QuerySolvingCounterfactual)
}
