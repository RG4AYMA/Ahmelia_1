package autonomy

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"ahmelia/consult"
)

// StateProvider define la interfaz requerida para consultar la última actividad del usuario (B72).
type StateProvider interface {
	LastUserActivity() time.Time
}

// GoalProvider define la interfaz requerida para consultar objetivos pendientes (B73).
type GoalProvider interface {
	PendingGoal() (hasPending bool, goalID string)
}

// OwnerRestrictionProvider reports whether the owner closed Ahmelia's own work
// initiative at now. *core.Engine satisfies it; the loop never reads the store
// directly, so there is a single gate for mode + restriction.
type OwnerRestrictionProvider interface {
	OwnerWorkInitiativeRestricted(now time.Time) bool
}

// InitiativeConsultant is the optional asynchronous lane for non-urgent
// initiative. ProposeInitiative must never block; FreshInitiative returns only
// a current proposal for the fingerprint.
type InitiativeConsultant interface {
	ProposeInitiative(req consult.Request, fingerprint string) bool
	FreshInitiative(fingerprint string) (consult.Proposal, bool)
}

// LoopConfig define los parámetros operativos del bucle autónomo.
type LoopConfig struct {
	Interval   time.Duration
	BufferSize int
}

// AutonomousLoop mantiene vivo el procesamiento autónomo.
// B77 conserva el camino conversacional legado; B83 añade el camino real
// Cycle.RunStep(), que reutiliza B78-B82 sin duplicar su orquestación.
// B88.5: cuando ambos caminos están presentes, runIntegratedTick calcula la
// InitiativeDecision una sola vez y la usa como autoridad sobre Cycle
// (autonomy.AllowsCycleExecution) además de como entrada del camino
// conversacional — ya no son dos ciclos ejecutivos independientes.
type AutonomousLoop struct {
	mu      sync.Mutex
	wg      sync.WaitGroup
	runLock sync.Locker // serializa un tick completo con el turno interactivo del Engine

	config     LoopConfig
	running    bool
	cancel     context.CancelFunc
	outputChan chan ConversationDecision

	// B83: ciclo completo B78 + adaptadores B79-B82.
	cycle *Cycle

	// Camino legado B77, conservado para no romper el cableado existente.
	observer   *ContextObserver
	initiative *InitiativeEngine
	dialogue   *ConversationManager
	stateProv  StateProvider
	goalProv   GoalProvider

	// Escena 1: restricción temporal del propietario y reloj inyectable. El
	// provider es opcional; sin él el loop conserva su comportamiento previo.
	restrictions OwnerRestrictionProvider
	now          func() time.Time

	// Consulta asíncrona de reanudación no urgente. El modelo solo puede
	// proponer esperar o proceder; nunca crea una acción ni toca lo crítico.
	consultant         InitiativeConsultant
	consultFingerprint string
	consultSubmitted   bool
}

// NewAutonomousLoop crea el orquestador B77 existente.
func NewAutonomousLoop(
	cfg LoopConfig,
	obs *ContextObserver,
	initEngine *InitiativeEngine,
	convMgr *ConversationManager,
	sProv StateProvider,
	gProv GoalProvider,
) *AutonomousLoop {
	return newAutonomousLoop(cfg, obs, initEngine, convMgr, sProv, gProv, nil)
}

// NewAutonomousExecutionLoop crea el loop B83 sobre un Cycle ya cableado.
// No construye especialistas ni adaptadores por su cuenta: la composición
// queda fuera del loop y B83 se limita a mantener vivo el ciclo.
func NewAutonomousExecutionLoop(cfg LoopConfig, cycle *Cycle) *AutonomousLoop {
	return newAutonomousLoop(cfg, nil, nil, nil, nil, nil, cycle)
}

func newAutonomousLoop(
	cfg LoopConfig,
	obs *ContextObserver,
	initEngine *InitiativeEngine,
	convMgr *ConversationManager,
	sProv StateProvider,
	gProv GoalProvider,
	cycle *Cycle,
) *AutonomousLoop {
	if cfg.Interval <= 0 {
		cfg.Interval = 5 * time.Second
	}
	if cfg.BufferSize <= 0 {
		cfg.BufferSize = 10
	}

	return &AutonomousLoop{
		config:     cfg,
		outputChan: make(chan ConversationDecision, cfg.BufferSize),
		observer:   obs,
		initiative: initEngine,
		dialogue:   convMgr,
		stateProv:  sProv,
		goalProv:   gProv,
		cycle:      cycle,
	}
}

