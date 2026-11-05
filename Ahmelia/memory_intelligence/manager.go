package memory_intelligence

import (
	"ahmelia/comprehension"
	"ahmelia/memory"
)

// Manager es el orquestador de esta capa: junta la sesión activa
// (comprehension) con lo persistido (memory) para responder preguntas
// que ninguna de las dos partes podía responder sola. No es dueño de
// ninguna de las dos — solo las consulta, mismo patrón que ya usa
// reference.Resolver con environment.Manager.
type Manager struct {
	convo *comprehension.ConversationContext
	long  *memory.LongMemory
}

// New crea el Manager sobre las instancias YA existentes de
// ConversationContext y LongMemory — no duplica ningún estado.
func New(convo *comprehension.ConversationContext, long *memory.LongMemory) *Manager {
	return &Manager{convo: convo, long: long}
}

// Query es el único método que knowledge/sources.go necesita llamar.
// Si topic viene con algo ("Unity", "el volumen"), busca eso puntual.
// Si viene vacío ("¿qué hicimos?" sin más), primero intenta el tema
// activo de la sesión y, si no hay nada, cae al resumen del episodio
// más reciente (ver episode.go) — es la respuesta más razonable a una
// pregunta genérica sin nombre de por medio.
func (m *Manager) Query(topic string) (string, bool) {
	if topic != "" {
		return RecallTopic(m.long, topic)
	}
	if active := ActiveTopic(m.convo); active != "" {
		if answer, ok := RecallTopic(m.long, active); ok {
			return answer, true
		}
	}
	return latestEpisode(m.long)
}
