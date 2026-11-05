package autonomy

import (
	"context"
	"testing"

	"ahmelia/planning"
)

// TestGoalPlanningAdapter_EnsurePlan_MismoPunteroSinImportarQuienLlamaPrimero
// es la prueba central de B90.8.1: dos callers independientes (uno
// simulando B46 conversacional, otro simulando el ciclo autónomo)
// llamando EnsurePlan sobre el MISMO Goal.ID deben recibir el MISMO
// *planning.Plan — no dos planes con el mismo contenido, el mismo
// puntero. Esto es lo que demuestra que GoalPlanningAdapter es ahora el
// único dueño, no que "los dos lados tienen los mismos datos".
func TestGoalPlanningAdapter_EnsurePlan_MismoPunteroSinImportarQuienLlamaPrimero(t *testing.T) {
	state := New()
	monitor := NewGoalMonitor(state, 0)
	planner := planning.NewPlanner()
	mapper := &b81TaskMapper{}

	adapter, err := NewGoalPlanningAdapter(state, monitor, planner, mapper, nil, nil)
	if err != nil {
		t.Fatalf("NewGoalPlanningAdapter: %v", err)
	}

	goal := planning.NewGoal("Meta B90.8.1", "desc", planning.PriorityMedium, "user")
	planner.RegisterGoal(goal)

	planB46, err := adapter.EnsurePlan(goal)
	if err != nil {
		t.Fatalf("EnsurePlan (B46): %v", err)
	}
	planAuto, err := adapter.EnsurePlan(goal)
	if err != nil {
		t.Fatalf("EnsurePlan (autónomo): %v", err)
	}

	if planB46 != planAuto {
		t.Fatalf("EnsurePlan devolvió punteros distintos: %p (B46) != %p (autónomo) — dos Plans para el mismo Goal", planB46, planAuto)
	}
	if len(planB46.Tasks) == 0 {
		t.Fatal("el plan generado no tiene Tasks — no hay nada real que verificar")
	}

	// La Task que ve el plan tiene que ser EXACTAMENTE (mismo puntero) la
	// que está registrada en el Planner — si fueran copias, mutar una
	// (task.Start(), task.Complete()) no se reflejaría en la otra, y
	// B90.7 persistiría un estado distinto del que el ciclo autónomo
	// realmente está ejecutando.
	registered := make(map[string]*planning.Task)
	for _, tsk := range planner.Tasks() {
		registered[tsk.ID] = tsk
	}
	for _, tsk := range planB46.Tasks {
		got, ok := registered[tsk.ID]
		if !ok {
			t.Fatalf("Task %q del plan no está registrada en planner.Tasks()", tsk.ID)
		}
		if got != tsk {
			t.Fatalf("Task %q: planner.Tasks() devuelve un puntero distinto (%p) al del plan (%p)", tsk.ID, got, tsk)
		}
	}
}

// TestGoalPlanningAdapter_EnsurePlan_GetNextActionUsaElMismoPlan verifica
// el segundo ángulo del mismo hallazgo: si EnsurePlan ya fue llamado
// externamente (como haría B46 al crear el proyecto) ANTES de que el
// ciclo autónomo toque el Goal por primera vez, GetNextAction no debe
// generar un segundo plan — tiene que operar sobre las Tasks que EnsurePlan
// ya cacheó.
func TestGoalPlanningAdapter_EnsurePlan_GetNextActionUsaElMismoPlan(t *testing.T) {
	state := New()
	monitor := NewGoalMonitor(state, 0)
	planner := planning.NewPlanner()
	mapper := &b81TaskMapper{}

	adapter, err := NewGoalPlanningAdapter(state, monitor, planner, mapper, nil, nil)
	if err != nil {
		t.Fatalf("NewGoalPlanningAdapter: %v", err)
	}

	goal := planning.NewGoal("Meta previa a autónomo", "desc", planning.PriorityMedium, "user")
	planner.RegisterGoal(goal)

	// Simula B46: alguien ya generó el plan del Goal antes de que el
	// ciclo autónomo lo seleccione.
	planPrevio, err := adapter.EnsurePlan(goal)
	if err != nil {
		t.Fatalf("EnsurePlan previo: %v", err)
	}
	primeraTask := planPrevio.Tasks[0]

	state.SetCurrentGoal(goal)

	ctx := context.Background()
	req, ok, err := adapter.GetNextAction(ctx)
	if err != nil || !ok {
		t.Fatalf("GetNextAction() = %+v, %v, %v", req, ok, err)
	}

	if req.ID != primeraTask.ID {
		t.Fatalf("GetNextAction operó sobre la Task %q, quería la ya cacheada por EnsurePlan (%q) — generó un plan nuevo en vez de reusar el existente", req.ID, primeraTask.ID)
	}

	plans := adapter.Plans()
	if plans[goal.ID] != planPrevio {
		t.Fatalf("adapter.Plans()[%q] = %p, quería el mismo puntero que EnsurePlan ya había devuelto (%p)", goal.ID, plans[goal.ID], planPrevio)
	}
}

// TestGoalPlanningAdapter_EnsurePlan_GoalNil cubre el caso trivial: no
// debe hacer panic ni cachear nada con un Goal nil.
func TestGoalPlanningAdapter_EnsurePlan_GoalNil(t *testing.T) {
	state := New()
	monitor := NewGoalMonitor(state, 0)
	planner := planning.NewPlanner()
	mapper := &b81TaskMapper{}

	adapter, err := NewGoalPlanningAdapter(state, monitor, planner, mapper, nil, nil)
	if err != nil {
		t.Fatalf("NewGoalPlanningAdapter: %v", err)
	}

	if _, err := adapter.EnsurePlan(nil); err == nil {
		t.Fatal("EnsurePlan(nil) = nil error, quería un error explícito")
	}
}
