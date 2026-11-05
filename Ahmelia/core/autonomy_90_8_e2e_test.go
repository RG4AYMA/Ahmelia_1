package core

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"ahmelia/autonomy"
	"ahmelia/planning"
)

// failingE2EMapper fuerza un fallo estable de resolución de capacidad para
// comprobar B90.8.3 y, sobre todo, que el estado bloqueado sobrevive al
// snapshot/reinicio.
type failingE2EMapper struct {
	calls      int
	failGoalID string
}

func (m *failingE2EMapper) MapTask(_ context.Context, goal *planning.Goal, task *planning.Task) (autonomy.ActionRequest, error) {
	m.calls++
	if m.failGoalID == "" || goal == nil || goal.ID == m.failGoalID {
		return autonomy.ActionRequest{}, errors.New("capacidad deliberadamente no resoluble")
	}
	return autonomy.ActionRequest{ID: task.ID, Action: "noop"}, nil
}

// TestB9084_PlanUnicoYContinuacionDelCiclo prueba el contrato completo
// B46 -> EnsurePlan -> ciclo autónomo: el Plan es único, sus Tasks son las
// registradas en Planner y el ciclo continúa sobre esas mismas Tasks.
func TestB9084_PlanUnicoYContinuacionDelCiclo(t *testing.T) {
	planner, state, adapter, _ := newReconciliationRuntime(t)
	goal := planning.NewGoal("E2E B90.8.4", "desc", planning.PriorityHigh, "user")
	planner.RegisterGoal(goal)
	state.SetCurrentGoal(goal)

	plan, err := adapter.EnsurePlan(goal)
	if err != nil {
		t.Fatalf("EnsurePlan: %v", err)
	}
	if len(plan.Tasks) == 0 {
		t.Fatal("EnsurePlan produjo un plan sin Tasks")
	}

	req, ok, err := adapter.GetNextAction(context.Background())
	if err != nil || !ok {
		t.Fatalf("GetNextAction = %+v, %v, %v", req, ok, err)
	}
	if req.ID != plan.Tasks[0].ID {
		t.Fatalf("GetNextAction usó Task %q, quería la Task cacheada %q", req.ID, plan.Tasks[0].ID)
	}

	if err := adapter.UpdateState(context.Background(), req, autonomy.VerificationResult{Success: true}); err != nil {
		t.Fatalf("UpdateState: %v", err)
	}
	if !plan.Tasks[0].IsCompleted() {
		t.Fatalf("la Task %q no quedó completada", plan.Tasks[0].ID)
	}
	if adapter.Plans()[goal.ID] != plan {
		t.Fatal("el adapter dejó de apuntar al mismo Plan después del ciclo")
	}
}

