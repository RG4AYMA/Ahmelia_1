package planning

import (
	"fmt"
)

// DependencyType describe el tipo de dependencia
type DependencyType string

const (
	DependsOn    DependencyType = "depends_on"   // A depende de B
	Blocks       DependencyType = "blocks"       // A bloquea B
	Related      DependencyType = "related"      // A está relacionado con B
	Prerequisite DependencyType = "prerequisite" // B es prerrequisito de A
)

// Dependency representa una relación entre objetivos/tareas
type Dependency struct {
	ID       string
	Source   string // ID de origen
	Target   string // ID de destino
	Type     DependencyType
	Reason   string // Por qué existe esta dependencia
	Critical bool   // Si es crítica (bloquea todo)
}

// DependencyGraph maneja todas las dependencias
type DependencyGraph struct {
	dependencies map[string][]Dependency
	goals        map[string]*Goal
	tasks        map[string]*Task
}

// NewDependencyGraph crea un nuevo grafo de dependencias
func NewDependencyGraph() *DependencyGraph {
	return &DependencyGraph{
		dependencies: make(map[string][]Dependency),
		goals:        make(map[string]*Goal),
		tasks:        make(map[string]*Task),
	}
}

// AddGoal añade un objetivo al grafo
func (dg *DependencyGraph) AddGoal(goal *Goal) {
	dg.goals[goal.ID] = goal
}

// AddTask añade una tarea al grafo
func (dg *DependencyGraph) AddTask(task *Task) {
	dg.tasks[task.ID] = task
}

// AddDependency añade una dependencia entre dos elementos
func (dg *DependencyGraph) AddDependency(source, target string, depType DependencyType, reason string, critical bool) error {
	dep := Dependency{
		ID:       fmt.Sprintf("%s->%s", source, target),
		Source:   source,
		Target:   target,
		Type:     depType,
		Reason:   reason,
		Critical: critical,
	}
	dg.dependencies[source] = append(dg.dependencies[source], dep)
	return nil
}

// GetDependencies retorna todas las dependencias de un elemento
func (dg *DependencyGraph) GetDependencies(id string) []Dependency {
	return dg.dependencies[id]
}

// GetCriticalPath retorna el camino crítico (ruta más larga)
func (dg *DependencyGraph) GetCriticalPath(startID string) []*Goal {
	path := []*Goal{}
	visited := make(map[string]bool)

	var traverse func(id string)
	traverse = func(id string) {
		if visited[id] {
			return
		}
		visited[id] = true

		if goal, ok := dg.goals[id]; ok {
			path = append(path, goal)

			for _, dep := range dg.dependencies[id] {
				if dep.Critical {
					traverse(dep.Target)
				}
			}
		}
	}

	traverse(startID)
	return path
}

// GetBlockers retorna los elementos que bloquean un objetivo/tarea
func (dg *DependencyGraph) GetBlockers(id string) []string {
	blockers := []string{}

	for _, dep := range dg.dependencies[id] {
		if dep.Type == DependsOn || dep.Type == Prerequisite {
			// Verificar si el target no está completado
			if goal, ok := dg.goals[dep.Target]; ok {
				if !goal.IsComplete() {
					blockers = append(blockers, dep.Target)
				}
			}
			if task, ok := dg.tasks[dep.Target]; ok {
				if !task.IsCompleted() {
					blockers = append(blockers, dep.Target)
				}
			}
		}
	}

	return blockers
}

// GetDependents retorna los elementos que dependen de uno dado
func (dg *DependencyGraph) GetDependents(id string) []string {
	dependents := []string{}

	for sourceID, deps := range dg.dependencies {
		for _, dep := range deps {
			if dep.Target == id && (dep.Type == DependsOn || dep.Type == Prerequisite) {
				dependents = append(dependents, sourceID)
			}
		}
	}

	return dependents
}

// DetectCircularDependencies detecta ciclos
func (dg *DependencyGraph) DetectCircularDependencies(id string) bool {
	visited := make(map[string]int) // 0=unvisited, 1=visiting, 2=visited

	var hasCycle func(current string) bool
	hasCycle = func(current string) bool {
		visited[current] = 1

		for _, dep := range dg.dependencies[current] {
			if visited[dep.Target] == 1 {
				return true // Ciclo detectado
			}
			if visited[dep.Target] == 0 && hasCycle(dep.Target) {
				return true
			}
		}

		visited[current] = 2
		return false
	}

	return hasCycle(id)
}

// GetExecutionOrder retorna el orden correcto de ejecución
func (dg *DependencyGraph) GetExecutionOrder(startID string) ([]*Goal, error) {
	// Verificar ciclos
	if dg.DetectCircularDependencies(startID) {
		return nil, fmt.Errorf("circular dependency detected for %s", startID)
	}

	executed := make(map[string]bool)
	order := []*Goal{}

	var execute func(id string) error
	execute = func(id string) error {
		if executed[id] {
			return nil
		}

		// Ejecutar dependencias primero
		blockers := dg.GetBlockers(id)
		for _, blocker := range blockers {
			if err := execute(blocker); err != nil {
				return err
			}
		}

		// Ejecutar este
		if goal, ok := dg.goals[id]; ok {
			order = append(order, goal)
			executed[id] = true
		}

		return nil
	}

	if err := execute(startID); err != nil {
		return nil, err
	}

	return order, nil
}

// SuggestOptimizations sugiere mejoras en el orden
func (dg *DependencyGraph) SuggestOptimizations(goalID string) []string {
	suggestions := []string{}

	if goal, ok := dg.goals[goalID]; ok {
		// Si hay muchas dependencias, sugiere paralelizar
		if len(goal.Dependencies) > 2 {
			suggestions = append(suggestions, "Este objetivo tiene muchas dependencias. Considera si algunas pueden ejecutarse en paralelo.")
		}

		// Si hay dependencias críticas, señala las más importantes
		criticalDeps := []string{}
		for _, dep := range dg.dependencies[goalID] {
			if dep.Critical {
				criticalDeps = append(criticalDeps, dep.Target)
			}
		}

		if len(criticalDeps) > 0 {
			suggestions = append(suggestions, fmt.Sprintf("Tienes %d dependencias críticas. Prioriza: %v", len(criticalDeps), criticalDeps))
		}
	}

	return suggestions
}
