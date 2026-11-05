package learning

import (
	"ahmelia/memory"
)

// Manager orquesta todo el subsistema de aprendizaje (Capa 7).
// Es el único punto que conoce memory.LongMemory; el resto de los
// archivos de este paquete (experience.go, pattern.go, reinforcement.go,
// generalization.go, reflection.go, consolidation.go, forgetting.go,
// metrics.go) son métodos de Manager agrupados por responsabilidad.
type Manager struct {
	longMemory *memory.LongMemory
}

// NewManager crea el orquestador de aprendizaje sobre una memoria larga existente.
func NewManager(mem *memory.LongMemory) *Manager {
	return &Manager{longMemory: mem}
}
