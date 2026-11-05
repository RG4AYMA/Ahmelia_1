package core

import (
	"errors"
	"testing"

	"ahmelia/autonomy"
)

func TestB921FailureCategory_IsValid(t *testing.T) {
	valid := []FailureCategory{
		FailureCategoryMissingResource,
		FailureCategoryPermission,
		FailureCategoryInvalidParameters,
		FailureCategoryEnvironmentMismatch,
		FailureCategoryTimeout,
		FailureCategoryExecutorError,
		FailureCategoryUnknown,
	}
	for _, c := range valid {
		if !c.IsValid() {
			t.Fatalf("category %q debería ser válida", c)
		}
	}
	if FailureCategory("invented").IsValid() {
		t.Fatal("una categoría inventada no debería ser válida")
	}
}

func TestB921FailureCause_IsValid(t *testing.T) {
	valid := []FailureCause{
		FailureCauseAction,
		FailureCauseVerification,
		FailureCauseEnvironment,
		FailureCauseUnknown,
	}
	for _, c := range valid {
		if !c.IsValid() {
			t.Fatalf("cause %q debería ser válida", c)
		}
	}
	if FailureCause("invented").IsValid() {
		t.Fatal("una causa inventada no debería ser válida")
	}
}

func TestB921FailureRecoveryStrategy_IsValid(t *testing.T) {
	valid := []FailureRecoveryStrategy{
		StrategyRetry, StrategyReplan, StrategyWait, StrategyBlock, StrategyAskUser,
	}
	for _, s := range valid {
		if !s.IsValid() {
			t.Fatalf("strategy %q debería ser válida", s)
		}
	}
	if FailureRecoveryStrategy("invented").IsValid() {
		t.Fatal("una estrategia inventada no debería ser válida")
	}
}

func TestB921FailureRecoveryStrategy_DoesNotCollideWithB91RecoveryDecision(t *testing.T) {
	// B91's RecoveryDecision answers "resume this Goal after restart?".
	// B92's FailureRecoveryStrategy answers "what to do about one failed
	// Action in a live cycle?". They must remain distinct types with
	// disjoint value sets so no caller can accidentally compare across
	// domains.
	strategies := map[string]bool{
		string(StrategyRetry):   true,
		string(StrategyReplan):  true,
		string(StrategyWait):    true,
		string(StrategyBlock):   true,
		string(StrategyAskUser): true,
	}
	decisions := []RecoveryDecision{RecoveryResume, RecoveryIdle, RecoveryBlocked, RecoveryNeedsUser}
	for _, d := range decisions {
		if strategies[string(d)] {
			t.Fatalf("RecoveryDecision %q colisiona con un valor de FailureRecoveryStrategy", d)
		}
	}
}

func TestB921Failure_IsValid(t *testing.T) {
	f := NewFailure(
		FailureCategoryMissingResource,
		FailureCauseVerification,
		"archivo no existe: informe.txt",
		true,
		0.9,
		StrategyReplan,
	)
	if !f.IsValid() {
		t.Fatal("failure bien formado debería ser válido")
	}
}

func TestB921Failure_InvalidCases(t *testing.T) {
	cases := []struct {
		name string
		f    *Failure
	}{
		{"nil", nil},
		{"categoria invalida", NewFailure("bogus", FailureCauseAction, "e", true, 0.5, StrategyRetry)},
		{"causa invalida", NewFailure(FailureCategoryUnknown, "bogus", "e", true, 0.5, StrategyRetry)},
		{"estrategia invalida", NewFailure(FailureCategoryUnknown, FailureCauseUnknown, "e", true, 0.5, "bogus")},
		{"confianza negativa", NewFailure(FailureCategoryUnknown, FailureCauseUnknown, "e", true, -0.1, StrategyWait)},
		{"confianza mayor a 1", NewFailure(FailureCategoryUnknown, FailureCauseUnknown, "e", true, 1.1, StrategyWait)},
		{"retry sin recoverable", NewFailure(FailureCategoryUnknown, FailureCauseUnknown, "e", false, 0.5, StrategyRetry)},
	}
	for _, tc := range cases {
		if tc.f.IsValid() {
			t.Fatalf("%s: esperaba inválido", tc.name)
		}
	}
}

