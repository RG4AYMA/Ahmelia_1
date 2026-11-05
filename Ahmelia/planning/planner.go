package planning

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync/atomic"
	"time"
)

// GoalTaskBlueprint is a provider-neutral description of one planned step.
// Dependencies are zero-based indexes into the same ordered blueprint list;
// requiring dependencies to point backward makes the projected graph acyclic
// by construction while still allowing fan-in/fan-out between steps.
type GoalTaskBlueprint struct {
	Title        string
	Description  string
	Dependencies []int
	TimeEstimate time.Duration
}

// GoalTaskDecomposer lets the canonical planner use a general reasoning
// capability instead of requiring a hard-coded case for every goal name.
// The planner remains the owner that validates and materializes Task objects.
type GoalTaskDecomposer interface {
	DecomposeGoal(context.Context, Goal) ([]GoalTaskBlueprint, error)
}

type planningContextKey struct{}

// WithEnvironmentBaseline attaches a bounded, read-only environment baseline to
// the planning request. It is context-scoped and is not persisted as goal state.
func WithEnvironmentBaseline(ctx context.Context, baseline string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, planningContextKey{}, strings.TrimSpace(baseline))
}

// EnvironmentBaseline returns the baseline supplied by the observation owner.
func EnvironmentBaseline(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	baseline, _ := ctx.Value(planningContextKey{}).(string)
	return baseline
}

// Plan representa un plan completo para lograr un objetivo
type Plan struct {
	ID            string
	GoalID        string
	CreatedAt     time.Time
	Tasks         []*Task
	TotalEstimate time.Duration
	Priority      GoalPriority
	Feasible      bool
	Issues        []string
}

// Planner genera planes automáticos basados en análisis
type Planner struct {
	goals        map[string]*Goal
	tasks        map[string]*Task
	dependencies *DependencyGraph
	decomposer   GoalTaskDecomposer
}

// planIDSeq — ver comentario de cabecera de GeneratePlan (hallazgo de
// B92.5). Global y no por-Planner a propósito: mismo criterio que
// taskIDSeq (task.go), el contador solo necesita garantizar unicidad
// dentro del proceso, no por instancia.
var planIDSeq uint64

// NewPlanner crea un nuevo planificador
func NewPlanner() *Planner {
	return &Planner{
		goals:        make(map[string]*Goal),
		tasks:        make(map[string]*Task),
		dependencies: NewDependencyGraph(),
	}
}

// SetGoalTaskDecomposer configures the reasoning-backed decomposition used by
// future plans. Existing cached plans are not mutated retroactively.
func (p *Planner) SetGoalTaskDecomposer(decomposer GoalTaskDecomposer) {
	if p == nil {
		return
	}
	p.decomposer = decomposer
}

// RegisterGoal registra un objetivo
func (p *Planner) RegisterGoal(goal *Goal) {
	p.goals[goal.ID] = goal
	p.dependencies.AddGoal(goal)
}

// RegisterTask registra una tarea
func (p *Planner) RegisterTask(task *Task) {
	p.tasks[task.ID] = task
	p.dependencies.AddTask(task)
}

// ============================================================================
// GOALS() / TASKS() — Bloque 90.2, Fase 6 (Persistencia del subsistema
// autónomo)
// ============================================================================
//
// Hasta este bloque, Planner no exponía ningún accessor de lectura sobre
// goals/tasks — quien necesitara enumerarlos no tenía forma pública de
// hacerlo (ver auditoría de B90). Goals()/Tasks() son estrictamente de
// lectura: no crean, no mutan, no reconstruyen nada, no tocan
// DependencyGraph. El único camino para agregar un Goal/Task sigue siendo
// RegisterGoal/RegisterTask.
//
// Devuelven un slice NUEVO (el caller no puede agregar/quitar elementos del
// mapa interno del Planner mutando el slice recibido) pero con los MISMOS
// punteros *Goal/*Task ya registrados — copiar los structs en sí
// reintroduciría exactamente el problema de identidad que B90 existe para
// evitar (un Goal que ve el Planner dejando de ser el mismo objeto que ve
// AutonomousState). El orden no está garantizado (goals/tasks son mapas
// por ID): quien consuma esto para persistencia reconcilia por ID, no por
// posición.

// Goals devuelve todos los Goals registrados hasta el momento.
func (p *Planner) Goals() []*Goal {
	out := make([]*Goal, 0, len(p.goals))
	for _, g := range p.goals {
		out = append(out, g)
	}
	return out
}

