package memory_intelligence

import "sync"

// ============================================================================
// LEARNINGSTORE.GO — Bloque 93.3 (Ownership único sobre LearningState)
// ============================================================================
//
// Mismo criterio que EventStore, aplicado a LearningState. La diferencia:
// LearningState ya tenía casi todas sus mutaciones detrás de métodos
// (AdaptStrategy, NoteOutcome, UpdateFromReflection, Save) — el hueco real
// estaba en las LECTURAS: seis call-sites fuera de memory_intelligence
// leían ActionConfidence/Strategies/FailurePatternsByAction directo del
// mapa, sin pasar por ningún método. LearningStore cierra ese hueco: ya no
// se expone el *LearningState, todo pasa por acá.

// LearningStore protege el LearningState compartido entre la conversación y
// el ciclo autónomo.
type LearningStore struct {
	mu    sync.RWMutex
	state *LearningState
}

// NewLearningStore envuelve un LearningState ya cargado (o crea uno nuevo
// con estrategias por defecto si state es nil).
func NewLearningStore(state *LearningState) *LearningStore {
	if state == nil {
		state = NewLearningState()
	}
	return &LearningStore{state: state}
}

// LoadLearningStore carga learning.json y lo envuelve en un Store —
// reemplazo directo de LoadLearningState(path).
func LoadLearningStore(path string) *LearningStore {
	return NewLearningStore(LoadLearningState(path))
}

// Confidence reemplaza las lecturas directas de ActionConfidence[action]
// que antes hacían core/engine.go, solving/learning_validator.go,
// solving/knowledge_mapper.go, solving/strategy_selector.go y
// autonomy/memory_skill_adapter.go.
func (s *LearningStore) Confidence(action string) float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.state.ActionConfidence[action]
}

// HasConfidence distingue "0% de confianza real" de "sin historial" —
// mismo uso que antes hacía `_, ok := learningState.ActionConfidence[x]`.
func (s *LearningStore) HasConfidence(action string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.state.ActionConfidence[action]
	return ok
}

// Strategy reemplaza las lecturas directas de Strategies[action] (por
// ejemplo solving/learning_integration.go). Retorna una copia por valor:
// nadie fuera del Store vuelve a tener un *StrategySet mutable.
func (s *LearningStore) Strategy(action string) (StrategySet, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	strat, ok := s.state.Strategies[action]
	if !ok || strat == nil {
		return StrategySet{}, false
	}
	return *strat, true
}

// FailureCategories reemplaza la lectura directa de
// FailurePatternsByAction[action] (solving/failure_analysis.go). Retorna
// una copia.
func (s *LearningStore) FailureCategories(action string) map[string]int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	src := s.state.FailurePatternsByAction[action]
	out := make(map[string]int, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
}

func (s *LearningStore) SuggestStrategyChange(action string) (string, float64) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.state.SuggestStrategyChange(action)
}

func (s *LearningStore) AdaptStrategy(action string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state.AdaptStrategy(action)
}

func (s *LearningStore) NoteOutcome(action, strategyUsed string, success bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state.NoteOutcome(action, strategyUsed, success)
}

func (s *LearningStore) DescribeLearning() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.state.DescribeLearning()
}

// UpdateFromReflection recalcula ActionConfidence/FailurePatterns sobre un
// snapshot ya tomado del EventStore (events []RawEvent). El caller obtiene
// ese snapshot con EventStore.Snapshot() ANTES de llamar esto — a propósito
// LearningStore nunca toma el lock de EventStore por su cuenta, para que el
// orden de locks entre los dos Store no dependa de quién llama a quién.
func (s *LearningStore) UpdateFromReflection(events []RawEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state.UpdateFromReflection(&EventLog{Events: events})
}

// Save persiste learning.json.
func (s *LearningStore) Save(path string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state.Save(path)
}
