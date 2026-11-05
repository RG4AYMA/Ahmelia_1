package learning

import (
	"ahmelia/memory"
)

/* =======================
   PATTERN
   Reconocer y navegar patrones ya aprendidos: agrupar por
   tema (tag), por tipo de conocimiento, y conectar comandos
   que llevan a la misma acción.
======================= */

// GetByTag recupera todos los Knowledge de un tema.
// Ej: GetByTag("audio") → comandos de volumen.
func (m *Manager) GetByTag(tag string) []*memory.Knowledge {
	return m.longMemory.RecallByTag(tag)
}

// GetByType recupera todos los Knowledge de un tipo.
// Ej: GetByType(memory.KnowError) → todos los errores registrados.
func (m *Manager) GetByType(ktype memory.KnowType) []*memory.Knowledge {
	return m.longMemory.RecallByType(ktype)
}

// Relate conecta dos comandos en la memoria para que la Capa 3
// pueda navegar desde uno hasta el otro. El engine llama esto
// cuando detecta que dos inputs producen la misma acción.
func (m *Manager) Relate(inputA, inputB string) {
	m.longMemory.Relate(inputA, inputB)
}