// OutputChan retorna el canal de lectura para las decisiones conversacionales emitidas.
func (al *AutonomousLoop) OutputChan() <-chan ConversationDecision {
	return al.outputChan
}

// SetRunLock conecta el loop a un lock de ownership externo. Cuando está
// configurado, cada tick completo queda serializado con el camino interactivo
// que usa el mismo lock. Esto evita que Process() y la autonomía muten a la vez
// AutonomousState, Planner, Goals/Tasks o cualquier otra dependencia viva
// compartida por Engine. El lock es opcional para conservar los usos directos
// del paquete autonomy.
func (al *AutonomousLoop) SetRunLock(lock sync.Locker) {
	al.mu.Lock()
	al.runLock = lock
	al.mu.Unlock()
}

// SetOwnerRestrictions connects the loop to the owner's bounded prohibitions.
// A nil provider keeps the previous behavior. now may be nil; it defaults to
// time.Now. Both values are read under the loop mutex at the start of a tick.
func (al *AutonomousLoop) SetOwnerRestrictions(provider OwnerRestrictionProvider, now func() time.Time) {
	if al == nil {
		return
	}
	al.mu.Lock()
	defer al.mu.Unlock()
	al.restrictions = provider
	al.now = now
}

func (al *AutonomousLoop) restrictionSnapshot() (OwnerRestrictionProvider, func() time.Time) {
	al.mu.Lock()
	defer al.mu.Unlock()
	return al.restrictions, al.now
}

// SetInitiativeConsultant connects the optional background consultation lane.
// A nil consultant keeps the previous deterministic behavior.
func (al *AutonomousLoop) SetInitiativeConsultant(c InitiativeConsultant) {
	if al == nil {
		return
	}
	al.mu.Lock()
	defer al.mu.Unlock()
	al.consultant = c
	al.consultFingerprint = ""
	al.consultSubmitted = false
}

// IsRunning indica si el bucle autónomo está activo.
func (al *AutonomousLoop) IsRunning() bool {
	al.mu.Lock()
	defer al.mu.Unlock()
	return al.running
}

// Start inicia una sola goroutine del bucle.
// El contexto se deriva internamente para que Stop() pueda cerrar el loop
// de forma determinista sin exigir que el caller conserve otro cancel.
func (al *AutonomousLoop) Start(ctx context.Context) {
	al.mu.Lock()
	if al.running {
		al.mu.Unlock()
		return
	}

	loopCtx, cancel := context.WithCancel(ctx)
	al.running = true
	al.cancel = cancel
	al.wg.Add(1)
	al.mu.Unlock()

	go func() {
		defer al.wg.Done()
		defer func() {
			al.mu.Lock()
			al.running = false
			al.cancel = nil
			al.mu.Unlock()
		}()

		ticker := time.NewTicker(al.config.Interval)
		defer ticker.Stop()

		for {
			select {
			case <-loopCtx.Done():
				return
			case <-ticker.C:
				al.runTick(loopCtx)
			}
		}
	}()
}

// Stop solicita el cierre y espera a que la goroutine termine.
// Es idempotente y no hace nada si el loop ya estaba detenido.
func (al *AutonomousLoop) Stop() {
	al.mu.Lock()
	cancel := al.cancel
	al.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	al.wg.Wait()
}

// runTick selecciona el camino integrado (Cycle + InitiativeEngine) cuando
// ambos están disponibles. B88.5: antes de este bloque, existían dos
// caminos independientes en el mismo tick — Cycle.RunStep() se ejecutaba
// siempre, sin que InitiativeEngine pudiera bloquearlo, mientras el camino
// conversacional consultaba a InitiativeEngine solo para decidir cómo
// hablarle al usuario. runIntegratedTick calcula la InitiativeDecision UNA
// sola vez y la usa para las dos cosas: autorizar (o no) que Cycle avance,
// y alimentar la estrategia conversacional — ver autonomy/authority.go.
func (al *AutonomousLoop) runTick(ctx context.Context) {
	if err := ctx.Err(); err != nil {
		return
	}
	al.mu.Lock()
	runLock := al.runLock
	al.mu.Unlock()
	if runLock != nil {
		// Engine supplies a sync.Mutex, whose TryLock lets the ticker skip a
		// cycle while an interactive turn owns shared state. Blocking here can
		// queue an uncancellable tick behind Process() and make Stop() wait for
		// work that should have been discarded with the loop context.
		if tryLock, ok := runLock.(interface{ TryLock() bool }); ok {
			if !tryLock.TryLock() {
				return
			}
		} else {
			runLock.Lock()
		}
		defer runLock.Unlock()
	}

	if al.observer != nil && al.initiative != nil {
		al.runIntegratedTick(ctx)
		return
	}
	if al.cycle != nil {
		_ = al.cycle.RunStep(ctx)
	}
}

