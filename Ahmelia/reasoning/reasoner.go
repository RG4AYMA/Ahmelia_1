package reasoning

import (
	"fmt"
	"strings"
	"time"

	"ahmelia/selfknowledge"
	"ahmelia/shared"
)

// Decision representa una decisión completa con justificación
type Decision struct {
	Input            string
	ChosenAction     string
	Target           string // qué resolvió el clasificador/engine (puede diferir del Input crudo: pronombres resueltos, artículos, etc.)
	Confidence       float64
	Reason           string
	Alternatives     []string
	Timestamp        time.Time
	InternalThoughts string // Pensamiento sin mostrar
}

// EvalResult es el resultado de autoevaluarse
type EvalResult struct {
	UnderstoodWell bool
	CouldDoBetter  bool
	WhatWasMissing string
	WhatLearned    string
}

// Reasoner es el orquestador de toda la Capa 4
// Ejecuta el pipeline: Observe → Evaluate → Prioritize → Hypothesize →
//
//	Decide → Criticize → Conclude → Explain
type Reasoner struct {
	history      *selfknowledge.ActionHistory
	analyzer     *Analyzer
	evaluator    *Evaluator
	prioritizer  *Prioritizer
	hypothesizer *Hypothesizer
	decider      *Decider
	critic       *Critic
	insights     *InsightGenerator
	explainer    *Explainer
	decisions    []Decision // historial
	// Bloque 14 (Fase 2.6) — reconecta reasoning.Decider, que existía
	// desde antes (se instancia acá abajo, en NewReasoner) pero nunca
	// se llamaba: NewDecider() armaba el Decider y quedaba huérfano,
	// mismo patrón que AgendaManager antes del Bloque 9. lastResponseTree
	// guarda la última decisión de ESTRATEGIA DE RESPUESTA (no de acción)
	// para poder explicarla si preguntan "cómo decidiste responder".
	lastResponseTree *DecisionTree
}

// NewReasoner crea el sistema de razonamiento completo
func NewReasoner(history *selfknowledge.ActionHistory) *Reasoner {
	analyzer := NewAnalyzer(history)
	evaluator := NewEvaluator(analyzer)
	prioritizer := NewPrioritizer(evaluator, analyzer)
	hypothesizer := NewHypothesizer(analyzer)
	decider := NewDecider(analyzer, prioritizer, hypothesizer)
	critic := NewCritic(analyzer)
	insights := NewInsightGenerator(analyzer)
	explainer := NewExplainer(nil, critic) // Pasará a sí mismo después

	reasoner := &Reasoner{
		history:      history,
		analyzer:     analyzer,
		evaluator:    evaluator,
		prioritizer:  prioritizer,
		hypothesizer: hypothesizer,
		decider:      decider,
		critic:       critic,
		insights:     insights,
		explainer:    explainer,
		decisions:    make([]Decision, 0, 20),
	}

	// Ahora pasar el reasoner al explainer (circular)
	explainer.reasoner = reasoner

	return reasoner
}

/* =======================
   REGISTRAR DECISIÓN
   Se llama cuando el engine ejecuta una acción
======================= */

func (r *Reasoner) RecordDecision(input, action, target string, confidence float64, alternatives []string) {
	// Construir pensamiento interno
	thought := r.buildInternalThought(input, action, confidence)

	decision := Decision{
		Input:            input,
		ChosenAction:     action,
		Target:           target,
		Confidence:       confidence,
		Reason:           r.buildReason(action, confidence),
		Alternatives:     alternatives,
		Timestamp:        time.Now(),
		InternalThoughts: thought,
	}

	r.decisions = append(r.decisions, decision)
	if len(r.decisions) > 20 {
		r.decisions = r.decisions[1:]
	}
}

/* =======================
   AUTOEVALUACIÓN POST-EJECUCIÓN
   El engine llama esto después de ejecutar una acción
======================= */

func (r *Reasoner) Evaluate(input, action string, success bool) EvalResult {
	review := r.critic.ReviewAction(action, success, 0.7)

	result := EvalResult{
		UnderstoodWell: success,
		CouldDoBetter:  review.WouldChangeIt,
		WhatWasMissing: review.PostAnalysis,
		WhatLearned:    strings.Join(review.Lessons, " | "),
	}

	return result
}

/* =======================
   PIPELINE COMPLETO: RESPONDER PREGUNTAS
   Esta es la función clave que ejecuta todo el razonamiento
======================= */

