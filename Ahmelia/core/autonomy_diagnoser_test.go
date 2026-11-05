package core

import (
	"testing"

	"ahmelia/autonomy"
)

func mkFailedCtx(errorReason string) *FailureContext {
	req := autonomy.ActionRequest{ID: "a1", Action: "accion_de_prueba"}
	resp := autonomy.ActionResponse{RequestID: "a1"}
	verif := autonomy.VerificationResult{Success: false, ErrorReason: errorReason}
	return NewFailureContext(req, resp, verif, nil)
}

func TestB922Diagnoser_ReturnsNilOnSuccessfulVerification(t *testing.T) {
	req := autonomy.ActionRequest{ID: "a1"}
	resp := autonomy.ActionResponse{RequestID: "a1"}
	verif := autonomy.VerificationResult{Success: true}
	fc := NewFailureContext(req, resp, verif, nil)

	if got := NewDiagnoser().Diagnose(fc); got != nil {
		t.Fatalf("esperaba nil sobre una verificación exitosa, obtuve %+v", got)
	}
}

func TestB922Diagnoser_ReturnsNilOnNilContext(t *testing.T) {
	if got := NewDiagnoser().Diagnose(nil); got != nil {
		t.Fatalf("esperaba nil sobre contexto nil, obtuve %+v", got)
	}
}

func TestB922Diagnoser_EmptyErrorReason(t *testing.T) {
	fc := mkFailedCtx("")
	d := NewDiagnoser().Diagnose(fc)
	if d == nil || !d.Valid() {
		t.Fatal("esperaba un diagnóstico válido incluso sin ErrorReason")
	}
	if d.Failure.Category != FailureCategoryUnknown || d.Failure.SuggestedRecovery != StrategyAskUser {
		t.Fatalf("ErrorReason vacío debería clasificar como Unknown/AskUser, obtuve %+v", d.Failure)
	}
}

