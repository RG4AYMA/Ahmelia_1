package autonomy

import (
	"context"
	"fmt"
	"testing"

	"ahmelia/planning"
)

// alwaysFailMapper simula una Task cuyo texto nunca resuelve a ninguna
// capacidad ejecutable (el caso real y típico: decomposeGoal generando
// texto genérico que CapabilityResolver no puede mapear, B89) — cada
// llamada a MapTask falla, sin importar cuántas veces se reintente.
type alwaysFailMapper struct {
	calls int
}

func (m *alwaysFailMapper) MapTask(_ context.Context, _ *planning.Goal, task *planning.Task) (ActionRequest, error) {
	m.calls++
	return ActionRequest{}, fmt.Errorf("autonomy: la tarea %q no resolvió a ninguna capacidad ejecutable", task.ID)
}

// flakyThenSucceedsMapper falla las primeras `failFor` llamadas y after
// eso siempre resuelve — simula un fallo transitorio real (a diferencia
// de alwaysFailMapper, que modela el caso sin solución).
type flakyThenSucceedsMapper struct {
	calls   int
	failFor int
}

func (m *flakyThenSucceedsMapper) MapTask(_ context.Context, goal *planning.Goal, task *planning.Task) (ActionRequest, error) {
	m.calls++
	if m.calls <= m.failFor {
		return ActionRequest{}, fmt.Errorf("autonomy: fallo transitorio #%d resolviendo %q", m.calls, task.ID)
	}
	return ActionRequest{ID: task.ID, Action: "crear_carpeta", Target: task.Title}, nil
}

func newMapFailureAdapter(t *testing.T, goal *planning.Goal, mapper GoalActionMapper) (*GoalPlanningAdapter, *AutonomousState) {
	t.Helper()
	state := New()
	state.SetCurrentGoal(goal)
	monitor := NewGoalMonitor(state, 0)
	planner := planning.NewPlanner()
	planner.RegisterGoal(goal)

	adapter, err := NewGoalPlanningAdapter(state, monitor, planner, mapper, nil, nil)
	if err != nil {
		t.Fatalf("NewGoalPlanningAdapter: %v", err)
	}
	return adapter, state
}

// TestGoalPlanningAdapter_BloqueaTrasNFallosConsecutivosDeMapeo es la
// prueba central de B90.8.3: un Goal cuya Task nunca resuelve a ninguna
// capacidad se bloquea después de DefaultMaxPlanMapFailures intentos —
// no se reintenta para siempre en silencio.
func TestGoalPlanningAdapter_BloqueaTrasNFallosConsecutivosDeMapeo(t *testing.T) {
	goal := planning.NewGoal("Meta irresoluble", "desc", planning.PriorityMedium, "user")
	mapper := &alwaysFailMapper{}
	adapter, state := newMapFailureAdapter(t, goal, mapper)

	ctx := context.Background()
	for i := 1; i < DefaultMaxPlanMapFailures; i++ {
		req, ok, err := adapter.GetNextAction(ctx)
		if ok {
			t.Fatalf("intento %d: GetNextAction() ok=true, quería false (todavía no se agotaron los intentos)", i)
		}
		if err == nil {
			t.Fatalf("intento %d: GetNextAction() sin error, quería el fallo de mapeo envuelto", i)
		}
		if req.ID != "" {
			t.Fatalf("intento %d: req = %+v, quería ActionRequest vacío", i, req)
		}
		if goal.Status == planning.GoalBlocked {
			t.Fatalf("intento %d: el Goal ya está Blocked antes de agotar DefaultMaxPlanMapFailures=%d", i, DefaultMaxPlanMapFailures)
		}
	}

	// El intento número DefaultMaxPlanMapFailures agota el margen.
	req, ok, err := adapter.GetNextAction(ctx)
	if ok {
		t.Fatalf("último intento: GetNextAction() ok=true, quería false")
	}
	if err == nil {
		t.Fatal("último intento: esperaba un error explicando el bloqueo")
	}
	if req.ID != "" {
		t.Fatalf("último intento: req = %+v, quería ActionRequest vacío", req)
	}
	if goal.Status != planning.GoalBlocked {
		t.Fatalf("goal.Status = %v tras %d fallos, quería GoalBlocked", goal.Status, DefaultMaxPlanMapFailures)
	}
	if len(goal.Blockers) == 0 {
		t.Fatal("goal.Blockers está vacío — el motivo del bloqueo debería quedar registrado")
	}
	if state.CurrentGoal() != nil {
		t.Fatalf("state.CurrentGoal() = %+v tras bloquear, quería nil", state.CurrentGoal())
	}
	if mapper.calls != DefaultMaxPlanMapFailures {
		t.Fatalf("mapper.calls = %d, quería exactamente %d (una por intento, ninguna de más)", mapper.calls, DefaultMaxPlanMapFailures)
	}

	// El Goal bloqueado no debe volver a ofrecerse — próxima llamada,
	// selectGoal ya no lo elige (o el guard explícito de GoalBlocked lo
	// corta), y no debe seguir incrementando ningún contador.
	req, ok, err = adapter.GetNextAction(ctx)
	if ok || err != nil || req.ID != "" {
		t.Fatalf("tras bloquear, GetNextAction() = %+v, %v, %v — quería (vacío, false, nil)", req, ok, err)
	}
	if mapper.calls != DefaultMaxPlanMapFailures {
		t.Fatalf("mapper.calls subió a %d tras el bloqueo — el Goal bloqueado no debería volver a intentarse", mapper.calls)
	}
}

