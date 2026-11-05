package core

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"ahmelia/autonomy"
	"ahmelia/environment"
	"ahmelia/memory_intelligence"
	"ahmelia/planning"
)

func TestB88ExplicitGoalActionMapper(t *testing.T) {
	dir := t.TempDir()
	goal := planning.NewGoal("Crear estructura", "", planning.PriorityHigh, "ahmelia")
	task := planning.NewTask(goal.ID, "crear", `action=crear_carpeta target="`+dir+`"`, 1, 0)

	req, err := (ExplicitGoalActionMapper{}).MapTask(context.Background(), goal, task)
	if err != nil {
		t.Fatalf("MapTask() error: %v", err)
	}
	if req.Action != "crear_carpeta" || req.Target != dir {
		t.Fatalf("request inesperado: %+v", req)
	}
}

func TestB88ExplicitGoalActionMapperRejectsImplicitAction(t *testing.T) {
	goal := planning.NewGoal("x", "", planning.PriorityMedium, "ahmelia")
	task := planning.NewTask(goal.ID, "Crear carpeta", "haz una carpeta en algun sitio", 1, 0)
	if _, err := (ExplicitGoalActionMapper{}).MapTask(context.Background(), goal, task); err == nil {
		t.Fatal("esperaba rechazo: el mapper no debe inferir una Action ni un target desde texto libre")
	}
}

func TestB88EnvironmentInspectorConfirmsCreateFolder(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "real")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	obs, err := (engineEnvironmentInspector{}).InspectAction(context.Background(), autonomy.ActionRequest{
		ID: "b88-1", Action: "crear_carpeta", Target: dir,
	})
	if err != nil || !obs.Confirmed {
		t.Fatalf("esperaba confirmación real del entorno: obs=%+v err=%v", obs, err)
	}
}

func TestB88CycleWithEnvironmentVerifierConfirmsRealExecution(t *testing.T) {
	state := autonomy.New()
	e := newB963334AuditEngine(t)
	target := filepath.Join(t.TempDir(), "b88-created")
	planner := &onceActionPlanner{req: autonomy.ActionRequest{
		ID: "b88-cycle", Action: "crear_carpeta", Target: target,
	}}

	cycle := autonomy.NewCycle(
		state, nil, nil, planner,
		NewAutonomyExecutor(e), nil, nil, nil, nil,
	)
	cycle.SetEnvironmentVerifier(autonomy.NewEnvironmentVerifier(engineEnvironmentInspector{}))

	if err := cycle.RunStep(context.Background()); err != nil {
		t.Fatalf("RunStep() error: %v", err)
	}
	if _, err := os.Stat(target); err != nil {
		t.Fatalf("la acción real no creó la carpeta: %v", err)
	}
	if state.Execution() != autonomy.ExecIdle {
		t.Fatalf("estado final inesperado: %s", state.Execution())
	}
}

func TestB88BuildsIntegratedCycleFromEngine(t *testing.T) {
	state := autonomy.New()
	monitor := autonomy.NewGoalMonitor(state, 0)
	planner := planning.NewPlanner()

	// B90.3 — GoalPlanningAdapter ya no lo construye
	// buildAutonomousExecutionCycle(); es responsabilidad de quien arma el
	// Engine (NewEngine en producción). Este test construye Engine a mano
	// (no vía NewEngine), así que tiene que replicar esa misma
	// construcción para que buildAutonomousExecutionCycle() encuentre
	// e.goalPlanningAdapter ya configurado — mismo criterio de nil=default
	// que NewEngine usa (selector/replanner nil).
	goalPlanningAdapter, err := autonomy.NewGoalPlanningAdapter(
		state, monitor, planner, autonomy.NewCapabilityResolver(nil), nil, nil,
	)
	if err != nil {
		t.Fatalf("NewGoalPlanningAdapter() error: %v", err)
	}

	engine := &Engine{
		autonomousState:     state,
		goalMonitor:         monitor,
		planner:             planner,
		goalPlanningAdapter: goalPlanningAdapter,
		world:               environment.NewManager(),
		eventStore:          memory_intelligence.NewEventStore(&memory_intelligence.EventLog{}),
		learningState:       memory_intelligence.NewLearningStore(memory_intelligence.NewLearningState()),
	}

	cycle, err := engine.buildAutonomousExecutionCycle()
	if err != nil {
		t.Fatalf("buildAutonomousExecutionCycle() error: %v", err)
	}
	if cycle == nil {
		t.Fatal("el ciclo integrado no puede ser nil")
	}
}

