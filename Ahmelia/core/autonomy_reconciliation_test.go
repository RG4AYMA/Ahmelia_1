package core

import (
	"context"
	"testing"
	"time"

	"ahmelia/autonomy"
	"ahmelia/planning"
)

// reconciliationTestMapper es un GoalActionMapper trivial (misma idea que
// b81TaskMapper en autonomy/goal_planner_test.go, reimplementado acá
// porque ese helper es interno al paquete autonomy): siempre resuelve
// cualquier Task a una ActionRequest válida, para poder probar que un
// adapter reconciliado por B90.6 de verdad puede seguir el ciclo
// (GetNextAction/UpdateState) sin depender de que el texto de la tarea
// mapee contra CapabilityResolver real (eso es alcance de B89, no de este
// test).
type reconciliationTestMapper struct{ calls int }

func (m *reconciliationTestMapper) MapTask(_ context.Context, goal *planning.Goal, task *planning.Task) (autonomy.ActionRequest, error) {
	m.calls++
	return autonomy.ActionRequest{
		ID:     task.ID,
		Action: "crear_carpeta",
		Target: task.Title,
		Parameters: map[string]string{
			"goal_id": goal.ID,
			"task_id": task.ID,
		},
	}, nil
}

// newReconciliationRuntime arma un planner/state/adapter frescos — mismo
// precondición que ApplyAutonomyPersistenceSnapshot exige (Planner recién
// construido).
func newReconciliationRuntime(t *testing.T) (*planning.Planner, *autonomy.AutonomousState, *autonomy.GoalPlanningAdapter, *reconciliationTestMapper) {
	t.Helper()
	planner := planning.NewPlanner()
	state := autonomy.New()
	monitor := autonomy.NewGoalMonitor(state, 0)
	mapper := &reconciliationTestMapper{}

	adapter, err := autonomy.NewGoalPlanningAdapter(state, monitor, planner, mapper, nil, nil)
	if err != nil {
		t.Fatalf("NewGoalPlanningAdapter: %v", err)
	}
	return planner, state, adapter, mapper
}

// TestApplyAutonomyPersistenceSnapshot_SnapshotVacioEsArranqueLimpio cubre
// el criterio 7 del diseño de B90.6: sin snapshot en disco,
// LoadAutonomyPersistenceSnapshot ya devuelve un snapshot vacío (B90.1);
// aplicarlo sobre runtime fresco no debe dejar ningún rastro.
func TestApplyAutonomyPersistenceSnapshot_SnapshotVacioEsArranqueLimpio(t *testing.T) {
	planner, state, adapter, _ := newReconciliationRuntime(t)
	snap := NewAutonomyPersistenceSnapshot()

	if err := ApplyAutonomyPersistenceSnapshot(snap, planner, state, adapter); err != nil {
		t.Fatalf("ApplyAutonomyPersistenceSnapshot() con snapshot vacío: %v", err)
	}

	if len(planner.Goals()) != 0 || len(planner.Tasks()) != 0 {
		t.Fatalf("planner no quedó vacío: Goals=%v Tasks=%v", planner.Goals(), planner.Tasks())
	}
	if state.CurrentGoal() != nil {
		t.Fatalf("state.CurrentGoal() = %+v, quería nil", state.CurrentGoal())
	}
	if len(state.PendingGoals()) != 0 {
		t.Fatalf("state.PendingGoals() = %v, quería vacío", state.PendingGoals())
	}
	if len(adapter.Plans()) != 0 || len(adapter.ActiveTasks()) != 0 {
		t.Fatalf("adapter no quedó vacío: Plans=%v ActiveTasks=%v", adapter.Plans(), adapter.ActiveTasks())
	}
	if len(adapter.Replanner().Regenerations()) != 0 {
		t.Fatalf("Regenerations() = %v, quería vacío", adapter.Replanner().Regenerations())
	}
}

// TestApplyAutonomyPersistenceSnapshot_ExigePlannerVacio cubre el criterio
// 6 (no merge accidental) del lado de Planner, que no tiene ningún método
// de reemplazo/reset: si ya tiene Goals/Tasks propios, la función debe
// fallar en vez de mergear en silencio.
func TestApplyAutonomyPersistenceSnapshot_ExigePlannerVacio(t *testing.T) {
	planner, state, adapter, _ := newReconciliationRuntime(t)
	planner.RegisterGoal(planning.NewGoal("Goal preexistente", "", planning.PriorityMedium, "user"))

	err := ApplyAutonomyPersistenceSnapshot(NewAutonomyPersistenceSnapshot(), planner, state, adapter)
	if err == nil {
		t.Fatal("esperaba error con un Planner que ya tiene Goals propios, got nil")
	}
}

