package cognition

import (
	"sync"
	"time"
)

// ============================================================================
// CONTEXT_CACHE.GO — Bloque 10, Fase 2.6 (Integración Cognitiva)
// ============================================================================
//
// El propio Bloque 9 (context_builder.go) dejó anotada su limitación:
// Build() recorre el EventLog completo (Habits + Insights + Reflection,
// cada uno con su propio recorrido) cada vez que se llama. Si el usuario
// hace tres preguntas seguidas y nada cambió entre medio, Ahmelia
// recalculaba las tres veces el mismo contexto.
//
// Cache no cambia QUÉ se calcula (eso sigue siendo 100% responsabilidad
// de Builder) — solo decide CUÁNDO hace falta recalcular.

// DefaultCacheTTL es la red de seguridad por tiempo: incluso si nadie
// llamó Invalidate() a mano, un contexto de más de este tiempo se
// reconstruye igual. Cubre cambios que no pasan por execute() — por
// ejemplo el tema de conversación actualizándose dentro del branch de
// IntentQuestion, que no dispara ninguna invalidación explícita.
const DefaultCacheTTL = 15 * time.Second

// Cache envuelve un Builder. Invalidación por dos vías:
//   - Evento: el Engine llama Invalidate() después de cada acción real
//     (execute() ya sabe, mejor que nadie, cuándo algo pudo haber
//     cambiado).
//   - Crecimiento del EventLog: señal barata — comparar len(Events) no
//     cuesta nada, y agarra cualquier acción que Invalidate() se haya
//     olvidado de marcar.
//   - Tiempo: TTL, para todo lo demás.
type Cache struct {
	builder *Builder
	ttl     time.Duration

	mu        sync.Mutex
	cached    *UnifiedContext
	builtAt   time.Time
	dirty     bool
	eventsLen int // último len(EventLog.Events) visto al construir
	hits      int // Get() que reusaron el cache
	misses    int // Get() que reconstruyeron
}

// NewCache arma el cache alrededor de un Builder ya existente. dirty
// arranca en true: la primera llamada a Get() siempre construye.
func NewCache(builder *Builder, ttl time.Duration) *Cache {
	return &Cache{builder: builder, ttl: ttl, dirty: true}
}

// Invalidate fuerza que el próximo Get() reconstruya sin esperar al TTL
// ni al crecimiento del EventLog. Pensado para llamarse justo después de
// cualquier acción que pueda haber cambiado el mundo (execute() en
// core/engine.go).
func (c *Cache) Invalidate() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.dirty = true
}

// Get devuelve el UnifiedContext vigente: el cacheado si sigue siendo
// válido, o uno recién construido si venció el TTL, alguien invalidó a
// mano, o el EventLog creció desde la última construcción.
func (c *Cache) Get() *UnifiedContext {
	c.mu.Lock()
	defer c.mu.Unlock()

	currentLen := 0
	if c.builder.EventStore != nil {
		currentLen = c.builder.EventStore.Len()
	}

	expired := c.cached == nil || time.Since(c.builtAt) > c.ttl
	grew := currentLen != c.eventsLen

	if !c.dirty && !expired && !grew {
		c.hits++
		return c.cached
	}

	c.misses++
	c.cached = c.builder.Build()
	c.builtAt = time.Now()
	c.eventsLen = currentLen
	c.dirty = false
	return c.cached
}

// Stats reporta hits/misses acumulados — sirve para confirmar en
// runtime que el cache está sirviendo de algo, sin exponer los campos
// privados directamente.
type Stats struct {
	Hits        int
	Misses      int
	LastBuiltAt time.Time
	TTL         time.Duration
}

func (c *Cache) Stats() Stats {
	c.mu.Lock()
	defer c.mu.Unlock()
	return Stats{Hits: c.hits, Misses: c.misses, LastBuiltAt: c.builtAt, TTL: c.ttl}
}
