package cognition

import (
	"sync"
	"time"

	memoryintelligence "ahmelia/memory_intelligence"
)

// ============================================================================
// REASONING_CACHE.GO — Bloque 32, Fase 3.0 (cierre del grupo 31→35)
// ============================================================================
//
// Reasoning Cache era, a propósito, el último bloque de este grupo —
// se acordó explícitamente al planear el orden (31→33→34→35→32) porque
// hasta Bloque 35 no existía un objeto único que valiera la pena
// cachear: ReasoningContext era solo el contrato (31), después empezó
// a llenarse de verdad (33), y recién en 34/35 alguien lo LEÍA para
// algo (ResponseComposer). Cachear antes de eso habría sido cachear
// fragmentos sueltos, no un resultado de razonamiento completo.
//
// Mismo criterio que Cache (Bloque 10, context_cache.go), aplicado a
// algo más caro: Cache memoiza un ÚNICO UnifiedContext sin parámetros
// (no hay "por pregunta", hay "el contexto actual"). ReasoningContext
// sí depende del input — cada pregunta puede necesitar razonamiento
// distinto — así que ReasoningCache memoiza POR CLAVE (el input
// normalizado), no un solo valor.
//
// "No cambia QUÉ se calcula, decide CUÁNDO hace falta recalcular" —
// literalmente la misma frase que ya usaba el comentario de Bloque 10,
// porque es exactamente el mismo principio.

// ReasoningCacheTTL es la red de seguridad por tiempo, mismo rol que
// DefaultCacheTTL en Bloque 10: incluso si el EventLog no creció,
// una entrada de más de este tiempo se recalcula igual. Más corto que
// DefaultCacheTTL (15s) porque el razonamiento de Fase 3 depende
// también de LearningState (ActionConfidence), que puede cambiar sin
// que el EventLog en sí haya crecido todavía en el mismo instante.
const ReasoningCacheTTL = 10 * time.Second

// reasoningCacheEntry es una entrada cacheada: el ReasoningContext ya
// resuelto (Activated=true con las 6 capas de Fase 3 pobladas, o
// Activated=false con solo GoalAnalysis/ReasoningNeed) y cuándo se
// calculó.
type reasoningCacheEntry struct {
	ctx     *ReasoningContext
	builtAt time.Time
}

// ReasoningCache envuelve el resultado de razonar sobre un input.
// Invalidación por dos vías, igual que Cache:
//   - Crecimiento del EventLog: señal barata — un EventLog más largo
//     puede significar que ActionConfidence cambió, así que CUALQUIER
//     entrada cacheada podría estar desactualizada, no solo la del
//     input que se está pidiendo ahora — por eso el crecimiento
//     invalida TODO el cache, no solo una entrada.
//   - Tiempo: TTL, para todo lo demás (cambios que no pasan por
//     execute(), como ajustes manuales a LearningState).
//
// No hay invalidación "por evento" explícita (a diferencia de Cache,
// que expone Invalidate() para que execute() la llame después de cada
// acción real) — la señal de EventLog ya cubre ese caso, porque
// ejecutar una acción real es, literalmente, lo que hace crecer el
// EventLog. Invalidate() igual se expone, para uso manual o futuro,
// con el mismo criterio de "mejor tenerlo y no usarlo todavía".
type ReasoningCache struct {
	mu sync.Mutex

	entries    map[string]reasoningCacheEntry
	ttl        time.Duration
	eventStore *memoryintelligence.EventStore
	eventsLen  int // último EventStore.Len() visto

	hits   int
	misses int
}

// NewReasoningCache arma el cache. eventStore puede ser nil (por ejemplo
// en tests que no lo necesitan) — checkGrowth() lo trata como longitud
// 0 y simplemente nunca dispara la invalidación por crecimiento.
func NewReasoningCache(eventStore *memoryintelligence.EventStore, ttl time.Duration) *ReasoningCache {
	return &ReasoningCache{
		entries:    make(map[string]reasoningCacheEntry),
		ttl:        ttl,
		eventStore: eventStore,
	}
}

// checkGrowth vacía TODO el cache si el EventLog creció desde la
// última vez que se miró — mismo criterio que Cache.Get(), pero acá
// aplicado a todas las entradas a la vez en lugar de a un solo valor,
// porque no hay forma barata de saber CUÁL entrada puntual quedó
// desactualizada sin recalcularla (lo que anularía el propósito del
// cache). Debe llamarse con mu ya tomado.
func (c *ReasoningCache) checkGrowth() {
	currentLen := 0
	if c.eventStore != nil {
		currentLen = c.eventStore.Len()
	}
	if currentLen != c.eventsLen {
		c.entries = make(map[string]reasoningCacheEntry)
		c.eventsLen = currentLen
	}
}

// Get devuelve el ReasoningContext cacheado para key, si existe y
// sigue vigente (ni venció el TTL ni el EventLog creció desde que se
// guardó). Nunca calcula nada — a diferencia de Cache.Get(), que
// reconstruye solo cuando hace falta, acá el llamador (reasonAboutQuestion
// en core/engine.go) es quien sabe cómo calcular un ReasoningContext,
// así que Get()/Store() quedan separados en vez de un único método que
// reciba una función de cómputo.
func (c *ReasoningCache) Get(key string) (*ReasoningContext, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.checkGrowth()

	entry, ok := c.entries[key]
	if !ok || time.Since(entry.builtAt) > c.ttl {
		c.misses++
		return nil, false
	}

	c.hits++
	return entry.ctx, true
}

// Store guarda un ReasoningContext ya resuelto bajo key, reemplazando
// cualquier entrada previa para la misma clave.
func (c *ReasoningCache) Store(key string, ctx *ReasoningContext) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries[key] = reasoningCacheEntry{ctx: ctx, builtAt: time.Now()}
}

// Invalidate vacía todo el cache a mano — no hace falta hoy (el chequeo
// de crecimiento del EventLog ya cubre el caso real), pero queda
// expuesto por paridad con Cache.Invalidate() y por si algún bloque
// futuro encuentra un caso real que lo necesite.
func (c *ReasoningCache) Invalidate() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = make(map[string]reasoningCacheEntry)
}

// ReasoningCacheStats reporta hits/misses/tamaño acumulados — sirve
// para confirmar en runtime que el cache está sirviendo de algo, mismo
// propósito que Stats (Bloque 10) pero sin mezclar los dos tipos: Stats
// tiene LastBuiltAt/TTL de un único valor, esto tiene Size porque acá
// hay muchas entradas a la vez.
type ReasoningCacheStats struct {
	Hits   int
	Misses int
	Size   int
}

func (c *ReasoningCache) Stats() ReasoningCacheStats {
	c.mu.Lock()
	defer c.mu.Unlock()
	return ReasoningCacheStats{Hits: c.hits, Misses: c.misses, Size: len(c.entries)}
}