// TestB88BuildsIntegratedCycleFromEngine_SinAdapterFalla confirma el otro
// lado del contrato de B90.3: si Engine no tiene goalPlanningAdapter
// configurado (ej. un Engine armado a mano que se olvidó de construirlo,
// como hacía este mismo test antes de B90.3), buildAutonomousExecutionCycle
// falla explícito en vez de fabricar uno nuevo en silencio — esa
// fabricación silenciosa es justamente el comportamiento que B90.3 eliminó.
func TestB88BuildsIntegratedCycleFromEngine_SinAdapterFalla(t *testing.T) {
	state := autonomy.New()
	engine := &Engine{
		autonomousState: state,
		goalMonitor:     autonomy.NewGoalMonitor(state, 0),
		planner:         planning.NewPlanner(),
		world:           environment.NewManager(),
		eventStore:      memory_intelligence.NewEventStore(&memory_intelligence.EventLog{}),
		learningState:   memory_intelligence.NewLearningStore(memory_intelligence.NewLearningState()),
		// goalPlanningAdapter deliberadamente sin configurar.
	}

	if _, err := engine.buildAutonomousExecutionCycle(); err == nil {
		t.Fatal("esperaba error sin goalPlanningAdapter configurado, got nil")
	}
}

func TestB88GoalActionVerifierRequiresObservedExternalEffect(t *testing.T) {
	existing := filepath.Join(t.TempDir(), "existing")
	if err := os.Mkdir(existing, 0o755); err != nil {
		t.Fatal(err)
	}
	verifier := goalActionVerifier{technical: autonomy.NewActionVerifier(), inspector: engineEnvironmentInspector{}}

	confirmed := verifier.Verify(context.Background(), autonomy.ActionRequest{ID: "confirmed", Action: "crear_carpeta", Target: existing}, autonomy.ActionResponse{RequestID: "confirmed", Executed: true})
	if !confirmed.Success || confirmed.ObservationStatus != autonomy.ObservationConfirmed {
		t.Fatalf("existing expected folder should be confirmed: %+v", confirmed)
	}
	if confirmed.Observation.ActionID != "confirmed" || confirmed.Observation.Target != existing || confirmed.Observation.Source == "" || confirmed.Observation.ObservedAt.IsZero() || len(confirmed.Observation.Facts) == 0 {
		t.Fatalf("goal verification discarded structured environmental evidence: %+v", confirmed.Observation)
	}

	missing := filepath.Join(t.TempDir(), "missing")
	contradicted := verifier.Verify(context.Background(), autonomy.ActionRequest{ID: "missing", Action: "crear_carpeta", Target: missing}, autonomy.ActionResponse{RequestID: "missing", Executed: true})
	if contradicted.Success || contradicted.ObservationStatus != autonomy.ObservationContradicted {
		t.Fatalf("missing expected folder must not be accepted as success: %+v", contradicted)
	}
}

func TestB964422EnvironmentInspectorUsesCanonicalFilesystemPostconditions(t *testing.T) {
	t.Run("move is reconciled against origin and destination", func(t *testing.T) {
		base := t.TempDir()
		source := filepath.Join(base, "source.txt")
		destination := filepath.Join(base, "destination.txt")
		if err := os.WriteFile(destination, []byte("already moved"), 0o644); err != nil {
			t.Fatal(err)
		}
		obs, err := (engineEnvironmentInspector{}).InspectForReconciliation(context.Background(), autonomy.ActionRequest{
			ID: "move-reconcile", Action: "mover_archivo",
			Parameters: map[string]string{"origen": source, "destino": destination},
		})
		if err != nil || obs.Status != autonomy.ObservationConfirmed || !obs.Confirmed {
			t.Fatalf("expected canonical move postcondition to confirm: obs=%+v err=%v", obs, err)
		}
	})

	t.Run("rename is reconciled against original path and new name", func(t *testing.T) {
		base := t.TempDir()
		oldPath := filepath.Join(base, "old.txt")
		newPath := filepath.Join(base, "new.txt")
		if err := os.WriteFile(newPath, []byte("already renamed"), 0o644); err != nil {
			t.Fatal(err)
		}
		obs, err := (engineEnvironmentInspector{}).InspectForReconciliation(context.Background(), autonomy.ActionRequest{
			ID: "rename-reconcile", Action: "renombrar_archivo",
			Parameters: map[string]string{"origen": oldPath, "destino": newPath},
		})
		if err != nil || obs.Status != autonomy.ObservationConfirmed || !obs.Confirmed {
			t.Fatalf("expected canonical rename postcondition to confirm: obs=%+v err=%v", obs, err)
		}
	})

	t.Run("unsupported action remains insufficient", func(t *testing.T) {
		obs, err := (engineEnvironmentInspector{}).InspectAction(context.Background(), autonomy.ActionRequest{Action: "abrir_universo"})
		if err != nil || obs.Status != autonomy.ObservationInsufficient || obs.Confirmed {
			t.Fatalf("unsupported observation must not be inferred: obs=%+v err=%v", obs, err)
		}
	})
}

