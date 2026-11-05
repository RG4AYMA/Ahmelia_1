package memory_intelligence

import (
	"ahmelia/interaction"
	"fmt"
	"sort"
	"strings"
	"time"
)

// ============================================================================
// INSIGHTS.GO — Bloque 4 (Insights)
// ============================================================================
//
// La diferencia con todo lo anterior (Patrones, Hábitos, Experiencia): esos
// tres responden preguntas puntuales sobre algo que el usuario ya nombró
// ("qué patrones tienes", "por qué falla blender"). Insights no espera que
// le señalen el tema — mira el EventLog completo y decide QUÉ vale la pena
// contar. Por eso esto no inventa una tercera forma de contar eventos: usa
// exactamente los mismos datos que ya tiene FailureCategory (eventlog.go/
// experience.go) y Action (classifier.go), solo que agregados distinto:
// por ventana de tiempo (foco semanal) y por categoría global de fallo
// (en vez de por un paso puntual, como hace experience.go).
//
// Deliberadamente solo dos insights por ahora, no una lista abierta —
// mismo criterio que el resto de esta fase: agregar una tercera fuente de
// insight (ej. sobre Hábitos o sobre Relations) cuando haya un caso real
// que lo pida, no de forma especulativa.

// DefaultMinFocusEvents / DefaultMinFailuresForInsight son pisos
// PROVISIONALES (mismo criterio que DefaultPatternThreshold) para no sacar
// una "conclusión" de 2 eventos sueltos.
const DefaultMinFocusEvents = 3
const DefaultMinFailuresForInsight = 3

// countedItem/topByCount: helper compartido por los dos insights para
// encontrar la clave más frecuente de un map[string]int de forma
// determinística — Go aleatoriza el orden de un map, así que sin este
// desempate por nombre la respuesta cambiaría entre corridas sin que
// cambiaran los datos.
type countedItem struct {
	key   string
	count int
}

func topByCount(counts map[string]int) (string, int) {
	items := make([]countedItem, 0, len(counts))
	for k, c := range counts {
		items = append(items, countedItem{k, c})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].count != items[j].count {
			return items[i].count > items[j].count
		}
		return items[i].key < items[j].key
	})
	if len(items) == 0 {
		return "", 0
	}
	return items[0].key, items[0].count
}

// FocusInsight mira esta semana (lunes más reciente hasta ahora — mismo
// rango que ParseTimelineQuery para "esta semana", reusando
// mostRecentWeekday/startOfDay de timeline.go) y reporta qué Action
// concentró más intentos. No baja al nivel de stepKey (Action+Target)
// porque a esa escala "esta semana te enfocaste en abrir_programa:vscode"
// es casi siempre cierto y no dice nada — Action solo es el nivel donde
// esto empieza a ser una síntesis real.
func FocusInsight(log *EventLog) (string, bool) {
	today := startOfDay(time.Now())
	monday := mostRecentWeekday(today, time.Monday)

	counts := map[string]int{}
	total := 0
	for _, ev := range log.Events {
		if ev.Action == "" || ev.Action == "desconocido" {
			continue
		}
		if ev.Timestamp.Before(monday) {
			continue
		}
		counts[ev.Action]++
		total++
	}
	if total < DefaultMinFocusEvents {
		return "", false
	}

	topAction, topCount := topByCount(counts)
	share := float64(topCount) / float64(total) * 100
	return fmt.Sprintf("Esta semana te enfocaste principalmente en %s (%d de %d acciones, %.0f%%).", topAction, topCount, total, share), true
}

// FailureInsight mira el EventLog completo (no solo esta semana — un
// patrón de fallo tiene sentido aunque lleve más de 7 días formándose) y
// reporta qué Category de fallo domina y, dentro de esa categoría, qué
// paso puntual la explica más.
func FailureInsight(log *EventLog) (string, bool) {
	catCounts := map[string]int{}
	catStepCounts := map[string]map[string]int{}
	totalFailures := 0

	for _, ev := range log.Events {
		if ev.Outcome.Success || ev.Outcome.Category == "" {
			continue
		}
		totalFailures++
		catCounts[ev.Outcome.Category]++
		if catStepCounts[ev.Outcome.Category] == nil {
			catStepCounts[ev.Outcome.Category] = map[string]int{}
		}
		catStepCounts[ev.Outcome.Category][stepKey(ev.Action, ev.Target)]++
	}
	if totalFailures < DefaultMinFailuresForInsight {
		return "", false
	}

	topCategory, topCatCount := topByCount(catCounts)
	share := float64(topCatCount) / float64(totalFailures) * 100
	msg := fmt.Sprintf("La mayoría de tus errores últimamente son de tipo %s (%d de %d fallos, %.0f%%)", topCategory, topCatCount, totalFailures, share)

	if topStep, topStepCount := topByCount(catStepCounts[topCategory]); topStep != "" {
		msg += fmt.Sprintf(" — sobre todo con %s (%d veces)", topStep, topStepCount)
	}
	return msg + ".", true
}

// GenerateInsights corre todas las fuentes de insight disponibles y
// devuelve solo las que tuvieron suficiente evidencia — orden fijo (foco
// primero, fallos después) para que la respuesta no cambie de estructura
// entre llamadas.
func GenerateInsights(log *EventLog) []string {
	var insights []string
	if s, ok := FocusInsight(log); ok {
		insights = append(insights, s)
	}
	if s, ok := FailureInsight(log); ok {
		insights = append(insights, s)
	}
	return insights
}

// DescribeInsights arma la narrativa final. No rellena con algo genérico
// si no hay suficiente evidencia todavía — dice exactamente eso.
func DescribeInsights(insights []string) string {
	if len(insights) == 0 {
		return "Todavía no tengo suficientes datos para sacar conclusiones — necesito ver más actividad."
	}
	return strings.Join(insights, " ")
}

// insightTriggers, mismo criterio que patternTriggers/habitTriggers.
var insightTriggers = []string{
	"que insights tienes",
	"que conclusiones sacaste",
	"que conclusiones tienes",
	"que has notado ultimamente",
	"dame un resumen de tu actividad",
}

// ParseInsightsQuery detecta si la pregunta pide una síntesis general.
func ParseInsightsQuery(req interaction.SemanticRequest) bool {
	return interaction.HasSemanticQuery(req, interaction.QueryHistoryInsights)
}
