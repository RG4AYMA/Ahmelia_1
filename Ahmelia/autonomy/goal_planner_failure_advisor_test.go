package autonomy

import (
	"context"
	"testing"

	"ahmelia/planning"
)

// stubFailureAdvisor permite a los tests de B92.4 fijar de antemano qué
// debe responder Advise para un taskID dado, sin depender del Diagnoser/
// RecoveryStrategist reales de core (autonomy no puede importar core, ver
// FailureAdvisor en goal_planner.go).
type stubFailureAdvisor struct {
	calls    []string
	response map[string]struct {
		action ReplanAction
		reason string
		ok     bool
	}
}

func (s *stubFailureAdvisor) Advise(taskID string) (ReplanAction, string, bool) {
	s.calls = append(s.calls, taskID)
	r, ok := s.response[taskID]
	if !ok {
		return "", "", false
	}
	return r.action, r.reason, r.ok
}

// TestGoalPlanningAdapter_NoAdvisor_BehavesExactlyAsBeforeB924 es la red de
// seguridad de compatibilidad: sin llamar nunca a SetFailureAdvisor (el
// caso de los 16 call sites existentes antes de B92.4), el resultado debe
// ser idéntico al de TestGoalPlanningAdapter_ImmediateGiveUpBlocksGoal.
func TestGoalPlanningAdapter_NoAdvisor_BehavesExactlyAsBeforeB924(t *testing.T) {
	goal := planning.NewGoal(
		"B92.4 sin advisor",
		"validar que el adapter sin advisor configurado no cambia de comportamiento",
		planning.PriorityMedium,
		"user",
	)

	replanner := NewReplanner(ReplanLimits{MaxTaskRetries: -1, MaxPlanRegenerations: -1})
	adapter, _, _ := newB81AdapterWithReplanner(t, goal, replanner)
	// Deliberadamente sin adapter.SetFailureAdvisor(...): a.advisor queda nil.

	req, ok, err := adapter.GetNextAction(context.Background())
	if err != nil || !ok {
		t.Fatalf("GetNextAction: ok=%v err=%v", ok, err)
	}

	if err := adapter.UpdateState(context.Background(), req, VerificationResult{
		Success:     false,
		ErrorReason: "fallo de ejecución de prueba",
	}); err != nil {
		t.Fatalf("UpdateState: %v", err)
	}

	if goal.Status != planning.GoalBlocked {
		t.Fatalf("goal.Status = %q, esperaba %q (advisor nil no debe cambiar nada)", goal.Status, planning.GoalBlocked)
	}
}

// TestGoalPlanningAdapter_AdvisorOverridesReplanner_NoOpinionFallsBackToIt
// cubre las dos mitades del contrato de FailureAdvisor en un mismo test:
//  1. Replanner (con límites generosos, reintentaría) es VETADO cuando el
//     advisor tiene opinión (ok=true) para esa tarea puntual — se bloquea
//     el objetivo de inmediato, sin haber agotado ningún reintento.
//  2. Un segundo Goal/tarea, para la que el advisor NO tiene opinión
//     (ok=false), sigue exactamente el camino normal de Replanner.
func TestGoalPlanningAdapter_AdvisorOverridesReplanner_NoOpinionFallsBackToIt(t *testing.T) {
	// Límites generosos a propósito: si el advisor no vetara, este primer
	// fallo reintentaría en vez de bloquear (ver
	// TestGoalPlanningAdapter_RetriesBeforeBlocking, mismo límite por
	// defecto).
	goalVetoed := planning.NewGoal(
		"B92.4 con veto del advisor",
		"un fallo no recuperable diagnosticado externamente debe bloquear de inmediato",
		planning.PriorityMedium,
		"user",
	)
	adapter, _, _ := newB81Adapter(t, goalVetoed)

	req, ok, err := adapter.GetNextAction(context.Background())
	if err != nil || !ok {
		t.Fatalf("GetNextAction: ok=%v err=%v", ok, err)
	}

	advisor := &stubFailureAdvisor{
		response: map[string]struct {
			action ReplanAction
			reason string
			ok     bool
		}{
			req.ID: {action: ReplanGiveUp, reason: "diagnóstico externo: permiso denegado, no recuperable", ok: true},
		},
	}
	adapter.SetFailureAdvisor(advisor)

	if err := adapter.UpdateState(context.Background(), req, VerificationResult{
		Success:     false,
		ErrorReason: "permiso denegado",
	}); err != nil {
		t.Fatalf("UpdateState: %v", err)
	}

	if goalVetoed.Status != planning.GoalBlocked {
		t.Fatalf("goal.Status = %q, esperaba %q: el advisor debía vetar el reintento de Replanner", goalVetoed.Status, planning.GoalBlocked)
	}
	if len(advisor.calls) != 1 || advisor.calls[0] != req.ID {
		t.Fatalf("Advise debía consultarse exactamente una vez con taskID=%q, calls=%v", req.ID, advisor.calls)
	}

	// Segundo Goal, mismo adapter/advisor: el advisor no tiene opinión
	// (no está en advisor.response) -> debe comportarse como Replanner de
	// siempre, es decir reintentar en vez de bloquear.
	goalNormal := planning.NewGoal(
		"B92.4 sin opinión del advisor",
		"sin diagnóstico externo, Replanner sigue decidiendo solo",
		planning.PriorityMedium,
		"user",
	)
	adapter.state.SetCurrentGoal(goalNormal)
	adapter.planner.RegisterGoal(goalNormal)

	req2, ok, err := adapter.GetNextAction(context.Background())
	if err != nil || !ok {
		t.Fatalf("GetNextAction (goal normal): ok=%v err=%v", ok, err)
	}

	if err := adapter.UpdateState(context.Background(), req2, VerificationResult{
		Success:     false,
		ErrorReason: "fallo transitorio de prueba",
	}); err != nil {
		t.Fatalf("UpdateState (goal normal): %v", err)
	}

	if goalNormal.Status == planning.GoalBlocked {
		t.Fatal("sin opinión del advisor, un solo fallo no debe bloquear el objetivo (límites por defecto de B87)")
	}
}
