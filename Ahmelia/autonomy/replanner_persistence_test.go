package autonomy

import (
	"context"
	"sync"
	"testing"

	"ahmelia/planning"
)

// TestReplanner_RegenerationsVacioAlPrincipio confirma que un Replanner
// recién creado no tiene ninguna regeneración registrada — mismo criterio
// que Planner.Goals()/Tasks() (B90.2) y GoalPlanningAdapter.Plans()/
// ActiveTasks() (B90.4) para el caso vacío.
func TestReplanner_RegenerationsVacioAlPrincipio(t *testing.T) {
	r := NewReplanner(ReplanLimits{})
	if regen := r.Regenerations(); len(regen) != 0 {
		t.Fatalf("Regenerations() = %v, quería vacío", regen)
	}
}

// TestReplanner_RegenerationsDespuesDeRecordRegeneration confirma que
// Regenerations() refleja lo que RecordRegeneration ya venía acumulando
// desde B87 — sin este bloque, ese contador no era observable desde
// afuera.
func TestReplanner_RegenerationsDespuesDeRecordRegeneration(t *testing.T) {
	r := NewReplanner(ReplanLimits{})
	r.RecordRegeneration("goal-A")
	r.RecordRegeneration("goal-A")
	r.RecordRegeneration("goal-B")

	regen := r.Regenerations()
	if regen["goal-A"] != 2 || regen["goal-B"] != 1 {
		t.Fatalf("Regenerations() = %v, quería {goal-A:2, goal-B:1}", regen)
	}
}

// TestReplanner_RegenerationsDespuesDeUnCicloReal reproduce el mismo
// camino que TestGoalPlanningAdapter_ReplanEventuallyGivesUp (B87): dos
// fallos consecutivos sobre la primera tarea de un plan con dependencias
// agotan los reintentos y disparan una regeneración real vía
// GoalPlanningAdapter.UpdateState -> Replanner.Decide/RecordRegeneration —
// no una llamada directa y aislada a RecordRegeneration como el test de
// arriba. Confirma que Regenerations(), a través de
// GoalPlanningAdapter.Replanner(), refleja el estado que el ciclo real deja.
func TestReplanner_RegenerationsDespuesDeUnCicloReal(t *testing.T) {
	goal := planning.NewGoal(
		"B90.5 ciclo real",
		"validar Regenerations() tras una regeneración disparada por UpdateState",
		planning.PriorityMedium,
		"user",
	)
	adapter, _, _ := newB81Adapter(t, goal)
	ctx := context.Background()

	fail := func(reason string) {
		req, ok, err := adapter.GetNextAction(ctx)
		if err != nil || !ok {
			t.Fatalf("GetNextAction: ok=%v err=%v", ok, err)
		}
		if err := adapter.UpdateState(ctx, req, VerificationResult{
			Success:     false,
			ErrorReason: reason,
		}); err != nil {
			t.Fatalf("UpdateState: %v", err)
		}
	}

	// Mismo patrón que TestGoalPlanningAdapter_ReplanEventuallyGivesUp:
	// con DefaultReplanLimits (MaxTaskRetries=2) y un plan por defecto
	// donde la segunda tarea depende de la primera, dos fallos sobre la
	// primera tarea agotan el reintento y disparan ReplanRegeneratePlan.
	fail("fallo 1")
	if regen := adapter.Replanner().Regenerations(); regen[goal.ID] != 0 {
		t.Fatalf("Regenerations()[%q] = %d tras el primer fallo, quería 0 (todavía no se agotó el reintento)", goal.ID, regen[goal.ID])
	}

	fail("fallo 2")
	regen := adapter.Replanner().Regenerations()
	if regen[goal.ID] != 1 {
		t.Fatalf("Regenerations()[%q] = %d tras el segundo fallo, quería 1 (regeneración real disparada por UpdateState)", goal.ID, regen[goal.ID])
	}
}

// TestReplanner_RegenerationsEsCopiaDefensiva confirma que mutar el mapa
// devuelto por Regenerations() no toca el estado interno del Replanner —
// mismo criterio que TestGoalPlanningAdapter_PlansEsCopiaDefensiva (B90.4).
func TestReplanner_RegenerationsEsCopiaDefensiva(t *testing.T) {
	r := NewReplanner(ReplanLimits{})
	r.RecordRegeneration("goal-A")

	regen := r.Regenerations()
	regen["goal-A"] = 99
	regen["goal-fantasma"] = 1

	fresh := r.Regenerations()
	if fresh["goal-A"] != 1 {
		t.Fatalf("mutar el mapa devuelto por Regenerations() afectó el estado interno: %v", fresh)
	}
	if _, ok := fresh["goal-fantasma"]; ok {
		t.Fatalf("mutar el mapa devuelto por Regenerations() agregó una entrada fantasma al estado interno: %v", fresh)
	}
}