// Tasks devuelve todas las Tasks registradas hasta el momento.
func (p *Planner) Tasks() []*Task {
	out := make([]*Task, 0, len(p.tasks))
	for _, t := range p.tasks {
		out = append(out, t)
	}
	return out
}

// GeneratePlan crea automáticamente un plan para un objetivo
//
// HALLAZGO (auditoría B92.5): plan.ID usaba time.Now().Unix(), resolución
// de SEGUNDO sin contador — el mismo bug que ya se había corregido para
// planning.Goal.generateID() en B90.8.4 (ver ese archivo), pero nunca se
// aplicó acá. Dos GeneratePlan() para el mismo Goal en el mismo segundo de
// reloj —el caso normal de un test, y perfectamente posible en producción
// cuando Replanner regenera un plan justo después de crear el original—
// producían el MISMO Plan.ID, rompiendo cualquier comparación que use
// Plan.ID para distinguir "es este plan, o es el anterior" (el propio
// B92.5 lo detectó al verificar que una regeneración de plan REEMPLAZA,
// no reutiliza, el plan cacheado). Mismo patrón de corrección que
// Task.generateTaskID() (task.go) y Goal.generateID() (goal.go, B90.8.4)
// ya usan para el mismo problema: nanosegundo + contador atómico.
func (p *Planner) GeneratePlan(goal *Goal) *Plan {
	return p.GeneratePlanContext(context.Background(), goal)
}

// GeneratePlanContext preserves request cancellation and observation context
// through adaptive decomposition. GeneratePlan remains the compatibility entry.
func (p *Planner) GeneratePlanContext(ctx context.Context, goal *Goal) *Plan {
	if ctx == nil {
		ctx = context.Background()
	}
	seq := atomic.AddUint64(&planIDSeq, 1)
	plan := &Plan{
		ID:        fmt.Sprintf("plan-%d-%d", time.Now().UnixNano(), seq),
		GoalID:    goal.ID,
		CreatedAt: time.Now(),
		Tasks:     []*Task{},
		Priority:  goal.Priority,
		Issues:    []string{},
	}
	if err := ctx.Err(); err != nil {
		plan.Feasible = false
		plan.Issues = []string{"planning cancelled before decomposition: " + err.Error()}
		return plan
	}

	// Step 1: derive the task graph from the goal. The runtime may supply a
	// general decomposer; deterministic legacy decomposition remains the
	// fallback when the model capability is unavailable or returns invalid data.
	var tasks []*Task
	var planningWarnings []string
	if p.decomposer != nil {
		blueprints, err := p.decomposer.DecomposeGoal(ctx, *goal)
		if err == nil {
			tasks, err = materializeTaskBlueprints(goal, blueprints)
		}
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				plan.Feasible = false
				plan.Issues = []string{"planning cancelled during decomposition: " + ctxErr.Error()}
				return plan
			}
			tasks = p.decomposeGoal(goal)
			planningWarnings = append(planningWarnings, "adaptive decomposition unavailable; deterministic fallback used: "+err.Error())
		}
	} else {
		tasks = p.decomposeGoal(goal)
	}
	plan.Tasks = tasks

	// Step 2: deterministic fallback uses the legacy sequential dependencies;
	// adaptive blueprints already carry validated dependencies.
	if p.decomposer == nil || len(planningWarnings) > 0 {
		p.detectDependencies(tasks)
	}

	// Paso 3: Calcular total
	totalTime := time.Duration(0)
	for _, task := range tasks {
		totalTime += task.TimeEstimate
	}
	plan.TotalEstimate = totalTime

	// Paso 4: Validar viabilidad
	plan.Feasible, plan.Issues = p.validatePlan(plan)
	plan.Issues = append(plan.Issues, planningWarnings...)

	// Paso 5: Ordenar tareas por dependencias
	p.orderTasksByDependencies(plan)

	return plan
}

