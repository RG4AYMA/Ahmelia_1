package core

import (
	"context"
	"testing"

	"ahmelia/autonomy"
	"ahmelia/planning"
)

func TestB924DiagnosingRecoveryAdvisor_NonRecoverable_AdvisesGiveUp(t *testing.T) {
	advisor := NewDiagnosingRecoveryAdvisor()
	req := autonomy.ActionRequest{ID: "task-1"}
	verif := autonomy.VerificationResult{Success: false, ErrorReason: "permiso denegado"}

	if err := advisor.DiagnoseFailure(context.Background(), req, verif); err != nil {
		t.Fatalf("DiagnoseFailure: %v", err)
	}

	action, reason, ok := advisor.Advise(req.ID)
	if !ok {
		t.Fatal("un fallo de permiso (no recuperable) debería producir una opinión")
	}
	if action != autonomy.ReplanGiveUp {
		t.Fatalf("action = %q, esperaba %q", action, autonomy.ReplanGiveUp)
	}
	if reason == "" {
		t.Fatal("el motivo no debería quedar vacío: se pierde auditabilidad")
	}
}

func TestB924DiagnosingRecoveryAdvisor_ConsumesDiagnosisOnce(t *testing.T) {
	advisor := NewDiagnosingRecoveryAdvisor()
	req := autonomy.ActionRequest{ID: "task-1"}
	verif := autonomy.VerificationResult{Success: false, ErrorReason: "permiso denegado"}

	if err := advisor.DiagnoseFailure(context.Background(), req, verif); err != nil {
		t.Fatalf("DiagnoseFailure: %v", err)
	}

	if _, _, ok := advisor.Advise(req.ID); !ok {
		t.Fatal("la primera consulta debería tener opinión")
	}
	if _, _, ok := advisor.Advise(req.ID); ok {
		t.Fatal("un mismo diagnóstico no debe seguir vetando después de consumirse una vez")
	}
}

func TestB924DiagnosingRecoveryAdvisor_Recoverable_NoOpinion(t *testing.T) {
	advisor := NewDiagnosingRecoveryAdvisor()
	req := autonomy.ActionRequest{ID: "task-2"}
	// "contexto cancelado" clasifica como timeout, recuperable, sugerencia
	// retry (ver core/autonomy_diagnoser.go) — Replanner debe seguir
	// decidiendo solo en este caso.
	verif := autonomy.VerificationResult{Success: false, ErrorReason: "contexto cancelado"}

	if err := advisor.DiagnoseFailure(context.Background(), req, verif); err != nil {
		t.Fatalf("DiagnoseFailure: %v", err)
	}

	if _, _, ok := advisor.Advise(req.ID); ok {
		t.Fatal("un fallo recuperable no debe vetar a Replanner: Advise debía devolver ok=false")
	}
}

func TestB924DiagnosingRecoveryAdvisor_ObservedContradictionChangesContinuation(t *testing.T) {
	advisor := NewDiagnosingRecoveryAdvisor()
	req := autonomy.ActionRequest{ID: "task-observed-contradiction", Action: "renombrar_archivo"}
	verif := autonomy.VerificationResult{
		Success:            false,
		ErrorReason:        "la ejecución técnica terminó correctamente, pero el entorno no confirmó el resultado",
		ObservationStatus:  autonomy.ObservationContradicted,
		ObservationDetails: "origen y destino no coinciden con el postcondición esperada",
	}
	if err := advisor.DiagnoseFailure(context.Background(), req, verif); err != nil {
		t.Fatalf("DiagnoseFailure: %v", err)
	}
	action, reason, ok := advisor.Advise(req.ID)
	if !ok {
		t.Fatal("la contradicción ambiental debe llegar a la autoridad de continuación")
	}
	if action != autonomy.ReplanRegeneratePlan {
		t.Fatalf("action=%q, want %q", action, autonomy.ReplanRegeneratePlan)
	}
	if reason == "" {
		t.Fatal("la decisión debe conservar un motivo auditable")
	}
}

func TestB924DiagnosingRecoveryAdvisor_NoDiagnosisYet_NoOpinion(t *testing.T) {
	advisor := NewDiagnosingRecoveryAdvisor()
	if _, _, ok := advisor.Advise("tarea-nunca-diagnosticada"); ok {
		t.Fatal("sin diagnóstico guardado, Advise debe devolver ok=false")
	}
}

func TestB924DiagnosingRecoveryAdvisor_SuccessfulVerification_StoresNothing(t *testing.T) {
	advisor := NewDiagnosingRecoveryAdvisor()
	req := autonomy.ActionRequest{ID: "task-3"}
	verif := autonomy.VerificationResult{Success: true, Output: "ok"}

	if err := advisor.DiagnoseFailure(context.Background(), req, verif); err != nil {
		t.Fatalf("DiagnoseFailure: %v", err)
	}
	if _, _, ok := advisor.Advise(req.ID); ok {
		t.Fatal("una verificación exitosa no describe un fallo: no debería quedar diagnóstico guardado")
	}
}

