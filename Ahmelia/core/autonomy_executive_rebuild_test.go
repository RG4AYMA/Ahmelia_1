package core

import (
	"context"
	"path/filepath"
	"testing"

	"ahmelia/autonomy"
	"ahmelia/planning"
	"ahmelia/shared"
)

// TestRebuildExecutiveViews_TrasReinicioCompleto reproduce el escenario
// real que motiva B90.8.2: un Goal creado (como haría B46), con una Task
// ya completada y otra pendiente, sobrevive a un reinicio completo
// (save -> load -> ApplyAutonomyPersistenceSnapshot contra un runtime
// NUEVO) y rebuildExecutiveViews debe dejar executiveGoals/executivePlans/
// executiveProgress apuntando exactamente a ese estado restaurado — no a
// una copia, no a un Goal/Plan fabricado de nuevo.
func TestRebuildExecutiveViews_TrasReinicioCompleto(t *testing.T) {
	planner, state, adapter, _ := newReconciliationRuntime(t)

	goal := planning.NewGoal("Proyecto B90.8.2", "desc", planning.PriorityMedium, "user")
	planner.RegisterGoal(goal)
	state.SetCurrentGoal(goal)

	plan, err := adapter.EnsurePlan(goal)
	if err != nil {
		t.Fatalf("EnsurePlan: %v", err)
	}
	wantTotal := len(plan.Tasks)
	if wantTotal < 2 {
		t.Fatal("el plan generado necesita al menos 2 Tasks para que este test tenga sentido")
	}

	ctx := context.Background()
	req, ok, err := adapter.GetNextAction(ctx)
	if err != nil || !ok {
		t.Fatalf("GetNextAction(): %+v, %v, %v", req, ok, err)
	}
	if err := adapter.UpdateState(ctx, req, autonomy.VerificationResult{Success: true}); err != nil {
		t.Fatalf("UpdateState: %v", err)
	}
	// Exactamente 1 Task completada, el resto sigue Pending — no
	// alcanzamos a completar el Goal entero, a propósito.

	// "Reinicio": persistir el estado real y reconciliarlo contra un
	// runtime completamente nuevo, mismo contrato que usa NewEngine.
	path := filepath.Join(t.TempDir(), "autonomy_state.json")
	snap := buildAutonomyPersistenceSnapshot(planner, state, adapter)
	if err := snap.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	loaded, err := LoadAutonomyPersistenceSnapshot(path)
	if err != nil {
		t.Fatalf("LoadAutonomyPersistenceSnapshot: %v", err)
	}

	newPlanner, newState, newAdapter, _ := newReconciliationRuntime(t)
	if err := ApplyAutonomyPersistenceSnapshot(loaded, newPlanner, newState, newAdapter); err != nil {
		t.Fatalf("ApplyAutonomyPersistenceSnapshot: %v", err)
	}

	executiveGoals := make(map[string]*planning.Goal)
	executivePlans := make(map[string]*planning.Plan)
	executiveProgress := make(map[string]*planning.Progress)
	rebuildExecutiveViews(newPlanner, newAdapter, executiveGoals, executivePlans, executiveProgress)

	// 1) executiveGoals indexa por el mismo nombre normalizado que usa
	// B46 al crear el proyecto, y apunta al Goal restaurado real (mismo
	// ID — no hay forma de comparar puntero contra el runtime viejo
	// porque B90.6 reconstruye Goals nuevos por diseño, pero tiene que
	// ser EXACTAMENTE el que vive en newPlanner.Goals()).
	restoredGoal, found := executiveGoals[shared.Normalize(goal.Name)]
	if !found {
		t.Fatalf("executiveGoals no tiene entrada para %q tras reconstruir", goal.Name)
	}
	var plannerGoal *planning.Goal
	for _, g := range newPlanner.Goals() {
		if g.ID == goal.ID {
			plannerGoal = g
		}
	}
	if plannerGoal == nil {
		t.Fatalf("newPlanner.Goals() no tiene el Goal %q tras reconciliar", goal.ID)
	}
	if restoredGoal != plannerGoal {
		t.Fatalf("executiveGoals[...] = %p, quería el mismo puntero que newPlanner.Goals() (%p) — no una copia", restoredGoal, plannerGoal)
	}

	// 2) executivePlans[goal.ID] tiene que ser EL MISMO puntero que
	// newAdapter.Plans()[goal.ID] — la prueba de "vista, no duplicado"
	// que exige B90.8.2.
	adapterPlan := newAdapter.Plans()[goal.ID]
	if adapterPlan == nil {
		t.Fatalf("newAdapter.Plans()[%q] = nil tras reconciliar", goal.ID)
	}
	if executivePlans[goal.ID] != adapterPlan {
		t.Fatalf("executivePlans[%q] = %p, quería el mismo puntero que adapter.Plans() (%p)", goal.ID, executivePlans[goal.ID], adapterPlan)
	}

	// 3) executiveProgress refleja el conteo real de Tasks completadas
	// (1 de wantTotal) — lo único que el snapshot sí garantiza.
	progress, found := executiveProgress[goal.ID]
	if !found {
		t.Fatalf("executiveProgress no tiene entrada para %q tras reconstruir", goal.ID)
	}
	if progress.TasksTotal != wantTotal {
		t.Fatalf("progress.TasksTotal = %d, quería %d", progress.TasksTotal, wantTotal)
	}
	if progress.TasksCompleted != 1 {
		t.Fatalf("progress.TasksCompleted = %d, quería 1 (la única Task completada antes del reinicio)", progress.TasksCompleted)
	}
}

// TestRebuildExecutiveViews_SnapshotVacioNoPanickea cubre la primera
// corrida (nada que reconciliar): rebuildExecutiveViews sobre un runtime
// vacío no debe fallar ni dejar entradas fantasma.
func TestRebuildExecutiveViews_SnapshotVacioNoPanickea(t *testing.T) {
	planner, _, adapter, _ := newReconciliationRuntime(t)

	executiveGoals := make(map[string]*planning.Goal)
	executivePlans := make(map[string]*planning.Plan)
	executiveProgress := make(map[string]*planning.Progress)
	rebuildExecutiveViews(planner, adapter, executiveGoals, executivePlans, executiveProgress)

	if len(executiveGoals) != 0 || len(executivePlans) != 0 || len(executiveProgress) != 0 {
		t.Fatalf("esperaba vistas vacías, got Goals=%v Plans=%v Progress=%v", executiveGoals, executivePlans, executiveProgress)
	}
}

// TestRebuildExecutiveViews_DependenciasNil cubre el guard defensivo: no
// debe hacer panic si planner/adapter no están configurados (no debería
// pasar en NewEngine real, pero tampoco debe explotar si pasa).
func TestRebuildExecutiveViews_DependenciasNil(t *testing.T) {
	executiveGoals := make(map[string]*planning.Goal)
	executivePlans := make(map[string]*planning.Plan)
	executiveProgress := make(map[string]*planning.Progress)
	rebuildExecutiveViews(nil, nil, executiveGoals, executivePlans, executiveProgress)

	if len(executiveGoals) != 0 || len(executivePlans) != 0 || len(executiveProgress) != 0 {
		t.Fatal("esperaba vistas vacías con planner/adapter nil")
	}
}
