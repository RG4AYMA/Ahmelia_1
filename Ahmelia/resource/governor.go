package resource

import (
	"context"
	"sync"
	"time"
)

// Priority define la prioridad de una petición al gobernador.
// Valores menores = mayor prioridad.
type Priority int

// Convención: valores MENORES = mayor urgencia (ver sortQueue y preempt).
// El turno del usuario es la máxima prioridad y nunca es desplazado; el
// trabajo propio es el primero en ceder. (Corregido 2026-10-17 durante el
// cableado T3: los valores estaban invertidos respecto de esta intención —
// Learning desplazaba a UserGoal y un UserTurn encolado quedaba último.)
const (
	PriorityUserTurn Priority = 0 // turno del usuario: máxima prioridad, nunca desplazado
	PriorityUrgent   Priority = 1 // urgente
	PriorityUserGoal Priority = 2 // meta del usuario
	PriorityLearning Priority = 3 // aprendizaje
	PriorityOwn      Priority = 4 // trabajo propio (reflexión, práctica, exploración)
)

// Kind clasifica el tipo de trabajo para trazabilidad.
type Kind string

const (
	KindOwn      Kind = "own"
	KindLearning Kind = "learning"
	KindUserGoal Kind = "user_goal"
	KindUrgent   Kind = "urgent"
	KindUserTurn Kind = "user_turn"
)

// Request representa una petición de uso del LLM.
type Request struct {
	Priority Priority
	Kind     Kind
	ID       string // identificador único para trazabilidad
}

// Lease es la concesión concedida por el gobernador.
// Ctx se cancela al liberar la concesión (o al preemptar/cerrar) y también
// cuando se cancela el contexto del llamador: el presupuesto del llamador
// acota siempre el trabajo concedido.
type Lease struct {
	Request Request
	Ctx     context.Context
	Cancel  context.CancelFunc
	Release func() // idempotente
}

// Clock permite inyectar tiempo (para tests deterministas).
type Clock interface {
	Now() time.Time
	After(d time.Duration) <-chan time.Time
}

type realClock struct{}

func (realClock) Now() time.Time                         { return time.Now() }
func (realClock) After(d time.Duration) <-chan time.Time { return time.After(d) }

// Governor gestiona una única ranura de LLM con prioridades y anti-inanición.
type Governor struct {
	mu sync.Mutex

	slot         *Lease       // concesión actual (nil si libre)
	queue        []*queuedReq // peticiones en espera
	clock        Clock
	preemptGrace time.Duration // tiempo para que el titular libere tras preempt
	agingAfter   time.Duration // envejecimiento: sube prioridad tras esta espera
	maxPriority  Priority      // tope de envejecimiento (PriorityUserGoal)
}

type queuedReq struct {
	req               Request
	ctx               context.Context
	cancel            context.CancelFunc
	done              chan struct{} // señal de concesión
	enqueuedAt        time.Time
	effectivePriority Priority // prioridad actual (puede subir por envejecimiento)
}

// GovernorConfig configura el gobernador.
type GovernorConfig struct {
	Clock        Clock         // nil = realClock
	PreemptGrace time.Duration // default 500ms
	AgingAfter   time.Duration // default 30s
	MaxPriority  Priority      // default PriorityUserGoal
}

// NewGovernor crea un gobernador con configuración.
func NewGovernor(cfg GovernorConfig) *Governor {
	if cfg.Clock == nil {
		cfg.Clock = realClock{}
	}
	if cfg.PreemptGrace <= 0 {
		cfg.PreemptGrace = 500 * time.Millisecond
	}
	if cfg.AgingAfter <= 0 {
		cfg.AgingAfter = 30 * time.Second
	}
	// Tope de envejecimiento: 0 (sin configurar, o UserTurn) y cualquier
	// tope más agresivo que UserGoal se normalizan a UserGoal — el
	// envejecimiento no debe alcanzar los niveles urgente/turno.
	if cfg.MaxPriority < PriorityUserGoal {
		cfg.MaxPriority = PriorityUserGoal
	}
	return &Governor{
		clock:        cfg.Clock,
		preemptGrace: cfg.PreemptGrace,
		agingAfter:   cfg.AgingAfter,
		maxPriority:  cfg.MaxPriority,
	}
}