// TestGoalPlanningAdapter_MapeoExitosoReseteaElContador confirma que
// B90.8.3 cuenta fallos CONSECUTIVOS, no históricos: si el mapeo falla
// una vez y después resuelve bien, un fallo posterior vuelve a arrancar
// desde 1 — no hereda la racha anterior.
func TestGoalPlanningAdapter_MapeoExitosoReseteaElContador(t *testing.T) {
	goal := planning.NewGoal("Meta con fallo transitorio", "desc", planning.PriorityMedium, "user")
	mapper := &flakyThenSucceedsMapper{failFor: DefaultMaxPlanMapFailures - 1}
	adapter, _ := newMapFailureAdapter(t, goal, mapper)

	ctx := context.Background()

	// Falla DefaultMaxPlanMapFailures-1 veces (justo por debajo del
	// límite) y en la siguiente ya resuelve — no debería bloquear el
	// Goal, porque el éxito llega antes de agotar el margen.
	for i := 1; i < DefaultMaxPlanMapFailures; i++ {
		if _, ok, err := adapter.GetNextAction(ctx); ok || err == nil {
			t.Fatalf("intento %d: GetNextAction() = ok=%v err=%v, quería (false, error transitorio)", i, ok, err)
		}
	}
	req, ok, err := adapter.GetNextAction(ctx)
	if err != nil || !ok {
		t.Fatalf("GetNextAction() tras resolverse = %+v, %v, %v", req, ok, err)
	}
	if goal.Status == planning.GoalBlocked {
		t.Fatal("el Goal quedó Blocked a pesar de que el mapeo terminó resolviendo bien")
	}

	// Ahora que el contador está en cero (el último intento tuvo éxito),
	// una nueva racha de fallos necesita agotar DefaultMaxPlanMapFailures
	// completos de nuevo — no hereda los fallos previos.
	if err := adapter.UpdateState(ctx, req, VerificationResult{Success: true}); err != nil {
		t.Fatalf("UpdateState: %v", err)
	}
}

// TestGoalPlanningAdapter_SetMaxMapFailures confirma que el tope es
// configurable, no una constante enterrada en la lógica.
func TestGoalPlanningAdapter_SetMaxMapFailures(t *testing.T) {
	goal := planning.NewGoal("Meta con tope bajo", "desc", planning.PriorityMedium, "user")
	mapper := &alwaysFailMapper{}
	adapter, _ := newMapFailureAdapter(t, goal, mapper)
	adapter.SetMaxMapFailures(1)

	ctx := context.Background()
	if _, ok, err := adapter.GetNextAction(ctx); ok || err == nil {
		t.Fatalf("primer intento con tope=1: ok=%v err=%v, quería (false, error de bloqueo)", ok, err)
	}
	if goal.Status != planning.GoalBlocked {
		t.Fatalf("goal.Status = %v tras 1 fallo con tope=1, quería GoalBlocked de inmediato", goal.Status)
	}
	if mapper.calls != 1 {
		t.Fatalf("mapper.calls = %d, quería exactamente 1", mapper.calls)
	}
}

// TestGoalPlanningAdapter_SetMaxMapFailures_CeroONegativoUsaDefault cubre
// el mismo criterio que ya aplican NewReplanner/NewGoalMonitor: un tope
// en cero (o negativo) no debería dejar el adapter bloqueando todo al
// primer fallo por no haberlo configurado a propósito.
func TestGoalPlanningAdapter_SetMaxMapFailures_CeroONegativoUsaDefault(t *testing.T) {
	goal := planning.NewGoal("Meta con tope inválido", "desc", planning.PriorityMedium, "user")
	mapper := &alwaysFailMapper{}
	adapter, _ := newMapFailureAdapter(t, goal, mapper)
	adapter.SetMaxMapFailures(0)

	ctx := context.Background()
	for i := 1; i < DefaultMaxPlanMapFailures; i++ {
		if _, ok, err := adapter.GetNextAction(ctx); ok || err == nil {
			t.Fatalf("intento %d con tope=0 (-> default): ok=%v err=%v, quería (false, error)", i, ok, err)
		}
		if goal.Status == planning.GoalBlocked {
			t.Fatalf("intento %d: bloqueó antes de agotar el default", i)
		}
	}
	if _, ok, err := adapter.GetNextAction(ctx); ok || err == nil {
		t.Fatal("último intento: quería (false, error de bloqueo)")
	}
	if goal.Status != planning.GoalBlocked {
		t.Fatalf("goal.Status = %v, quería GoalBlocked tras agotar DefaultMaxPlanMapFailures", goal.Status)
	}
}
