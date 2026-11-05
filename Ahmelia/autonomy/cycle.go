package autonomy

import (
	"context"
	"fmt"

	"ahmelia/planning"
	"ahmelia/situation"
)

// Observer obtiene una observación del entorno para una iteración.
type CycleObserver interface {
	Observe(ctx context.Context) (string, error)
}

// CycleObservationProvider adds structured evidence without breaking the
// legacy CycleObserver contract. The evidence belongs to this RunStep only.
type CycleObservationProvider interface {
	SituationObservations() []situation.Observation
}

// SituationalCycleEvaluator evaluates shared context before planning. It can
// abstain from planning, but it cannot select or execute an action.
type SituationalCycleEvaluator interface {
	EvaluateSituation(ctx context.Context, evidence situation.DecisionEvidence) (bool, error)
}

// Evaluator determina si la observación merece procesamiento autónomo.
type CycleEvaluator interface {
	EvaluateRelevance(ctx context.Context, observation string) (bool, error)
}

// Planner produce como máximo una acción para la iteración actual.
type CyclePlanner interface {
	GetNextAction(ctx context.Context) (ActionRequest, bool, error)
}

// CandidateCyclePlanner consumes the same Goal candidate that was used to
// build the cycle's Situation. Implementations must validate that it remains
// canonical before mutating planning state.
type CandidateCyclePlanner interface {
	GetNextActionForCandidate(ctx context.Context, candidateID string) (ActionRequest, bool, error)
}

type CyclePlanningCandidateProvider interface {
	CyclePlanningCandidateID() string
}

// CyclePlanningBaselineProvider exposes the observer's current bounded
// snapshot to a planner that may need to create a new plan. It is context,
// not a precondition or a decision authority.
type CyclePlanningBaselineProvider interface {
	CyclePlanningBaseline() string
}

// Diagnoser analiza un resultado fallido sin ejecutar por sí mismo una acción.
type CycleDiagnoser interface {
	DiagnoseFailure(ctx context.Context, req ActionRequest, verif VerificationResult) error
}

// Learner registra el resultado exitoso de una acción.
type CycleLearner interface {
	RecordOutcome(ctx context.Context, req ActionRequest, verif VerificationResult) error
}

// Updater aplica el resultado al estado de trabajo/historial/métricas.
// B78 solo define el contrato; la implementación real se conectará después.
type CycleUpdater interface {
	UpdateState(ctx context.Context, req ActionRequest, verif VerificationResult) error
}

// Cycle orquesta una iteración cerrada usando AutonomousState (B72) como
// única fuente de verdad para el estado de ejecución.
//
// B72 no contiene estados Observing/Evaluating/Planning/Learning/Updating.
// Por eso B78 no inventa una segunda máquina de estados: Thinking representa
// la fase cognitiva previa a la ejecución; Executing y Verifying representan
// sus fases homónimas; Completed representa el cierre exitoso; Recovering y
// Blocked representan el camino de fallo. Updating ocurre mientras el estado
// está en Completed/Blocked y antes de volver a Idle.
type Cycle struct {
	state                *AutonomousState
	observer             CycleObserver
	evaluator            CycleEvaluator
	situationalEvaluator SituationalCycleEvaluator
	planner              CyclePlanner
	executor             ExecutorAdapter
	verifier             *ActionVerifier
	environmentVerifier  *EnvironmentVerifier
	diagnoser            CycleDiagnoser
	learner              CycleLearner
	updater              CycleUpdater
}

func (c *Cycle) SetSituationalEvaluator(evaluator SituationalCycleEvaluator) {
	if c != nil {
		c.situationalEvaluator = evaluator
	}
}

func NewCycle(
	state *AutonomousState,
	observer CycleObserver,
	evaluator CycleEvaluator,
	planner CyclePlanner,
	executor ExecutorAdapter,
	verifier *ActionVerifier,
	diagnoser CycleDiagnoser,
	learner CycleLearner,
	updater CycleUpdater,
) *Cycle {
	if verifier == nil {
		verifier = NewActionVerifier()
	}
	return &Cycle{
		state:     state,
		observer:  observer,
		evaluator: evaluator,
		planner:   planner,
		executor:  executor,
		verifier:  verifier,
		diagnoser: diagnoser,
		learner:   learner,
		updater:   updater,
	}
}

// SetEnvironmentVerifier conecta la verificación técnica B78 con la evidencia
// observable de B80. Se mantiene separado para no romper el contrato histórico
// de NewCycle ni sus callers existentes.
func (c *Cycle) SetEnvironmentVerifier(verifier *EnvironmentVerifier) {
	c.environmentVerifier = verifier
}

