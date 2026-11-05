package solving

import (
	"ahmelia/interaction"
	"fmt"
	"sort"
	"strings"

	memoryintelligence "ahmelia/memory_intelligence"
)

// ============================================================================
// FAILURE_ANALYSIS.GO — Bloque 44, Fase 3 Profundizada (Nivel III — Executive Reasoning)
// ============================================================================
//
// Otro caso de "integrar, no crear": memory_intelligence.LearningState
// ya tenía FailurePatternsByAction (Bloque 7/8) — cuántas veces cada
// Action falló y POR QUÉ CATEGORÍA (program_not_found, file_not_found,
// etc., ver memory_intelligence/eventlog.go) — pero nadie lo consumía
// fuera del propio LearningState. Strategy Selector (Bloque 39) ya usa
// ActionConfidence de la misma struct para el track record; Failure
// Analysis es el primero que mira el POR QUÉ, no solo el CUÁNTO.
//
// La diferencia con Strategy Selector: ActionConfidence responde "¿qué
// tan bien le fue?" (un número). FailurePatternsByAction responde "¿qué
// salió mal, específicamente, las veces que no le fue bien?" — dos
// preguntas distintas sobre el mismo historial, no una duplicando a la
// otra.

// categoryLabels traduce las categorías fijas de
// memory_intelligence/eventlog.go a español legible — mismo criterio
// que shared.ActionLabels para las Action.
var categoryLabels = map[string]string{
	memoryintelligence.CategoryProgramNotFound:     "el programa no se encontró",
	memoryintelligence.CategoryVolumeError:         "hubo un error de volumen",
	memoryintelligence.CategoryWindowNotFound:      "la ventana no se encontró",
	memoryintelligence.CategoryFileNotFound:        "el archivo no se encontró",
	memoryintelligence.CategoryClarificationNeeded: "hizo falta aclarar algo (no es un fallo real)",
}

// FailureRisk es el perfil de fallos históricos de UNA Action.
type FailureRisk struct {
	Action             string
	Label              string
	TotalFailures      int
	MostCommonCategory string
	MostCommonCount    int
	Categories         map[string]int // todas las categorías con su conteo, para quien quiera el detalle completo
}

// HasHistory es false si esta Action nunca falló (o nunca se ejecutó)
// — distinto de "falló poco": sin ningún registro, no hay riesgo que
// reportar, no un riesgo bajo inventado.
func (fr *FailureRisk) HasHistory() bool {
	return fr.TotalFailures > 0
}

// Describe arma la narrativa de un perfil de riesgo.
func (fr *FailureRisk) Describe() string {
	if !fr.HasHistory() {
		return fmt.Sprintf("%s no tiene ningún fallo registrado.", fr.Label)
	}
	catLabel, ok := categoryLabels[fr.MostCommonCategory]
	if !ok {
		catLabel = fr.MostCommonCategory
	}
	return fmt.Sprintf("%s falló %d vez(veces) antes — lo más común: %s (%d de %d veces).",
		fr.Label, fr.TotalFailures, catLabel, fr.MostCommonCount, fr.TotalFailures)
}

// FailureAnalyzer es el Failure Analysis en sí. Sostiene una
// referencia a LearningState (no una copia) — mismo criterio de
// dependencia real que Knowledge Mapper (Bloque 23) y Strategy
// Selector (Bloque 39).
type FailureAnalyzer struct {
	Learning *memoryintelligence.LearningStore
}

// NewFailureAnalyzer arma el Failure Analyzer. learning puede ser nil
// — sin él, ninguna Action tiene historial que reportar.
func NewFailureAnalyzer(learning *memoryintelligence.LearningStore) *FailureAnalyzer {
	return &FailureAnalyzer{Learning: learning}
}

// AnalyzeAction arma el perfil de riesgo de UNA Action puntual, por su
// nombre técnico (ej. "abrir_programa"). label es el texto en español
// a mostrar — se pasa desde afuera en vez de buscarlo en
// shared.ActionLabels acá, porque quien llama (AnalyzeSelection) ya lo
// tiene a mano en el Candidate.
func (fa *FailureAnalyzer) AnalyzeAction(action, label string) *FailureRisk {
	risk := &FailureRisk{Action: action, Label: label}
	if fa.Learning == nil {
		return risk
	}

	cats := fa.Learning.FailureCategories(action)
	if len(cats) == 0 {
		return risk
	}

	risk.Categories = cats
	// Orden alfabético de claves antes de buscar el máximo, para que un
	// empate entre dos categorías siempre resuelva igual entre
	// corridas — mismo criterio de determinismo que rankActions
	// (Bloque 36).
	keys := make([]string, 0, len(cats))
	for k := range cats {
		keys = append(keys, k)
		risk.TotalFailures += cats[k]
	}
	sort.Strings(keys)

	for _, k := range keys {
		if cats[k] > risk.MostCommonCount {
			risk.MostCommonCount = cats[k]
			risk.MostCommonCategory = k
		}
	}

	return risk
}

// AnalyzeSelection arma el perfil de riesgo de la Action que Strategy
// Selector (Bloque 39) efectivamente eligió para un gap — no la de
// mayor Similarity a secas, la que ganó de verdad (track record
// incluido). sel nil, o sin elección, -> nil.
func (fa *FailureAnalyzer) AnalyzeSelection(sel *Selection) *FailureRisk {
	if sel == nil || !sel.HasChoice() {
		return nil
	}
	return fa.AnalyzeAction(sel.Chosen.Action, sel.Chosen.Label)
}

// AnalyzeAllSelections arma un FailureRisk por cada Selection con
// elección real. selections puede venir vacío o con entradas sin
// elección — se omiten, no se inventa nada.
func (fa *FailureAnalyzer) AnalyzeAllSelections(selections []*Selection) []*FailureRisk {
	var out []*FailureRisk
	for _, sel := range selections {
		if risk := fa.AnalyzeSelection(sel); risk != nil {
			out = append(out, risk)
		}
	}
	return out
}

// DescribeRisks arma la narrativa conjunta, omitiendo las Action sin
// ningún historial de fallo (mismo criterio de "no llenar la respuesta
// de líneas vacías" que DescribeAll en Bloque 38).
func DescribeRisks(risks []*FailureRisk) string {
	var useful []string
	for _, r := range risks {
		if r.HasHistory() {
			useful = append(useful, r.Describe())
		}
	}
	if len(useful) == 0 {
		return "Ninguna de las Actions consideradas tiene fallos registrados en el historial."
	}
	return strings.Join(useful, " ")
}

// ── Trigger de consulta ──────────────────────────────────────────────

var failureTriggers = []string{
	"que riesgo de fallo tiene esto",
	"que suele salir mal con esto",
	"hay antecedentes de fallo",
	"esto fallo antes",
}

// ParseFailureQuery detecta si se pregunta por el historial de fallos
// — distinta de ParseStrategyQuery (Bloque 39, que pregunta CUÁL
// Action conviene) y de ParseCausalExplainQuery (Bloque 43, que
// pregunta por qué algo está bloqueado en el PLAN, no por qué falló
// una Action en el PASADO).
func ParseFailureQuery(req interaction.SemanticRequest) bool {
	return interaction.HasSemanticQuery(req, interaction.QuerySolvingFailure)
}
