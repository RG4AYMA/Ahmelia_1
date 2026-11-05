package autonomy

import (
	"context"
	"testing"

	"ahmelia/planning"
)

// TestGoalPlanningAdapter_PlansVacioAlPrincipio confirma que un adapter
// recién creado no tiene ningún plan cacheado — mismo criterio que
// Planner.Goals()/Tasks() de B90.2 para el caso vacío.
func TestGoalPlanningAdapter_PlansVacioAlPrincipio(t *testing.T) {
	goal := planning.NewGoal("Goal vacío", "", planning.PriorityMedium, "user")
	adapter, _, _ := newB81Adapter(t, goal)

	if plans := adapter.Plans(); len(plans) != 0 {
		t.Fatalf("Plans() = %v, quería vacío", plans)
	}
	if tasks := adapter.ActiveTasks(); len(tasks) != 0 {
		t.Fatalf("ActiveTasks() = %v, quería vacío", tasks)
	}
}

// TestGoalPlanningAdapter_PlansYActiveTasksReflejanElUsoReal confirma que,
// tras un ciclo GetNextAction/UpdateState real (no simulado), Plans() y
// ActiveTasks() exponen exactamente lo que el adapter generó internamente
// — mismos *Plan/*Task, no copias con datos equivalentes.
func TestGoalPlanningAdapter_PlansYActiveTasksReflejanElUsoReal(t *testing.T) {
	goal := planning.NewGoal(
		"B90.4 integration test",
		"validar Plans()/ActiveTasks() contra un ciclo real",
		planning.PriorityHigh,
		"user",
	)
	adapter, _, _ := newB81Adapter(t, goal)
	ctx := context.Background()

	req, ok, err := adapter.GetNextAction(ctx)
	if err != nil || !ok {
		t.Fatalf("GetNextAction() = (%+v, %v, %v), quería una acción válida", req, ok, err)
	}

	plans := adapter.Plans()
	if len(plans) != 1 {
		t.Fatalf("len(Plans()) = %d, quería 1", len(plans))
	}
	plan, ok := plans[goal.ID]
	if !ok {
		t.Fatalf("Plans() no tiene entrada para goal.ID=%q: %+v", goal.ID, plans)
	}

	active := adapter.ActiveTasks()
	if len(active) != 1 {
		t.Fatalf("len(ActiveTasks()) = %d, quería 1", len(active))
	}
	task, ok := active[req.ID]
	if !ok {
		t.Fatalf("ActiveTasks() no tiene entrada para req.ID=%q: %+v", req.ID, active)
	}
	if task.Status != planning.TaskInProgress {
		t.Fatalf("task.Status = %v, quería TaskInProgress (recién arrancada por GetNextAction)", task.Status)
	}

	// La Task activa debe pertenecer al Plan cacheado — misma identidad,
	// no una reconstrucción equivalente.
	found := false
	for _, t2 := range plan.Tasks {
		if t2 == task {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("la Task de ActiveTasks() no es el mismo puntero que la del Plan cacheado")
	}
}

// TestGoalPlanningAdapter_PlansEsCopiaDefensiva confirma que mutar el mapa
// devuelto por Plans() (agregar/quitar entradas) no toca el estado interno
// del adapter — mismo criterio que TestPlanner_GoalsSliceEsIndependiente
// de B90.2, adaptado a mapas.
func TestGoalPlanningAdapter_PlansEsCopiaDefensiva(t *testing.T) {
	goal := planning.NewGoal("Goal A", "", planning.PriorityMedium, "user")
	adapter, _, _ := newB81Adapter(t, goal)

	if _, _, err := adapter.GetNextAction(context.Background()); err != nil {
		t.Fatalf("GetNextAction(): %v", err)
	}

	plans := adapter.Plans()
	if len(plans) != 1 {
		t.Fatalf("setup: len(Plans()) = %d, quería 1", len(plans))
	}
	delete(plans, goal.ID)
	plans["goal-fantasma"] = &planning.Plan{}

	fresh := adapter.Plans()
	if len(fresh) != 1 {
		t.Fatalf("mutar el mapa devuelto por Plans() afectó el estado interno del adapter: %v", fresh)
	}
	if _, ok := fresh[goal.ID]; !ok {
		t.Fatalf("el plan original desapareció del adapter tras mutar la copia devuelta: %v", fresh)
	}
}

// TestGoalPlanningAdapter_LoadStateReemplazaEstado confirma el contrato
// central de LoadState: reemplaza integralmente plans/activeTasks (no
// hace merge), y los *Plan/*Task que carga pasan a ser los que
// GetNextAction/UpdateState usan de ahí en adelante — misma identidad, no
// una copia. También confirma que nil se trata como "vacío", no como "no
// tocar nada".
func TestGoalPlanningAdapter_LoadStateReemplazaEstado(t *testing.T) {
	goal := planning.NewGoal("Goal reconstruido", "", planning.PriorityMedium, "user")
	adapter, _, mapper := newB81Adapter(t, goal)

	task := planning.NewTask(goal.ID, "Tarea reconstruida", "desc", 1, 0)
	plan := &planning.Plan{
		GoalID: goal.ID,
		ID:     "plan-reconstruido",
		Tasks:  []*planning.Task{task},
	}

	adapter.LoadState(
		map[string]*planning.Plan{goal.ID: plan},
		map[string]*planning.Task{}, // ninguna Task activa tras un reinicio (ver regla acordada de B90)
	)

	if plans := adapter.Plans(); len(plans) != 1 || plans[goal.ID] != plan {
		t.Fatalf("Plans() tras LoadState() = %+v, quería {%q: plan}", plans, goal.ID)
	}
	if active := adapter.ActiveTasks(); len(active) != 0 {
		t.Fatalf("ActiveTasks() tras LoadState() = %v, quería vacío", active)
	}

	// GetNextAction debe usar el plan cargado (mismo *Task), no generar
	// uno nuevo — es la prueba de que LoadState reemplaza el estado real
	// que el resto del adapter consulta, no un mapa aislado.
	req, ok, err := adapter.GetNextAction(context.Background())
	if err != nil || !ok {
		t.Fatalf("GetNextAction() tras LoadState() = (%+v, %v, %v), quería la tarea reconstruida", req, ok, err)
	}
	if req.ID != task.ID {
		t.Fatalf("GetNextAction() devolvió la tarea %q, quería la reconstruida %q — LoadState no reemplazó el plan que usa el adapter", req.ID, task.ID)
	}
	if mapper.calls != 1 {
		t.Fatalf("mapper.calls = %d, quería 1 (una sola tarea disponible tras LoadState)", mapper.calls)
	}

	// Segunda llamada a LoadState reemplaza otra vez, no acumula.
	adapter.LoadState(nil, nil)
	if plans := adapter.Plans(); len(plans) != 0 {
		t.Fatalf("Plans() tras LoadState(nil, nil) = %v, quería vacío", plans)
	}
	if active := adapter.ActiveTasks(); len(active) != 0 {
		t.Fatalf("ActiveTasks() tras LoadState(nil, nil) = %v, quería vacío", active)
	}
}