// RunStep ejecuta una única iteración completa.
// Siempre intenta cerrar el ciclo en Idle. Los errores de las fases se
// propagan; no se silencian para mantener observabilidad y permitir que el
// loop superior decida cómo tratar el fallo.
func (c *Cycle) RunStep(ctx context.Context) (err error) {
	if c.state == nil {
		return fmt.Errorf("autonomy: estado B72 no configurado")
	}

	if c.state.Execution() != ExecIdle {
		return fmt.Errorf("autonomy: no se puede iniciar un ciclo desde %s", c.state.Execution())
	}

	// La tabla real de B72 exige pasar por estados intermedios para volver a
	// Idle. El cierre se hace explícitamente según el estado actual.
	defer func() {
		if closeErr := c.returnToIdle(); err == nil && closeErr != nil {
			err = fmt.Errorf("autonomy: cierre del ciclo: %w", closeErr)
		}
	}()

	if err = c.transition(ExecThinking); err != nil {
		return err
	}

	observation, err := c.observe(ctx)
	if err != nil {
		return err
	}
	var structuredObservations []situation.Observation
	if provider, ok := c.observer.(CycleObservationProvider); ok {
		structuredObservations = provider.SituationObservations()
	}
	if provider, ok := c.executor.(CycleSituationConsumer); ok {
		provider.SetCycleSituationObservations(structuredObservations)
		defer provider.SetCycleSituationObservations(nil)
	}

	if c.evaluator != nil && observation != "" {
		relevant, evalErr := c.evaluator.EvaluateRelevance(ctx, observation)
		if evalErr != nil {
			return fmt.Errorf("autonomy: fase EVALUATE: %w", evalErr)
		}
		if !relevant {
			return nil
		}
	}
	if c.situationalEvaluator != nil {
		evidence, ok := c.situationalEvidence(ctx, structuredObservations)
		if ok {
			relevant, evalErr := c.situationalEvaluator.EvaluateSituation(ctx, evidence)
			if evalErr != nil {
				return fmt.Errorf("autonomy: fase SITUATION EVALUATE: %w", evalErr)
			}
			if !relevant {
				return nil
			}
		}
	}

	if c.planner == nil {
		return nil
	}
	planningCtx := ctx
	if baselineProvider, ok := c.observer.(CyclePlanningBaselineProvider); ok {
		if baseline := baselineProvider.CyclePlanningBaseline(); baseline != "" {
			planningCtx = planning.WithEnvironmentBaseline(ctx, baseline)
		}
	}

	var req ActionRequest
	var hasAction bool
	var planErr error
	if candidateProvider, ok := c.executor.(CyclePlanningCandidateProvider); ok {
		candidateID := candidateProvider.CyclePlanningCandidateID()
		if candidateID != "" {
			if planner, supportsCandidate := c.planner.(CandidateCyclePlanner); supportsCandidate {
				req, hasAction, planErr = planner.GetNextActionForCandidate(planningCtx, candidateID)
			}
		} else {
			req, hasAction, planErr = c.planner.GetNextAction(planningCtx)
		}
	} else {
		req, hasAction, planErr = c.planner.GetNextAction(planningCtx)
	}
	if planErr != nil {
		return fmt.Errorf("autonomy: fase PLAN: %w", planErr)
	}
	if !hasAction {
		return nil
	}

	if c.executor == nil {
		return fmt.Errorf("autonomy: ExecutorAdapter no configurado")
	}

	if err = c.transition(ExecExecuting); err != nil {
		return err
	}
	response := c.executor.ExecuteAction(ctx, req)

	if err = c.transition(ExecVerifying); err != nil {
		return err
	}
	verification := c.verifier.Verify(ctx, req, response)
	if verification.Success && c.environmentVerifier != nil {
		envVerification := c.environmentVerifier.Verify(ctx, req, verification)
		verification = VerificationResult{
			Success:            envVerification.Success,
			Output:             envVerification.Output,
			ErrorReason:        envVerification.ErrorReason,
			ObservationStatus:  envVerification.ObservationStatus,
			ObservationDetails: envVerification.Details,
			Observation:        envVerification.Observation,
			AuthorityResult:    verification.AuthorityResult,
		}
	}

	if verification.Success {
		if err = c.transition(ExecCompleted); err != nil {
			return err
		}

		// LEARN y UPDATING ocurren antes de Idle. B72 no necesita un estado
		// adicional para representarlos: Completed es el estado terminal de
		// la tarea y Updater es el punto de integración de memoria/métricas.
		if c.learner != nil {
			if learnErr := c.learner.RecordOutcome(ctx, req, verification); learnErr != nil {
				return fmt.Errorf("autonomy: fase LEARN: %w", learnErr)
			}
		}
		if c.updater != nil {
			if updateErr := c.updater.UpdateState(ctx, req, verification); updateErr != nil {
				return fmt.Errorf("autonomy: fase UPDATING: %w", updateErr)
			}
		}
		return nil
	}

	// Camino de fallo: Verifying -> Recovering -> diagnóstico -> Blocked.
	// Blocked -> Idle es una transición válida en B72 y evita inventar una
	// transición Recovering -> Idle que B72 explícitamente no permite.
	if err = c.transition(ExecRecovering); err != nil {
		return err
	}
	// Unknown/insufficient evidence is not a known execution failure. Preserve
	// it for the updater and stop this cycle without manufacturing a failure
	// diagnosis or triggering a recovery strategy from missing evidence alone.
	if !verificationOutcomeUnknown(verification) && c.diagnoser != nil {
		if diagErr := c.diagnoser.DiagnoseFailure(ctx, req, verification); diagErr != nil {
			return fmt.Errorf("autonomy: fase DIAGNOSE: %w", diagErr)
		}
	}

	if err = c.transition(ExecBlocked); err != nil {
		return err
	}
	if c.updater != nil {
		if updateErr := c.updater.UpdateState(ctx, req, verification); updateErr != nil {
			return fmt.Errorf("autonomy: fase UPDATING: %w", updateErr)
		}
	}

	return nil
}

