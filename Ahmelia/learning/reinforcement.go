package learning

/* =======================
   REINFORCEMENT
   El engine llama estos métodos después de ejecutar una acción
   para que la memoria sepa si funcionó o no, ajustando confianza.
======================= */

// ReportSuccess: la acción se ejecutó bien → sube confianza.
func (m *Manager) ReportSuccess(input string) {
	m.longMemory.UpdateConfidence(input, true)
}

// ReportFailure: la acción falló → baja confianza, registra el error.
func (m *Manager) ReportFailure(input, action, reason string) {
	m.longMemory.UpdateConfidence(input, false)
	m.longMemory.RegisterError(input, action, reason)
}
