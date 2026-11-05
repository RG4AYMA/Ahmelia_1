package core

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"ahmelia/autonomy"
	"ahmelia/planning"
)

// TestBuildAutonomyPersistenceSnapshot_DesdeRuntimeReal cubre el sentido
// "runtime -> snapshot" que B90.7 agrega: arranca un GoalPlanningAdapter
// real, corre un ciclo real (GetNextAction + UpdateState con éxito) y
// verifica que buildAutonomyPersistenceSnapshot refleje exactamente lo que
// quedó en Planner/AutonomousState/Replanner — incluyendo el hallazgo de
// auditoría de B90.7: las Tasks que GeneratePlan arma para el plan
// cacheado del adapter (no las que alguien haya registrado a mano) deben
// terminar en Planner.Tasks(), o el snapshot quedaría con un Plan que
// referencia Tasks inexistentes.
func TestBuildAutonomyPersistenceSnapshot_DesdeRuntimeReal(t *testing.T) {
	planner, state, adapter, _ := newReconciliationRuntime(t)

	goal := planning.NewGoal("Meta B90.7", "desc", planning.PriorityHigh, "user")
	planner.RegisterGoal(goal)
	state.SetCurrentGoal(goal)

	ctx := context.Background()
	req, ok, err := adapter.GetNextAction(ctx)
	if err != nil || !ok {
		t.Fatalf("GetNextAction() = %+v, %v, %v — esperaba una acción real", req, ok, err)
	}
	if err := adapter.UpdateState(ctx, req, autonomy.VerificationResult{Success: true}); err != nil {
		t.Fatalf("UpdateState: %v", err)
	}

	plan := adapter.Plans()[goal.ID]
	if plan == nil {
		t.Fatalf("adapter.Plans()[%q] = nil, esperaba el plan cacheado por GetNextAction", goal.ID)
	}
	wantTaskCount := len(plan.Tasks)
	if wantTaskCount == 0 {
		t.Fatal("el plan generado no tiene Tasks — no hay nada real que verificar")
	}

	snap := buildAutonomyPersistenceSnapshot(planner, state, adapter)

	if len(snap.Goals) != 1 || snap.Goals[0].ID != goal.ID {
		t.Fatalf("snap.Goals = %v, quería solo %q", snap.Goals, goal.ID)
	}
	if len(snap.Tasks) != wantTaskCount {
		t.Fatalf("snap.Tasks tiene %d, quería %d (las del plan real generado por GeneratePlan)", len(snap.Tasks), wantTaskCount)
	}
	if snap.CurrentGoalID != goal.ID {
		t.Fatalf("snap.CurrentGoalID = %q, quería %q", snap.CurrentGoalID, goal.ID)
	}
	if len(snap.Plans) != 1 || snap.Plans[0].GoalID != goal.ID {
		t.Fatalf("snap.Plans = %+v, quería un plan para %q", snap.Plans, goal.ID)
	}
	if len(snap.Plans[0].TaskIDs) != wantTaskCount {
		t.Fatalf("snap.Plans[0].TaskIDs tiene %d, quería %d", len(snap.Plans[0].TaskIDs), wantTaskCount)
	}
	// La Task que UpdateState resolvió (req.ID) tiene que estar entre las
	// persistidas y en estado terminal — no alcanza con que el conteo dé
	// bien, la identidad real tiene que aparecer.
	found := false
	for _, tsk := range snap.Tasks {
		if tsk.ID == req.ID {
			found = true
			if tsk.Status != planning.TaskCompleted {
				t.Fatalf("Task %q en snap.Tasks tiene Status=%v, quería TaskCompleted", tsk.ID, tsk.Status)
			}
		}
	}
	if !found {
		t.Fatalf("snap.Tasks no incluye la Task %q que UpdateState acaba de completar", req.ID)
	}
}