func TestB964422EnvironmentInspectorPreservesUnknownFilesystemState(t *testing.T) {
	base := t.TempDir()
	parentFile := filepath.Join(base, "not-a-directory")
	if err := os.WriteFile(parentFile, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	obs, err := (engineEnvironmentInspector{}).InspectForReconciliation(context.Background(), autonomy.ActionRequest{
		ID: "folder-read-error", Action: "crear_carpeta", Target: filepath.Join(parentFile, "child"),
	})
	if err != nil {
		t.Fatalf("InspectForReconciliation() error: %v", err)
	}
	if obs.Status != autonomy.ObservationInsufficient || obs.Confirmed {
		t.Fatalf("filesystem read failure must remain unknown, not contradicted or confirmed: %+v", obs)
	}
}

func TestB964423WindowFocusReconciliationUsesObservedForegroundState(t *testing.T) {
	processes := []environment.OSProcessObservation{
		{ProcessID: 10, ProcessName: "chrome", WindowTitle: "Search"},
		{ProcessID: 20, ProcessName: "Code", WindowTitle: "main.go"},
	}
	inspector := engineEnvironmentInspector{
		processScanner: func() ([]environment.OSProcessObservation, error) {
			return append([]environment.OSProcessObservation(nil), processes...), nil
		},
		foregroundProcess: func() (string, uint32, error) { return "chrome", 10, nil },
	}

	confirmed, err := inspector.InspectForReconciliation(context.Background(), autonomy.ActionRequest{Action: "cambiar_ventana", Target: "chrome"})
	if err != nil || !confirmed.Confirmed || confirmed.Status != autonomy.ObservationConfirmed {
		t.Fatalf("expected foreground observation to confirm target: obs=%+v err=%v", confirmed, err)
	}

	inspector.foregroundProcess = func() (string, uint32, error) { return "Code", 20, nil }
	contradicted, err := inspector.InspectForReconciliation(context.Background(), autonomy.ActionRequest{Action: "cambiar_ventana", Target: "chrome"})
	if err != nil || contradicted.Confirmed || contradicted.Status != autonomy.ObservationContradicted {
		t.Fatalf("observed different foreground process must contradict target focus: obs=%+v err=%v", contradicted, err)
	}
}

func TestB964423WindowFocusReconciliationPreservesUnknown(t *testing.T) {
	t.Run("AltTab without target is insufficient", func(t *testing.T) {
		inspector := engineEnvironmentInspector{}
		obs, err := inspector.InspectForReconciliation(context.Background(), autonomy.ActionRequest{Action: "cambiar_ventana"})
		if err != nil || obs.Confirmed || obs.Status != autonomy.ObservationInsufficient {
			t.Fatalf("targetless Alt+Tab must remain insufficient: obs=%+v err=%v", obs, err)
		}
	})
	t.Run("missing target window is insufficient, not failure", func(t *testing.T) {
		inspector := engineEnvironmentInspector{
			processScanner: func() ([]environment.OSProcessObservation, error) {
				return []environment.OSProcessObservation{{ProcessID: 20, ProcessName: "Code", WindowTitle: "main.go"}}, nil
			},
			foregroundProcess: func() (string, uint32, error) { return "Code", 20, nil },
		}
		obs, err := inspector.InspectForReconciliation(context.Background(), autonomy.ActionRequest{Action: "cambiar_ventana", Target: "chrome"})
		if err != nil || obs.Confirmed || obs.Status != autonomy.ObservationInsufficient {
			t.Fatalf("missing target must not be interpreted as confirmed failure: obs=%+v err=%v", obs, err)
		}
	})
	t.Run("scanner outage is insufficient", func(t *testing.T) {
		inspector := engineEnvironmentInspector{
			processScanner:    func() ([]environment.OSProcessObservation, error) { return nil, fmt.Errorf("access denied") },
			foregroundProcess: func() (string, uint32, error) { return "chrome", 10, nil },
		}
		obs, err := inspector.InspectForReconciliation(context.Background(), autonomy.ActionRequest{Action: "cambiar_ventana", Target: "chrome"})
		if err != nil || obs.Confirmed || obs.Status != autonomy.ObservationInsufficient {
			t.Fatalf("scanner outage must remain unknown: obs=%+v err=%v", obs, err)
		}
	})
}
