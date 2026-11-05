package autonomy

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"ahmelia/planning"
)

type b81TaskMapper struct {
	mu    sync.Mutex
	calls int
}

func (m *b81TaskMapper) MapTask(_ context.Context, goal *planning.Goal, task *planning.Task) (ActionRequest, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls++
	return ActionRequest{
		ID:     task.ID,
		Action: "crear_carpeta",
		Target: task.Title,
		Parameters: map[string]string{
			"goal_id": goal.ID,
			"task_id": task.ID,
		},
	}, nil
}

func newB81Adapter(t *testing.T, goal *planning.Goal) (*GoalPlanningAdapter, *AutonomousState, *b81TaskMapper) {
	t.Helper()
	return newB81AdapterWithReplanner(t, goal, nil)
}

// newB81AdapterWithReplanner permite a los tests de B87 fijar límites de
// reintento/regeneración explícitos en vez del Replanner por defecto.
func newB81AdapterWithReplanner(
	t *testing.T,
	goal *planning.Goal,
	replanner *Replanner,
) (*GoalPlanningAdapter, *AutonomousState, *b81TaskMapper) {
	t.Helper()

	state := New()
	state.SetCurrentGoal(goal)

	monitor := NewGoalMonitor(state, 0)
	planner := planning.NewPlanner()
	mapper := &b81TaskMapper{}

	adapter, err := NewGoalPlanningAdapter(state, monitor, planner, mapper, nil, replanner)
	if err != nil {
		t.Fatalf("NewGoalPlanningAdapter: %v", err)
	}

	return adapter, state, mapper
}

func TestGoalPlanningAdapter_NextActionAndUpdate(t *testing.T) {
	goal := planning.NewGoal(
		"B81 integration test",
		"validar conexión Goal -> Planning -> ActionRequest",
		planning.PriorityHigh,
		"user",
	)

	adapter, state, mapper := newB81Adapter(t, goal)

	req, ok, err := adapter.GetNextAction(context.Background())
	if err != nil {
		t.Fatalf("GetNextAction: %v", err)
	}
	if !ok {
		t.Fatal("se esperaba una acción para el objetivo activo")
	}
	if req.ID == "" {
		t.Fatal("la acción debe conservar el ID de la tarea")
	}
	if state.ActiveTask() == "" {
		t.Fatal("B81 debe reflejar la tarea activa en B72")
	}

	// La tarea ya está in_progress: B81 no debe duplicarla antes de recibir
	// el resultado de ejecución.
	_, ok, err = adapter.GetNextAction(context.Background())
	if err != nil {
		t.Fatalf("GetNextAction repetido: %v", err)
	}
	if ok {
		t.Fatal("no se debe entregar dos veces la misma tarea activa")
	}

	if err := adapter.UpdateState(context.Background(), req, VerificationResult{
		Success: true,
		Output:  "ok",
	}); err != nil {
		t.Fatalf("UpdateState: %v", err)
	}

	if goal.TasksCompleted != 1 {
		t.Fatalf("TasksCompleted = %d, esperaba 1", goal.TasksCompleted)
	}
	if goal.TasksTotal != 4 {
		t.Fatalf("TasksTotal = %d, esperaba 4 para el plan genérico", goal.TasksTotal)
	}
	if goal.Progress <= 0 {
		t.Fatal("el progreso del objetivo debe avanzar tras completar la tarea")
	}
	if mapper.calls != 1 {
		t.Fatalf("MapTask fue llamado %d veces, esperaba 1", mapper.calls)
	}
}

