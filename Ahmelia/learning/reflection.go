package learning

import (
	"ahmelia/memory"
)

/* =======================
   REFLECTION
   Consulta directa: "¿qué sé exactamente sobre este input?"
======================= */

// GetKnowledge recupera un Knowledge por input exacto.
func (m *Manager) GetKnowledge(input string) (*memory.Knowledge, bool) {
	return m.longMemory.Recall(input)
}
