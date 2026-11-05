package learning

import (
	"fmt"

	"ahmelia/memory"
)

/* =======================
   GENERALIZATION
   Qué hacer cuando no hay una coincidencia exacta: preguntar,
   o aceptar un recuerdo aproximado solo si su confianza supera
   el umbral pedido.
======================= */

// Ask genera la pregunta que Ahmelia hace cuando no sabe qué hacer con un input.
func (m *Manager) Ask(input string) string {
	return fmt.Sprintf("No sé qué hacer con \"%s\". Dime qué debería hacer", input)
}

// GetConfident recupera un Knowledge solo si su confianza supera minConfidence.
// Útil para el engine: no ejecutar algo poco confiable sin confirmar.
func (m *Manager) GetConfident(input string, minConfidence float64) (*memory.Knowledge, bool) {
	return m.longMemory.RecallConfident(input, minConfidence)
}
