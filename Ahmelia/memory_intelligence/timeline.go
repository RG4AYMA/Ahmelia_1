package memory_intelligence

import (
	"ahmelia/interaction"
	"fmt"
	"sort"
	"strings"
	"time"

	"ahmelia/memory"
)

// ============================================================================
// TIMELINE.GO — Bloque 2, Nivel 1: navegar el pasado, no aprender nada nuevo
// ============================================================================
//
// episode.go ya persiste CADA sesión con clave única (episodio_<inicio>) —
// el dato para un timeline YA EXISTE, uno por sesión, sin pisarse entre sí.
// Lo único que faltaba era la forma de recorrerlos por rango de fecha en
// vez de traer siempre "el más reciente" (recall.go/latestEpisode). Esto
// no inventa memoria nueva: es una vista distinta sobre la misma data.
//
// Deliberadamente conservador con lo que puede responder: entiende "hoy",
// "ayer", "esta semana" y los 7 días de la semana ("el lunes"). Si el
// texto no calza con ninguno de esos patrones, ParseTimelineQuery
// devuelve ok=false — no intenta adivinar fechas relativas más
// ambiguas ("hace poco", "la semana pasada") todavía.

// ParseTimelineQuery detecta si la pregunta pide un RANGO de tiempo
// explícito ("qué hicimos hoy/ayer/esta semana/el lunes") y devuelve el
// rango [from, to) correspondiente más una etiqueta legible para la
// respuesta. normalizedText ya debe venir de shared.Normalize (minúsculas,
// sin tildes) — mismo contrato que ParseCategoryQuery en environment/.
func ParseTimelineQuery(req interaction.SemanticRequest) (from, to time.Time, label string, ok bool) {
	if !interaction.HasSemanticQuery(req, interaction.QueryHistoryTimeline) {
		return time.Time{}, time.Time{}, "", false
	}
	return req.Temporal.From, req.Temporal.To, req.Temporal.Label, req.Temporal.Known
}

// weekdayNames mapea el nombre normalizado (sin tildes) al time.Weekday
// correspondiente — "miercoles" sin tilde porque normalizedText ya pasó
// por shared.Normalize antes de llegar acá.
var weekdayNames = map[string]time.Weekday{
	"lunes":     time.Monday,
	"martes":    time.Tuesday,
	"miercoles": time.Wednesday,
	"jueves":    time.Thursday,
	"viernes":   time.Friday,
	"sabado":    time.Saturday,
	"domingo":   time.Sunday,
}

// mostRecentWeekday devuelve la fecha (medianoche) del día de la semana
// pedido más reciente, contando hoy — si hoy ES ese día, devuelve hoy.
func mostRecentWeekday(today time.Time, target time.Weekday) time.Time {
	diff := int(today.Weekday() - target)
	if diff < 0 {
		diff += 7
	}
	return today.AddDate(0, 0, -diff)
}

func startOfDay(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, t.Location())
}

// EpisodesInRange devuelve los episodios (ver episode.go/Summarize) cuyo
// CreatedAt cae en [from, to), ordenados cronológicamente (más viejo
// primero — así se lee como una historia, no como un feed).
func EpisodesInRange(long *memory.LongMemory, from, to time.Time) []*memory.Knowledge {
	if long == nil {
		return nil
	}
	var out []*memory.Knowledge
	for _, k := range long.RecallByTag("episodio") {
		if (k.CreatedAt.Equal(from) || k.CreatedAt.After(from)) && k.CreatedAt.Before(to) {
			out = append(out, k)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	return out
}

// DescribeTimeline arma la narrativa en español de un rango de episodios.
// label es la referencia temporal ya resuelta por ParseTimelineQuery
// ("hoy", "ayer", "esta semana", "el lunes").
func DescribeTimeline(episodes []*memory.Knowledge, label string) string {
	if len(episodes) == 0 {
		return fmt.Sprintf("No tengo ningún episodio registrado para %s", label)
	}
	if len(episodes) == 1 {
		return fmt.Sprintf("Sobre %s: %s", label, episodes[0].Input)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Sobre %s, %d sesiones: ", label, len(episodes))
	var parts []string
	for _, ep := range episodes {
		parts = append(parts, ep.Input)
	}
	b.WriteString(strings.Join(parts, ". "))
	return b.String()
}