func (r *Reasoner) Query(question string) (string, bool) {
	// shared.Normalize (quita acentos + minúsculas) en vez de solo
	// ToLower — mismo motivo que selfknowledge.SelfQuery.Query: un STT
	// que transcribe con acentos mixtos ("qué fallo" con acento solo en
	// la primera palabra) no calzaba con ninguna de las variantes
	// pre-armadas ("qué falló" / "que fallo") y esta fuente, siendo la
	// de MAYOR prioridad (se consulta primero), se saltaba en silencio
	// dejando pasar la pregunta a memoria/self sin razón real.
	q := shared.Normalize(question)

	// Bloque 14 — meta-pregunta sobre CÓMO se decidió la última
	// respuesta (no qué se respondió, cuál fue la estrategia elegida:
	// "responder con confianza total" vs "responder con contexto y
	// limitaciones", etc.). Va ANTES de recalcular nada para esta
	// pregunta nueva, porque lastResponseTree todavía guarda la
	// decisión de LA PREGUNTA ANTERIOR — que es justo lo que se está
	// preguntando.
	if containsAny(q, []string{"como decidiste responder", "que estrategia de respuesta", "por que respondes asi"}) {
		if r.lastResponseTree == nil {
			return "Todavía no respondí ninguna pregunta en esta sesión como para explicar la estrategia", true
		}
		return r.decider.SummarizeDecision(*r.lastResponseTree), true
	}

	// 1. OBSERVAR — ¿Qué datos tengo?
	actionCount := len(r.history.GetLast(50))
	successRate := r.calculateSuccessRate()
	problems := r.prioritizer.PrioritizeProblems()

	// 2. EVALUAR — ¿Hay suficientes datos?
	analysisDepth := r.determineAnalysisDepth(actionCount)

	// Bloque 14 — ahora sí se llama al Decider: antes de construir la
	// respuesta, decide CON QUÉ ESTRATEGIA responder según analysisDepth
	// (admitir que faltan datos / responder con matices / responder con
	// confianza total). No cambia las respuestas ya armadas más abajo —
	// esas ya venían aplicando ese mismo criterio a mano en cada
	// answerX — pero ahora la decisión queda registrada y explicable en
	// vez de implícita en cada función.
	tree := r.decider.DecideResponse(question, analysisDepth)
	r.lastResponseTree = &tree

	// 3. DETERMINAR RESPUESTA
	if containsAny(q, []string{"por que hiciste", "por que elegiste"}) {
		return r.answerWhyDecision(analysisDepth), true
	}

	if containsAny(q, []string{"que analizas", "que piensas", "que detectas"}) {
		return r.answerWhatAnalyze(actionCount, successRate, len(problems), analysisDepth), true
	}

	if containsAny(q, []string{"tendencia", "como vas", "progreso"}) {
		return r.answerTrend(analysisDepth, actionCount), true
	}

	if containsAny(q, []string{"que mejorarias", "propuesta", "mejora"}) {
		return r.answerImprovements(problems, actionCount, analysisDepth), true
	}

	if containsAny(q, []string{"que insight", "que aprendiste"}) {
		return r.answerInsight(analysisDepth), true
	}

	if containsAny(q, []string{"que fallo", "mayor falla", "falla mas"}) {
		return r.answerTopFailure(), true
	}

	return "", false
}

/* =======================
   RESPUESTAS CON PIPELINE
======================= */

func (r *Reasoner) answerWhyDecision(analysisDepth string) string {
	if len(r.decisions) == 0 {
		last := r.history.GetLastAction()
		if last == nil {
			return "Aún no he tomado ninguna decisión en esta sesión"
		}
		return fmt.Sprintf("Ejecuté '%s' porque dijiste '%s'", last.Action, last.Input)
	}

	d := r.decisions[len(r.decisions)-1]

	// Pipeline: Observe → Hypothesize → Review → Explain
	_ = r.hypothesizer.ExplainSuccess(d.ChosenAction)

	// Crítica
	_ = r.critic.ReviewAction(d.ChosenAction, true, d.Confidence)

	// Explicación final
	exp := r.explainer.ExplainDecision(d.ChosenAction, d.Input, d.Target, d.Confidence)

	// Retornar con pensamiento interno
	if analysisDepth != "insufficient" {
		return r.explainer.Format(exp, false)
	}
	return exp.Summary
}

func (r *Reasoner) answerWhatAnalyze(actionCount int, successRate float64, problemCount int, analysisDepth string) string {
	// Observe → Evaluate → Conclude
	exp := r.explainer.ExplainAnalysis(actionCount, successRate, problemCount)
	return r.explainer.Format(exp, false)
}