// Acquire solicita la ranura del LLM.
// Devuelve un Lease con su propio contexto y Release idempotente.
// Si el contexto se cancela antes de conceder, devuelve error.
func (g *Governor) Acquire(ctx context.Context, req Request) (*Lease, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	// Check for cancellation immediately
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if req.ID == "" {
		req.ID = "req-" + time.Now().UTC().Format("20060102T150405.000000000")
	}

	// Fast path: ranura libre. El lease deriva del ctx del llamador: si el
	// llamador cancela (presupuesto sync de 900 ms, apagado, etc.), el
	// trabajo concedido se corta con él.
	g.mu.Lock()
	if g.slot == nil {
		leaseCtx, cancel := context.WithCancel(ctx)
		lease := &Lease{
			Request: req,
			Ctx:     leaseCtx,
			Cancel:  cancel,
			Release: func() { g.release(leaseCtx, cancel) },
		}
		g.slot = lease
		g.mu.Unlock()
		return lease, nil
	}

	// Encolar
	reqCtx, reqCancel := context.WithCancel(ctx)
	q := &queuedReq{
		req:               req,
		ctx:               reqCtx,
		cancel:            reqCancel,
		done:              make(chan struct{}),
		enqueuedAt:        g.clock.Now(),
		effectivePriority: req.Priority,
	}
	g.queue = append(g.queue, q)
	g.sortQueue()
	// Un encolado es un cambio: si lo que llegó (o la cabeza de la cola)
	// es más urgente que el titular, el preempt no espera al driver
	// periódico (latido N0) para dispararse.
	g.preemptIfNeededLocked()
	g.mu.Unlock()

	// Esperar concesión o cancelación
	select {
	case <-reqCtx.Done():
		g.mu.Lock()
		g.removeFromQueue(q)
		g.mu.Unlock()
		reqCancel()
		return nil, reqCtx.Err()
	case <-q.done:
		// Concedido: el gobernador cerró q.done al dar la concesión
	}

	// La concesión ya fue asignada en g.slot por el loop de preempt/aging
	// Buscar el lease correspondiente
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.slot != nil && g.slot.Request.ID == req.ID {
		return g.slot, nil
	}
	return nil, context.Canceled
}

// sortQueue ordena por prioridad efectiva (menor = más urgente) y luego por llegada.
func (g *Governor) sortQueue() {
	// Recalcular prioridad efectiva por envejecimiento
	now := g.clock.Now()
	for _, q := range g.queue {
		waited := now.Sub(q.enqueuedAt)
		if waited >= g.agingAfter && q.effectivePriority > g.maxPriority {
			// Subir un nivel, tope en maxPriority
			newP := q.effectivePriority - 1
			if newP < g.maxPriority {
				newP = g.maxPriority
			}
			q.effectivePriority = newP
		}
	}
	// Ordenar: prioridad ascendente, luego FIFO
	// Usar selección simple para mantener estabilidad
	for i := 0; i < len(g.queue); i++ {
		best := i
		for j := i + 1; j < len(g.queue); j++ {
			if g.queue[j].effectivePriority < g.queue[best].effectivePriority ||
				(g.queue[j].effectivePriority == g.queue[best].effectivePriority &&
					g.queue[j].enqueuedAt.Before(g.queue[best].enqueuedAt)) {
				best = j
			}
		}
		if best != i {
			g.queue[i], g.queue[best] = g.queue[best], g.queue[i]
		}
	}
}

// removeFromQueue elimina una petición de la cola.
func (g *Governor) removeFromQueue(target *queuedReq) {
	for i, q := range g.queue {
		if q == target {
			g.queue = append(g.queue[:i], g.queue[i+1:]...)
			return
		}
	}
}