// TestGoalPlanningAdapter_ImmediateGiveUpBlocksGoal fija un Replanner sin
// margen de reintento ni regeneración (ambos límites en 0) para conservar,
// de forma explícita y configurable, el comportamiento que existía ANTES de
// B87: una tarea fallida bloquea el objetivo en el primer intento. Con los
// límites por defecto de B87 este mismo escenario ya no bloquea de
// inmediato — ver TestGoalPlanningAdapter_RetriesBeforeBlocking.
func TestGoalPlanningAdapter_ImmediateGiveUpBlocksGoal(t *testing.T) {
	goal := planning.NewGoal(
		"B81 failure test",
		"validar propagación de fallo de ejecución a planning",
		planning.PriorityMedium,
		"user",
	)

	// -1 en vez de 0: ReplanLimits{} (todo en cero) es indistinguible del
	// zero value que NewReplanner sustituye por DefaultReplanLimits (mismo
	// criterio de ambigüedad que GoalSelectionWeights{} ya acepta en
	// goal_selector.go). -1 desactiva reintento/regeneración sin chocar
	// con esa sustitución.
	replanner := NewReplanner(ReplanLimits{MaxTaskRetries: -1, MaxPlanRegenerations: -1})
	adapter, _, _ := newB81AdapterWithReplanner(t, goal, replanner)

	req, ok, err := adapter.GetNextAction(context.Background())
	if err != nil || !ok {
		t.Fatalf("GetNextAction: ok=%v err=%v", ok, err)
	}

	err = adapter.UpdateState(context.Background(), req, VerificationResult{
		Success:     false,
		ErrorReason: "fallo de ejecución de prueba",
	})
	if err != nil {
		t.Fatalf("UpdateState: %v", err)
	}

	if goal.Status != planning.GoalBlocked {
		t.Fatalf("goal.Status = %q, esperaba %q", goal.Status, planning.GoalBlocked)
	}
	if len(goal.Blockers) != 1 || goal.Blockers[0] != "fallo de ejecución de prueba" {
		t.Fatalf("blockers = %v, esperaba razón propagada", goal.Blockers)
	}
}

// TestGoalPlanningAdapter_RetriesBeforeBlocking valida el aporte central de
// B87 con los límites por defecto (DefaultReplanLimits): un primer fallo de
// ejecución ya NO bloquea el objetivo — se reintenta la misma tarea.
func TestGoalPlanningAdapter_RetriesBeforeBlocking(t *testing.T) {
	goal := planning.NewGoal(
		"B87 retry test",
		"validar que un fallo transitorio reintenta antes de bloquear",
		planning.PriorityMedium,
		"user",
	)

	adapter, _, _ := newB81Adapter(t, goal)

	req1, ok, err := adapter.GetNextAction(context.Background())
	if err != nil || !ok {
		t.Fatalf("GetNextAction: ok=%v err=%v", ok, err)
	}

	if err := adapter.UpdateState(context.Background(), req1, VerificationResult{
		Success:     false,
		ErrorReason: "fallo transitorio de prueba",
	}); err != nil {
		t.Fatalf("UpdateState (1er fallo): %v", err)
	}

	if goal.Status == planning.GoalBlocked {
		t.Fatal("un solo fallo no debe bloquear el objetivo con los límites por defecto de B87")
	}

	// La misma tarea debe volver a ofrecerse: Retry() la deja en Pending.
	req2, ok, err := adapter.GetNextAction(context.Background())
	if err != nil || !ok {
		t.Fatalf("GetNextAction tras reintento: ok=%v err=%v", ok, err)
	}
	if req2.ID != req1.ID {
		t.Fatalf("se esperaba la misma tarea reintentada, IDs %q != %q", req2.ID, req1.ID)
	}

	if err := adapter.UpdateState(context.Background(), req2, VerificationResult{
		Success: true,
		Output:  "ok en el segundo intento",
	}); err != nil {
		t.Fatalf("UpdateState (éxito tras reintento): %v", err)
	}

	if goal.Status == planning.GoalBlocked {
		t.Fatal("el objetivo no debe quedar bloqueado tras un reintento exitoso")
	}
	if goal.TasksCompleted != 1 {
		t.Fatalf("TasksCompleted = %d, esperaba 1 tras el reintento exitoso", goal.TasksCompleted)
	}
}