func (r *Reasoner) answerTrend(analysisDepth string, actionCount int) string {
	// Observe → Detect → Conclude
	trend := r.analyzer.DetectTrend()
	exp := r.explainer.ExplainTrend(trend, actionCount)

	if analysisDepth == "insufficient" {
		return "No tengo suficientes datos para detectar una tendencia"
	}

	return r.explainer.Format(exp, false)
}

func (r *Reasoner) answerImprovements(problems []Priority, actionCount int, analysisDepth string) string {
	// Observe → Evaluate → Prioritize → Conclude
	exp := r.explainer.ExplainImprovements(problems, actionCount)
	return r.explainer.Format(exp, false)
}

func (r *Reasoner) answerInsight(analysisDepth string) string {
	// Observe → Generate → Prioritize → Conclude
	insights := r.insights.GenerateInsights()

	if len(insights) == 0 {
		return "Aún no tengo suficientes datos para generar insights"
	}

	if analysisDepth == "insufficient" {
		return "Necesito más datos para generar insights confiables"
	}

	top := insights[0]
	return fmt.Sprintf("🎯 %s: %s\n💡 Recomendación: %s", top.Title, top.Description, top.Action)
}

func (r *Reasoner) answerTopFailure() string {
	failures := r.analyzer.AnalyzeFailurePatterns()
	if len(failures) == 0 {
		return "No he registrado fallos recurrentes"
	}

	f := failures[0]
	hypotheses := r.hypothesizer.ExplainFailure(f.Action)
	summary := r.hypothesizer.SummarizeHypotheses(hypotheses)

	return fmt.Sprintf("❌ '%s' falló %d veces\n%s", f.Action, f.Count, summary)
}

/* =======================
   HELPERS
======================= */

/* =======================
   BLOQUE 14 — RECONECTAR reasoning.Decider
   Wrappers públicos: el resto del código (core/engine.go) no debería
   tocar r.decider directamente (campo privado, a propósito — mismo
   encapsulamiento que el resto de Reasoner), así que estos métodos son
   la puerta de entrada para lo que Decider sabe hacer y todavía no se
   usaba: DecideTrust y SummarizeDecision.
======================= */

// DecideTrust delega en Decider.DecideTrust si conviene ejecutar una
// acción sin preguntar, monitoreando, o pidiendo confirmación primero,
// según su tasa de éxito histórica. successRatePercent viene en escala
// 0-100 (así es como memory_intelligence.LearningState.ActionConfidence
// ya lo guarda — Bloque 7/8) para que quien llama no tenga que acordarse
// de dividir entre 100; adentro se convierte a la escala 0.0-1.0 que
// espera Decider.
func (r *Reasoner) DecideTrust(action string, successRatePercent float64) DecisionTree {
	return r.decider.DecideTrust(action, successRatePercent/100)
}

// Summarize expone Decider.SummarizeDecision sin exponer el campo
// decider en sí — para que "por qué confías/no confías en X" tenga una
// frase lista, igual que ya la tiene "por qué hiciste eso".
func (r *Reasoner) Summarize(tree DecisionTree) string {
	return r.decider.SummarizeDecision(tree)
}

func (r *Reasoner) calculateSuccessRate() float64 {
	actions := r.history.GetLast(50)
	if len(actions) == 0 {
		return 0
	}

	successCount := 0
	for _, a := range actions {
		if a.Success {
			successCount++
		}
	}
	return float64(successCount) / float64(len(actions))
}

func (r *Reasoner) determineAnalysisDepth(actionCount int) string {
	if actionCount < 5 {
		return "insufficient"
	}
	if actionCount < 10 {
		return "partial"
	}
	return "sufficient"
}

func (r *Reasoner) buildInternalThought(input, action string, confidence float64) string {
	if confidence < 0.5 {
		return fmt.Sprintf("Tengo dudas reales. Confianza solo: %.0f%%", confidence*100)
	}
	if confidence < 0.7 {
		return fmt.Sprintf("Parcialmente seguro (%.0f%%). Varios datos pero no conclusivos", confidence*100)
	}
	return fmt.Sprintf("Bastante seguro (%.0f%%). Patrones coinciden", confidence*100)
}

func (r *Reasoner) buildReason(action string, confidence float64) string {
	patterns := r.analyzer.AnalyzeSuccessPatterns()
	for _, p := range patterns {
		if p.Action == action {
			return fmt.Sprintf(
				"Elegí '%s' porque tiene %.0f%% de éxito histórico",
				action, p.SuccessRate,
			)
		}
	}
	return fmt.Sprintf("Elegí '%s' sin historial previo", action)
}

func containsAny(text string, keywords []string) bool {
	for _, kw := range keywords {
		if strings.Contains(text, kw) {
			return true
		}
	}
	return false
}
