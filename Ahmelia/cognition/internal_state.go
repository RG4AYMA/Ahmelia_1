package cognition

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// ============================================================================
// INTERNAL_STATE.GO — Bloque 15, Fase 2.6 (Integración Cognitiva, 6 de 6)
// ============================================================================
//
// El documento del roadmap marca este bloque como más importante de lo
// que parece a simple vista: no es solo "una etiqueta más para
// responder preguntas" — es la base que Fase 2.8 (Conversación) va a
// necesitar para decisiones como "¿la puedo interrumpir ahora o está
// ejecutando algo?" (Interruption Manager) o "¿hace cuánto que no
// pasa nada?" (Turn Manager). Por eso va acá, aunque a diferencia de
// los cinco bloques anteriores de esta fase no tiene todavía un
// consumidor real más allá de la propia meta-pregunta — el consumidor
// real llega en Fase 2.8.
//
// Seis estados, tal como los nombra el roadmap:
//
//   Esperando respuesta → Escuchando → Pensando → Ejecutando → Aprendiendo
//                              ↑                                    |
//                              └────────── vuelve a Esperando ──────┘
//
// Ocupada es el sexto y no es un paso más de esa cadena: es lo que pasa
// cuando llega un input mientras hay una tarea SIN TERMINAR de antes
// (e.sess.WaitingForFileSelection/Renaming/FileAction/VolumeSet/
// Learning — Ahmelia ya empezó algo y necesita que el usuario complete
// un paso, no que arranque uno nuevo). Se distingue a propósito de
// "Esperando respuesta": esa es el reposo real, sin nada pendiente;
// Ocupada es "tengo algo a medio terminar".
//
// Igual que Pipeline (Bloque 11): InternalState no decide nada ni
// reemplaza ningún módulo, es un observador liviano. core/engine.go
// marca la transición en los MISMOS puntos de decisión que ya usa
// Pipeline para su propio Enter() — no hizo falta buscar puntos
// nuevos: Pensando cubre las cuatro etapas "mentales" de Pipeline
// (Comprender/Construir contexto/Razonar/Elegir estrategia); Ejecutando
// y Aprendiendo son 1 a 1 con StageExecute y StageLearn/StageExperience.

// State es uno de los seis estados internos.
type State string

const (
	StateWaiting   State = "Esperando respuesta"
	StateBusy      State = "Ocupada"
	StateListening State = "Escuchando"
	StateThinking  State = "Pensando"
	StateExecuting State = "Ejecutando"
	StateLearning  State = "Aprendiendo"
)

// StateManager sostiene el estado interno actual: cuál es, desde
// cuándo, y qué lo disparó. Suficiente para "qué estás haciendo" sin
// pretender ser un sistema de logging (para eso ya está Pipeline).
type StateManager struct {
	mu      sync.Mutex
	current State
	detail  string
	since   time.Time
}

// NewStateManager arranca en Esperando respuesta — recién instanciada,
// Ahmelia todavía no está haciendo nada.
func NewStateManager() *StateManager {
	return &StateManager{current: StateWaiting, since: time.Now()}
}

// Enter registra una transición de estado. Barato a propósito — se
// llama en el mismo camino caliente que Pipeline.Enter(), a veces en
// el mismo punto exacto del código.
func (m *StateManager) Enter(s State, detail string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.current = s
	m.detail = detail
	m.since = time.Now()
}

// Current devuelve el estado actual.
func (m *StateManager) Current() State {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.current
}

// Busy es un atajo booleano: true en cualquier estado que no sea
// Esperando respuesta. Pensado para código futuro que solo necesita
// "¿está libre o no?" sin importarle el detalle — el caso de uso
// concreto es Interruption Manager (Fase 2.8), que hoy no existe
// todavía, por eso Busy() no tiene consumidor propio en este bloque.
func (m *StateManager) Busy() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.current != StateWaiting
}

// Snapshot returns the current state without exposing mutable internal fields.
func (m *StateManager) Snapshot() (State, string, time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.current, m.detail, m.since
}

// Narrative arma la respuesta hablada de la meta-pregunta, con
// antigüedad legible en vez de un timestamp crudo.
func (m *StateManager) Narrative() string {
	m.mu.Lock()
	defer m.mu.Unlock()

	age := time.Since(m.since)
	var ageText string
	switch {
	case age < time.Second:
		ageText = "recién"
	case age < time.Minute:
		ageText = fmt.Sprintf("hace %d segundos", int(age.Seconds()))
	default:
		ageText = fmt.Sprintf("hace %d minutos", int(age.Minutes()))
	}

	base := fmt.Sprintf("%s (%s)", m.current, ageText)
	if m.detail != "" {
		base += ": " + m.detail
	}
	return base
}

var internalStateTriggers = []string{
	"que estas haciendo",
	"en que andas",
	"estas ocupada",
	"cual es tu estado interno",
	"como estas ahora mismo",
	"que estado tenes",
}

// ParseInternalStateQuery detecta la meta-pregunta sobre el estado
// interno — distinta de "en que etapa estas" (Bloque 11, sobre el
// PROCESO técnico) y de "que te importa ahora" (Bloque 12, sobre
// CONTENIDO ya filtrado): esta es sobre la disponibilidad de Ahmelia
// misma, "¿puedo hablarte o estás con algo?".
func ParseInternalStateQuery(normalizedText string) bool {
	for _, trigger := range internalStateTriggers {
		if strings.Contains(normalizedText, trigger) {
			return true
		}
	}
	return false
}