// TestApplyAutonomyPersistenceSnapshot_TaskInProgressSeNormaliza cubre el
// criterio 3 del diseño: una Task que quedó TaskInProgress al morir el
// proceso se restaura como TaskPending; los estados terminales
// (Completed/Blocked/Cancelled) se conservan tal cual.
func TestApplyAutonomyPersistenceSnapshot_TaskInProgressSeNormaliza(t *testing.T) {
	planner, state, adapter, _ := newReconciliationRuntime(t)

	goal := planning.NewGoal("Goal con tareas mixtas", "", planning.PriorityMedium, "user")
	inProgress := planning.NewTask(goal.ID, "En curso al morir", "", 1, 0)
	inProgress.Status = planning.TaskInProgress
	completed := planning.NewTask(goal.ID, "Ya completada", "", 2, 0)
	completed.Status = planning.TaskCompleted
	blocked := planning.NewTask(goal.ID, "Bloqueada", "", 3, 0)
	blocked.Status = planning.TaskBlocked
	cancelled := planning.NewTask(goal.ID, "Cancelada", "", 4, 0)
	cancelled.Status = planning.TaskCancelled
	pending := planning.NewTask(goal.ID, "Ya pendiente", "", 5, 0)
	pending.Status = planning.TaskPending

	snap := NewAutonomyPersistenceSnapshot()
	snap.Goals = []*planning.Goal{goal}
	snap.Tasks = []*planning.Task{inProgress, completed, blocked, cancelled, pending}

	if err := ApplyAutonomyPersistenceSnapshot(snap, planner, state, adapter); err != nil {
		t.Fatalf("ApplyAutonomyPersistenceSnapshot(): %v", err)
	}

	if inProgress.Status != planning.TaskPending {
		t.Fatalf("Task que estaba TaskInProgress quedó %q, quería TaskPending", inProgress.Status)
	}
	if completed.Status != planning.TaskCompleted {
		t.Fatalf("Task TaskCompleted no debía cambiar, quedó %q", completed.Status)
	}
	if blocked.Status != planning.TaskBlocked {
		t.Fatalf("Task TaskBlocked no debía cambiar, quedó %q", blocked.Status)
	}
	if cancelled.Status != planning.TaskCancelled {
		t.Fatalf("Task TaskCancelled no debía cambiar, quedó %q", cancelled.Status)
	}
	if pending.Status != planning.TaskPending {
		t.Fatalf("Task TaskPending no debía cambiar, quedó %q", pending.Status)
	}
}

// TestApplyAutonomyPersistenceSnapshot_PreservaIdentidad es la propiedad
// central de B90 (ver nota de cabecera de autonomy_persistence.go): el
// mismo *Goal debe ser el que ve planner.Goals(), state.CurrentGoal() y
// state.PendingGoals(); el mismo *Task debe ser el que ve planner.Tasks()
// y el Plan cacheado en adapter.Plans().
func TestApplyAutonomyPersistenceSnapshot_PreservaIdentidad(t *testing.T) {
	planner, state, adapter, _ := newReconciliationRuntime(t)

	current := planning.NewGoal("Goal actual", "", planning.PriorityHigh, "user")
	pending1 := planning.NewGoal("Goal pendiente 1", "", planning.PriorityMedium, "user")
	pending1.ID = current.ID + "-p1" // IDs distintos garantizados (ver TECH-DEBT de B90.2)
	pending2 := planning.NewGoal("Goal pendiente 2", "", planning.PriorityMedium, "user")
	pending2.ID = current.ID + "-p2"

	task1 := planning.NewTask(current.ID, "Tarea 1", "", 1, time.Hour)
	task2 := planning.NewTask(current.ID, "Tarea 2", "", 2, time.Hour)

	snap := NewAutonomyPersistenceSnapshot()
	snap.Goals = []*planning.Goal{current, pending1, pending2}
	snap.Tasks = []*planning.Task{task1, task2}
	snap.Plans = []PersistedPlan{{
		GoalID:  current.ID,
		PlanID:  "plan-1",
		TaskIDs: []string{task1.ID, task2.ID},
	}}
	snap.CurrentGoalID = current.ID
	snap.PendingGoalIDs = []string{pending1.ID, pending2.ID}

	if err := ApplyAutonomyPersistenceSnapshot(snap, planner, state, adapter); err != nil {
		t.Fatalf("ApplyAutonomyPersistenceSnapshot(): %v", err)
	}

	// Identidad de Goal: mismo puntero en las tres vistas.
	if state.CurrentGoal() != current {
		t.Fatalf("state.CurrentGoal() no es el mismo puntero que el Goal del snapshot")
	}
	plannerGoalsByID := map[string]*planning.Goal{}
	for _, g := range planner.Goals() {
		plannerGoalsByID[g.ID] = g
	}
	if plannerGoalsByID[current.ID] != current {
		t.Fatalf("planner.Goals() no tiene el mismo puntero *Goal para CurrentGoal")
	}
	pendingByID := map[string]*planning.Goal{}
	for _, g := range state.PendingGoals() {
		pendingByID[g.ID] = g
	}
	if pendingByID[pending1.ID] != pending1 || pendingByID[pending2.ID] != pending2 {
		t.Fatalf("state.PendingGoals() no preserva los mismos punteros: %+v", state.PendingGoals())
	}
	if plannerGoalsByID[pending1.ID] != pending1 || plannerGoalsByID[pending2.ID] != pending2 {
		t.Fatalf("planner.Goals() no preserva los mismos punteros que PendingGoals")
	}

	// Identidad de Task: mismo puntero entre planner.Tasks() y el Plan
	// cacheado por el adapter.
	plannerTasksByID := map[string]*planning.Task{}
	for _, tsk := range planner.Tasks() {
		plannerTasksByID[tsk.ID] = tsk
	}
	if plannerTasksByID[task1.ID] != task1 || plannerTasksByID[task2.ID] != task2 {
		t.Fatalf("planner.Tasks() no preserva los mismos punteros *Task")
	}
	plan, ok := adapter.Plans()[current.ID]
	if !ok {
		t.Fatalf("adapter.Plans() no tiene entrada para %q", current.ID)
	}
	if len(plan.Tasks) != 2 || plan.Tasks[0] != task1 || plan.Tasks[1] != task2 {
		t.Fatalf("Plan.Tasks no preserva los mismos punteros *Task: %+v", plan.Tasks)
	}
}