func TestB924TranslateFailureStrategyToReplanAction(t *testing.T) {
	cases := []struct {
		strategy FailureRecoveryStrategy
		want     autonomy.ReplanAction
	}{
		{StrategyReplan, autonomy.ReplanRegeneratePlan},
		{StrategyBlock, autonomy.ReplanGiveUp},
		{StrategyAskUser, autonomy.ReplanGiveUp},
		{StrategyWait, autonomy.ReplanGiveUp},
		// No debería ocurrir en la práctica (Failure.IsValid lo prohíbe
		// cuando Recoverable es false), pero el mapeo debe seguir siendo
		// conservador y nunca reintentar por su cuenta.
		{StrategyRetry, autonomy.ReplanGiveUp},
	}
	for _, tc := range cases {
		if got := translateFailureStrategyToReplanAction(tc.strategy); got != tc.want {
			t.Errorf("translateFailureStrategyToReplanAction(%q) = %q, esperaba %q", tc.strategy, got, tc.want)
		}
	}
}

// TestB924Integration_NonRecoverableFailure_BlocksGoalImmediately es la
// prueba de comportamiento real de B92.4: con los límites POR DEFECTO de
// Replanner (2 reintentos antes de darse por vencido — ver
// autonomy.DefaultReplanLimits), un fallo que el Diagnoser clasifica como
// no recuperable debe bloquear el Goal en el PRIMER fallo, exactamente
// como en la cadena real Cycle.diagnoser.DiagnoseFailure ->
// Cycle.updater.UpdateState. Sin B92.4, este mismo escenario reintentaría
// dos veces antes de bloquear (ver TestGoalPlanningAdapter_RetriesBeforeBlocking
// en el paquete autonomy).
func TestB924Integration_NonRecoverableFailure_BlocksGoalImmediately(t *testing.T) {
	planner, state, adapter := newB912Runtime(t)
	goal := planning.NewGoal("B92.4 e2e", "permiso denegado no debe reintentarse", planning.PriorityHigh, "ahmelia")
	planner.RegisterGoal(goal)
	state.SetCurrentGoal(goal)

	advisor := NewDiagnosingRecoveryAdvisor()
	adapter.SetFailureAdvisor(advisor)

	req, ok, err := adapter.GetNextAction(context.Background())
	if err != nil || !ok {
		t.Fatalf("GetNextAction: ok=%v err=%v", ok, err)
	}

	verif := autonomy.VerificationResult{Success: false, ErrorReason: "permiso denegado"}

	// Cadena real: primero DIAGNOSE (Cycle.diagnoser), después UPDATING
	// (Cycle.updater) — ver autonomy/cycle.go, fase de fallo.
	if err := advisor.DiagnoseFailure(context.Background(), req, verif); err != nil {
		t.Fatalf("DiagnoseFailure: %v", err)
	}
	if err := adapter.UpdateState(context.Background(), req, verif); err != nil {
		t.Fatalf("UpdateState: %v", err)
	}

	if goal.Status != planning.GoalBlocked {
		t.Fatalf("goal.Status = %q, esperaba %q: un permiso denegado no debe reintentarse pese a los límites por defecto de Replanner", goal.Status, planning.GoalBlocked)
	}
}

// TestB924Integration_RecoverableFailure_StillRetriesNormally confirma que
// B92.4 no cambia el camino feliz de B87: sin diagnóstico o con un
// diagnóstico recuperable, un solo fallo sigue reintentando en vez de
// bloquear, con los mismos límites por defecto que el test anterior.
func TestB924Integration_RecoverableFailure_StillRetriesNormally(t *testing.T) {
	planner, state, adapter := newB912Runtime(t)
	goal := planning.NewGoal("B92.4 e2e recuperable", "un timeout sí debe reintentarse", planning.PriorityHigh, "ahmelia")
	planner.RegisterGoal(goal)
	state.SetCurrentGoal(goal)

	advisor := NewDiagnosingRecoveryAdvisor()
	adapter.SetFailureAdvisor(advisor)

	req, ok, err := adapter.GetNextAction(context.Background())
	if err != nil || !ok {
		t.Fatalf("GetNextAction: ok=%v err=%v", ok, err)
	}

	verif := autonomy.VerificationResult{Success: false, ErrorReason: "contexto cancelado"}

	if err := advisor.DiagnoseFailure(context.Background(), req, verif); err != nil {
		t.Fatalf("DiagnoseFailure: %v", err)
	}
	if err := adapter.UpdateState(context.Background(), req, verif); err != nil {
		t.Fatalf("UpdateState: %v", err)
	}

	if goal.Status == planning.GoalBlocked {
		t.Fatal("un fallo recuperable (timeout) no debe bloquear el Goal en el primer intento")
	}
}