// runIntegratedTick es el camino real cuando hay InitiativeEngine
// disponible: calcula la decisión de B75 una sola vez y la usa como
// autoridad (B88.5, AllowsCycleExecution) antes de dejar avanzar al ciclo
// ejecutivo, y luego reutiliza esa misma decisión para el camino
// conversacional legado B77 — nunca se calcula dos veces.
func (al *AutonomousLoop) runIntegratedTick(ctx context.Context) {
	defer func() {
		if recover() != nil {
			// Un tick defectuoso no debe matar la goroutine del loop.
		}
	}()

	observations := al.observer.GetUnprocessedObservations()

	provider, clock := al.restrictionSnapshot()
	now := time.Now()
	if clock != nil {
		now = clock()
	}
	workClosed := provider != nil && provider.OwnerWorkInitiativeRestricted(now)
	quiet := false
	if provider != nil {
		if q, ok := provider.(interface{ OwnerQuiet(time.Time) bool }); ok {
			quiet = q.OwnerQuiet(now)
		}
	}

	var lastActivity time.Time
	if al.stateProv != nil {
		lastActivity = al.stateProv.LastUserActivity()
	}

	var hasPending bool
	var pendingGoalID string
	if al.goalProv != nil {
		hasPending, pendingGoalID = al.goalProv.PendingGoal()
	}

	// Escena 1: la prohibición del propietario o un estado sensible activo
	// restan la iniciativa propia (reanudar metas, abrir trabajo), pero no
	// silencian el camino de interrupción. Las señales críticas siguen llegando
	// al motor de decisión.
	evaluationObservations := observations
	if workClosed || quiet {
		hasPending = false
		pendingGoalID = ""
		evaluationObservations = criticalSignals(observations)
	}

	initDecision := al.initiative.Evaluate(
		evaluationObservations,
		hasPending,
		pendingGoalID,
		lastActivity,
	)
	al.applyInitiativeConsultation(&initDecision, hasPending, pendingGoalID, lastActivity, now)

	// B88.5: esto es lo que antes faltaba. Cycle.RunStep() ya no corre
	// incondicionalmente — solo cuando InitiativeEngine decidió que hay
	// motivo real para actuar ahora (RESUME_GOAL, o EXECUTE si algún día
	// Evaluate() lo produce). WAIT/NO_ACTION/ASK_USER/INFORM_USER/SUGGEST
	// dejan a Cycle sin correr este tick.
	if al.cycle != nil && AllowsCycleExecution(initDecision) {
		_ = al.cycle.RunStep(ctx)
	}

	if al.dialogue == nil {
		for _, obs := range observations {
			if !obs.Processed {
				al.observer.MarkAsProcessed(obs.ID)
			}
		}
		return
	}

	convDecision := al.dialogue.EvaluateStrategy(initDecision)

	if convDecision.Strategy != StrategyNone {
		select {
		case al.outputChan <- convDecision:
			for _, obs := range observations {
				if !obs.Processed {
					al.observer.MarkAsProcessed(obs.ID)
				}
			}
		default:
		}
		return
	}

	for _, obs := range observations {
		if !obs.Processed {
			al.observer.MarkAsProcessed(obs.ID)
		}
	}
}

// initiativeConsultLead is how long before the inactivity threshold the loop
// asks the model for a judgment. On this hardware the model takes tens of
// seconds, so the consultation starts early and nobody waits for it.
const initiativeConsultLead = 90 * time.Second

