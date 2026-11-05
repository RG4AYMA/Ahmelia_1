package memory_intelligence

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"ahmelia/memory"
)

// RecallTopic busca en memory.LongMemory (la memoria YA persistida —
// esto no crea ningún almacén nuevo) todo lo relacionado a un topic, y
// arma una respuesta con lo más reciente primero. Devuelve ok=false si
// el topic no resolvió a ningún tag conocido, o si no hay nada guardado
// bajo esos tags — dos motivos distintos, pero el caller solo necesita
// saber "no tengo nada que contarte" en ambos casos.
func RecallTopic(long *memory.LongMemory, topic string) (string, bool) {
	if long == nil {
		return "", false
	}
	tags := RelatedTags(topic)
	if len(tags) == 0 {
		return "", false
	}

	seen := make(map[string]bool)
	var matches []*memory.Knowledge
	for _, tag := range tags {
		// RecallByTag: coincide con la categoría (ej: "programa").
		// RecallByTarget: coincide con el nombre literal (ej: "unity")
		// — esto es lo que de verdad hace falta para "qué hicimos en
		// Unity", porque los Tags nunca guardan nombres de app, solo
		// categorías (ver TagMappings en shared/constants.go).
		for _, k := range long.RecallByTag(tag) {
			if seen[k.ID] {
				continue
			}
			seen[k.ID] = true
			matches = append(matches, k)
		}
		for _, k := range long.RecallByTarget(tag) {
			if seen[k.ID] {
				continue
			}
			seen[k.ID] = true
			matches = append(matches, k)
		}
	}
	if len(matches) == 0 {
		return "", false
	}

	// Más reciente primero — es lo que casi siempre importa cuando
	// alguien pregunta "¿qué hicimos con X?".
	sort.Slice(matches, func(i, j int) bool {
		return matches[i].LastUsed.After(matches[j].LastUsed)
	})
	if len(matches) > 5 {
		matches = matches[:5]
	}

	var parts []string
	for _, k := range matches {
		if k.Input == "" {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s (%s)", k.Input, relativeWhen(k.LastUsed)))
	}
	if len(parts) == 0 {
		return "", false
	}
	return "Lo último que recuerdo de eso: " + strings.Join(parts, "; "), true
}

// latestEpisode devuelve el resumen de sesión (ver episode.go, tag
// "episodio") más reciente — la respuesta a "¿qué hicimos?" cuando no
// se nombró ninguna app ni tema puntual.
func latestEpisode(long *memory.LongMemory) (string, bool) {
	episodes := long.RecallByTag("episodio")
	if len(episodes) == 0 {
		return "", false
	}
	latest := episodes[0]
	for _, k := range episodes[1:] {
		if k.CreatedAt.After(latest.CreatedAt) {
			latest = k
		}
	}
	return latest.Input, true
}

// relativeWhen da una referencia temporal legible ("hoy", "ayer", fecha)
// en vez de un timestamp crudo — para que la respuesta suene como algo
// que diría una persona, no un log.
func relativeWhen(t time.Time) string {
	if t.IsZero() {
		return "hace tiempo"
	}
	d := time.Since(t)
	switch {
	case d < time.Hour:
		return "hace un rato"
	case d < 24*time.Hour:
		return "hoy"
	case d < 48*time.Hour:
		return "ayer"
	default:
		return t.Format("2 Jan")
	}
}
