package core

import (
	"strings"

	"ahmelia/autonomy"
)

// Diagnoser clasifica un FailureContext en un Failure usando únicamente el
// texto de evidencia literal que ya producen ActionVerifier (B78) y
// EnvironmentVerifier (B80) — nunca heurística inventada sobre datos que
// el ciclo no genera.
//
// Regla explícita de B92: el Diagnoser no ejecuta nada y no decide qué
// pasa después. Solo lee un FailureContext (evidencia) y produce un
// FailureDiagnosis (conclusión). La decisión real (B92.3) y el replanning
// (B92.4) son responsabilidad de bloques posteriores.
type Diagnoser struct{}

// NewDiagnoser crea un Diagnoser. No tiene estado propio: cada llamada a
// Diagnose es independiente y no conserva memoria entre fallos.
func NewDiagnoser() *Diagnoser {
	return &Diagnoser{}
}

// Diagnose produce un FailureDiagnosis a partir de fc. Devuelve nil cuando
// fc no describe un fallo real (p. ej. construido sobre una verificación
// exitosa) — el Diagnoser nunca inventa un fallo donde no lo hay.
func (d *Diagnoser) Diagnose(fc *FailureContext) *FailureDiagnosis {
	if !fc.Valid() {
		return nil
	}
	return NewFailureDiagnosis(fc, d.classify(fc))
}

// classify mapea fc.Verification.ErrorReason a un Failure. Los patrones de
// texto usados acá son exactamente los strings literales que emiten
// autonomy.ActionVerifier.Verify y autonomy.EnvironmentVerifier.Verify —
// verificado leyendo esos dos archivos, no inventado. Cuando ninguno
// coincide, clasifica como Unknown con confianza baja en vez de adivinar.
func (d *Diagnoser) classify(fc *FailureContext) *Failure {
	// B80 now preserves the actual epistemic state of the environment
	// observation. This is stronger evidence than reparsing the generic
	// "environment did not confirm" sentence and is what lets recovery react
	// differently to contradiction versus insufficient evidence.
	switch fc.Verification.ObservationStatus {
	case autonomy.ObservationInsufficient:
		return NewFailure(FailureCategoryEnvironmentMismatch, FailureCauseEnvironment, fc.Verification.ErrorReason, true, 0.6, StrategyRetry)
	case autonomy.ObservationContradicted:
		return NewFailure(FailureCategoryEnvironmentMismatch, FailureCauseEnvironment, fc.Verification.ErrorReason, true, 0.9, StrategyReplan)
	case autonomy.ObservationConfirmed:
		// A confirmed observation should already have produced Success=true in
		// EnvironmentVerifier. Do not manufacture a different diagnosis here;
		// fall through only for defensive compatibility with hand-built tests.
	}

	raw := fc.Verification.ErrorReason
	reason := strings.ToLower(raw)

	switch {
	case reason == "":
		// Verificación fallida sin ErrorReason: no hay evidencia textual
		// para clasificar. Confianza mínima, requiere intervención.
		return NewFailure(FailureCategoryUnknown, FailureCauseUnknown, raw, false, 0.1, StrategyAskUser)

	// --- Evidencia de EnvironmentVerifier (B80) ---
	case strings.Contains(reason, "no hay inspector de entorno configurado"):
		// Falta de configuración, no un fallo de la tarea en sí: no tiene
		// sentido reintentar ni replanificar hasta que se configure B80.
		return NewFailure(FailureCategoryEnvironmentMismatch, FailureCauseEnvironment, raw, false, 0.9, StrategyBlock)

	case strings.Contains(reason, "error al inspeccionar el entorno"):
		// El inspector falló al consultar, no al confirmar: plausible que
		// sea transitorio.
		return NewFailure(FailureCategoryEnvironmentMismatch, FailureCauseEnvironment, raw, true, 0.5, StrategyRetry)

	case strings.Contains(reason, "el entorno no confirmó el resultado"):
		// La ejecución técnica terminó bien pero el entorno real no
		// coincide con lo esperado: no es un problema de reintentar la
		// misma acción, es un problema de qué se pidió.
		return NewFailure(FailureCategoryEnvironmentMismatch, FailureCauseEnvironment, raw, true, 0.6, StrategyReplan)

	// --- Evidencia de ActionVerifier (B78) ---
	case strings.Contains(reason, "contexto cancelado"):
		return NewFailure(FailureCategoryTimeout, FailureCauseVerification, raw, true, 0.7, StrategyRetry)

	case strings.Contains(reason, "no existe") ||
		strings.Contains(reason, "no encontrad") ||
		strings.Contains(reason, "not found"):
		return NewFailure(FailureCategoryMissingResource, FailureCauseAction, raw, true, 0.8, StrategyReplan)

	case strings.Contains(reason, "permiso") ||
		strings.Contains(reason, "denegad") ||
		strings.Contains(reason, "denied"):
		return NewFailure(FailureCategoryPermission, FailureCauseAction, raw, false, 0.8, StrategyAskUser)

	case strings.Contains(reason, "parámetro") ||
		strings.Contains(reason, "parametro") ||
		strings.Contains(reason, "argumento inválido") ||
		strings.Contains(reason, "argumento invalido"):
		return NewFailure(FailureCategoryInvalidParameters, FailureCauseAction, raw, true, 0.7, StrategyReplan)

	case strings.Contains(reason, "no fue ejecutada por el motor"):
		return NewFailure(FailureCategoryExecutorError, FailureCauseAction, raw, true, 0.6, StrategyRetry)

	case strings.Contains(reason, "no corresponde a la solicitud ejecutada"):
		// Anomalía de correlación req/res: no es algo que un retry o un
		// replan resuelvan solos, huele a bug del executor.
		return NewFailure(FailureCategoryExecutorError, FailureCauseAction, raw, false, 0.4, StrategyBlock)

	case strings.Contains(reason, "error técnico de ejecución") ||
		strings.Contains(reason, "error tecnico de ejecucion"):
		return NewFailure(FailureCategoryExecutorError, FailureCauseAction, raw, true, 0.5, StrategyRetry)

	default:
		return NewFailure(FailureCategoryUnknown, FailureCauseUnknown, raw, false, 0.2, StrategyAskUser)
	}
}