// release libera la ranura actual y concede al siguiente en cola si hay.
func (g *Governor) release(leaseCtx context.Context, cancel context.CancelFunc) {
	g.mu.Lock()
	defer g.mu.Unlock()

	// Solo liberar si este lease sigue siendo el titular
	if g.slot == nil || g.slot.Ctx != leaseCtx {
		return
	}
	cancel()
	g.slot = nil

	// Conceder al siguiente en cola
	if len(g.queue) > 0 {
		next := g.queue[0]
		g.queue = g.queue[1:]
		close(next.done) // señal de concesión
		leaseCtx2, cancel2 := context.WithCancel(next.ctx)
		lease := &Lease{
			Request: next.req,
			Ctx:     leaseCtx2,
			Cancel:  cancel2,
			Release: func() { g.release(leaseCtx2, cancel2) },
		}
		g.slot = lease
	}
}

// PreemptIfNeeded verifica si la petición en cola de mayor prioridad
// debe preemptar al titular actual. Debe llamarse periódicamente (driver
// de continuidad / latido N0) o tras cambios; el envejecimiento de la
// cola se recalcula en cada consulta.
func (g *Governor) PreemptIfNeeded() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.sortQueue()
	g.preemptIfNeededLocked()
}

// preemptIfNeededLocked aplica la regla de preempt con el lock tomado.
func (g *Governor) preemptIfNeededLocked() {
	if g.slot == nil || len(g.queue) == 0 {
		return
	}

	next := g.queue[0]
	// El turno de usuario nunca es desplazado (PriorityUserTurn = 0,
	// valor mínimo: máxima prioridad).
	if g.slot.Request.Priority == PriorityUserTurn {
		return
	}

	// Si el siguiente es más urgente (valor menor), preemptar.
	if next.effectivePriority < g.slot.Request.Priority {
		// Cancelar titular actual con gracia
		oldCancel := g.slot.Cancel
		oldCtx := g.slot.Ctx
		g.slot.Cancel = func() {} // evitar double-release
		go func() {
			select {
			case <-time.After(g.preemptGrace):
				oldCancel()
			case <-oldCtx.Done():
				// Ya liberó por su cuenta
			}
		}()

		// Conceder al siguiente
		close(next.done)
		leaseCtx, cancel := context.WithCancel(next.ctx)
		lease := &Lease{
			Request: next.req,
			Ctx:     leaseCtx,
			Cancel:  cancel,
			Release: func() { g.release(leaseCtx, cancel) },
		}
		g.slot = lease
		g.queue = g.queue[1:]
	}
}

// Close cierra el gobernador: cancela todo y vacía la cola.
func (g *Governor) Close() {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.slot != nil {
		g.slot.Cancel()
		g.slot = nil
	}
	for _, q := range g.queue {
		close(q.done)
	}
	g.queue = nil
}

// Stats devuelve estado actual para observabilidad.
func (g *Governor) Stats() GovernorStats {
	g.mu.Lock()
	defer g.mu.Unlock()

	stats := GovernorStats{
		QueueLength: len(g.queue),
	}
	if g.slot != nil {
		stats.Active = &LeaseInfo{
			ID:       g.slot.Request.ID,
			Priority: g.slot.Request.Priority,
			Kind:     g.slot.Request.Kind,
		}
	}
	for _, q := range g.queue {
		stats.Queued = append(stats.Queued, QueuedInfo{
			ID:                q.req.ID,
			Priority:          q.req.Priority,
			EffectivePriority: q.effectivePriority,
			Kind:              q.req.Kind,
			WaitTime:          g.clock.Now().Sub(q.enqueuedAt),
		})
	}
	return stats
}

type GovernorStats struct {
	Active      *LeaseInfo
	QueueLength int
	Queued      []QueuedInfo
}

type LeaseInfo struct {
	ID       string
	Priority Priority
	Kind     Kind
}

type QueuedInfo struct {
	ID                string
	Priority          Priority
	EffectivePriority Priority
	Kind              Kind
	WaitTime          time.Duration
}
