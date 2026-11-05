package planning

import (
	"fmt"
	"sync/atomic"
	"time"
)

// GoalStatus representa el estado de un objetivo
type GoalStatus string

const (
	GoalActive    GoalStatus = "active"
	GoalCompleted GoalStatus = "completed"
	GoalBlocked   GoalStatus = "blocked"
	GoalPaused    GoalStatus = "paused"
)

// GoalPriority es la prioridad de un objetivo
type GoalPriority int

const (
	PriorityLow    GoalPriority = 1
	PriorityMedium GoalPriority = 2
	PriorityHigh   GoalPriority = 3
	PriorityCritical GoalPriority = 4
	PriorityUrgent GoalPriority = 5
)

// Goal representa un objetivo que Ahmelia puede perseguir
type Goal struct {
	ID              string
	Name            string
	Description     string
	Status          GoalStatus
	Priority        GoalPriority
	Confidence      float64      // 0.0 - 1.0 (qué tan segura está)
	Owner           string       // "user" | "ahmelia"
	CreatedAt       time.Time
	Deadline        time.Time
	EstimatedHours  int
	CompletedAt     time.Time
	
	// Relaciones
	Dependencies    []string     // Goal IDs que deben completarse primero
	DependentOn     []string     // Goal IDs que dependen de este
	
	// Progreso
	Progress        float64      // 0.0 - 1.0
	TasksCompleted  int
	TasksTotal      int
	
	// Bloqueos
	Blockers        []string     // Razones por las que está bloqueado
	BlockedAt       time.Time
	
	// Tracking
	TimeInvested    time.Duration
	LastUpdated     time.Time
	Notes           []string
}

// NewGoal crea un nuevo objetivo
func NewGoal(name, description string, priority GoalPriority, owner string) *Goal {
	return &Goal{
		ID:          generateID(),
		Name:        name,
		Description: description,
		Status:      GoalActive,
		Priority:    priority,
		Confidence:  0.5,
		Owner:       owner,
		CreatedAt:   time.Now(),
		LastUpdated: time.Now(),
		Dependencies: []string{},
		DependentOn: []string{},
		Blockers:    []string{},
		Notes:       []string{},
	}
}

// IsComplete verifica si el objetivo está completado
func (g *Goal) IsComplete() bool {
	return g.Status == GoalCompleted
}

// IsBlocked verifica si está bloqueado
func (g *Goal) IsBlocked() bool {
	return g.Status == GoalBlocked
}

// CanStart verifica si todas las dependencias están completadas
func (g *Goal) CanStart(allGoals map[string]*Goal) bool {
	for _, depID := range g.Dependencies {
		if dep, ok := allGoals[depID]; ok {
			if !dep.IsComplete() {
				return false
			}
		}
	}
	return true
}

// GetDependencyChain retorna el orden correcto de ejecución
func (g *Goal) GetDependencyChain(allGoals map[string]*Goal) []*Goal {
	chain := []*Goal{}
	visited := make(map[string]bool)
	
	var traverse func(goalID string)
	traverse = func(goalID string) {
		if visited[goalID] {
			return
		}
		visited[goalID] = true
		
		if goal, ok := allGoals[goalID]; ok {
			for _, depID := range goal.Dependencies {
				traverse(depID)
			}
			chain = append(chain, goal)
		}
	}
	
	traverse(g.ID)
	return chain
}

// SetBlocked marca el objetivo como bloqueado
func (g *Goal) SetBlocked(reason string) {
	g.Status = GoalBlocked
	g.Blockers = append(g.Blockers, reason)
	g.BlockedAt = time.Now()
}

// Unblock desbloquea el objetivo
func (g *Goal) Unblock(reason string) {
	if g.Status == GoalBlocked {
		g.Status = GoalActive
		g.Blockers = []string{}
		g.BlockedAt = time.Time{}
		g.AddNote("Desbloqueado: " + reason)
	}
}

// Complete marca el objetivo como completado
func (g *Goal) Complete() {
	g.Status = GoalCompleted
	g.Progress = 1.0
	g.CompletedAt = time.Now()
}

// AddNote añade una nota al objetivo
func (g *Goal) AddNote(note string) {
	g.Notes = append(g.Notes, note)
	g.LastUpdated = time.Now()
}

// UpdateProgress actualiza el progreso
func (g *Goal) UpdateProgress(completed, total int) {
	g.TasksCompleted = completed
	g.TasksTotal = total
	if total > 0 {
		g.Progress = float64(completed) / float64(total)
	}
	g.LastUpdated = time.Now()
}

// helper para generar IDs
//
// HALLAZGO (auditoría B90.8.4, TestB9084_GoalsIndependientes): esta función
// solo tenía resolución de SEGUNDO ("20060102150405"), sin contador. Dos
// NewGoal() en el mismo segundo de reloj —el caso normal en un test, o en
// cualquier ráfaga real de creación de Goals— producían el MISMO ID. Ese
// bug ya estaba documentado como "preexistente, fuera de alcance" en
// planner_state_access_test.go (TestPlanner_GoalsDevuelveLosRegistrados,
// que lo esquivaba a mano con `g2.ID = g2.ID + "-2"`), pero seguía sin
// corregirse acá. TestB9084_GoalsIndependientes lo convirtió en un fallo
// real: mapper.failGoalID se fijaba al ID de "Goal A", y como "Goal B" se
// creó en el mismo segundo, terminaba con el ID IDÉNTICO — el mapper
// fallaba para los dos, no solo para A, y el test reportaba "Goal B no
// pudo continuar" cuando el problema nunca fue la independencia de
// Goals, sino la colisión de IDs.
//
// La corrección replica el mismo patrón que Task.generateTaskID() ya usa
// (mismo archivo, task.go) para el mismo problema: UnixNano() + contador
// atómico. El timestamp se conserva para que el ID siga siendo
// legible/ordenable; el contador es lo que realmente garantiza que nunca
// se repita, incluso si el reloj del sistema no tiene resolución real de
// nanosegundo (el mismo caso Windows que motivó goalIDSeq en task.go).
var goalIDSeq uint64

func generateID() string {
	seq := atomic.AddUint64(&goalIDSeq, 1)
	return fmt.Sprintf("%s-%d-goal", time.Now().Format("20060102150405"), seq)
}
