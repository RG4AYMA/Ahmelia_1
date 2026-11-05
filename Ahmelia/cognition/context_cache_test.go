package cognition

import (
	"testing"
	"time"

	"ahmelia/comprehension"
	"ahmelia/environment"
	"ahmelia/memory"
	memoryintelligence "ahmelia/memory_intelligence"
	"ahmelia/planning"
	"ahmelia/reasoning_deep"
)

func newTestBuilder(store *memoryintelligence.EventStore) *Builder {
	return NewBuilder(
		environment.NewManager(),
		store,
		&memory.LongMemory{Data: map[string]*memory.Knowledge{}},
		planning.NewAgendaManager(),
		reasoning_deep.NewMentalAgendaV2(),
		comprehension.NewConversationContext(),
		memoryintelligence.NewLearningStore(memoryintelligence.NewLearningState()),
	)
}

// TestCache_ReusaSiNadaCambio confirma el caso central: dos Get()
// seguidos sin que nada cambie deberían devolver el mismo puntero (no
// solo contenido igual) y contar como un solo miss + un hit.
func TestCache_ReusaSiNadaCambio(t *testing.T) {
	log := &memoryintelligence.EventLog{}
	store := memoryintelligence.NewEventStore(log)
	c := NewCache(newTestBuilder(store), time.Minute)

	first := c.Get()
	second := c.Get()

	if first != second {
		t.Fatal("sin cambios de por medio, Get() debería devolver el mismo *UnifiedContext cacheado, no reconstruir")
	}

	stats := c.Stats()
	if stats.Misses != 1 || stats.Hits != 1 {
		t.Fatalf("esperaba 1 miss (la construcción inicial) y 1 hit (la reutilización), obtuve misses=%d hits=%d", stats.Misses, stats.Hits)
	}
}

// TestCache_InvalidateFuerzaReconstruccion confirma que Invalidate()
// realmente fuerza un rebuild en el próximo Get(), sin esperar TTL.
func TestCache_InvalidateFuerzaReconstruccion(t *testing.T) {
	log := &memoryintelligence.EventLog{}
	store := memoryintelligence.NewEventStore(log)
	c := NewCache(newTestBuilder(store), time.Minute)

	first := c.Get()
	c.Invalidate()
	second := c.Get()

	if first == second {
		t.Fatal("después de Invalidate(), Get() debería reconstruir (puntero distinto), no reusar el cache viejo")
	}
	if c.Stats().Misses != 2 {
		t.Fatalf("esperaba 2 misses (construcción inicial + post-Invalidate), obtuve %d", c.Stats().Misses)
	}
}

// TestCache_CreceElEventLogFuerzaReconstruccion confirma la segunda vía
// de invalidación: si el EventLog crece entre un Get() y el siguiente
// (una acción se ejecutó pero nadie llamó Invalidate() a mano), el
// cache lo detecta solo por el cambio en len(Events).
func TestCache_CreceElEventLogFuerzaReconstruccion(t *testing.T) {
	log := &memoryintelligence.EventLog{}
	store := memoryintelligence.NewEventStore(log)
	c := NewCache(newTestBuilder(store), time.Minute)

	first := c.Get()

	log.Events = append(log.Events, memoryintelligence.RawEvent{
		Action:  "crear_carpeta",
		Outcome: memoryintelligence.Outcome{Success: true},
	})

	second := c.Get()

	if first == second {
		t.Fatal("si el EventLog creció, Get() debería reconstruir aunque nadie haya llamado Invalidate()")
	}
}

// TestCache_TTLVencidoFuerzaReconstruccion confirma la red de
// seguridad por tiempo, para cambios que no pasan por EventLog ni por
// Invalidate() explícito.
func TestCache_TTLVencidoFuerzaReconstruccion(t *testing.T) {
	log := &memoryintelligence.EventLog{}
	store := memoryintelligence.NewEventStore(log)
	c := NewCache(newTestBuilder(store), 10*time.Millisecond)

	first := c.Get()
	time.Sleep(20 * time.Millisecond)
	second := c.Get()

	if first == second {
		t.Fatal("con el TTL vencido, Get() debería reconstruir aunque nada haya invalidado a mano")
	}
}