// TestPersistAutonomyState_CicloCompletoSaveLoadReconcile prueba el
// contrato de punta a punta que B90.7 promete: correr el ciclo real,
// persistir, y reconciliar ese mismo archivo contra un runtime NUEVO
// (mismo precondition que B90.6 exige) reproduce Goal/Tasks — exactamente
// el escenario de "reinicio del proceso" que motiva todo B90. La Task que
// GetNextAction dejó TaskInProgress (nunca se llegó a resolver, como si
// el proceso hubiera muerto en medio de la ejecución) debe volver
// TaskPending al reconciliar (B90.6), no desaparecer ni quedar a medias.
func TestPersistAutonomyState_CicloCompletoSaveLoadReconcile(t *testing.T) {
	planner, state, adapter, _ := newReconciliationRuntime(t)

	goalA := planning.NewGoal("Meta persistente", "desc", planning.PriorityMedium, "user")
	planner.RegisterGoal(goalA)
	state.SetCurrentGoal(goalA)

	ctx := context.Background()
	req, ok, err := adapter.GetNextAction(ctx)
	if err != nil || !ok {
		t.Fatalf("GetNextAction(): %+v, %v, %v", req, ok, err)
	}
	wantTaskCount := len(adapter.Plans()[goalA.ID].Tasks)

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

	if got := newState.CurrentGoal(); got == nil || got.ID != goalA.ID {
		t.Fatalf("CurrentGoal tras reconciliar = %+v, quería %q", got, goalA.ID)
	}
	restoredTasks := newPlanner.Tasks()
	if len(restoredTasks) != wantTaskCount {
		t.Fatalf("Tasks restauradas = %d, quería %d", len(restoredTasks), wantTaskCount)
	}
	for _, tsk := range restoredTasks {
		if tsk.Status == planning.TaskInProgress {
			t.Fatalf("Task %q quedó TaskInProgress tras reconciliar — B90.6 debe normalizarla a TaskPending", tsk.ID)
		}
		if tsk.ID == req.ID && tsk.Status != planning.TaskPending {
			t.Fatalf("la Task %q que había quedado en curso (%q) restauró Status=%v, quería TaskPending", tsk.ID, req.ID, tsk.Status)
		}
	}
}

// TestPersistAutonomyState_FaltanDependencias cubre el caso de un Engine a
// medio construir (bug de wiring, no debería pasar en NewEngine real):
// PersistAutonomyState debe fallar explícito, no hacer panic ni escribir
// un snapshot parcial.
func TestPersistAutonomyState_FaltanDependencias(t *testing.T) {
	e := &Engine{}
	if err := e.PersistAutonomyState(); err == nil {
		t.Fatal("esperaba error con planner/autonomousState/goalPlanningAdapter nil, got nil")
	}
}

// TestPersistingCycleAdapter_PersisteSoloTrasMutacionReal verifica el
// disparador central de B90.7: GetNextAction persiste en disco cuando
// devuelve una acción real (ok=true), y UpdateState persiste siempre que
// no falle. PersistAutonomyState guarda en la ruta fija AutonomyStatePath
// (mismo criterio que solving.SkillLibraryPath: una constante, no un
// parámetro) — para no tocar memory/autonomy_state.json del repositorio
// real ni inventarle una forma de override que B90 no pidió, el test
// corre con el directorio de trabajo apuntado a un temporal.
func TestPersistingCycleAdapter_PersisteSoloTrasMutacionReal(t *testing.T) {
	origWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() {
		if err := os.Chdir(origWD); err != nil {
			t.Fatalf("Chdir de vuelta: %v", err)
		}
	}()

	planner, state, adapter, _ := newReconciliationRuntime(t)

	goal := planning.NewGoal("Meta wrapper", "desc", planning.PriorityLow, "user")
	planner.RegisterGoal(goal)
	state.SetCurrentGoal(goal)

	e := &Engine{planner: planner, autonomousState: state, goalPlanningAdapter: adapter}
	wrapped := &persistingCycleAdapter{engine: e, goalAdapter: adapter}

	ctx := context.Background()
	req, ok, err := wrapped.GetNextAction(ctx)
	if err != nil || !ok {
		t.Fatalf("GetNextAction() = %+v, %v, %v", req, ok, err)
	}
	if _, statErr := os.Stat(AutonomyStatePath); statErr != nil {
		t.Fatalf("esperaba que GetNextAction (ok=true) persistiera en disco: %v", statErr)
	}

	// Borramos el archivo para verificar limpiamente que UpdateState
	// también dispara su propio guardado, sin depender del anterior.
	if err := os.Remove(AutonomyStatePath); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	if err := wrapped.UpdateState(ctx, req, autonomy.VerificationResult{Success: true}); err != nil {
		t.Fatalf("UpdateState: %v", err)
	}
	if _, statErr := os.Stat(AutonomyStatePath); statErr != nil {
		t.Fatalf("esperaba que UpdateState persistiera en disco: %v", statErr)
	}
}
