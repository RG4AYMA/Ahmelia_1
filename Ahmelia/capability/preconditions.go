package capability

import (
	"context"
	"fmt"
	"sort"
	"sync"
)

// Facts es la observación estructurada del entorno (clave -> valor) que provee
// la capa de observación. Los predicados leen hechos, nunca texto del usuario.
type Facts map[string]string

type Predicate func(ctx context.Context, f Facts) (ok bool, evidence string)

type PreconditionFailure struct {
	Name     string
	Evidence string
}

// PreconditionChecker evalúa de verdad Capability.Preconditions, que hoy solo
// se documentan. Falla CERRADO: una precondición sin predicado registrado se
// reporta como no verificable y bloquea la ejecución.
type PreconditionChecker struct {
	mu    sync.RWMutex
	preds map[string]Predicate
}

func NewPreconditionChecker() *PreconditionChecker {
	return &PreconditionChecker{preds: map[string]Predicate{}}
}

func (c *PreconditionChecker) Register(name string, p Predicate) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.preds[name] = p
}

func (c *PreconditionChecker) Check(ctx context.Context, cap Capability, f Facts) []PreconditionFailure {
	c.mu.RLock()
	defer c.mu.RUnlock()
	var out []PreconditionFailure
	names := append([]string(nil), cap.Preconditions...)
	sort.Strings(names)
	for _, n := range names {
		p, ok := c.preds[n]
		if !ok {
			out = append(out, PreconditionFailure{n, "no_predicate_registered"})
			continue
		}
		func() {
			defer func() {
				if r := recover(); r != nil {
					out = append(out, PreconditionFailure{n, fmt.Sprintf("predicate_panic:%v", r)})
				}
			}()
			if ok, ev := p(ctx, f); !ok {
				out = append(out, PreconditionFailure{n, ev})
			}
		}()
	}
	return out
}
