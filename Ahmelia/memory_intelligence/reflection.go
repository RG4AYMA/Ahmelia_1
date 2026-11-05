package memory_intelligence

import (
	"ahmelia/interaction"
	"fmt"
	"sort"
	"strings"
	"time"
)

// ============================================================================
// REFLECTION.GO — Bloque 6 (Self Reflection)
// ============================================================================
//
// A diferencia de Insights (que analiza la actividad del USUARIO y saca
// conclusiones sobre qué hizo, qué patrones tiene, etc.), Reflection
// analiza la PROPIA EVOLUCIÓN DE AHMELIA — esto es, cómo está funcionando,
// si mejora o empeora, en qué tipos de acciones es mejor, qué ha aprendido.
//
// Dos fuentes de reflexión:
//
// 1. SuccessRateReflection: mira la tasa de éxito históricamente.
//    - Si está mejorando → Ahmelia está aprendiendo
//    - Si está empeorando → algo se rompió o necesita ajustar
//    - Compara tendencia antigua (primeras 2 semanas) vs. reciente (última semana)
//
// 2. StrengthReflection: analiza en qué tipos de acciones Ahmelia es MÁS
//    efectiva, separando las fortalezas de las debilidades.

const DefaultMinReflectionEvents = 5

// SuccessRateReflection mira la evolución de la tasa de éxito en el tiempo.
// Si hay suficiente historia, compara "entonces" vs "ahora" y reporta
// si Ahmelia está mejorando o degradándose.
func SuccessRateReflection(log *EventLog) (string, bool) {
	if len(log.Events) < DefaultMinReflectionEvents {
		return "", false
	}

	now := time.Now()
	oneWeekAgo := now.Add(-7 * 24 * time.Hour)
	twoWeeksAgo := now.Add(-14 * 24 * time.Hour)

	// Histórico: primeras 2 semanas (o todo lo que haya si es muy reciente)
	var historicSuccesses, historicTotal int
	// Reciente: última semana
	var recentSuccesses, recentTotal int

	for _, ev := range log.Events {
		if ev.Timestamp.After(now) {
			continue
		}

		if ev.Timestamp.Before(twoWeeksAgo) {
			// Histórico
			if ev.Outcome.Success {
				historicSuccesses++
			}
			historicTotal++
		} else if ev.Timestamp.After(oneWeekAgo) {
			// Reciente
			if ev.Outcome.Success {
				recentSuccesses++
			}
			recentTotal++
		}
	}

	// Si uno de los períodos no tiene suficientes datos, no hay reflexión
	if historicTotal < DefaultMinReflectionEvents || recentTotal < DefaultMinReflectionEvents {
		return "", false
	}

	historicRate := float64(historicSuccesses) / float64(historicTotal) * 100
	recentRate := float64(recentSuccesses) / float64(recentTotal) * 100
	delta := recentRate - historicRate

	var trend string
	if delta > 5 {
		trend = "mejorando"
	} else if delta < -5 {
		trend = "degradándome"
	} else {
		trend = "estable"
	}

	return fmt.Sprintf(
		"Mi tasa de éxito es %s: entonces tenía %.0f%%, ahora tengo %.0f%% (delta: %+.0f%%).",
		trend, historicRate, recentRate, delta,
	), true
}

// StrengthReflection mira las tasas de éxito por Action y reporta cuál
// es la fortaleza más clara (qué tipo de acción hago mejor) y si hay
// alguna debilidad notable.
func StrengthReflection(log *EventLog) (string, bool) {
	if len(log.Events) < DefaultMinReflectionEvents {
		return "", false
	}

	// Por cada Action, contar éxitos y totales
	actionStats := map[string]struct {
		successes int
		total     int
	}{}

	for _, ev := range log.Events {
		if ev.Action == "" || ev.Action == "desconocido" {
			continue
		}

		stat := actionStats[ev.Action]
		if ev.Outcome.Success {
			stat.successes++
		}
		stat.total++
		actionStats[ev.Action] = stat
	}

	// Filtrar acciones con al menos 2 intentos (para no sacar conclusiones
	// de "1 de 1 intentos exitosos")
	type actionRate struct {
		action    string
		rate      float64
		successes int
		total     int
	}
	var rates []actionRate

	for action, stat := range actionStats {
		if stat.total >= 2 {
			rate := float64(stat.successes) / float64(stat.total) * 100
			rates = append(rates, actionRate{action, rate, stat.successes, stat.total})
		}
	}

	if len(rates) < 2 {
		return "", false
	}

	// Ordenar por tasa de éxito
	sort.Slice(rates, func(i, j int) bool {
		if rates[i].rate != rates[j].rate {
			return rates[i].rate > rates[j].rate
		}
		return rates[i].action < rates[j].action
	})

	best := rates[0]
	worst := rates[len(rates)-1]

	// Solo reportar si hay diferencia notable (>20%)
	if best.rate-worst.rate < 20 {
		return "", false
	}

	return fmt.Sprintf(
		"Mis fortalezas y debilidades: %s es donde mejor me va (%.0f%% exitosas), pero %s me cuesta (%d de %d).",
		best.action, best.rate, worst.action, worst.successes, worst.total,
	), true
}

// GenerateReflections corre todas las fuentes de reflexión disponibles
// y devuelve solo las que tuvieron suficiente evidencia — orden fijo
// para consistencia.
func GenerateReflections(log *EventLog) []string {
	var reflections []string
	if s, ok := SuccessRateReflection(log); ok {
		reflections = append(reflections, s)
	}
	if s, ok := StrengthReflection(log); ok {
		reflections = append(reflections, s)
	}
	return reflections
}

// DescribeReflections arma la narrativa final de Self Reflection.
func DescribeReflections(reflections []string) string {
	if len(reflections) == 0 {
		return "Todavía no tengo suficientes datos sobre mi propio desempeño — necesito más actividad."
	}
	return strings.Join(reflections, " ")
}

// reflectionTriggers: patrones para detectar si el usuario pregunta
// por reflexión / autoconocimiento sobre cómo funciona Ahmelia.
var reflectionTriggers = []string{
	"como te va",
	"como funcionas",
	"mejorando",
	"empeorando",
	"que tal te va",
	"cual es tu desempenio",
	"cual es tu desempeno",
	"en que eres bueno",
	"cuales son tus fortalezas",
	"cuales son tus debilidades",
	"reflexion",
	"reflexión",
	"como has evolucionado",
	"como esta tu tasa",
	"te estas volviendo mejor",
}

// ParseReflectionQuery detecta si la pregunta pide reflexión sobre
// cómo está funcionando Ahmelia misma.
func ParseReflectionQuery(req interaction.SemanticRequest) bool {
	return interaction.HasSemanticQuery(req, interaction.QueryHistoryReflection)
}