// TestGoalPlanningAdapter_ReplanEventuallyGivesUp recorre el camino
// completo de B87 con los límites por defecto (2 reintentos, 1
// regeneración) contra el plan genérico de 4 tareas encadenadas: los
// primeros fallos reintentan la misma tarea; agotados los reintentos y con
// una tarea dependiente pendiente (la cadena genérica siempre la tiene,
// salvo en la última tarea), se pide un plan nuevo una vez; agotada también
// esa regeneración, el objetivo recién ahí se bloquea — nunca antes.
func TestGoalPlanningAdapter_ReplanEventuallyGivesUp(t *testing.T) {
	goal := planning.NewGoal(
		"B87 give up test",
		"validar que el bloqueo final llega solo tras agotar reintentos y regeneración",
		planning.PriorityMedium,
		"user",
	)

	adapter, _, _ := newB81Adapter(t, goal)
	ctx := context.Background()

	seenTaskIDs := make(map[string]bool)
	fail := func(reason string) (ActionRequest, bool) {
		req, ok, err := adapter.GetNextAction(ctx)
		if err != nil {
			t.Fatalf("GetNextAction: %v", err)
		}
		if !ok {
			return req, false
		}
		seenTaskIDs[req.ID] = true
		if err := adapter.UpdateState(ctx, req, VerificationResult{
			Success:     false,
			ErrorReason: reason,
		}); err != nil {
			t.Fatalf("UpdateState: %v", err)
		}
		return req, true
	}

	// Intento 1 y 2 sobre la primera tarea de la primera versión del plan:
	// ambos reintentan (Attempts pasa de 1 a 2, todavía < MaxTaskRetries=2
	// no aplica hasta el 3er intento realmente, pero acá alcanza con dos
	// fallos para agotar el margen y disparar la regeneración).
	if _, ok := fail("fallo 1"); !ok {
		t.Fatal("se esperaba una acción disponible en el primer intento")
	}
	if goal.Status == planning.GoalBlocked {
		t.Fatal("no debía bloquearse tras el primer fallo")
	}

	if _, ok := fail("fallo 2"); !ok {
		t.Fatal("se esperaba una acción disponible en el segundo intento")
	}
	if goal.Status == planning.GoalBlocked {
		t.Fatal("no debía bloquearse tras el segundo fallo: corresponde regenerar el plan")
	}

	// A partir de acá, GetNextAction debe entregar tareas de un plan NUEVO
	// (IDs distintos a los ya vistos) porque la regeneración descartó el
	// plan anterior.
	if _, ok := fail("fallo 3"); !ok {
		t.Fatal("se esperaba una acción disponible tras la regeneración")
	}
	if goal.Status == planning.GoalBlocked {
		t.Fatal("no debía bloquearse: la tarea del plan regenerado recién va por su primer fallo")
	}

	req4, ok := fail("fallo 4")
	if !ok {
		t.Fatal("se esperaba una acción disponible en el fallo final")
	}
	if goal.Status != planning.GoalBlocked {
		t.Fatalf("goal.Status = %q, esperaba %q tras agotar reintentos y regeneración", goal.Status, planning.GoalBlocked)
	}
	if len(goal.Blockers) == 0 || goal.Blockers[len(goal.Blockers)-1] != "fallo 4" {
		t.Fatalf("blockers = %v, esperaba que el último fuera la razón del fallo final", goal.Blockers)
	}
	_ = req4
}

func TestGoalPlanningAdapter_ConcurrentReadsAreSafe(t *testing.T) {
	goal := planning.NewGoal(
		"B81 race test",
		"validar serialización interna del adaptador",
		planning.PriorityLow,
		"user",
	)
	adapter, _, _ := newB81Adapter(t, goal)

	const workers = 16
	var wg sync.WaitGroup
	errs := make(chan error, workers)

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, err := adapter.GetNextAction(context.Background())
			if err != nil {
				errs <- fmt.Errorf("GetNextAction concurrente: %w", err)
			}
		}()
	}

	wg.Wait()
	close(errs)

	for err := range errs {
		t.Error(err)
	}
}