func TestB921Failure_NonRecoverableAllowsNonRetryStrategies(t *testing.T) {
	for _, s := range []FailureRecoveryStrategy{StrategyReplan, StrategyWait, StrategyBlock, StrategyAskUser} {
		f := NewFailure(FailureCategoryPermission, FailureCauseAction, "permiso denegado", false, 0.8, s)
		if !f.IsValid() {
			t.Fatalf("estrategia %q debería seguir siendo válida sin Recoverable", s)
		}
	}
}

func TestB921FailureContext_ValidRequiresFailedVerification(t *testing.T) {
	req := autonomy.ActionRequest{ID: "a1", Action: "mover_archivo"}
	resp := autonomy.ActionResponse{RequestID: "a1", Executed: false, Error: errors.New("archivo no existe")}

	failedVerif := autonomy.VerificationResult{Success: false, ErrorReason: "archivo no existe"}
	ctx := NewFailureContext(req, resp, failedVerif, nil)
	if !ctx.Valid() {
		t.Fatal("un contexto sobre una verificación fallida debería ser válido")
	}

	successVerif := autonomy.VerificationResult{Success: true}
	ctxOK := NewFailureContext(req, resp, successVerif, nil)
	if ctxOK.Valid() {
		t.Fatal("un contexto sobre una verificación exitosa no debería ser un fallo diagnosticable")
	}
}

func TestB921FailureContext_PreservesOptionalSnapshot(t *testing.T) {
	req := autonomy.ActionRequest{ID: "a1"}
	resp := autonomy.ActionResponse{RequestID: "a1"}
	verif := autonomy.VerificationResult{Success: false, ErrorReason: "mismatch"}
	snap := &autonomy.EnvironmentSnapshot{FocusedApp: "explorer"}

	ctx := NewFailureContext(req, resp, verif, snap)
	if ctx.Snapshot != snap {
		t.Fatal("el snapshot debe conservar la misma identidad de puntero, no copiarse")
	}

	ctxNoSnap := NewFailureContext(req, resp, verif, nil)
	if ctxNoSnap.Snapshot != nil {
		t.Fatal("sin EnvironmentVerifier, Snapshot debe quedar nil explícito")
	}
}

func TestB921FailureDiagnosis_Valid(t *testing.T) {
	req := autonomy.ActionRequest{ID: "a1"}
	resp := autonomy.ActionResponse{RequestID: "a1"}
	verif := autonomy.VerificationResult{Success: false, ErrorReason: "archivo no existe"}
	ctx := NewFailureContext(req, resp, verif, nil)
	failure := NewFailure(FailureCategoryMissingResource, FailureCauseVerification, "archivo no existe", true, 0.9, StrategyReplan)

	diagnosis := NewFailureDiagnosis(ctx, failure)
	if !diagnosis.Valid() {
		t.Fatal("diagnóstico bien formado sobre un fallo real debería ser válido")
	}
}

func TestB921FailureDiagnosis_InvalidCases(t *testing.T) {
	req := autonomy.ActionRequest{ID: "a1"}
	resp := autonomy.ActionResponse{RequestID: "a1"}
	okVerif := autonomy.VerificationResult{Success: true}
	failVerif := autonomy.VerificationResult{Success: false, ErrorReason: "x"}

	validFailure := NewFailure(FailureCategoryUnknown, FailureCauseUnknown, "x", true, 0.5, StrategyWait)
	invalidFailure := NewFailure("bogus", FailureCauseUnknown, "x", true, 0.5, StrategyWait)

	cases := []*FailureDiagnosis{
		nil,
		NewFailureDiagnosis(nil, validFailure),
		NewFailureDiagnosis(NewFailureContext(req, resp, failVerif, nil), nil),
		NewFailureDiagnosis(NewFailureContext(req, resp, okVerif, nil), validFailure),
		NewFailureDiagnosis(NewFailureContext(req, resp, failVerif, nil), invalidFailure),
	}
	for i, d := range cases {
		if d.Valid() {
			t.Fatalf("caso %d: esperaba diagnóstico inválido", i)
		}
	}
}