// TestB9084_SaveLoadRebuildYContinua prueba el recorrido completo:
// runtime -> snapshot -> disco -> runtime nuevo -> reconciliación -> vistas
// ejecutivas -> siguiente acción. No alcanza con que JSON cargue: el ciclo
// debe poder continuar usando el estado restaurado.
func TestB9084_SaveLoadRebuildYContinua(t *testing.T) {
	planner, state, adapter, _ := newReconciliationRuntime(t)
	goal := planning.NewGoal("E2E reinicio B90.8.4", "desc", planning.PriorityMedium, "user")
	planner.RegisterGoal(goal)
	state.SetCurrentGoal(goal)

	req, ok, err := adapter.GetNextAction(context.Background())
	if err != nil || !ok {
		t.Fatalf("GetNextAction inicial = %+v, %v, %v", req, ok, err)
	}
	if err := adapter.UpdateState(context.Background(), req, autonomy.VerificationResult{Success: true}); err != nil {
		t.Fatalf("UpdateState inicial: %v", err)
	}

	path := filepath.Join(t.TempDir(), "autonomy_state.json")
	if err := buildAutonomyPersistenceSnapshot(planner, state, adapter).Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	loaded, err := LoadAutonomyPersistenceSnapshot(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	newPlanner, newState, newAdapter, _ := newReconciliationRuntime(t)
	if err := ApplyAutonomyPersistenceSnapshot(loaded, newPlanner, newState, newAdapter); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	goals := map[string]*planning.Goal{}
	plans := map[string]*planning.Plan{}
	progress := map[string]*planning.Progress{}
	rebuildExecutiveViews(newPlanner, newAdapter, goals, plans, progress)

	restoredGoal := goals[goal.Name]
	if restoredGoal == nil {
		// El índice ejecutivo usa Normalize; para este nombre simple ambos
		// accesos son equivalentes, pero dejamos la búsqueda por ID como
		// respaldo para que el test falle por una razón realmente útil.
		for _, g := range newPlanner.Goals() {
			if g.ID == goal.ID {
				restoredGoal = g
				break
			}
		}
	}
	if restoredGoal == nil {
		t.Fatalf("Goal %q no fue reconstruido en la vista ejecutiva", goal.ID)
	}
	restoredPlan := newAdapter.Plans()[goal.ID]
	if restoredPlan == nil || plans[goal.ID] != restoredPlan {
		t.Fatalf("Plan restaurado no coincide con adapter.Plans(): %p vs %p", plans[goal.ID], restoredPlan)
	}
	if progress[goal.ID] == nil || progress[goal.ID].TasksCompleted < 1 {
		t.Fatalf("progreso restaurado inválido: %+v", progress[goal.ID])
	}

	// El runtime restaurado debe poder continuar; no se crea un Plan nuevo.
	newState.SetCurrentGoal(restoredGoal)
	next, ok, err := newAdapter.GetNextAction(context.Background())
	if err != nil || !ok {
		t.Fatalf("GetNextAction tras reinicio = %+v, %v, %v", next, ok, err)
	}
	if next.ID == req.ID {
		t.Fatalf("tras reinicio volvió a ofrecer la Task ya completada %q", req.ID)
	}
	if newAdapter.Plans()[goal.ID] != restoredPlan {
		t.Fatal("GetNextAction tras reinicio generó/reemplazó el Plan")
	}
}

// TestB9084_BlockedPersisteYNoReintenta prueba el otro extremo del ciclo:
// incapacidad persistente -> Blocked -> persistencia -> reinicio -> no se
// vuelve a invocar MapTask. Esto demuestra que B90.8.3 no deja un loop vivo
// después del reinicio.
func TestB9084_BlockedPersisteYNoReintenta(t *testing.T) {
	planner := planning.NewPlanner()
	state := autonomy.New()
	monitor := autonomy.NewGoalMonitor(state, 0)
	mapper := &failingE2EMapper{}
	adapter, err := autonomy.NewGoalPlanningAdapter(state, monitor, planner, mapper, nil, nil)
	if err != nil {
		t.Fatalf("NewGoalPlanningAdapter: %v", err)
	}
	adapter.SetMaxMapFailures(2)

	goal := planning.NewGoal("E2E bloqueable B90.8.4", "desc", planning.PriorityMedium, "user")
	planner.RegisterGoal(goal)
	state.SetCurrentGoal(goal)

	for i := 0; i < 2; i++ {
		_, _, _ = adapter.GetNextAction(context.Background())
	}
	if !goal.IsBlocked() {
		t.Fatalf("Goal quedó %q después del límite, quería blocked", goal.Status)
	}
	callsBeforeSave := mapper.calls

	path := filepath.Join(t.TempDir(), "autonomy_state.json")
	if err := buildAutonomyPersistenceSnapshot(planner, state, adapter).Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	loaded, err := LoadAutonomyPersistenceSnapshot(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	newPlanner := planning.NewPlanner()
	newState := autonomy.New()
	newMonitor := autonomy.NewGoalMonitor(newState, 0)
	newMapper := &failingE2EMapper{}
	newAdapter, err := autonomy.NewGoalPlanningAdapter(newState, newMonitor, newPlanner, newMapper, nil, nil)
	if err != nil {
		t.Fatalf("NewGoalPlanningAdapter nuevo: %v", err)
	}
	if err := ApplyAutonomyPersistenceSnapshot(loaded, newPlanner, newState, newAdapter); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	restored := newPlanner.Goals()[0]
	if !restored.IsBlocked() {
		t.Fatalf("Goal restaurado quedó %q, quería blocked", restored.Status)
	}
	_, ok, err := newAdapter.GetNextAction(context.Background())
	if err != nil {
		t.Fatalf("GetNextAction sobre Goal bloqueado devolvió error: %v", err)
	}
	if ok {
		t.Fatal("GetNextAction ofreció una acción para un Goal bloqueado")
	}
	if newMapper.calls != 0 {
		t.Fatalf("MapTask fue invocado %d veces después del reinicio; quería 0", newMapper.calls)
	}
	if callsBeforeSave != 2 {
		t.Fatalf("esperaba exactamente 2 intentos antes de persistir, got %d", callsBeforeSave)
	}
}

// TestB9084_GoalsIndependientes prueba que el bloqueo de un Goal no contamina
// otro Goal activo. Es una propiedad importante para que la autonomía pueda
// administrar varios objetivos sin convertir un fallo local en una parada
// global.
func TestB9084_GoalsIndependientes(t *testing.T) {
	planner := planning.NewPlanner()
	state := autonomy.New()
	monitor := autonomy.NewGoalMonitor(state, 0)
	mapper := &failingE2EMapper{}
	adapter, err := autonomy.NewGoalPlanningAdapter(state, monitor, planner, mapper, nil, nil)
	if err != nil {
		t.Fatalf("NewGoalPlanningAdapter: %v", err)
	}
	adapter.SetMaxMapFailures(1)

	blocked := planning.NewGoal("Goal A bloqueable", "desc", planning.PriorityHigh, "user")
	healthy := planning.NewGoal("Goal B sano", "desc", planning.PriorityMedium, "user")
	planner.RegisterGoal(blocked)
	planner.RegisterGoal(healthy)
	mapper.failGoalID = blocked.ID
	state.SetCurrentGoal(blocked)
	state.AddPendingGoal(healthy)

	_, _, _ = adapter.GetNextAction(context.Background())
	if !blocked.IsBlocked() {
		t.Fatal("Goal A no quedó bloqueado")
	}
	if healthy.IsBlocked() || healthy.IsComplete() {
		t.Fatal("Goal B fue contaminado por el bloqueo de Goal A")
	}

	// El siguiente ciclo debe poder seleccionar y mapear el Goal B con éxito.
	// El mapper falla deliberadamente solo para A; así este test comprueba
	// independencia real y no una segunda falla global del mapper.
	req, ok, err := adapter.GetNextAction(context.Background())
	if err != nil || !ok {
		t.Fatalf("Goal B no pudo continuar después de bloquear A: req=%+v ok=%v err=%v", req, ok, err)
	}
	if state.CurrentGoal() != healthy {
		t.Fatalf("CurrentGoal = %v, quería el Goal B sano", state.CurrentGoal())
	}
}
