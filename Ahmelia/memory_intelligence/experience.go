package memory_intelligence

import (
	"ahmelia/interaction"
	"fmt"

	"ahmelia/classifier"
)

// ============================================================================
// EXPERIENCE.GO — Bloque 3 (Experience Engine), consulta en lenguaje natural
// ============================================================================
//
// El Nivel 1 del Experience Engine (Outcome{Success,Category,Reason}) ya se
// graba en cada evento desde eventlog.go — lo que faltaba era la mitad de
// consulta: poder preguntar "por qué falla blender" y que Ahmelia responda
// con el balance completo del EventLog, no con el último intento. Mismo
// espíritu que Habits sobre Patterns: agregar sobre el historial entero,
// no sobre un solo evento.
//
// A diferencia de Relations (que busca por ID de texto normalizado en
// memory.LongMemory), esto busca por stepKey(action,target) — el mismo
// identificador que ya usan Fase 2-4 sobre el EventLog. El puente entre
// "lo que el usuario preguntó" (texto suelto, ej. "blender") y ese stepKey
// es classifier.ClassifyCommand: el mismo camino que ya recorre execute()
// para saber a qué acción se refiere una frase — reusado acá, no duplicado.

// experienceTriggers, mismo criterio que relationTriggers/habitTriggers:
// cada frase corta justo antes del sujeto.
var experienceTriggers = []string{
	"por que falla ",
	"por que te falla ",
	"por que no funciona ",
	"que tan bien te va con ",
	"cual es tu experiencia con ",
}

// ParseExperienceQuery detecta si la pregunta pide un balance de éxito/
// fallo y devuelve el sujeto en crudo. No lo resuelve a action/target acá
// — eso requiere el clasificador, no una simple normalización, y separar
// los dos pasos deja este parser simétrico con ParseRelationsQuery.
func ParseExperienceQuery(req interaction.SemanticRequest) (subject string, ok bool) {
	if !interaction.HasSemanticQuery(req, interaction.QueryHistoryExperience) {
		return "", false
	}
	if req.Topic == "" {
		return "", false
	}
	return req.Topic, true
}

// ExperienceStats agrega, para un paso (Action+Target), el balance
// completo visto en el EventLog — no el último intento, todos.
type ExperienceStats struct {
	Action       string
	Target       string
	Attempts     int
	Successes    int
	SuccessRate  float64
	TopCategory  string // categoría de fallo más frecuente; "" si nunca falló
	TopReason    string // un ejemplo legible de esa categoría, no el texto de cada intento
	FailureCount int    // cuántos fallos cayeron en TopCategory
}

// ComputeExperience calcula el balance de un paso concreto sobre el
// EventLog completo. ok=false significa "nunca se intentó" — distinto de
// 0% de éxito, que es un dato real y debe poder mostrarse como tal.
func ComputeExperience(log *EventLog, action, target string) (ExperienceStats, bool) {
	key := stepKey(action, target)
	attempts, successes := 0, 0
	categoryCounts := map[string]int{}
	categoryReason := map[string]string{}

	for _, ev := range log.Events {
		if stepKey(ev.Action, ev.Target) != key {
			continue
		}
		attempts++
		if ev.Outcome.Success {
			successes++
			continue
		}
		if ev.Outcome.Category == "" {
			continue
		}
		categoryCounts[ev.Outcome.Category]++
		if categoryReason[ev.Outcome.Category] == "" {
			categoryReason[ev.Outcome.Category] = ev.Outcome.Reason
		}
	}
	if attempts == 0 {
		return ExperienceStats{}, false
	}

	topCategory, topCount := "", 0
	for cat, count := range categoryCounts {
		if count > topCount {
			topCategory, topCount = cat, count
		}
	}

	return ExperienceStats{
		Action:       action,
		Target:       target,
		Attempts:     attempts,
		Successes:    successes,
		SuccessRate:  float64(successes) / float64(attempts),
		TopCategory:  topCategory,
		TopReason:    categoryReason[topCategory],
		FailureCount: topCount,
	}, true
}

// ExperienceForSubject traduce una frase suelta ("blender", "abrir
// blender") al stepKey correspondiente usando el mismo clasificador que
// execute() en core/engine.go — así "por qué falla blender" y el comando
// real "abre blender" caen en la misma clave, sin mantener un mapeo aparte
// que se pueda desincronizar del clasificador real.
func ExperienceForSubject(log *EventLog, subject string) (ExperienceStats, bool) {
	result := classifier.ClassifyCommand(subject)
	if result.Action == "" || result.Action == "desconocido" {
		return ExperienceStats{}, false
	}
	return ComputeExperience(log, result.Action, result.Target)
}

// DescribeExperience arma la narrativa en español. Distingue "nunca lo
// intenté" de "siempre falla" — son respuestas distintas y confundirlas
// sería peor que no responder.
func DescribeExperience(stats ExperienceStats, found bool) string {
	if !found {
		return "Todavía no tengo experiencia registrada con eso."
	}
	step := displayStep(stats)
	if stats.SuccessRate == 1.0 {
		return fmt.Sprintf("Con %s me fue bien las %d veces que lo intenté (100%% de éxito).", step, stats.Attempts)
	}
	base := fmt.Sprintf("Con %s tengo %.0f%% de éxito en %d intentos.", step, stats.SuccessRate*100, stats.Attempts)
	if stats.TopCategory != "" {
		failures := stats.Attempts - stats.Successes
		base += fmt.Sprintf(" El problema más frecuente: %s (%d de %d fallos).", stats.TopReason, stats.FailureCount, failures)
	}
	return base
}

func displayStep(stats ExperienceStats) string {
	if stats.Target == "" {
		return stats.Action
	}
	return stats.Action + " " + stats.Target
}