// TestApplyAutonomyPersistenceSnapshot_ReemplazaNoMergeaStateYAdapter
// cubre el criterio 6 sobre AutonomousState/GoalPlanningAdapter/Replanner
// (que Planner no puede cubrir por no tener reset, ver test de arriba):
// aplicar un segundo snapshot sobre state/adapter que YA tenían contenido
// de una reconciliación anterior debe reemplazar ese contenido entero, no
// combinarlo. Usa un Planner nuevo en cada llamada porque
// ApplyAutonomyPersistenceSnapshot exige Planner vacío — lo que se prueba
// acá es el contrato de reemplazo de state/adapter/replanner, no de
// Planner.
func TestApplyAutonomyPersistenceSnapshot_ReemplazaNoMergeaStateYAdapter(t *testing.T) {
	state := autonomy.New()
	monitor := autonomy.NewGoalMonitor(state, 0)
	mapper := &reconciliationTestMapper{}

	firstPlanner := planning.NewPlanner()
	adapter, err := autonomy.NewGoalPlanningAdapter(state, monitor, firstPlanner, mapper, nil, nil)
	if err != nil {
		t.Fatalf("NewGoalPlanningAdapter: %v", err)
	}

	goalOld := planning.NewGoal("Goal viejo", "", planning.PriorityMedium, "user")
	taskOld := planning.NewTask(goalOld.ID, "Tarea vieja", "", 1, 0)
	snapOld := NewAutonomyPersistenceSnapshot()
	snapOld.Goals = []*planning.Goal{goalOld}
	snapOld.Tasks = []*planning.Task{taskOld}
	snapOld.Plans = []PersistedPlan{{GoalID: goalOld.ID, PlanID: "plan-viejo", TaskIDs: []string{taskOld.ID}}}
	snapOld.CurrentGoalID = goalOld.ID
	snapOld.ReplanRegenerations = map[string]int{goalOld.ID: 3}

	if err := ApplyAutonomyPersistenceSnapshot(snapOld, firstPlanner, state, adapter); err != nil {
		t.Fatalf("ApplyAutonomyPersistenceSnapshot() (1a carga): %v", err)
	}

	// Segunda reconciliación, con un Planner nuevo (el primero ya no está
	// vacío) pero el MISMO state/adapter — simula un segundo Load sobre
	// el mismo Engine.
	secondPlanner := planning.NewPlanner()
	goalNew := planning.NewGoal("Goal nuevo", "", planning.PriorityMedium, "user")
	snapNew := NewAutonomyPersistenceSnapshot()
	snapNew.Goals = []*planning.Goal{goalNew}
	snapNew.CurrentGoalID = goalNew.ID
	// Sin Plans ni ReplanRegenerations: la segunda carga debe dejar esas
	// dos cosas vacías, no conservar lo que trajo snapOld.

	if err := ApplyAutonomyPersistenceSnapshot(snapNew, secondPlanner, state, adapter); err != nil {
		t.Fatalf("ApplyAutonomyPersistenceSnapshot() (2a carga): %v", err)
	}

	if state.CurrentGoal() != goalNew {
		t.Fatalf("state.CurrentGoal() = %+v, quería goalNew (reemplazo, no merge)", state.CurrentGoal())
	}
	if plans := adapter.Plans(); len(plans) != 0 {
		t.Fatalf("adapter.Plans() = %v tras la 2a carga, quería vacío (snapNew no tenía Plans)", plans)
	}
	if regen := adapter.Replanner().Regenerations(); len(regen) != 0 {
		t.Fatalf("Regenerations() = %v tras la 2a carga, quería vacío (snapNew no tenía ReplanRegenerations)", regen)
	}
}