// Los siguientes casos usan EXACTAMENTE los strings literales que emiten
// autonomy.ActionVerifier.Verify y autonomy.EnvironmentVerifier.Verify, para
// que el test se rompa si esos mensajes cambian sin que B92.2 se entere.
func TestB922Diagnoser_ClassifiesRealEvidenceStrings(t *testing.T) {
	cases := []struct {
		name         string
		errorReason  string
		wantCategory FailureCategory
		wantCause    FailureCause
		wantRecov    bool
		wantStrategy FailureRecoveryStrategy
	}{
		{
			name:         "contexto cancelado (ActionVerifier)",
			errorReason:  "contexto cancelado: context deadline exceeded",
			wantCategory: FailureCategoryTimeout,
			wantCause:    FailureCauseVerification,
			wantRecov:    true,
			wantStrategy: StrategyRetry,
		},
		{
			name:         "no fue ejecutada por el motor (ActionVerifier)",
			errorReason:  "la acción no fue ejecutada por el motor",
			wantCategory: FailureCategoryExecutorError,
			wantCause:    FailureCauseAction,
			wantRecov:    true,
			wantStrategy: StrategyRetry,
		},
		{
			name:         "no corresponde a la solicitud ejecutada (ActionVerifier)",
			errorReason:  "el resultado no corresponde a la solicitud ejecutada",
			wantCategory: FailureCategoryExecutorError,
			wantCause:    FailureCauseAction,
			wantRecov:    false,
			wantStrategy: StrategyBlock,
		},
		{
			name:         "error técnico de ejecución genérico (ActionVerifier)",
			errorReason:  "error técnico de ejecución: fallo desconocido",
			wantCategory: FailureCategoryExecutorError,
			wantCause:    FailureCauseAction,
			wantRecov:    true,
			wantStrategy: StrategyRetry,
		},
		{
			name:         "error técnico con archivo inexistente (ActionVerifier + causa real)",
			errorReason:  "error técnico de ejecución: el archivo no existe",
			wantCategory: FailureCategoryMissingResource,
			wantCause:    FailureCauseAction,
			wantRecov:    true,
			wantStrategy: StrategyReplan,
		},
		{
			name:         "error técnico con permiso denegado (ActionVerifier + causa real)",
			errorReason:  "error técnico de ejecución: permiso denegado",
			wantCategory: FailureCategoryPermission,
			wantCause:    FailureCauseAction,
			wantRecov:    false,
			wantStrategy: StrategyAskUser,
		},
		{
			name:         "no hay inspector de entorno configurado (EnvironmentVerifier)",
			errorReason:  "no hay inspector de entorno configurado para B80",
			wantCategory: FailureCategoryEnvironmentMismatch,
			wantCause:    FailureCauseEnvironment,
			wantRecov:    false,
			wantStrategy: StrategyBlock,
		},
		{
			name:         "error al inspeccionar el entorno (EnvironmentVerifier)",
			errorReason:  "error al inspeccionar el entorno: fs unavailable",
			wantCategory: FailureCategoryEnvironmentMismatch,
			wantCause:    FailureCauseEnvironment,
			wantRecov:    true,
			wantStrategy: StrategyRetry,
		},
		{
			name:         "el entorno no confirmó el resultado (EnvironmentVerifier)",
			errorReason:  "la ejecución técnica terminó correctamente, pero el entorno no confirmó el resultado",
			wantCategory: FailureCategoryEnvironmentMismatch,
			wantCause:    FailureCauseEnvironment,
			wantRecov:    true,
			wantStrategy: StrategyReplan,
		},
		{
			name:         "parametro invalido no reconocido por el motor",
			errorReason:  "parámetro inválido: ruta vacía",
			wantCategory: FailureCategoryInvalidParameters,
			wantCause:    FailureCauseAction,
			wantRecov:    true,
			wantStrategy: StrategyReplan,
		},
		{
			name:         "evidencia no reconocida cae en Unknown",
			errorReason:  "algo salió mal que ningún patrón conoce",
			wantCategory: FailureCategoryUnknown,
			wantCause:    FailureCauseUnknown,
			wantRecov:    false,
			wantStrategy: StrategyAskUser,
		},
	}

	d := NewDiagnoser()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fc := mkFailedCtx(tc.errorReason)
			diagnosis := d.Diagnose(fc)
			if diagnosis == nil || !diagnosis.Valid() {
				t.Fatalf("esperaba diagnóstico válido para %q", tc.errorReason)
			}
			f := diagnosis.Failure
			if f.Category != tc.wantCategory {
				t.Errorf("Category = %q, quería %q", f.Category, tc.wantCategory)
			}
			if f.Cause != tc.wantCause {
				t.Errorf("Cause = %q, quería %q", f.Cause, tc.wantCause)
			}
			if f.Recoverable != tc.wantRecov {
				t.Errorf("Recoverable = %v, quería %v", f.Recoverable, tc.wantRecov)
			}
			if f.SuggestedRecovery != tc.wantStrategy {
				t.Errorf("SuggestedRecovery = %q, quería %q", f.SuggestedRecovery, tc.wantStrategy)
			}
			if f.Evidence != tc.errorReason {
				t.Errorf("Evidence debe copiar el ErrorReason verbatim: got %q, want %q", f.Evidence, tc.errorReason)
			}
		})
	}
}

func TestB922Diagnoser_AlwaysProducesAValidFailure(t *testing.T) {
	// Ningún patrón de classify debe poder violar Failure.IsValid() (p. ej.
	// sugerir retry sobre un fallo no-recuperable).
	reasons := []string{
		"", "contexto cancelado: x", "la acción no fue ejecutada por el motor",
		"el resultado no corresponde a la solicitud ejecutada",
		"error técnico de ejecución: x", "error técnico de ejecución: no existe",
		"error técnico de ejecución: permiso denegado",
		"no hay inspector de entorno configurado para B80",
		"error al inspeccionar el entorno: x",
		"la ejecución técnica terminó correctamente, pero el entorno no confirmó el resultado",
		"parámetro inválido: x", "cualquier cosa",
	}
	d := NewDiagnoser()
	for _, r := range reasons {
		diagnosis := d.Diagnose(mkFailedCtx(r))
		if !diagnosis.Valid() {
			t.Fatalf("classify produjo un Failure inválido para ErrorReason %q: %+v", r, diagnosis)
		}
	}
}
