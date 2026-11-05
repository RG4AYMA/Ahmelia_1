package solving

import (
	"ahmelia/interaction"
	"fmt"
)

// ============================================================================
// SELF_EVALUATION.GO — Bloque 29, Fase 3 (Cognitive Mind), Capa 3.9 — ÚLTIMA CAPA
// ============================================================================
//
// Las ocho capas anteriores (3.1-3.8), entre todas, producen un plan:
// qué hay que hacer, cuánto ya se sabe, en qué orden, por dónde probar
// lo nuevo, qué está en riesgo, si conviene avanzar. Self Evaluation no
// agrega una novena opinión sobre EL PLAN — mira el PROCESO que lo
// produjo: ¿la cadena de razonamiento funcionó bien para este pedido
// puntual, o hubo capas que se quedaron sin nada con qué trabajar?
// Metacognición sobre el razonamiento, no sobre el resultado — mismo
// espíritu que cognition.Pipeline (Bloque 11), pero mirando las 8
// capas de Fase 3 en vez de las 8 etapas de Process().
//
// Ninguna capa vale más que otra a propósito: el promedio es simple,
// sin ponderar. Ponderar significaría decidir de antemano que, por
// ejemplo, Decision Engine importa más que Goal Analyzer — una opinión
// que este bloque no tiene forma honesta de sostener todavía.

// LayerEvaluation es el veredicto de UNA capa, con la señal concreta
// que lo explica — nunca un score sin justificar.
type LayerEvaluation struct {
	Layer  string
	Signal string
	Score  float64 // 0..1
}

// SelfEvaluationReport es el resultado de evaluar las ocho capas para
// un pedido puntual.
type SelfEvaluationReport struct {
	Goal         string
	Layers       []LayerEvaluation
	OverallScore float64 // promedio simple de Layers
}

// Weakest devuelve la capa con el score más bajo — la primera
// candidata a mirar si algo salió mal. En caso de empate gana la que
// aparece primero en la cadena: un problema temprano (Goal Analyzer)
// suele arrastrar a las de abajo, así que es más útil señalarlo a él
// primero que a una capa tardía con el mismo score.
func (r *SelfEvaluationReport) Weakest() LayerEvaluation {
	weakest := r.Layers[0]
	for _, l := range r.Layers[1:] {
		if l.Score < weakest.Score {
			weakest = l
		}
	}
	return weakest
}

// Describe arma la vista de conjunto: score general + la capa más
// floja. No lista las ocho una por una — eso ya lo puede pedir cada
// capa por separado con su propia meta-pregunta (Bloques 21-28); esto
// es el resumen que ninguna de ellas da por sí sola.
func (r *SelfEvaluationReport) Describe() string {
	weakest := r.Weakest()
	return fmt.Sprintf(
		"Para \"%s\": el proceso de razonamiento completo (8 capas) tuvo un desempeño de %.0f%%. La capa más floja fue %s (%.0f%%): %s.",
		r.Goal, r.OverallScore*100, weakest.Layer, weakest.Score*100, weakest.Signal,
	)
}

// SelfEvaluator es el Self Evaluation en sí. Se llama SelfEvaluator, no
// Evaluator a secas: reasoning_deep.Evaluator (CAPA 5.5) ya existe en
// otro paquete — mismo motivo por el que Bloque 28 usó
// AnalogicalReasoner en vez de Reasoner. Sin estado propio, mismo
// criterio que el resto de Fase 3.
type SelfEvaluator struct{}

// NewSelfEvaluator arma el Self Evaluator.
func NewSelfEvaluator() *SelfEvaluator {
	return &SelfEvaluator{}
}

// Evaluate junta las salidas de las ocho capas anteriores y arma el
// reporte. Cualquier puntero puede venir nil (por ejemplo, si se llama
// con una cadena parcial) — mismo criterio defensivo que el resto de
// Fase 3: una capa sin datos da Score=0 con un Signal honesto, no
// rompe ni inventa un valor.
func (se *SelfEvaluator) Evaluate(
	ga *GoalAnalysis,
	plan *TaskPlan,
	km *KnowledgeMap,
	dg *DependencyGraph,
	hs *HypothesisSet,
	cr *ConstraintReport,
	dec *Decision,
	as *AnalogySet,
) *SelfEvaluationReport {
	goal := ""
	if ga != nil {
		goal = ga.MainGoal
	}

	layers := []LayerEvaluation{
		evalGoalAnalyzer(ga),
		evalProblemDecomposer(plan),
		evalKnowledgeMapper(km),
		evalDependencySolver(dg),
		evalHypothesisGenerator(hs),
		evalConstraintSolver(cr),
		evalDecisionEngine(dec),
		evalAnalogicalReasoning(as),
	}

	total := 0.0
	for _, l := range layers {
		total += l.Score
	}

	return &SelfEvaluationReport{
		Goal:         goal,
		Layers:       layers,
		OverallScore: total / float64(len(layers)),
	}
}

// ── Evaluación por capa ──────────────────────────────────────────────
// Cada función mira solo lo que su capa produjo — sin inventar señales
// que esa capa no tiene forma de dar.