// TestApplyAutonomyPersistenceSnapshot_FallaHonestoSinAplicarNada cubre el
// criterio 8: un snapshot inválido (un Plan que referencia un Task.ID
// inexistente) debe fallar ANTES de tocar planner/state/adapter — nunca
// un runtime a medio reconciliar.
func TestApplyAutonomyPersistenceSnapshot_FallaHonestoSinAplicarNada(t *testing.T) {
	planner, state, adapter, _ := newReconciliationRuntime(t)

	goal := planning.NewGoal("Goal con plan corrupto", "", planning.PriorityMedium, "user")
	snap := NewAutonomyPersistenceSnapshot()
	snap.Goals = []*planning.Goal{goal}
	snap.Plans = []PersistedPlan{{
		GoalID:  goal.ID,
		PlanID:  "plan-corrupto",
		TaskIDs: []string{"task-que-no-existe"},
	}}

	err := ApplyAutonomyPersistenceSnapshot(snap, planner, state, adapter)
	if err == nil {
		t.Fatal("esperaba error por Plan referenciando un Task.ID inexistente, got nil")
	}

	if len(planner.Goals()) != 0 || len(planner.Tasks()) != 0 {
		t.Fatalf("planner no debía tener nada tras un snapshot inválido: Goals=%v Tasks=%v", planner.Goals(), planner.Tasks())
	}
	if state.CurrentGoal() != nil || len(state.PendingGoals()) != 0 {
		t.Fatalf("state no debía tener nada tras un snapshot inválido: CurrentGoal=%+v PendingGoals=%v", state.CurrentGoal(), state.PendingGoals())
	}
	if len(adapter.Plans()) != 0 {
		t.Fatalf("adapter no debía tener nada tras un snapshot inválido: Plans=%v", adapter.Plans())
	}
}

// TestApplyAutonomyPersistenceSnapshot_CicloRealTrasReconciliar es la
// prueba de "estado runtime válido -> Autonomous Cycle puede continuar"
// del diseño de B90.6: tras reconciliar, el adapter debe poder seguir
// operando con GetNextAction/UpdateState como si nunca hubiera habido un
// reinicio — sin generar ninguna tarea nueva, usando la que ya traía el
// plan reconstruido.
func TestApplyAutonomyPersistenceSnapshot_CicloRealTrasReconciliar(t *testing.T) {
	planner, state, adapter, mapper := newReconciliationRuntime(t)

	goal := planning.NewGoal("Goal a continuar", "", planning.PriorityMedium, "user")
	task := planning.NewTask(goal.ID, "Tarea a continuar", "", 1, 0)
	task.Status = planning.TaskInProgress // estaba en curso cuando "murió" el proceso

	snap := NewAutonomyPersistenceSnapshot()
	snap.Goals = []*planning.Goal{goal}
	snap.Tasks = []*planning.Task{task}
	snap.Plans = []PersistedPlan{{GoalID: goal.ID, PlanID: "plan-1", TaskIDs: []string{task.ID}, Feasible: true}}
	snap.CurrentGoalID = goal.ID

	if err := ApplyAutonomyPersistenceSnapshot(snap, planner, state, adapter); err != nil {
		t.Fatalf("ApplyAutonomyPersistenceSnapshot(): %v", err)
	}
	if task.Status != planning.TaskPending {
		t.Fatalf("setup: task.Status = %q, quería TaskPending tras reconciliar", task.Status)
	}

	req, ok, err := adapter.GetNextAction(context.Background())
	if err != nil || !ok {
		t.Fatalf("GetNextAction() tras reconciliar = (%+v, %v, %v), quería la tarea reconstruida disponible", req, ok, err)
	}
	if req.ID != task.ID {
		t.Fatalf("GetNextAction() devolvió %q, quería la tarea reconstruida %q — el ciclo generó una tarea nueva en vez de continuar con la persistida", req.ID, task.ID)
	}
	if mapper.calls != 1 {
		t.Fatalf("mapper.calls = %d, quería 1 (una sola tarea pendiente tras reconciliar)", mapper.calls)
	}
	if task.Status != planning.TaskInProgress {
		t.Fatalf("task.Status tras GetNextAction() = %q, quería TaskInProgress (GetNextAction sí puede arrancarla — la reconciliación en sí no ejecuta nada, pero el ciclo posterior sigue normal)", task.Status)
	}
}
