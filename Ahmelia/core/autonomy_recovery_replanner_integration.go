package core

import (
	"context"
	"sync"

	"ahmelia/autonomy"
)

// ============================================================================
// AUTONOMY_RECOVERY_REPLANNER_INTEGRATION.GO — Bloque 92.4 (Diagnoser +
// Recovery, cuarto sub-bloque: Integración con Replanner)
// ============================================================================
//
// B92.2 (Diagnoser) y B92.3 (RecoveryStrategist) ya producen, respectivamente,
// un FailureDiagnosis y una FailureRecoveryDecision — pero ninguno de los dos
// estaba conectado todavía al ciclo real. Dos límites concretos, ya nombrados
// explícitamente al cerrar B92.2 y B92.3, se resuelven acá, sin tocar ninguno
// de los dos:
//
//   - autonomy.CycleDiagnoser.DiagnoseFailure solo puede devolver un error
//     (ver autonomy/cycle.go) — Cycle no tiene ningún otro lugar donde
//     entregarle el FailureDiagnosis a nadie. DiagnosingRecoveryAdvisor
//     resuelve esto guardando el último FailureDiagnosis por
//     ActionRequest.ID (== Task.ID, ver GoalActionMapper.MapTask) en un
//     mapa propio protegido por mutex, en vez de forzar un cambio en la
//     firma de CycleDiagnoser que rompería su único caller real (Cycle).
//   - GoalPlanningAdapter.UpdateState (autonomy/goal_planner.go) es quien
//     de verdad decide RETRY/SKIP/REGENERATE/GIVE-UP llamando a
//     autonomy.Replanner.Decide — vive en package autonomy, que no puede
//     importar package core sin crear un ciclo de imports (core ya importa
//     autonomy). Por eso B92.4 no muda el Diagnoser/RecoveryStrategist a
//     autonomy: define autonomy.FailureAdvisor (ver goal_planner.go), una
//     interfaz mínima que GoalPlanningAdapter sí puede consultar sin
//     conocer ningún tipo de core, y DiagnosingRecoveryAdvisor la
//     implementa acá.
//
// Cadena real resultante, dentro de un mismo Cycle.RunStep (cycle.go):
//
//	Verify falla
//	  -> Cycle.diagnoser.DiagnoseFailure (este archivo) construye el
//	     FailureContext, llama Diagnoser.Diagnose (B92.2) y guarda el
//	     FailureDiagnosis por Task.ID
//	  -> Cycle.updater.UpdateState (persistingCycleAdapter ->
//	     GoalPlanningAdapter.UpdateState) llama a.advisor.Advise(task.ID)
//	     ANTES de aplicar la decisión de Replanner
//	  -> si el Diagnoser marcó el fallo como no recuperable,
//	     RecoveryStrategist (B92.3) gana sobre Replanner (regla ya
//	     documentada en autonomy_recovery_strategy.go); si no, Advise
//	     devuelve ok=false y Replanner decide exactamente como antes de B92.
//
// Ningún camino de este archivo ejecuta una Action ni muta Goal/Plan/Task
// directamente: solo diagnostica (B92.2), decide (B92.3) y traduce esa
// decisión al vocabulario que GoalPlanningAdapter ya sabe aplicar
// (autonomy.ReplanAction) — la autoridad real de aplicar la decisión sigue
// siendo, exclusivamente, GoalPlanningAdapter.UpdateState (regla B88.5: sin
// segundo cerebro paralelo).
type DiagnosingRecoveryAdvisor struct {
	mu         sync.Mutex
	diagnoser  *Diagnoser
	strategist *RecoveryStrategist
	diagnoses  map[string]*FailureDiagnosis // keyed by ActionRequest.ID / Task.ID
}

// NewDiagnosingRecoveryAdvisor crea el puente B92.2+B92.3 -> ciclo real.
func NewDiagnosingRecoveryAdvisor() *DiagnosingRecoveryAdvisor {
	return &DiagnosingRecoveryAdvisor{
		diagnoser:  NewDiagnoser(),
		strategist: NewRecoveryStrategist(),
		diagnoses:  make(map[string]*FailureDiagnosis),
	}
}

// DiagnoseFailure implementa autonomy.CycleDiagnoser. Nunca devuelve error
// por sí sola: un fallo al diagnosticar (evidencia inválida, fc.Valid() ==
// false) simplemente no deja diagnóstico guardado para req.ID, y Advise más
// abajo se comporta como si no hubiera opinión — falla honesto hacia "seguí
// como antes de B92", nunca hacia "abortá el ciclo por no poder
// diagnosticar" (Cycle.RunStep propagaría ese error y ni siquiera llegaría
// a marcar el Goal como bloqueado).
//
// La firma real de CycleDiagnoser (ver autonomy/cycle.go, decisión ya
// tomada en B92.2) no entrega el ActionResponse crudo del executor — solo
// ActionRequest y VerificationResult. Se construye el FailureContext con un
// ActionResponse vacío: Diagnoser.classify (B92.2, ya auditado) solo lee
// VerificationResult.ErrorReason, así que esto no pierde ninguna señal real
// que el Diagnoser use hoy.
func (d *DiagnosingRecoveryAdvisor) DiagnoseFailure(ctx context.Context, req autonomy.ActionRequest, verif autonomy.VerificationResult) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	fc := NewFailureContext(req, autonomy.ActionResponse{}, verif, nil)
	diagnosis := d.diagnoser.Diagnose(fc)
	if diagnosis == nil || !diagnosis.Valid() {
		return nil
	}

	d.mu.Lock()
	d.diagnoses[req.ID] = diagnosis
	d.mu.Unlock()
	return nil
}