func (c *Cycle) situationalEvidence(ctx context.Context, observations []situation.Observation) (situation.DecisionEvidence, bool) {
	provider, ok := c.executor.(CycleSituationEvaluatorProvider)
	if !ok {
		return situation.DecisionEvidence{}, false
	}
	return provider.BuildCycleDecisionEvidence(ctx, observations)
}

// CycleSituationConsumer hands iteration evidence to the executor boundary;
// implementations must replace, not merge, each step's evidence.
type CycleSituationConsumer interface {
	SetCycleSituationObservations([]situation.Observation)
}

type CycleSituationEvaluatorProvider interface {
	BuildCycleDecisionEvidence(context.Context, []situation.Observation) (situation.DecisionEvidence, bool)
}

func verificationOutcomeUnknown(result VerificationResult) bool {
	if result.AuthorityResult != nil && result.AuthorityResult.AuthorityOutcomeStatus() == "unknown" {
		return true
	}
	return result.ObservationStatus == ObservationNotObserved || result.ObservationStatus == ObservationInsufficient
}

func (c *Cycle) observe(ctx context.Context) (string, error) {
	if c.observer == nil {
		return "", nil
	}
	observation, err := c.observer.Observe(ctx)
	if err != nil {
		return "", fmt.Errorf("autonomy: fase OBSERVE: %w", err)
	}
	return observation, nil
}

func (c *Cycle) transition(to ExecutionState) error {
	if err := c.state.TransitionTo(to); err != nil {
		return fmt.Errorf("autonomy: transición: %w", err)
	}
	return nil
}

func (c *Cycle) returnToIdle() error {
	switch c.state.Execution() {
	case ExecIdle:
		return nil
	case ExecThinking:
		return c.transition(ExecIdle)
	case ExecExecuting:
		if err := c.transition(ExecRecovering); err != nil {
			return err
		}
		if err := c.transition(ExecBlocked); err != nil {
			return err
		}
		return c.transition(ExecIdle)
	case ExecVerifying:
		if err := c.transition(ExecRecovering); err != nil {
			return err
		}
		if err := c.transition(ExecBlocked); err != nil {
			return err
		}
		return c.transition(ExecIdle)
	case ExecRecovering:
		if err := c.transition(ExecBlocked); err != nil {
			return err
		}
		return c.transition(ExecIdle)
	case ExecCompleted, ExecBlocked:
		return c.transition(ExecIdle)
	case ExecWaitingForUser:
		return c.transition(ExecIdle)
	default:
		return fmt.Errorf("autonomy: estado no manejado al cerrar ciclo: %s", c.state.Execution())
	}
}

// GetState expone la misma instancia de B72; no crea ni mantiene otra copia.
func (c *Cycle) GetState() *AutonomousState {
	return c.state
}
