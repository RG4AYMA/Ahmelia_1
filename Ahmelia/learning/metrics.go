package learning

/* =======================
   METRICS
   Estadísticas agregadas de lo que Ahmelia ha aprendido.
======================= */

// GetStats retorna estadísticas agregadas de la memoria de aprendizaje.
func (m *Manager) GetStats() map[string]interface{} {
	return m.longMemory.GetStats()
}
