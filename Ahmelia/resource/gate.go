package resource

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// LinguisticsGate adapta el Governor a la interfaz linguistics.Gate.
type LinguisticsGate struct {
	governor *Governor
	mu       sync.Mutex
	active   map[string]*leaseRef // class -> leaseRef
}

type leaseRef struct {
	lease *Lease
	class string
}

func NewLinguisticsGate(g *Governor) *LinguisticsGate {
	return &LinguisticsGate{
		governor: g,
		active:   make(map[string]*leaseRef),
	}
}

// Enter implementa linguistics.Gate.
func (g *LinguisticsGate) Enter(ctx context.Context, class string, priority int) (context.Context, func(), error) {
	if g.governor == nil {
		return ctx, func() {}, nil
	}

	// Mapear prioridad linguistics -> resource.Priority
	var p Priority
	switch priority {
	case 0:
		p = PriorityOwn
	case 1:
		p = PriorityLearning
	case 2:
		p = PriorityUserGoal
	case 3:
		p = PriorityUrgent
	case 4:
		p = PriorityUserTurn
	default:
		p = PriorityUserGoal
	}

	// Mapear clase linguistics -> resource.Kind
	var kind Kind
	switch class {
	case "interpretation":
		kind = KindLearning
	case "generation":
		kind = KindUserGoal
	case "proposal":
		kind = KindUserGoal
	default:
		kind = KindUserGoal
	}

	lease, err := g.governor.Acquire(ctx, Request{
		Priority: p,
		Kind:     kind,
		ID:       fmt.Sprintf("ling-%s-%d", class, time.Now().UnixNano()),
	})
	if err != nil {
		return nil, nil, err
	}

	// Registrar para evitar doble liberación
	g.mu.Lock()
	g.active[class] = &leaseRef{lease: lease, class: class}
	g.mu.Unlock()

	// Contexto que se cancela al liberar
	leaseCtx := lease.Ctx
	exitOnce := sync.Once{}
	exit := func() {
		exitOnce.Do(func() {
			g.mu.Lock()
			delete(g.active, class)
			g.mu.Unlock()
			lease.Release()
		})
	}

	return leaseCtx, exit, nil
}

// Close libera todas las concesiones activas.
func (g *LinguisticsGate) Close() {
	g.mu.Lock()
	defer g.mu.Unlock()
	for _, lr := range g.active {
		lr.lease.Release()
	}
	g.active = nil
}
