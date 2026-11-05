package memory_intelligence

import (
	"ahmelia/interaction"
	"fmt"
	"sort"
	"strings"
)

// ============================================================================
// HABITS.GO — Fase 4 (Hábitos) de Pattern Learning
// ============================================================================
//
// La pregunta que responde esta fase, a diferencia de Fase 3 (Patrones):
// no "¿qué secuencias ocurren?" sino "¿qué se sostiene en el tiempo?". Un
// patrón detectado 6 veces en una sola tarde es evidencia débil de una
// costumbre — igual de válido sería que fue una sesión de prueba, como la
// que armamos nosotros mismos para validar Fase 3. Un patrón visto en 6
// DÍAS DISTINTOS es otra cosa.
//
// HabitScore = DaysSeen × SuccessRate
//
// DaysSeen es la consistencia temporal: en cuántas fechas calendario
// distintas se vio la cadena — no cuenta repeticiones dentro del mismo día.
// SuccessRate es la tasa de éxito HISTÓRICA de cada paso (calculada sobre
// el EventLog completo, incluyendo los fallos que Fase 1-3 descartan al
// construir corridas) — así una cadena que incluye un paso poco confiable
// (ej. "abrir_programa:blender", si suele fallar en otros intentos) pesa
// menos como candidato a hábito, aunque las 6 veces que SÍ apareció en la
// cadena hayan sido, por construcción, éxitos.
//
// Esto es exactamente lo mínimo que propusiste vos (y confirmó el análisis
// que compartiste): usar el Success que ya se registra, sin construir un
// Experience Engine completo todavía.

// DefaultMinHabitDays es explícitamente PROVISIONAL, igual que
// DefaultPatternThreshold — 3 días distintos como piso para no promover
// algo a "hábito" en base a una sola sesión larga, ajustable con datos
// reales de uso más adelante.
const DefaultMinHabitDays = 3

// Habit es un Pattern que además demostró sostenerse en varios días, con su
// puntaje de confianza.
type Habit struct {
	Steps       []string
	Count       int
	DaysSeen    int
	SuccessRate float64
	Score       float64
}

// stepSuccessRates calcula, para cada paso distinto (Action+Target), qué
// fracción de sus intentos totales (éxitos Y fallos) tuvo éxito — a
// diferencia de Chain.Count, que solo cuenta ocurrencias dentro de corridas
// ya exitosas, esto mira el EventLog completo sin filtrar.
func stepSuccessRates(log *EventLog) map[string]float64 {
	successes := map[string]int{}
	totals := map[string]int{}
	for _, ev := range log.Events {
		if ev.Action == "" || ev.Action == "desconocido" {
			continue
		}
		key := stepKey(ev.Action, ev.Target)
		totals[key]++
		if ev.Outcome.Success {
			successes[key]++
		}
	}
	rates := map[string]float64{}
	for key, total := range totals {
		if total > 0 {
			rates[key] = float64(successes[key]) / float64(total)
		}
	}
	return rates
}

// averageStepRate promedia la tasa de éxito histórica de cada paso de una
// cadena. Un paso sin historial de fallos (no está en `rates` porque nunca
// falló, o directamente no hay datos) no se penaliza sin evidencia — se
// asume 1.0, no 0.
func averageStepRate(steps []string, rates map[string]float64) float64 {
	if len(steps) == 0 {
		return 0
	}
	sum := 0.0
	for _, s := range steps {
		if r, ok := rates[s]; ok {
			sum += r
		} else {
			sum += 1.0
		}
	}
	return sum / float64(len(steps))
}

// chainDayCounts cuenta, para cada cadena de largo 2..maxLen, en qué fechas
// calendario distintas apareció — mismo recorrido de n-gramas que
// BuildChains (sequence.go), pero agrupando por fecha en vez de por
// conteo total.
func chainDayCounts(log *EventLog, maxLen int) map[string]map[string]bool {
	result := map[string]map[string]bool{}
	for _, run := range runsWithDatesFromLog(log) {
		for length := 2; length <= maxLen && length <= len(run.Steps); length++ {
			for start := 0; start+length <= len(run.Steps); start++ {
				sub := run.Steps[start : start+length]
				key := chainKey(sub)
				if result[key] == nil {
					result[key] = map[string]bool{}
				}
				result[key][run.Date] = true
			}
		}
	}
	return result
}

// DetectHabits reusa la detección de Fase 3 (con el mismo filtro de
// periodicidad/rotación/absorción ya validado en patterns.go) y, sobre esos
// candidatos, exige además que se hayan visto en al menos minDays fechas
// distintas. Los que no llegan a ese piso siguen siendo Patrones válidos,
// simplemente no se promueven a Hábito todavía.
func DetectHabits(log *EventLog, minCount int, minDays int) []Habit {
	chains := BuildChains(log, MaxChainLength)
	patterns := DetectPatterns(chains, minCount)

	dayCounts := chainDayCounts(log, MaxChainLength)
	rates := stepSuccessRates(log)

	habits := make([]Habit, 0, len(patterns))
	for _, p := range patterns {
		days := len(dayCounts[chainKey(p.Steps)])
		if days < minDays {
			continue
		}
		rate := averageStepRate(p.Steps, rates)
		habits = append(habits, Habit{
			Steps:       p.Steps,
			Count:       p.Count,
			DaysSeen:    days,
			SuccessRate: rate,
			Score:       float64(days) * rate,
		})
	}

	sort.Slice(habits, func(i, j int) bool { return habits[i].Score > habits[j].Score })
	return habits
}

// DescribeHabits arma la narrativa en español. No inventa nada que no esté
// en los datos: si no hay hábitos todavía, lo dice tal cual.
func DescribeHabits(habits []Habit) string {
	if len(habits) == 0 {
		return "Todavía no tengo ningún hábito confirmado — necesito ver los mismos flujos sostenerse en varios días distintos, no solo repetirse seguido."
	}
	shown := habits
	if len(shown) > 5 {
		shown = shown[:5]
	}
	lines := make([]string, 0, len(shown))
	for _, h := range shown {
		lines = append(lines, fmt.Sprintf("%s (%d días, %.0f%% de éxito)", strings.Join(h.Steps, " → "), h.DaysSeen, h.SuccessRate*100))
	}
	return "Esto ya lo tengo como costumbre: " + strings.Join(lines, "; ")
}

// habitTriggers, separado de patternTriggers a propósito — "qué hábitos
// tienes" pregunta específicamente por lo que se sostuvo en el tiempo, no
// por cualquier repetición.
var habitTriggers = []string{
	"que habitos tienes",
	"que habitos has detectado",
	"que costumbres tienes",
	"que costumbres has notado",
}

// ParseHabitsQuery detecta si la pregunta pide un resumen de hábitos.
func ParseHabitsQuery(req interaction.SemanticRequest) bool {
	return interaction.HasSemanticQuery(req, interaction.QueryHistoryHabits)
}