func evalGoalAnalyzer(ga *GoalAnalysis) LayerEvaluation {
	if ga == nil {
		return LayerEvaluation{"Goal Analyzer", "no se ejecutó esta capa", 0}
	}
	if ga.Domain != "" && ga.Domain != "general" {
		return LayerEvaluation{
			"Goal Analyzer",
			fmt.Sprintf("reconoció el dominio \"%s\" y usó su plantilla específica", ga.Domain),
			1.0,
		}
	}
	return LayerEvaluation{"Goal Analyzer", "no reconoció un dominio específico, usó la plantilla genérica", 0.5}
}

func countTasks(tasks []*Task) int {
	n := len(tasks)
	for _, t := range tasks {
		n += countTasks(t.Subtasks)
	}
	return n
}

func evalProblemDecomposer(plan *TaskPlan) LayerEvaluation {
	if plan == nil || len(plan.Tasks) == 0 {
		return LayerEvaluation{"Problem Decomposer", "no produjo ninguna tarea", 0}
	}
	total := countTasks(plan.Tasks)
	score := 1.0
	if total < 2 {
		score = 0.6 // dividió el pedido en una sola tarea — técnicamente no rompió nada, pero tampoco descompuso mucho
	}
	return LayerEvaluation{
		"Problem Decomposer",
		fmt.Sprintf("dividió el pedido en %d tarea(s), contando subtareas", total),
		score,
	}
}

func evalKnowledgeMapper(km *KnowledgeMap) LayerEvaluation {
	if km == nil || len(km.Matches) == 0 {
		return LayerEvaluation{"Knowledge Mapper", "no tuvo tareas para cruzar contra lo que Ahmelia ya sabe", 0}
	}
	total := len(km.Matches)
	known := km.KnownCount()
	return LayerEvaluation{
		"Knowledge Mapper",
		fmt.Sprintf("%d de %d tareas ya son capacidad conocida", known, total),
		float64(known) / float64(total),
	}
}

func evalDependencySolver(dg *DependencyGraph) LayerEvaluation {
	if dg == nil || len(dg.Order) == 0 {
		return LayerEvaluation{"Dependency Solver", "no armó ningún orden de tareas", 0}
	}
	total := len(dg.Order)
	blocked := 0
	for _, id := range dg.Order {
		if node, ok := dg.Nodes[id]; ok && node.Blocked {
			blocked++
		}
	}
	return LayerEvaluation{
		"Dependency Solver",
		fmt.Sprintf("%d de %d tareas quedaron bloqueadas por un gap anterior", blocked, total),
		float64(total-blocked) / float64(total),
	}
}

func evalHypothesisGenerator(hs *HypothesisSet) LayerEvaluation {
	if hs == nil || len(hs.Hypotheses) == 0 {
		return LayerEvaluation{"Hypothesis Generator", "no había gaps que necesitaran una hipótesis", 1.0}
	}
	total := len(hs.Hypotheses)
	leads := 0
	for _, h := range hs.Hypotheses {
		if h.HasLead() {
			leads++
		}
	}
	return LayerEvaluation{
		"Hypothesis Generator",
		fmt.Sprintf("%d de %d gaps tienen al menos una pista de por dónde empezar", leads, total),
		float64(leads) / float64(total),
	}
}

func evalConstraintSolver(cr *ConstraintReport) LayerEvaluation {
	if cr == nil || len(cr.Checks) == 0 {
		return LayerEvaluation{"Constraint Solver", "no había requisitos detectados para verificar", 1.0}
	}
	total := len(cr.Checks)
	atRisk := cr.RiskCount()
	return LayerEvaluation{
		"Constraint Solver",
		fmt.Sprintf("%d de %d requisitos en riesgo", atRisk, total),
		float64(total-atRisk) / float64(total),
	}
}

func evalDecisionEngine(dec *Decision) LayerEvaluation {
	if dec == nil {
		return LayerEvaluation{"Decision Engine", "no llegó a un veredicto", 0}
	}
	switch dec.Verdict {
	case VerdictProceed:
		return LayerEvaluation{"Decision Engine", "veredicto: avanzar, sin ajustes", 1.0}
	case VerdictAdjust:
		return LayerEvaluation{"Decision Engine", "veredicto: avanzar con ajustes", 0.6}
	default:
		return LayerEvaluation{"Decision Engine", "veredicto: pedir más información antes de empezar", 0.3}
	}
}

func evalAnalogicalReasoning(as *AnalogySet) LayerEvaluation {
	if as == nil || len(as.Analogies) == 0 {
		return LayerEvaluation{"Analogical Reasoning", "no había gaps para buscarles un antecedente real", 1.0}
	}
	total := len(as.Analogies)
	found := len(as.Found())
	return LayerEvaluation{
		"Analogical Reasoning",
		fmt.Sprintf("%d de %d gaps tienen un antecedente real en el historial", found, total),
		float64(found) / float64(total),
	}
}

// ── Trigger de consulta ──────────────────────────────────────────────

var selfEvalTriggers = []string{
	"que tan bien razonaste",
	"como te fue pensando esto",
	"evalua tu propio razonamiento",
	"que tan solido fue tu analisis",
	"como te fue con ese razonamiento",
}

// ParseSelfEvalQuery detecta si el usuario pregunta por el PROCESO de
// razonamiento en sí — distinta de las ocho capas anteriores, que
// preguntan por un aspecto del PLAN (qué es, cuánto se sabe, si
// conviene avanzar). Esta es la única metapregunta sobre las otras
// ocho, no sobre el pedido original.
func ParseSelfEvalQuery(req interaction.SemanticRequest) bool {
	return interaction.HasSemanticQuery(req, interaction.QuerySolvingSelfEval)
}
