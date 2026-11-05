package solving

import (
	"ahmelia/interaction"
	"fmt"
	"strings"

	memoryintelligence "ahmelia/memory_intelligence"
)

// ============================================================================
// LEARNING_INTEGRATION.GO — Bloque 45, Fase 3 Profundizada (Nivel III — Executive Reasoning)
// ============================================================================
//
// Tercer "integrar, no crear" seguido dentro de Nivel III (después de
// Bloque 43 con DependencyGraph y Bloque 44 con
// FailurePatternsByAction): memory_intelligence.LearningState ya tenía
// SuggestStrategyChange() (Bloque 7/8) — decide si conviene usar la
// estrategia primaria, secundaria o de fallback para una Action, según
// ActionConfidence. Pero solo se llamaba desde core/engine.go, en el
// momento de EJECUTAR de verdad (AdaptStrategy, dentro de execute()) —
// nunca desde la planificación de Fase 3. Cuando Strategy Selector
// (Bloque 39) elige una Action para un gap, el plan sabe QUÉ Action
// conviene, pero no CÓMO intentarla — Learning Integration cierra esa
// brecha, sin duplicar la lógica de decisión: llama a
// SuggestStrategyChange() tal cual está, no reimplementa el criterio
// de umbrales (<20%, <40%) en otro lado.

// StrategyRecommendation es la estrategia de ejecución recomendada
// para UNA Action, según lo que Ahmelia ya aprendió.
type StrategyRecommendation struct {
	Action     string
	Label      string
	Strategy   string  // "" si no hay dato; si no, "directo"/"con reintentos"/"alternativa"/"desconocida"
	Confidence float64 // 0-100, tal cual ActionConfidence
	IsPrimary  bool    // true si Strategy coincide con la estrategia primaria — nada cambió por aprendizaje
}

// HasRecommendation es false si no hay ningún dato de estrategia para
// esta Action (sin LearningState, o Action fuera de
// LearningState.Strategies).
func (sr *StrategyRecommendation) HasRecommendation() bool {
	return sr.Strategy != "" && sr.Strategy != "desconocida"
}

// Describe arma la narrativa de una recomendación puntual.
func (sr *StrategyRecommendation) Describe() string {
	if !sr.HasRecommendation() {
		return fmt.Sprintf("%s: sin estrategias definidas todavía.", sr.Label)
	}
	if sr.IsPrimary {
		return fmt.Sprintf("%s: uso la estrategia habitual (confianza %.0f%%).", sr.Label, sr.Confidence)
	}
	return fmt.Sprintf("%s: cambio a la estrategia \"%s\" en vez de la habitual — la confianza actual es de solo %.0f%%.",
		sr.Label, sr.Strategy, sr.Confidence)
}

// LearningIntegrator es el Learning Integration en sí. Sostiene una
// referencia a LearningState, mismo criterio de dependencia real que
// Knowledge Mapper (23), Strategy Selector (39) y Failure Analysis (44).
type LearningIntegrator struct {
	Learning *memoryintelligence.LearningStore
}

// NewLearningIntegrator arma el Learning Integrator. learning puede
// ser nil — sin él, ninguna Action tiene recomendación.
func NewLearningIntegrator(learning *memoryintelligence.LearningStore) *LearningIntegrator {
	return &LearningIntegrator{Learning: learning}
}

// RecommendFor arma la recomendación de estrategia para una Action
// puntual, reusando SuggestStrategyChange() tal cual — no reimplementa
// el criterio de umbrales acá.
func (li *LearningIntegrator) RecommendFor(action, label string) *StrategyRecommendation {
	rec := &StrategyRecommendation{Action: action, Label: label}
	if li.Learning == nil {
		return rec
	}

	strategy, conf := li.Learning.SuggestStrategyChange(action)
	rec.Strategy = strategy
	rec.Confidence = conf

	if s, ok := li.Learning.Strategy(action); ok {
		rec.IsPrimary = strategy == s.Primary
	}
	return rec
}

// RecommendForSelection arma la recomendación para la Action que
// Strategy Selector (Bloque 39) eligió de verdad para un gap — mismo
// criterio que AnalyzeSelection en Failure Analysis (Bloque 44): se
// mira lo que ganó, no ClosestAction a secas.
func (li *LearningIntegrator) RecommendForSelection(sel *Selection) *StrategyRecommendation {
	if sel == nil || !sel.HasChoice() {
		return nil
	}
	return li.RecommendFor(sel.Chosen.Action, sel.Chosen.Label)
}

// RecommendAll arma una StrategyRecommendation por cada Selection con
// elección real.
func (li *LearningIntegrator) RecommendAll(selections []*Selection) []*StrategyRecommendation {
	var out []*StrategyRecommendation
	for _, sel := range selections {
		if rec := li.RecommendForSelection(sel); rec != nil {
			out = append(out, rec)
		}
	}
	return out
}

// DescribeRecommendations arma la narrativa conjunta — omitiendo las
// Action donde la recomendación es la estrategia primaria (nada
// cambió por aprendizaje, no aporta nada nuevo decirlo) y las que no
// tienen ningún dato. Solo aparecen las que el aprendizaje realmente
// modificó, mismo criterio de "no llenar de líneas sin valor" que
// DescribeAll (38), DescribeSelections (39) y DescribeRisks (44).
func DescribeRecommendations(recs []*StrategyRecommendation) string {
	var useful []string
	for _, r := range recs {
		if r.HasRecommendation() && !r.IsPrimary {
			useful = append(useful, r.Describe())
		}
	}
	if len(useful) == 0 {
		return "Para todas las Actions de este plan, uso la estrategia habitual — el aprendizaje no sugiere ningún cambio."
	}
	return strings.Join(useful, " ")
}

// ── Trigger de consulta ──────────────────────────────────────────────

var learningIntegrationTriggers = []string{
	"con que estrategia lo intentarias",
	"deberia cambiar de estrategia para esto",
	"que tan aprendido esta esto",
}

// ParseLearningIntegrationQuery detecta si se pregunta por CÓMO
// intentar ejecutar algo (estrategia de ejecución) — distinta de
// ParseStrategyQuery (Bloque 39, que pregunta CUÁL Action conviene
// entre varios candidatos, no cómo intentar la ya elegida).
func ParseLearningIntegrationQuery(req interaction.SemanticRequest) bool {
	return interaction.HasSemanticQuery(req, interaction.QuerySolvingLearningIntegration)
}
