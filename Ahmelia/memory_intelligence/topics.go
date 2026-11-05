package memory_intelligence

import (
	"strings"

	"ahmelia/comprehension"
)

// ============================================================================
// MEMORY_INTELLIGENCE/ — capa que conecta la sesión actual con lo que
// Ahmelia recuerda de antes, organizado por tema.
// ============================================================================
//
// Hallazgo real al diseñar esto (no es un supuesto, es lo que hay en el
// código): Ahmelia ya tiene DOS sistemas de "tema", construidos por
// separado, con vocabularios que no coinciden:
//
//  1. comprehension.ConversationContext.GetCurrentTopic() — en RAM, dura
//     lo que dura la sesión, granularidad de app ("programa_unity",
//     "navegacion_ventanas"), calculado por detectTopic() en
//     comprehension/context.go.
//  2. memory.Knowledge.Tags — persistido en memory/long.json entre
//     sesiones, granularidad de categoría ("audio", "sistema",
//     "archivos"), calculado por shared.InferTags()/TagMappings.
//
// Ninguno de los dos está mal — resuelven preguntas distintas ("¿de qué
// hablamos ahora?" vs "¿cómo se categoriza esto para encontrarlo
// después?"). Pero para responder algo tipo "¿qué hicimos ayer en
// Unity?" hace falta poder cruzar los dos, y hoy no existía ningún
// puente. Este archivo es ese puente — no reemplaza a ninguno de los
// dos sistemas.
//
// Otro hallazgo: knowledge/sources.go ya tenía un MemorySource
// completo, con Priority() y todo, pero NUNCA se registraba en
// NewEngine() — quedó armado y nunca conectado. Ese es exactamente el
// lugar donde este paquete se engancha (ver core/engine.go).

// ActiveTopic devuelve de qué se está hablando ahora mismo, tal como lo
// ve la sesión activa. Es un alias intencionalmente delgado sobre
// ConversationContext — este paquete no reimplementa detección de
// tema, solo lo consume.
func ActiveTopic(convo *comprehension.ConversationContext) string {
	if convo == nil {
		return ""
	}
	return convo.GetCurrentTopic()
}

// RelatedTags traduce un topic de sesión (ej: "programa_unity") a los
// tags de memoria persistida que probablemente le correspondan (ej:
// "unity"). Es una traducción de texto simple a propósito — mismo
// criterio de "sin inventar de más" que ya sigue shared.InferTags: si
// el topic no tiene el prefijo conocido "programa_", se usa tal cual
// como tag, sin adivinar nada más sofisticado.
func RelatedTags(topic string) []string {
	topic = strings.TrimSpace(topic)
	if topic == "" {
		return nil
	}
	if strings.HasPrefix(topic, "programa_") {
		return []string{strings.TrimPrefix(topic, "programa_")}
	}
	return []string{topic}
}