// applyInitiativeConsultation gives the model one bounded voice over a
// non-urgent resume: it can say "wait" once per context. It cannot create an
// action, and it is never consulted for a critical signal.
func (al *AutonomousLoop) applyInitiativeConsultation(decision *InitiativeDecision, hasPending bool, goalID string, lastActivity, now time.Time) {
	if al == nil || decision == nil || al.consultant == nil || !hasPending {
		return
	}
	goalID = strings.TrimSpace(goalID)
	if goalID == "" || lastActivity.IsZero() {
		return
	}
	fp := fmt.Sprintf("goal:%s:last:%d", goalID, lastActivity.Unix())
	if fp != al.consultFingerprint {
		al.consultFingerprint = fp
		al.consultSubmitted = false
	}

	if decision.Action == ActionResumeGoal {
		al.consumeResumeConsultation(decision, fp, now)
		return
	}
	// Only the quiet states before a resume are eligible. A critical signal,
	// an explicit question or an active goal-state notice is not.
	if decision.Action != ActionWait && decision.Action != ActionNoAction {
		return
	}
	if al.consultSubmitted {
		return
	}
	threshold := al.initiative.MinInactivityForResume()
	if threshold <= 0 {
		return
	}
	lead := initiativeConsultLead
	if lead >= threshold {
		lead = threshold / 2
	}
	if now.Sub(lastActivity) < threshold-lead {
		return
	}

	req := consult.Request{
		Scene:          "autonomy_resume_goal",
		Input:          fmt.Sprintf("objetivo pendiente %s; inactividad %s", goalID, now.Sub(lastActivity).Round(time.Second)),
		HarnessReading: decision.Reason,
		OwnerState:     "absent",
		Candidates:     initiativeResumeCandidates(),
		Evidence: []consult.Evidence{
			{ID: "goal", Text: "objetivo pendiente " + goalID},
			{ID: "inactivity", Text: now.Sub(lastActivity).Round(time.Second).String()},
			{ID: "decision", Text: decision.Reason},
		},
	}
	if al.consultant.ProposeInitiative(req, fp) {
		al.consultSubmitted = true
	}
}

func (al *AutonomousLoop) consumeResumeConsultation(decision *InitiativeDecision, fingerprint string, now time.Time) {
	proposal, ok := al.consultant.FreshInitiative(fingerprint)
	if !ok {
		return
	}
	verdict := consult.Decide(
		proposal,
		initiativeResumeCandidates(),
		func(c consult.Candidate) (bool, string) {
			switch c.ID {
			case "proceed", "wait":
				return true, ""
			default:
				return false, "not_authorized:" + c.ID
			}
		},
		"proceed",
	)
	if verdict.Accepted && verdict.Candidate.ID == "wait" {
		decision.Action = ActionWait
		decision.Reason = "model_consultation_wait"
		decision.Timestamp = now
	}
}

func initiativeResumeCandidates() []consult.Candidate {
	return []consult.Candidate{
		{ID: "proceed", Kind: "resume", Description: "Reanudar el objetivo pendiente", Authorized: true, Risk: "low", Reversible: true},
		{ID: "wait", Kind: "wait", Description: "Esperar; no interrumpir todavía", Authorized: true, Risk: "low", Reversible: true},
		{ID: "interrupt_now", Kind: "interrupt", Description: "Interrumpir de inmediato", Authorized: false, Risk: "high", Reversible: false},
	}
}

// criticalSignals keeps only the observations that belong to the interruption
// path. A work restriction subtracts Ahmelia's own initiative; it must not hide
// a critical system signal.
func criticalSignals(observations []ContextObservation) []ContextObservation {
	out := make([]ContextObservation, 0, len(observations))
	for _, obs := range observations {
		if obs.Processed || obs.Type != TypeSystemSignal {
			continue
		}
		if severity, _ := obs.Payload["severity"].(string); severity == "CRITICAL" {
			out = append(out, obs)
		}
	}
	return out
}

// NewIntegratedAutonomousLoop combina B83 (Cycle real) con el camino
// conversacional B77 en un solo ticker. B88 lo usa para conectar la nueva
// autonomía sin apagar el comportamiento conversacional existente.
func NewIntegratedAutonomousLoop(
	cfg LoopConfig,
	cycle *Cycle,
	obs *ContextObserver,
	initEngine *InitiativeEngine,
	convMgr *ConversationManager,
	sProv StateProvider,
	gProv GoalProvider,
) *AutonomousLoop {
	return newAutonomousLoop(cfg, obs, initEngine, convMgr, sProv, gProv, cycle)
}
