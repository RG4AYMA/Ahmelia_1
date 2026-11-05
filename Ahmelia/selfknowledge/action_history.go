package selfknowledge

import (
	"fmt"
	"sync"
	"time"
)

// Action representa una acción ejecutada por Ahmelia
type Action struct {
	ID        string    // ID único
	Input     string    // Lo que dijo el usuario
	Action    string    // Acción ejecutada
	Target    string    // Target de la acción
	Result    string    // Resultado/respuesta
	Success   bool      // ¿Funcionó?
	Timestamp time.Time // Cuándo pasó
	Duration  int       // Milisegundos que tardó
}

// ActionHistory mantiene las últimas N acciones en RAM (rápido)
type ActionHistory struct {
	mu       sync.Mutex
	history  []Action
	maxSize  int // máximo de acciones a recordar
	actionID int // contador para IDs únicos
}

// NewActionHistory crea un historial nuevo
func NewActionHistory(maxSize int) *ActionHistory {
	if maxSize <= 0 {
		maxSize = 20
	}
	return &ActionHistory{
		history: make([]Action, 0, maxSize),
		maxSize: maxSize,
		actionID: 0,
	}
}

// Record registra una acción
func (ah *ActionHistory) Record(input, action, target, result string, success bool, durationMs int) {
	ah.mu.Lock()
	defer ah.mu.Unlock()

	ah.actionID++
	act := Action{
		ID:        fmt.Sprintf("action_%d", ah.actionID),
		Input:     input,
		Action:    action,
		Target:    target,
		Result:    result,
		Success:   success,
		Timestamp: time.Now(),
		Duration:  durationMs,
	}

	ah.history = append(ah.history, act)

	// Mantener solo las últimas maxSize
	if len(ah.history) > ah.maxSize {
		ah.history = ah.history[len(ah.history)-ah.maxSize:]
	}
}

// GetLast retorna las últimas N acciones
func (ah *ActionHistory) GetLast(n int) []Action {
	ah.mu.Lock()
	defer ah.mu.Unlock()

	if n <= 0 || len(ah.history) == 0 {
		return nil
	}
	if n > len(ah.history) {
		n = len(ah.history)
	}

	result := make([]Action, n)
	copy(result, ah.history[len(ah.history)-n:])
	return result
}

// GetLastAction retorna la acción más reciente
func (ah *ActionHistory) GetLastAction() *Action {
	ah.mu.Lock()
	defer ah.mu.Unlock()

	if len(ah.history) == 0 {
		return nil
	}
	return &ah.history[len(ah.history)-1]
}

// GetLastSuccess retorna la última acción exitosa
func (ah *ActionHistory) GetLastSuccess() *Action {
	ah.mu.Lock()
	defer ah.mu.Unlock()

	for i := len(ah.history) - 1; i >= 0; i-- {
		if ah.history[i].Success {
			return &ah.history[i]
		}
	}
	return nil
}

// GetLastError retorna el último error
func (ah *ActionHistory) GetLastError() *Action {
	ah.mu.Lock()
	defer ah.mu.Unlock()

	for i := len(ah.history) - 1; i >= 0; i-- {
		if !ah.history[i].Success {
			return &ah.history[i]
		}
	}
	return nil
}

// GetByAction retorna todas las acciones de un tipo específico
func (ah *ActionHistory) GetByAction(actionType string) []Action {
	ah.mu.Lock()
	defer ah.mu.Unlock()

	results := []Action{}
	for _, act := range ah.history {
		if act.Action == actionType {
			results = append(results, act)
		}
	}
	return results
}

// GetSuccessRate retorna porcentaje de éxito general
func (ah *ActionHistory) GetSuccessRate() float64 {
	ah.mu.Lock()
	defer ah.mu.Unlock()

	if len(ah.history) == 0 {
		return 0.0
	}

	successes := 0
	for _, act := range ah.history {
		if act.Success {
			successes++
		}
	}
	return float64(successes) / float64(len(ah.history)) * 100
}

// GetStats retorna estadísticas del historial
func (ah *ActionHistory) GetStats() map[string]interface{} {
	ah.mu.Lock()
	defer ah.mu.Unlock()

	totalActions := len(ah.history)
	totalSuccesses := 0
	totalFailures := 0
	totalDuration := 0
	actionCounts := make(map[string]int)

	for _, act := range ah.history {
		if act.Success {
			totalSuccesses++
		} else {
			totalFailures++
		}
		totalDuration += act.Duration
		actionCounts[act.Action]++
	}

	avgDuration := 0
	if totalActions > 0 {
		avgDuration = totalDuration / totalActions
	}

	return map[string]interface{}{
		"total_actions":   totalActions,
		"successes":       totalSuccesses,
		"failures":        totalFailures,
		"success_rate":    float64(totalSuccesses) / float64(totalActions) * 100,
		"avg_duration_ms": avgDuration,
		"action_counts":   actionCounts,
	}
}

// Clear limpia el historial
func (ah *ActionHistory) Clear() {
	ah.mu.Lock()
	defer ah.mu.Unlock()
	ah.history = ah.history[:0]
}
