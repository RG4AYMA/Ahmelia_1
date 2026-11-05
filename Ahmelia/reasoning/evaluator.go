package reasoning

import (
	"fmt"
)

// Problem representa un problema detectado
type Problem struct {
	ID          string  // "high_failure_rate", "ambiguous_command", etc
	Severity    int     // 1-5 (5 = crítico)
	Category    string  // "reliability", "performance", "ambiguity", "data_scarcity"
	Description string
	Evidence    string // Por qué lo cree
	Confidence  float64
}

// EvaluationContext es lo que el Evaluator ve
type EvaluationContext struct {
	ActionCount      int
	SuccessRate      float64
	FailurePatterns  []FailurePattern
	SuccessPatterns  []ActionPattern
	AmbiguousInputs  []string
	Trend            string
	SlowestAction    string
	SlowActionsCount int
}

// Evaluator detecta problemas en el historial
type Evaluator struct {
	analyzer *Analyzer
}

// NewEvaluator crea un evaluador
func NewEvaluator(analyzer *Analyzer) *Evaluator {
	return &Evaluator{analyzer: analyzer}
}

/* =======================
   DETECTAR TODOS LOS PROBLEMAS
======================= */

func (e *Evaluator) DetectAllProblems() []Problem {
	problems := []Problem{}

	// Reunir contexto
	ctx := e.buildContext()

	// Detectar cada tipo de problema
	problems = append(problems, e.detectFailureProblems(ctx)...)
	problems = append(problems, e.detectAmbiguityProblems(ctx)...)
	problems = append(problems, e.detectPerformanceProblems(ctx)...)
	problems = append(problems, e.detectDataScarcityProblems(ctx)...)
	problems = append(problems, e.detectTrendProblems(ctx)...)

	// Ordenar por severidad (descendente)
	for i := 0; i < len(problems)-1; i++ {
		for j := i + 1; j < len(problems); j++ {
			if problems[i].Severity < problems[j].Severity {
				problems[i], problems[j] = problems[j], problems[i]
			}
		}
	}

	return problems
}

/* =======================
   PROBLEMAS: TASA DE FALLO ALTA
======================= */

func (e *Evaluator) detectFailureProblems(ctx EvaluationContext) []Problem {
	problems := []Problem{}

	for _, fail := range ctx.FailurePatterns {
		if fail.Count >= 2 {
			severity := 3
			if fail.Count >= 5 {
				severity = 5
			} else if fail.Count >= 3 {
				severity = 4
			}

			failRate := float64(fail.Count) / float64(ctx.ActionCount) * 100

			problems = append(problems, Problem{
				ID:          fmt.Sprintf("failure_%s", fail.Action),
				Severity:    severity,
				Category:    "reliability",
				Description: fmt.Sprintf("'%s' falló %d veces", fail.Action, fail.Count),
				Evidence:    fmt.Sprintf("Tasa de fallo: %.1f%%. Causa probable: %s", failRate, fail.Reason),
				Confidence:  0.85,
			})
		}
	}

	return problems
}

/* =======================
   PROBLEMAS: COMANDOS AMBIGUOS
======================= */

func (e *Evaluator) detectAmbiguityProblems(ctx EvaluationContext) []Problem {
	problems := []Problem{}

	for _, input := range ctx.AmbiguousInputs {
		problems = append(problems, Problem{
			ID:          fmt.Sprintf("ambiguity_%s", input),
			Severity:    3,
			Category:    "ambiguity",
			Description: fmt.Sprintf("Comando ambiguo: '%s'", input),
			Evidence:    "Este input produjo acciones diferentes en distintos momentos",
			Confidence:  0.8,
		})
	}

	return problems
}

/* =======================
   PROBLEMAS: RENDIMIENTO LENTO
======================= */

func (e *Evaluator) detectPerformanceProblems(ctx EvaluationContext) []Problem {
	problems := []Problem{}

	if ctx.SlowActionsCount > 0 && ctx.SlowestAction != "" {
		problems = append(problems, Problem{
			ID:          fmt.Sprintf("slow_%s", ctx.SlowestAction),
			Severity:    2,
			Category:    "performance",
			Description: fmt.Sprintf("'%s' es anormalmente lento", ctx.SlowestAction),
			Evidence:    fmt.Sprintf("%d acciones se ejecutan más lentamente de lo esperado", ctx.SlowActionsCount),
			Confidence:  0.7,
		})
	}

	return problems
}

/* =======================
   PROBLEMAS: DATOS INSUFICIENTES
======================= */

func (e *Evaluator) detectDataScarcityProblems(ctx EvaluationContext) []Problem {
	problems := []Problem{}

	if ctx.ActionCount < 5 {
		problems = append(problems, Problem{
			ID:          "data_scarcity",
			Severity:    2,
			Category:    "data_scarcity",
			Description: "Datos insuficientes para análisis confiable",
			Evidence:    fmt.Sprintf("Solo %d acciones registradas. Necesito al menos 10 para patrones válidos", ctx.ActionCount),
			Confidence:  0.95,
		})
	}

	return problems
}

/* =======================
   PROBLEMAS: TENDENCIA NEGATIVA
======================= */

func (e *Evaluator) detectTrendProblems(ctx EvaluationContext) []Problem {
	problems := []Problem{}

	if ctx.Trend == "empeorando" {
		problems = append(problems, Problem{
			ID:          "negative_trend",
			Severity:    4,
			Category:    "reliability",
			Description: "Rendimiento empeorando",
			Evidence:    "Las últimas acciones fallan más que las anteriores",
			Confidence:  0.75,
		})
	}

	return problems
}

/* =======================
   HELPERS: Construir contexto de evaluación
======================= */

func (e *Evaluator) buildContext() EvaluationContext {
	successPatterns := e.analyzer.AnalyzeSuccessPatterns()
	failurePatterns := e.analyzer.AnalyzeFailurePatterns()
	trend := e.analyzer.DetectTrend()
	perf := e.analyzer.AnalyzePerformance()
	ambiguous := e.analyzer.DetectAmbiguous()

	actions := e.analyzer.history.GetLast(50)
	successCount := 0
	for _, a := range actions {
		if a.Success {
			successCount++
		}
	}

	successRate := 0.0
	if len(actions) > 0 {
		successRate = float64(successCount) / float64(len(actions)) * 100
	}

	slowCount := 0
	slowest := perf["slowest_action"].(string)
	// countSlowActions no se implementó, simplemente asigna 0
	_ = slowest

	return EvaluationContext{
		ActionCount:      len(actions),
		SuccessRate:      successRate,
		FailurePatterns:  failurePatterns,
		SuccessPatterns:  successPatterns,
		AmbiguousInputs:  ambiguous,
		Trend:            trend,
		SlowestAction:    slowest,
		SlowActionsCount: slowCount,
	}
}

func countSlowActions(actions []interface{}, slowest string) int {
	// TODO: Implementar cuando estructura de Action esté clara
	return 0
}
