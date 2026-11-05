package autonomy

import (
	"context"
	"sync"
	"time"
)

// StateProvider define la interfaz requerida para consultar la última actividad del usuario (B72).
type StateProvider interface {
	LastUserActivity() time.Time
}

// GoalProvider define la interfaz requerida para consultar objetivos pendientes (B73).
type GoalProvider interface {
	PendingGoal() (hasPending bool, goalID string)
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

	var lastActivity time.Time
	if al.stateProv != nil {
		lastActivity = al.stateProv.LastUserActivity()
	}

	var hasPending bool
	var pendingGoalID string
	if al.goalProv != nil {
		hasPending, pendingGoalID = al.goalProv.PendingGoal()
	}

	initDecision := al.initiative.Evaluate(
		observations,
		hasPending,
		pendingGoalID,
		lastActivity,
	)

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
