package planning

import (
	"fmt"
	"sync/atomic"
	"time"
)

// TaskStatus representa el estado de una tarea
type TaskStatus string

const (
	TaskPending    TaskStatus = "pending"
	TaskInProgress TaskStatus = "in_progress"
	TaskCompleted  TaskStatus = "completed"
	TaskBlocked    TaskStatus = "blocked"
	TaskCancelled  TaskStatus = "cancelled"
)

// Task representa una tarea descompuesta de un objetivo
type Task struct {
	ID          string
	GoalID      string
	Title       string
	Description string
	Order       int // Orden en el plan (1, 2, 3...)
	Status      TaskStatus
	Priority    GoalPriority

	// Estimación y seguimiento
	TimeEstimate time.Duration // Cuánto tiempo debería tomar
	TimeInvested time.Duration // Cuánto tiempo se invirtió realmente
	StartedAt    time.Time
	CompletedAt  time.Time
	CreatedAt    time.Time

	// Dependencias
	Dependencies []string // Task IDs que deben completarse primero

	// Métricas
	Success     bool // Si se completó exitosamente
	Errors      int  // Errores encontrados
	Attempts    int  // Intentos realizados
	BlockReason string
}

// NewTask crea una nueva tarea
func NewTask(goalID, title, description string, order int, timeEstimate time.Duration) *Task {
	return &Task{
		ID:           generateTaskID(),
		GoalID:       goalID,
		Title:        title,
		Description:  description,
		Order:        order,
		Status:       TaskPending,
		Priority:     PriorityMedium,
		TimeEstimate: timeEstimate,
		CreatedAt:    time.Now(),
		Dependencies: []string{},
		Success:      false,
		Errors:       0,
		Attempts:     0,
	}
}

// Start inicia la tarea
func (t *Task) Start() {
	t.Status = TaskInProgress
	t.StartedAt = time.Now()
	t.Attempts++
}

// Complete completa la tarea exitosamente
func (t *Task) Complete() {
	t.Status = TaskCompleted
	t.CompletedAt = time.Now()
	t.Success = true
	t.TimeInvested = time.Since(t.StartedAt)
}

// Fail marca la tarea como fallida
func (t *Task) Fail(reason string) {
	t.Status = TaskBlocked
	t.BlockReason = reason
	t.Success = false
	t.Errors++
}

// Retry resetea para intentar de nuevo
func (t *Task) Retry() {
	t.Status = TaskPending
	t.StartedAt = time.Time{}
	t.CompletedAt = time.Time{}
}

// IsCompleted verifica si está completada
func (t *Task) IsCompleted() bool {
	return t.Status == TaskCompleted
}

// IsBlocked verifica si está bloqueada
func (t *Task) IsBlocked() bool {
	return t.Status == TaskBlocked
}

// CanStart verifica si todas las dependencias están completadas
func (t *Task) CanStart(allTasks map[string]*Task) bool {
	for _, depID := range t.Dependencies {
		if dep, ok := allTasks[depID]; ok {
			if !dep.IsCompleted() {
				return false
			}
		}
	}
	return true
}

// GetVelocity retorna tareas/hora
func (t *Task) GetVelocity() float64 {
	if t.TimeInvested == 0 {
		return 0
	}
	hours := t.TimeInvested.Hours()
	if hours == 0 {
		return 0
	}
	return 1.0 / hours
}

// GetEfficiency retorna porcentaje de éxito (0-1)
func (t *Task) GetEfficiency() float64 {
	if t.Attempts == 0 {
		return 0
	}
	if t.Success {
		return 1.0 - (float64(t.Errors) / float64(t.Attempts))
	}
	return float64(t.Errors) / float64(t.Attempts)
}

// EstimatedRemaining retorna tiempo restante estimado
func (t *Task) EstimatedRemaining() time.Duration {
	if t.Status == TaskCompleted {
		return 0
	}
	if t.TimeInvested > t.TimeEstimate {
		return 0
	}
	return t.TimeEstimate - t.TimeInvested
}

// helper
//
// taskIDSeq es un contador atómico que se suma al timestamp para garantizar
// unicidad incluso cuando el reloj del sistema operativo no tiene
// resolución de nanosegundo real. En Windows, time.Now().UnixNano() puede
// devolver el mismo valor para varias llamadas seguidas dentro de la misma
// función (p. ej. decomposeGoal creando 4-5 tareas en el mismo tick), lo
// que producía IDs de Task duplicados y rompía cualquier lógica que
// comparara IDs para saber "es esta misma tarea" (nextRunnableTask,
// Task.CanStart, y hasPendingDependent de B87). El timestamp se conserva
// para que el ID siga siendo legible/ordenable; el contador es lo que
// realmente asegura que nunca se repita.
var taskIDSeq uint64

func generateTaskID() string {
	seq := atomic.AddUint64(&taskIDSeq, 1)
	return fmt.Sprintf("%d-%d-task", time.Now().UnixNano(), seq)
}
