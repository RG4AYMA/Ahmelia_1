package learning

/* =======================
   CONSOLIDATION
   Punto de extensión: hoy la persistencia ocurre inline en
   experience.go (m.longMemory.Save()). Cuando se necesite lógica
   de consolidación real -fusionar Knowledge repetidos, subir
   importancia de patrones frecuentes, etc.- va aquí, sin tocar
   experience.go ni la fachada en system/Learning.go.
======================= */

// Consolidate persiste el estado actual de la memoria de aprendizaje.
// Hoy es un simple Save(); es el lugar donde en el futuro se agregará
// lógica real de consolidación (fusionar conocimiento repetido, etc.).
func (m *Manager) Consolidate() {
	m.longMemory.Save()
}
