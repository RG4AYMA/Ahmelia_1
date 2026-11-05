package learning

import (
	"fmt"
	"time"

	"ahmelia/comprehension"
	"ahmelia/memory"
)

/* =======================
   EXPERIENCE
   Registrar lo que Ahmelia vive: inputs, respuestas y
   acciones ejecutables, más el volcado/carga de contexto
   de sesión hacia y desde la memoria larga.
======================= */

// Learn guarda una respuesta de texto para un input (legacy).
func (m *Manager) Learn(input, response string) {
	m.longMemory.Learn(input, response)
}

// LearnAction guarda una acción ejecutable con todos los campos.
func (m *Manager) LearnAction(input, action, target string, params map[string]string) {
	m.longMemory.LearnAction(input, action, target, params)
}

// DumpContextToMemory guarda el contexto de la sesión actual a memoria larga.
// Llamar antes de cerrar Ahmelia para persistir el historial de la sesión.
func (m *Manager) DumpContextToMemory(interactions []comprehension.Interaction) {
	written := 0
	skipped := 0
	for _, interaction := range interactions {
		if interaction.Action == "" || interaction.Action == "desconocido" {
			skipped++
			fmt.Printf("📚 [DumpContext] Saltado (action vacío/desconocido): input=%q\n", interaction.Input)
			continue
		}

		key := interaction.Action + "_" + interaction.Target
		// Crear un Knowledge de tipo "contexto" para recordar qué se hizo
		m.longMemory.Data[key] = &memory.Knowledge{
			ID:         key,
			Type:       memory.KnowContext,
			Input:      interaction.Input,
			Action:     interaction.Action,
			Target:     interaction.Target,
			Importance: 0.3, // contexto = baja importancia
			Source:     memory.SourceLearned,
			CreatedAt:  time.Now(),
			LastUsed:   time.Now(),
		}
		written++
		fmt.Printf("📚 [DumpContext] Escrito key=%q action=%q target=%q\n", key, interaction.Action, interaction.Target)
	}
	m.longMemory.Save()
	fmt.Printf("📚 [DumpContext] Total: %d escritos, %d saltados, guardado en %s\n", written, skipped, memory.LongMemoryPath)
}

// LoadContextFromMemory recupera interacciones recientes de memoria para contexto inicial.
// Útil al iniciar para que Ahmelia tenga contexto de sesiones anteriores.
func (m *Manager) LoadContextFromMemory() []comprehension.Interaction {
	interactions := []comprehension.Interaction{}

	// Recuperar los últimos Knowledge de tipo "contexto"
	contextKnowledge := m.longMemory.RecallByType(memory.KnowContext)
	fmt.Printf("📚 [LoadContext] Entradas tipo 'contexto' encontradas en %s: %d\n", memory.LongMemoryPath, len(contextKnowledge))

	// Los últimos 10 (máximo del contexto)
	start := 0
	if len(contextKnowledge) > 10 {
		start = len(contextKnowledge) - 10
	}

	for _, k := range contextKnowledge[start:] {
		interactions = append(interactions, comprehension.Interaction{
			Input:  k.Input,
			Action: k.Action,
			Target: k.Target,
		})
	}

	return interactions
}