func materializeTaskBlueprints(goal *Goal, blueprints []GoalTaskBlueprint) ([]*Task, error) {
	if goal == nil {
		return nil, fmt.Errorf("goal is nil")
	}
	if len(blueprints) == 0 || len(blueprints) > 24 {
		return nil, fmt.Errorf("decomposer returned %d tasks; expected 1..24", len(blueprints))
	}
	tasks := make([]*Task, len(blueprints))
	for i, blueprint := range blueprints {
		title := strings.TrimSpace(blueprint.Title)
		description := strings.TrimSpace(blueprint.Description)
		if title == "" || description == "" {
			return nil, fmt.Errorf("task %d has empty title or description", i)
		}
		if len(title) > 160 || len(description) > 2000 {
			return nil, fmt.Errorf("task %d exceeds text limits", i)
		}
		estimate := blueprint.TimeEstimate
		if estimate <= 0 {
			estimate = time.Hour
		}
		tasks[i] = NewTask(goal.ID, title, description, i+1, estimate)
	}
	for i, blueprint := range blueprints {
		seen := make(map[int]struct{}, len(blueprint.Dependencies))
		for _, dep := range blueprint.Dependencies {
			if dep < 0 || dep >= i {
				return nil, fmt.Errorf("task %d dependency %d must reference an earlier task", i, dep)
			}
			if _, duplicate := seen[dep]; duplicate {
				return nil, fmt.Errorf("task %d repeats dependency %d", i, dep)
			}
			seen[dep] = struct{}{}
			tasks[i].Dependencies = append(tasks[i].Dependencies, tasks[dep].ID)
		}
	}
	return tasks, nil
}

// decomposeGoal descompone un objetivo en tareas
func (p *Planner) decomposeGoal(goal *Goal) []*Task {
	tasks := []*Task{}
	order := 1

	switch goal.Name {
	case "Mejorar Reasoner":
		tasks = append(tasks, NewTask(goal.ID, "Analizar módulos actuales", "Revisar analyzer, evaluator, hypothesis, decision, critic, explain", order, 2*time.Hour))
		order++
		tasks = append(tasks, NewTask(goal.ID, "Identificar cuellos de botella", "Detectar qué módulos producen respuestas genéricas", order, 1*time.Hour))
		order++
		tasks = append(tasks, NewTask(goal.ID, "Diseñar mejoras", "Especificar cambios a cada módulo", order, 3*time.Hour))
		order++
		tasks = append(tasks, NewTask(goal.ID, "Implementar cambios", "Escribir código mejorado", order, 6*time.Hour))
		order++
		tasks = append(tasks, NewTask(goal.ID, "Probar", "Validar que las respuestas son mejores", order, 2*time.Hour))
		order++

	case "Crear API Conversacional":
		tasks = append(tasks, NewTask(goal.ID, "Completar Planner (Capa 5)", "Terminar implementación del planificador", order, 8*time.Hour))
		order++
		tasks = append(tasks, NewTask(goal.ID, "Integrar Planner con Reasoning", "Conectar Capa 4 y 5", order, 3*time.Hour))
		order++
		tasks = append(tasks, NewTask(goal.ID, "Diseñar Generador de Lenguaje (Capa 6)", "Especificar arquitectura", order, 4*time.Hour))
		order++
		tasks = append(tasks, NewTask(goal.ID, "Implementar API REST", "Crear endpoints conversacionales", order, 8*time.Hour))
		order++
		tasks = append(tasks, NewTask(goal.ID, "Documentar y deployar", "Docs + servidor", order, 4*time.Hour))
		order++

	case "Optimizar Memory":
		tasks = append(tasks, NewTask(goal.ID, "Auditar estructura actual", "Revisar long.go y memory.go", order, 1*time.Hour))
		order++
		tasks = append(tasks, NewTask(goal.ID, "Identificar ineficiencias", "Búsquedas repetidas, índices faltantes", order, 2*time.Hour))
		order++
		tasks = append(tasks, NewTask(goal.ID, "Implementar índices", "Mejorar búsqueda", order, 4*time.Hour))
		order++
		tasks = append(tasks, NewTask(goal.ID, "Testear performance", "Medir mejoras", order, 2*time.Hour))
		order++

	default:
		// Descomposición genérica
		tasks = append(tasks, NewTask(goal.ID, "Análisis", "Entender el objetivo completamente", order, 2*time.Hour))
		order++
		tasks = append(tasks, NewTask(goal.ID, "Diseño", "Planificar la solución", order, 3*time.Hour))
		order++
		tasks = append(tasks, NewTask(goal.ID, "Implementación", "Ejecutar el plan", order, 6*time.Hour))
		order++
		tasks = append(tasks, NewTask(goal.ID, "Testing", "Validar resultados", order, 2*time.Hour))
		order++
	}

	return tasks
}

// detectDependencies detecta dependencias entre tareas
func (p *Planner) detectDependencies(tasks []*Task) {
	// Las tareas están ordenadas, cada una depende de la anterior
	for i := 1; i < len(tasks); i++ {
		tasks[i].Dependencies = append(tasks[i].Dependencies, tasks[i-1].ID)
	}
}