// TestReplanner_LoadRegenerationsReemplazaNoMergea confirma el contrato
// central de LoadRegenerations: reemplaza integralmente el contador, no
// hace merge con lo que ya había — mismo contrato que
// GoalPlanningAdapter.LoadState (B90.4).
func TestReplanner_LoadRegenerationsReemplazaNoMergea(t *testing.T) {
	r := NewReplanner(ReplanLimits{})
	r.RecordRegeneration("goal-viejo")
	r.RecordRegeneration("goal-viejo")

	r.LoadRegenerations(map[string]int{"goal-nuevo": 3})

	regen := r.Regenerations()
	if len(regen) != 1 || regen["goal-nuevo"] != 3 {
		t.Fatalf("Regenerations() tras LoadRegenerations() = %v, quería {goal-nuevo:3} (reemplazo, no merge)", regen)
	}

	r.LoadRegenerations(nil)
	if regen := r.Regenerations(); len(regen) != 0 {
		t.Fatalf("Regenerations() tras LoadRegenerations(nil) = %v, quería vacío", regen)
	}
}

// TestReplanner_LoadRegenerationsRestauracionEfectiva confirma que
// LoadRegenerations no es solo un valor observable: Decide() debe
// consultar el contador restaurado, no reiniciar la cuenta desde cero. Con
// MaxPlanRegenerations=1 y una regeneración ya cargada, la próxima tarea
// que agote reintentos debe ir directo a ReplanGiveUp, no a
// ReplanRegeneratePlan otra vez.
func TestReplanner_LoadRegenerationsRestauracionEfectiva(t *testing.T) {
	limits := ReplanLimits{MaxTaskRetries: 1, MaxPlanRegenerations: 1}
	r := NewReplanner(limits)
	r.LoadRegenerations(map[string]int{"goal-A": 1}) // ya se agotó la única regeneración permitida

	goal := planning.NewGoal("goal-A-nombre", "", planning.PriorityMedium, "user")
	goal.ID = "goal-A" // fuerza el mismo ID que se restauró
	task1 := planning.NewTask(goal.ID, "Tarea 1", "desc", 1, 0)
	task2 := planning.NewTask(goal.ID, "Tarea 2", "desc", 2, 0)
	task2.Dependencies = []string{task1.ID} // hasPendingDependent(task1) == true
	plan := &planning.Plan{GoalID: goal.ID, ID: "plan-1", Tasks: []*planning.Task{task1, task2}}

	task1.Start()  // Attempts = 1
	task1.Fail("") // Status = TaskBlocked, Attempts sigue en 1 (>= MaxTaskRetries=1)

	decision := r.Decide(goal, plan, task1)
	if decision.Action != ReplanGiveUp {
		t.Fatalf("Decide() tras restaurar regenerations=1 (límite=1) = %+v, quería ReplanGiveUp", decision)
	}
}

// TestGoalPlanningAdapter_ReplannerAccessorDevuelveElInterno confirma el
// segundo accessor de B90.5: GoalPlanningAdapter.Replanner() debe devolver
// el mismo *Replanner que el adapter usa internamente en Decide/
// RecordRegeneration — no una instancia nueva ni una copia. Se prueba
// registrando una regeneración vía el accessor y confirmando que un ciclo
// real (Decide interno) la ve.
func TestGoalPlanningAdapter_ReplannerAccessorDevuelveElInterno(t *testing.T) {
	goal := planning.NewGoal("Goal A", "", planning.PriorityMedium, "user")
	adapter, _, _ := newB81Adapter(t, goal)

	adapter.Replanner().RecordRegeneration(goal.ID)

	if regen := adapter.Replanner().Regenerations(); regen[goal.ID] != 1 {
		t.Fatalf("Regenerations()[%q] = %d tras RecordRegeneration vía el accessor, quería 1", goal.ID, regen[goal.ID])
	}
}

// TestReplanner_RegenerationsConcurrencia confirma que Regenerations()/
// LoadRegenerations() son seguras bajo -race junto con el uso normal de
// RecordRegeneration/Decide — mismo patrón que
// TestGoalPlanningAdapter_ConcurrentReadsAreSafe (B81/B90.4): todas pasan
// por r.mu, así que no hay una garantía nueva que inventar acá, solo
// confirmar que las dos funciones nuevas respetan el mutex existente.
func TestReplanner_RegenerationsConcurrencia(t *testing.T) {
	r := NewReplanner(ReplanLimits{})

	const workers = 16
	var wg sync.WaitGroup
	wg.Add(workers * 2)
	for i := 0; i < workers; i++ {
		go func(n int) {
			defer wg.Done()
			r.RecordRegeneration("goal-concurrente")
		}(i)
		go func() {
			defer wg.Done()
			_ = r.Regenerations()
		}()
	}
	wg.Wait()

	if regen := r.Regenerations(); regen["goal-concurrente"] != workers {
		t.Fatalf("Regenerations()[goal-concurrente] = %d, quería %d tras %d incrementos concurrentes", regen["goal-concurrente"], workers, workers)
	}
}
