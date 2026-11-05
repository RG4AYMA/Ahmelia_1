package comprehension

import (
	"strings"
	"sync"
	"time"
)

// Interaction representa una interacción completa usuario → Ahmelia
type Interaction struct {
	Input     string    // Lo que dijo el usuario
	Action    string    // Acción que ejecutó Ahmelia
	Target    string    // Target de la acción
	Timestamp time.Time // Cuándo ocurrió
}

// ConversationContext mantiene el historial de las últimas N interacciones
type ConversationContext struct {
	mu           sync.Mutex
	history      []Interaction
	maxSize      int
	currentTopic string // Tema activo de la conversación
}

// NewConversationContext crea un contexto nuevo
func NewConversationContext() *ConversationContext {
	return &ConversationContext{
		history: make([]Interaction, 0, 10),
		maxSize: 10,
	}
}

// Add agrega una interacción al historial
func (c *ConversationContext) Add(input, action, target string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	interaction := Interaction{
		Input:     input,
		Action:    action,
		Target:    target,
		Timestamp: time.Now(),
	}

	c.history = append(c.history, interaction)

	// Mantener solo las últimas maxSize interacciones
	if len(c.history) > c.maxSize {
		c.history = c.history[len(c.history)-c.maxSize:]
	}

	// Actualizar tema activo
	c.currentTopic = detectTopic(action, target)
}

// GetContext retorna las últimas N interacciones
func (c *ConversationContext) GetContext(steps int) []Interaction {
	c.mu.Lock()
	defer c.mu.Unlock()

	if steps <= 0 || len(c.history) == 0 {
		return nil
	}
	if steps > len(c.history) {
		steps = len(c.history)
	}
	// Retorna copia para evitar race conditions
	result := make([]Interaction, steps)
	copy(result, c.history[len(c.history)-steps:])
	return result
}

// GetLastAction retorna la última acción ejecutada
func (c *ConversationContext) GetLastAction() (string, string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if len(c.history) == 0 {
		return "", ""
	}
	last := c.history[len(c.history)-1]
	return last.Action, last.Target
}

// GetCurrentTopic retorna el tema activo
func (c *ConversationContext) GetCurrentTopic() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.currentTopic
}

// HasRecentAction verifica si una acción ocurrió en las últimas N interacciones
func (c *ConversationContext) HasRecentAction(action string, withinLast int) bool {
	recent := c.GetContext(withinLast)
	for _, i := range recent {
		if i.Action == action {
			return true
		}
	}
	return false
}

// ClearCurrentTopic removes only the active topic while retaining the
// interaction history. It is used for explicit conversational boundaries.
func (c *ConversationContext) ClearCurrentTopic() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.currentTopic = ""
}

// Clear limpia el historial
func (c *ConversationContext) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.history = c.history[:0]
	c.currentTopic = ""
}

// Summary retorna un resumen del contexto actual como string
func (c *ConversationContext) Summary(steps int) string {
	recent := c.GetContext(steps)
	if len(recent) == 0 {
		return ""
	}

	var parts []string
	for _, i := range recent {
		if i.Action != "" && i.Action != "desconocido" {
			parts = append(parts, i.Action+":"+i.Target)
		}
	}
	return strings.Join(parts, " → ")
}

// detectTopic infiere el tema activo basado en la acción
func detectTopic(action, target string) string {
	switch {
	case strings.HasPrefix(action, "volumen"):
		return "sistema_audio"
	case action == "abrir_programa":
		return "programa_" + target
	case action == "buscar_archivo":
		return "archivos"
	case action == "cambiar_ventana", action == "minimizar", action == "cerrar_ventana":
		return "navegacion_ventanas"
	case strings.HasPrefix(action, "reproducir"):
		return "media"
	default:
		return "general"
	}
}
