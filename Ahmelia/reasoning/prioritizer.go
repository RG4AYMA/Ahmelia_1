package reasoning

import (
	"sort"
)

// Priority representa algo priorizado
type Priority struct {
	Item       string  // qué es
	Score      float64 // 0.0 - 1.0 (importancia calculada)
	Reason     string  // por qué
	IsCritical bool    // si requiere atención inmediata
}

// Prioritizer compara múltiples factores y decide importancia
type Prioritizer struct {
	evaluator *Evaluator
	analyzer  *Analyzer
}

// NewPrioritizer crea un priorizador
func NewPrioritizer(evaluator *Evaluator, analyzer *Analyzer) *Prioritizer {
	return &Prioritizer{
		evaluator: evaluator,
		analyzer:  analyzer,
	}
}

/* =======================
   PRIORIZAR PROBLEMAS
   "¿Qué debería arreglarse primero?"
======================= */

func (p *Prioritizer) PrioritizeProblems() []Priority {
	problems := p.evaluator.DetectAllProblems()

	priorities := []Priority{}

	for _, prob := range problems {
		// Calcular score basado en severidad + evidencia
		baseScore := float64(prob.Severity) / 5.0 // 0.0 - 1.0
		score := baseScore * prob.Confidence

		priority := Priority{
			Item:       prob.Description,
			Score:      score,
			Reason:     prob.Evidence,
			IsCritical: prob.Severity >= 4,
		}

		priorities = append(priorities, priority)
	}

	// Ordenar por score descendente
	sort.Slice(priorities, func(i, j int) bool {
		return priorities[i].Score > priorities[j].Score
	})

	return priorities
}

/* =======================
   PRIORIZAR ACCIONES PARA MEJORAR
   "¿Cuál acción debería optimizar?"
======================= */

func (p *Prioritizer) PrioritizeActionsToImprove() []Priority {
	failures := p.analyzer.AnalyzeFailurePatterns()
	successes := p.analyzer.AnalyzeSuccessPatterns()

	// Los que fallan mucho deberían ir arriba
	// Los que funcionan bien van abajo
	priorities := []Priority{}

	// Primero: acciones problemáticas
	for _, fail := range failures {
		if fail.Count >= 2 {
			score := float64(fail.Count) / 10.0 // Más fallos = más alto
			if score > 1.0 {
				score = 1.0
			}

			priorities = append(priorities, Priority{
				Item:       fail.Action,
				Score:      score,
				Reason:     fail.Reason,
				IsCritical: fail.Count >= 5,
			})
		}
	}

	// Luego: acciones que funcionan bien (baja prioridad)
	for _, succ := range successes {
		if succ.IsReliable {
			score := (100 - succ.SuccessRate) / 100 // Menos necesario si ya funciona

			priorities = append(priorities, Priority{
				Item:       succ.Action,
				Score:      score,
				Reason:     "Funciona bien, probablemente no necesite cambios",
				IsCritical: false,
			})
		}
	}

	// Ordenar por score descendente
	sort.Slice(priorities, func(i, j int) bool {
		return priorities[i].Score > priorities[j].Score
	})

	return priorities
}

/* =======================
   PRIORIZAR INSIGHTS
   "¿Cuál insight debería saber primero?"
======================= */

func (p *Prioritizer) PrioritizeInsights(insights []Insight) []Insight {
	// Ordenar por prioridad y confianza
	sort.Slice(insights, func(i, j int) bool {
		// Primero por prioridad (descendente)
		if insights[i].Priority != insights[j].Priority {
			return insights[i].Priority > insights[j].Priority
		}
		// Si iguales, por confianza (descendente)
		return insights[i].Confidence > insights[j].Confidence
	})

	return insights
}

/* =======================
   COMPARAR ESTADO
   Muestra qué estamos priorizando vs qué podemos ignorar
======================= */

func (p *Prioritizer) CompareState() map[string][]Priority {
	return map[string][]Priority{
		"critical_problems":  p.PrioritizeProblems()[:minInt(3, p.countCritical(p.PrioritizeProblems()))],
		"actions_to_improve": p.PrioritizeActionsToImprove(),
	}
}

/* =======================
   HELPERS
======================= */

func (p *Prioritizer) countCritical(priorities []Priority) int {
	count := 0
	for _, pr := range priorities {
		if pr.IsCritical {
			count++
		}
	}
	return count
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
