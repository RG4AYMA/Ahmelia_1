package reasoning

import (
	"sort"
	"strings"

	"ahmelia/selfknowledge"
)

// ActionPattern representa un patrón detectado en el historial
type ActionPattern struct {
	Action      string
	Count       int
	SuccessRate float64 // 0.0 - 100.0
	AvgDuration int     // ms
	IsReliable  bool    // true si success > 80%
}

// FailurePattern representa un patrón de fallo recurrente
type FailurePattern struct {
	Action  string
	Count   int
	Inputs  []string // qué inputs causaron el fallo
	Reason  string   // por qué falló
}

// Analyzer analiza el historial de acciones y extrae patrones
type Analyzer struct {
	history *selfknowledge.ActionHistory
}

// NewAnalyzer crea un analizador nuevo
func NewAnalyzer(history *selfknowledge.ActionHistory) *Analyzer {
	return &Analyzer{history: history}
}

/* =======================
   ANÁLISIS DE PATRONES DE ÉXITO
======================= */

func (a *Analyzer) AnalyzeSuccessPatterns() []ActionPattern {
	actions := a.history.GetLast(50)
	counts := map[string]int{}
	successes := map[string]int{}
	durations := map[string]int{}

	for _, act := range actions {
		counts[act.Action]++
		if act.Success {
			successes[act.Action]++
		}
		durations[act.Action] += act.Duration
	}

	patterns := []ActionPattern{}
	for action, count := range counts {
		rate := float64(successes[action]) / float64(count) * 100
		avg := 0
		if count > 0 {
			avg = durations[action] / count
		}
		patterns = append(patterns, ActionPattern{
			Action:      action,
			Count:       count,
			SuccessRate: rate,
			AvgDuration: avg,
			IsReliable:  rate >= 80,
		})
	}

	// Ordenar por tasa de éxito descendente
	sort.Slice(patterns, func(i, j int) bool {
		return patterns[i].SuccessRate > patterns[j].SuccessRate
	})

	return patterns
}

/* =======================
   ANÁLISIS DE PATRONES DE FALLO
======================= */

func (a *Analyzer) AnalyzeFailurePatterns() []FailurePattern {
	actions := a.history.GetLast(50)
	failCounts := map[string]int{}
	failInputs := map[string][]string{}

	for _, act := range actions {
		if !act.Success {
			failCounts[act.Action]++
			failInputs[act.Action] = append(failInputs[act.Action], act.Input)
		}
	}

	patterns := []FailurePattern{}
	for action, count := range failCounts {
		patterns = append(patterns, FailurePattern{
			Action: action,
			Count:  count,
			Inputs: failInputs[action],
			Reason: inferFailureReason(action),
		})
	}

	// Ordenar por frecuencia de fallo descendente
	sort.Slice(patterns, func(i, j int) bool {
		return patterns[i].Count > patterns[j].Count
	})

	return patterns
}

/* =======================
   TENDENCIA GENERAL
   Compara las últimas 5 acciones vs las 5 anteriores
======================= */

func (a *Analyzer) DetectTrend() string {
	recent := a.history.GetLast(10)
	if len(recent) < 6 {
		return "estable" // datos insuficientes
	}

	// Dividir en dos mitades
	mid := len(recent) / 2
	older := recent[:mid]
	newer := recent[mid:]

	rateOlder := successRate(older)
	rateNewer := successRate(newer)

	switch {
	case rateNewer > rateOlder+15:
		return "mejorando"
	case rateNewer < rateOlder-15:
		return "empeorando"
	default:
		return "estable"
	}
}

/* =======================
   FRECUENCIA DE USO
======================= */

func (a *Analyzer) AnalyzeFrequency() map[string]int {
	actions := a.history.GetLast(50)
	freq := map[string]int{}
	for _, act := range actions {
		freq[act.Action]++
	}
	return freq
}

/* =======================
   RENDIMIENTO POR ACCIÓN
======================= */

func (a *Analyzer) AnalyzePerformance() map[string]interface{} {
	actions := a.history.GetLast(50)
	if len(actions) == 0 {
		return map[string]interface{}{
			"total_actions":   0,
			"slowest_action":  "",
			"slowest_time_ms": 0,
			"avg_duration_ms": 0,
		}
	}

	totalDuration := 0
	byAction := map[string]int{}
	byActionCount := map[string]int{}

	for _, act := range actions {
		totalDuration += act.Duration
		byAction[act.Action] += act.Duration
		byActionCount[act.Action]++
	}

	// Encontrar la más lenta
	slowestAction := ""
	slowestAvg := 0
	for action, total := range byAction {
		avg := total / byActionCount[action]
		if avg > slowestAvg {
			slowestAvg = avg
			slowestAction = action
		}
	}

	return map[string]interface{}{
		"total_actions":   len(actions),
		"slowest_action":  slowestAction,
		"slowest_time_ms": slowestAvg,
		"avg_duration_ms": totalDuration / len(actions),
	}
}

/* =======================
   DETECTAR COMANDOS AMBIGUOS
   Inputs que produjeron acciones diferentes en distintos momentos
======================= */

func (a *Analyzer) DetectAmbiguous() []string {
	actions := a.history.GetLast(50)
	inputActions := map[string]map[string]bool{}

	for _, act := range actions {
		key := strings.ToLower(strings.TrimSpace(act.Input))
		if _, ok := inputActions[key]; !ok {
			inputActions[key] = map[string]bool{}
		}
		inputActions[key][act.Action] = true
	}

	ambiguous := []string{}
	for input, actionSet := range inputActions {
		if len(actionSet) > 1 {
			ambiguous = append(ambiguous, input)
		}
	}
	return ambiguous
}

/* =======================
   HELPERS
======================= */

func successRate(actions []selfknowledge.Action) float64 {
	if len(actions) == 0 {
		return 0
	}
	ok := 0
	for _, a := range actions {
		if a.Success {
			ok++
		}
	}
	return float64(ok) / float64(len(actions)) * 100
}

func inferFailureReason(action string) string {
	reasons := map[string]string{
		"abrir_programa":  "Programa no encontrado en el índice",
		"cambiar_ventana": "Proceso no visible o sin foco",
		"buscar_archivo":  "Archivo no encontrado en el sistema",
		"volumen_subir":   "Error al comunicar con el sistema de audio",
		"volumen_bajar":   "Error al comunicar con el sistema de audio",
	}
	if r, ok := reasons[action]; ok {
		return r
	}
	return "Causa desconocida"
}
