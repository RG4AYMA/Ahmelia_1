package memory_intelligence

import (
	"sync"
	"time"
)

// ============================================================================
// EVENTSTORE.GO — Bloque 93.3 (Ownership único sobre EventLog)
// ============================================================================
//
// B93.2 confirmó el hueco: EventLog no tenía mutex propio y se tocaba desde
// dos goroutines reales (conversación, vía core/engine.go directo, y ciclo
// autónomo, vía autonomy.LearningMemoryAdapter). Un mutex propio en EventLog
// hubiera resuelto "no simultáneo", pero seguía dejando que cualquiera
// mutara el slice directamente. EventStore es el dueño único: nadie fuera de
// este archivo vuelve a hacer `eventLog.Events = append(...)` ni a rangear
// `log.Events` sin lock.
//
// Decisión de diseño: en vez de reescribir toda la lógica de lectura que ya
// vive en experience.go/habits.go/insights.go/reflection.go/sequence.go
// (que reciben *EventLog y no tienen estado propio, solo leen), EventStore
// las reusa tal cual — les pasa un *EventLog "prestado" armado a partir de
// un Snapshot() (copia). Esa copia nunca se comparte con nadie más, así que
// esas funciones siguen sin necesitar saber nada de locks.

// EventStore protege el EventLog compartido entre la conversación y el
// ciclo autónomo. Todas las mutaciones y lecturas del historial pasan por
// acá — el *EventLog subyacente nunca se expone.
type EventStore struct {
	mu  sync.RWMutex
	log *EventLog
}

// NewEventStore envuelve un EventLog ya cargado (o arranca uno vacío si log
// es nil, mismo criterio que LoadEventLog).
func NewEventStore(log *EventLog) *EventStore {
	if log == nil {
		log = &EventLog{Events: []RawEvent{}}
	}
	return &EventStore{log: log}
}

// LoadEventStore carga events.json y lo envuelve en un Store — reemplazo
// directo de LoadEventLog() para todo caller que necesite el Store.
func LoadEventStore() *EventStore {
	return NewEventStore(LoadEventLog())
}

// Append agrega un evento y persiste de inmediato — mismo contrato que
// EventLog.Record, ahora serializado.
func (s *EventStore) Append(action, target string, outcome Outcome, sessionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.log.Record(action, target, outcome, sessionID)
}

// AppendOnce stores one canonical experience once across interactive and
// autonomous callers. The ID is the durable work/attempt identity, not text.
func (s *EventStore) AppendOnce(action, target string, outcome Outcome, sessionID, experienceID string, metadata ...ExperienceMetadata) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if experienceID != "" {
		for _, event := range s.log.Events {
			if event.ExperienceID == experienceID {
				return false
			}
		}
	}
	event := RawEvent{
		Action: action, Target: target, Outcome: outcome,
		Timestamp: time.Now(), SessionID: sessionID, ExperienceID: experienceID,
	}
	if len(metadata) > 0 {
		event.Metadata = metadata[0]
	}
	s.log.Events = append(s.log.Events, event)
	s.log.Save()
	return true
}

// Snapshot retorna una copia del historial completo. Segura de recorrer sin
// lock porque el caller es el único dueño de esa copia.
func (s *EventStore) Snapshot() []RawEvent {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]RawEvent, len(s.log.Events))
	copy(out, s.log.Events)
	return out
}

// SnapshotLog es Snapshot() envuelto en un *EventLog "prestado" — puente
// para las funciones de memory_intelligence/solving que todavía reciben
// *EventLog (BuildChains, FindAnalogies, etc.). El *EventLog que retorna no
// es el real: es una copia de un solo uso, nunca hay que guardarla ni
// pasarla dos veces.
func (s *EventStore) SnapshotLog() *EventLog {
	return &EventLog{Events: s.Snapshot()}
}

// Len evita copiar todo el slice solo para comparar tamaños — mismo uso que
// ya hacían context_cache.go / reasoning_cache.go contra len(EventLog.Events)
// para decidir si invalidar un caché.
func (s *EventStore) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.log.Events)
}

// --- Atajos sobre las funciones de lectura existentes (sin estado propio,
// solo evitan que el caller tenga que armar el SnapshotLog() a mano) ---

func (s *EventStore) BuildChains(maxLen int) []Chain {
	return BuildChains(s.SnapshotLog(), maxLen)
}

func (s *EventStore) DetectHabits(minCount, minDays int) []Habit {
	return DetectHabits(s.SnapshotLog(), minCount, minDays)
}

func (s *EventStore) ExperienceForSubject(subject string) (ExperienceStats, bool) {
	return ExperienceForSubject(s.SnapshotLog(), subject)
}

func (s *EventStore) GenerateInsights() []string {
	return GenerateInsights(s.SnapshotLog())
}

func (s *EventStore) GenerateReflections() []string {
	return GenerateReflections(s.SnapshotLog())
}