// Advise implementa autonomy.FailureAdvisor. Consume (borra) el diagnóstico
// guardado para taskID la primera vez que se consulta — un mismo fallo
// diagnosticado no debe seguir vetando decisiones de Replanner para
// intentos futuros de la misma tarea, después de que este UpdateState ya
// lo usó una vez.
func (d *DiagnosingRecoveryAdvisor) Advise(taskID string) (autonomy.ReplanAction, string, bool) {
	d.mu.Lock()
	diagnosis, found := d.diagnoses[taskID]
	if found {
		delete(d.diagnoses, taskID)
	}
	d.mu.Unlock()

	if !found || !diagnosis.Valid() {
		return "", "", false
	}

	// RecoveryStrategist.Decide (B92.3): replan=nil porque este Advise se
	// llama ANTES de que Replanner.Decide corra para esta tarea (ver la
	// cadena real en el comentario de cabecera) — todavía no existe una
	// ReplanDecision real para combinar en este punto. Solo interesa el
	// caso donde RecoveryStrategist pesa más que Replanner: fallo
	// diagnosticado como NO recuperable (ver autonomy_recovery_strategy.go).
	// En cualquier otro caso (recuperable, o diagnóstico inválido) no hay
	// nada que este Advise deba decidir por su cuenta: Replanner.Decide
	// sigue siendo la única autoridad, exactamente igual que antes de
	// B92.4 — por eso se devuelve ok=false en vez de traducir también la
	// rama "recuperable" de RecoveryStrategist acá.
	// A contradicción ambiental es una señal que la autoridad de continuación
	// debe consumir inmediatamente: reintentar la misma Task sin cambiar el
	// plan ignora precisamente la evidencia que acaba de observarse. La
	// distinción es factual (contradicted vs insufficient) y no depende de una
	// frase concreta ni de una capability particular.
	if diagnosis.Failure.Recoverable && diagnosis.Context != nil &&
		diagnosis.Context.Verification.ObservationStatus != autonomy.ObservationContradicted {
		return "", "", false
	}

	decision := d.strategist.Decide(diagnosis, nil)
	return translateFailureStrategyToReplanAction(decision.Strategy), decision.Reason, true
}

// translateFailureStrategyToReplanAction traduce el vocabulario de B92
// (FailureRecoveryStrategy: retry/replan/wait/block/ask_user) al vocabulario
// más angosto que GoalPlanningAdapter.UpdateState ya sabe aplicar
// (autonomy.ReplanAction: retry_task/skip_task/regenerate_plan/give_up) —
// la traducción inversa de translateReplanAction (autonomy_recovery_strategy.go,
// B92.3), documentada acá porque atraviesa el límite de paquete.
//
// Este método solo se llama desde Advise cuando Failure.Recoverable ==
// false (ver arriba), y Failure.IsValid() (B92.1) ya prohíbe que un fallo
// no recuperable sugiera StrategyRetry — ese caso no debería poder llegar
// acá. Se lo mapea de todos modos a ReplanGiveUp, nunca a ReplanRetryTask,
// para que un dato corrupto o un futuro Diagnoser con un bug nunca pueda
// hacer que este puente reintente algo que el propio Diagnoser marcó como
// imposible de arreglar reintentando.
//
// StrategyWait/StrategyBlock/StrategyAskUser se traducen todos a
// ReplanGiveUp: GoalPlanningAdapter no tiene hoy ninguna noción de "pausar
// sin bloquear" (ver UpdateState, autonomy/goal_planner.go) — ReplanGiveUp
// ya bloquea el Goal, lo saca de Candidates() (B86) y lo deja visible para
// 73/75 (Goal Monitor/Initiative Engine) vía GoalMonitor.BlockedGoals(), el
// mismo camino que hoy ya termina en ASK_USER en la conversación. Inventar
// un ReplanAction nuevo solo para esto sería una quinta rama en el switch
// de UpdateState que ningún bloque anterior pidió.
func translateFailureStrategyToReplanAction(strategy FailureRecoveryStrategy) autonomy.ReplanAction {
	switch strategy {
	case StrategyReplan:
		return autonomy.ReplanRegeneratePlan
	default: // StrategyWait, StrategyBlock, StrategyAskUser, StrategyRetry (no debería darse) o valor inesperado
		return autonomy.ReplanGiveUp
	}
}
