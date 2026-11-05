package core

import "ahmelia/autonomy"

// FailureRecoveryDecision es la decisión final de B92.3 sobre un fallo
// diagnosticado — a diferencia de Failure.SuggestedRecovery (B92.2), que es
// solo una sugerencia sin autoridad. Esto es lo que efectivamente se le
// devuelve al circuito (Diagnoser → Recovery Decision → Replanner →
// Capability Resolution → Executor, como lo planteó el usuario).
type FailureRecoveryDecision struct {
	Strategy FailureRecoveryStrategy
	Reason   string
}

// NewFailureRecoveryDecision crea una decisión con su motivo.
func NewFailureRecoveryDecision(strategy FailureRecoveryStrategy, reason string) FailureRecoveryDecision {
	return FailureRecoveryDecision{Strategy: strategy, Reason: reason}
}

// IsValid reporta si la estrategia elegida es una de las definidas.
func (d FailureRecoveryDecision) IsValid() bool {
	return d.Strategy.IsValid()
}

// RecoveryStrategist decide qué hacer con un fallo diagnosticado.
//
// Regla central de este bloque: RecoveryStrategist NO reimplementa nada de
// lo que autonomy.Replanner (B87) ya decide en producción — conteo de
// reintentos (Task.Attempts vs MaxTaskRetries), si hay tareas pendientes
// que dependen de la fallida, ni el límite de regeneraciones de plan por
// Goal. Esa autoridad ya existe y ya está cableada en
// GoalPlanningAdapter.UpdateState (core/goal_planner.go). Crear una segunda
// fuente de verdad para lo mismo sería exactamente el "segundo cerebro
// paralelo" que B92 debe evitar (regla B88.5).
//
// Lo único que RecoveryStrategist decide es: cuándo el diagnóstico de
// B92.2 debe pesar MÁS que lo que Replanner concluyó — específicamente
// cuando el Diagnoser marcó el fallo como no recuperable, algo que
// Replanner no puede saber porque no clasifica causas, solo cuenta
// intentos.
type RecoveryStrategist struct{}

// NewRecoveryStrategist crea un RecoveryStrategist. Sin estado propio.
func NewRecoveryStrategist() *RecoveryStrategist {
	return &RecoveryStrategist{}
}

// Decide combina un FailureDiagnosis con la ReplanDecision que
// autonomy.Replanner ya produjo (o produciría) para la misma tarea.
//
// replan es nil cuando todavía no existe ese contexto — por ejemplo, si el
// fallo se diagnostica antes de que llegue a GoalPlanningAdapter, o en un
// escenario donde no hay Goal/Plan/Task real detrás (pruebas, diagnóstico
// aislado). Cablear la llamada real a Replanner.Decide(goal, plan, task) es
// responsabilidad de B92.4, no de este archivo.
func (s *RecoveryStrategist) Decide(diagnosis *FailureDiagnosis, replan *autonomy.ReplanDecision) FailureRecoveryDecision {
	if diagnosis == nil || !diagnosis.Valid() {
		return NewFailureRecoveryDecision(StrategyAskUser, "no hay un diagnóstico válido: se pide intervención")
	}

	failure := diagnosis.Failure

	if !failure.Recoverable {
		// El Diagnoser ya determinó que insistir no tiene sentido (permiso
		// denegado, anomalía de correlación req/res, falta de
		// configuración de B80, etc.). Esto pesa más que Task.Attempts:
		// Replanner reintentaría igual sin saber por qué falló.
		return NewFailureRecoveryDecision(
			failure.SuggestedRecovery,
			"Diagnoser marcó el fallo como no recuperable (categoría: "+string(failure.Category)+"): se respeta esa conclusión sobre cualquier conteo de reintentos",
		)
	}

	if replan != nil {
		// Recuperable y ya existe una decisión real de B87 para esta
		// misma tarea: se traduce tal cual, sin recalcular nada.
		return NewFailureRecoveryDecision(translateReplanAction(replan.Action), replan.Reason)
	}

	// Recuperable pero sin ReplanDecision disponible todavía: la mejor
	// estimación que hay es la sugerencia del propio Diagnoser.
	return NewFailureRecoveryDecision(
		failure.SuggestedRecovery,
		"sin decisión de Replanner disponible todavía: se usa la sugerencia del Diagnoser",
	)
}

// translateReplanAction traduce el vocabulario de autonomy.ReplanAction
// (B87: retry_task/skip_task/regenerate_plan/give_up) al vocabulario más
// amplio de FailureRecoveryStrategy (B92: retry/replan/wait/block/ask_user).
//
// La traducción es deliberadamente más gruesa que el original: tanto
// ReplanSkipTask como ReplanRegeneratePlan representan "el plan sigue,
// pero no exactamente como estaba" desde el punto de vista de B92, así que
// ambos caen en StrategyReplan. El ReplanDecision original (con su Reason
// específico) sigue disponible para quien lo necesite — no se pierde,
// solo se resume.
func translateReplanAction(action autonomy.ReplanAction) FailureRecoveryStrategy {
	switch action {
	case autonomy.ReplanRetryTask:
		return StrategyRetry
	case autonomy.ReplanSkipTask, autonomy.ReplanRegeneratePlan:
		return StrategyReplan
	case autonomy.ReplanGiveUp:
		return StrategyBlock
	default:
		return StrategyAskUser
	}
}
