package core

import (
	"testing"

	"ahmelia/autonomy"
)

func validDiagnosis(recoverable bool, suggested FailureRecoveryStrategy, category FailureCategory) *FailureDiagnosis {
	req := autonomy.ActionRequest{ID: "a1"}
	resp := autonomy.ActionResponse{RequestID: "a1"}
	verif := autonomy.VerificationResult{Success: false, ErrorReason: "x"}
	ctx := NewFailureContext(req, resp, verif, nil)
	failure := NewFailure(category, FailureCauseAction, "x", recoverable, 0.7, suggested)
	return NewFailureDiagnosis(ctx, failure)
}

func TestB923RecoveryStrategist_NilOrInvalidDiagnosis_AsksUser(t *testing.T) {
	s := NewRecoveryStrategist()

	decision := s.Decide(nil, nil)
	if decision.Strategy != StrategyAskUser {
		t.Fatalf("diagnóstico nil debería pedir intervención, obtuve %q", decision.Strategy)
	}

	invalid := NewFailureDiagnosis(nil, nil)
	decision = s.Decide(invalid, nil)
	if decision.Strategy != StrategyAskUser {
		t.Fatalf("diagnóstico inválido debería pedir intervención, obtuve %q", decision.Strategy)
	}
}

func TestB923RecoveryStrategist_NonRecoverable_OverridesReplanRegardlessOfItsAnswer(t *testing.T) {
	s := NewRecoveryStrategist()
	diagnosis := validDiagnosis(false, StrategyAskUser, FailureCategoryPermission)

	// Aunque Replanner (B87) diga "reintentá", un fallo que el Diagnoser
	// marcó como no recuperable no debe reintentarse ciegamente.
	replan := &autonomy.ReplanDecision{Action: autonomy.ReplanRetryTask, Reason: "intento 1 de 2"}

	decision := s.Decide(diagnosis, replan)
	if decision.Strategy != StrategyAskUser {
		t.Fatalf("un fallo no recuperable debe imponerse sobre la decisión de Replanner, obtuve %q", decision.Strategy)
	}
}

func TestB923RecoveryStrategist_Recoverable_TranslatesReplanDecision(t *testing.T) {
	s := NewRecoveryStrategist()
	diagnosis := validDiagnosis(true, StrategyRetry, FailureCategoryMissingResource)

	cases := []struct {
		action autonomy.ReplanAction
		want   FailureRecoveryStrategy
	}{
		{autonomy.ReplanRetryTask, StrategyRetry},
		{autonomy.ReplanSkipTask, StrategyReplan},
		{autonomy.ReplanRegeneratePlan, StrategyReplan},
		{autonomy.ReplanGiveUp, StrategyBlock},
	}
	for _, tc := range cases {
		replan := &autonomy.ReplanDecision{Action: tc.action, Reason: "motivo de prueba"}
		decision := s.Decide(diagnosis, replan)
		if decision.Strategy != tc.want {
			t.Errorf("ReplanAction %q -> esperaba %q, obtuve %q", tc.action, tc.want, decision.Strategy)
		}
		if decision.Reason != replan.Reason {
			t.Errorf("el Reason de Replanner debería preservarse tal cual, obtuve %q", decision.Reason)
		}
	}
}

func TestB923RecoveryStrategist_Recoverable_NoReplanYet_UsesDiagnoserSuggestion(t *testing.T) {
	s := NewRecoveryStrategist()
	diagnosis := validDiagnosis(true, StrategyReplan, FailureCategoryEnvironmentMismatch)

	decision := s.Decide(diagnosis, nil)
	if decision.Strategy != StrategyReplan {
		t.Fatalf("sin ReplanDecision disponible, debería usar la sugerencia del Diagnoser, obtuve %q", decision.Strategy)
	}
}

func TestB923FailureRecoveryDecision_IsValid(t *testing.T) {
	valid := NewFailureRecoveryDecision(StrategyWait, "motivo")
	if !valid.IsValid() {
		t.Fatal("decisión con estrategia válida debería ser válida")
	}
	invalid := NewFailureRecoveryDecision("bogus", "motivo")
	if invalid.IsValid() {
		t.Fatal("decisión con estrategia inventada no debería ser válida")
	}
}

func TestB923TranslateReplanAction_UnknownFallsBackToAskUser(t *testing.T) {
	if got := translateReplanAction(autonomy.ReplanAction("bogus")); got != StrategyAskUser {
		t.Fatalf("una ReplanAction desconocida debería traducirse a ask_user, obtuve %q", got)
	}
}