// validatePlan valida si el plan es viable
func (p *Planner) validatePlan(plan *Plan) (bool, []string) {
	issues := []string{}
	feasible := true

	// Validación 1: ¿Hay tareas?
	if len(plan.Tasks) == 0 {
		issues = append(issues, "No se pudieron generar tareas")
		feasible = false
	}

	// Validación 2: ¿Tiempo total es realista?
	if plan.TotalEstimate > 40*time.Hour {
		if plan.Priority != PriorityUrgent && plan.Priority != PriorityCritical {
			issues = append(issues, fmt.Sprintf("Tiempo estimado es alto (%.1f horas). Considera dividir en sub-objetivos", plan.TotalEstimate.Hours()))
		}
	}

	// Validación 3: ¿Hay circulares dependencias?
	for _, task := range plan.Tasks {
		if p.dependencies.DetectCircularDependencies(task.ID) {
			issues = append(issues, fmt.Sprintf("Dependencia circular detectada en tarea: %s", task.Title))
			feasible = false
		}
	}

	return feasible, issues
}

// orderTasksByDependencies ordena tareas respetando dependencias
func (p *Planner) orderTasksByDependencies(plan *Plan) {
	sort.Slice(plan.Tasks, func(i, j int) bool {
		taskI := plan.Tasks[i]
		taskJ := plan.Tasks[j]

		// Si j depende de i, i va primero
		for _, dep := range taskJ.Dependencies {
			if dep == taskI.ID {
				return true
			}
		}

		// Si i depende de j, j va primero
		for _, dep := range taskI.Dependencies {
			if dep == taskJ.ID {
				return false
			}
		}

		// Por defecto, mantener orden
		return taskI.Order < taskJ.Order
	})
}

// GetNextTask retorna la siguiente tarea a ejecutar
func (p *Planner) GetNextTask(plan *Plan) *Task {
	for _, task := range plan.Tasks {
		if task.Status == TaskPending {
			return task
		}
	}
	return nil
}

// EstimateCriticalPath retorna el tiempo mínimo para completar el plan
func (p *Planner) EstimateCriticalPath(plan *Plan) time.Duration {
	completed := make(map[string]time.Duration)

	for _, task := range plan.Tasks {
		maxDepTime := time.Duration(0)

		// Encontrar el tiempo máximo de las dependencias
		for _, depID := range task.Dependencies {
			if depTime, ok := completed[depID]; ok {
				if depTime > maxDepTime {
					maxDepTime = depTime
				}
			}
		}

		// El tiempo de esta tarea es el máximo de sus dependencias + su tiempo
		completed[task.ID] = maxDepTime + task.TimeEstimate
	}

	// El camino crítico es el máximo
	maxTime := time.Duration(0)
	for _, t := range completed {
		if t > maxTime {
			maxTime = t
		}
	}

	return maxTime
}

// AnalyzePlan proporciona análisis detallado del plan
func (p *Planner) AnalyzePlan(plan *Plan) string {
	criticalPath := p.EstimateCriticalPath(plan)
	parallelizable := p.countParallelizableTasks(plan)

	analysis := fmt.Sprintf(
		"ANÁLISIS DEL PLAN:\n"+
			"- Total de tareas: %d\n"+
			"- Tiempo estimado: %.1f horas\n"+
			"- Camino crítico: %.1f horas\n"+
			"- Tareas paralelizables: %d\n"+
			"- Viable: %v",
		len(plan.Tasks),
		plan.TotalEstimate.Hours(),
		criticalPath.Hours(),
		parallelizable,
		plan.Feasible,
	)

	if len(plan.Issues) > 0 {
		analysis += "\n\nPOTENTIALES PROBLEMAS:\n"
		for _, issue := range plan.Issues {
			analysis += fmt.Sprintf("- %s\n", issue)
		}
	}

	return analysis
}

// countParallelizableTasks cuenta tareas que pueden ejecutarse en paralelo
func (p *Planner) countParallelizableTasks(plan *Plan) int {
	count := 0
	for i := 0; i < len(plan.Tasks); i++ {
		for j := i + 1; j < len(plan.Tasks); j++ {
			taskI := plan.Tasks[i]
			taskJ := plan.Tasks[j]

			// Si no hay dependencia entre ellas, pueden ejecutarse en paralelo
			canParallelize := true
			for _, depI := range taskI.Dependencies {
				if depI == taskJ.ID {
					canParallelize = false
					break
				}
			}
			for _, depJ := range taskJ.Dependencies {
				if depJ == taskI.ID {
					canParallelize = false
					break
				}
			}

			if canParallelize {
				count++
			}
		}
	}
	return count
}
