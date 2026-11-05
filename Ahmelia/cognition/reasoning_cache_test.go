package cognition

import (
	"testing"
	"time"

	memoryintelligence "ahmelia/memory_intelligence"
)

// TestReasoningCache_MissLuegoHit confirma la ruta central: la primera
// consulta de una clave es un miss, y después de Store() la misma
// clave es un hit que devuelve el mismo *ReasoningContext.
func TestReasoningCache_MissLuegoHit(t *testing.T) {
	c := NewReasoningCache(nil, time.Minute)

	if _, hit := c.Get("necesito automatizar algo"); hit {
		t.Fatal("la primera consulta de una clave nueva no debería ser un hit")
	}

	ctx := NewReasoningContext("necesito automatizar algo")
	ctx.Activated = true
	c.Store("necesito automatizar algo", ctx)

	got, hit := c.Get("necesito automatizar algo")
	if !hit {
		t.Fatal("después de Store(), la misma clave debería ser un hit")
	}
	if got != ctx {
		t.Error("Get() debería devolver el mismo *ReasoningContext que se guardó")
	}
}

// TestReasoningCache_ClavesDistintasNoSeMezclan confirma que dos inputs
// distintos no comparten cache — cada uno guarda y devuelve su propio
// ReasoningContext.
func TestReasoningCache_ClavesDistintasNoSeMezclan(t *testing.T) {
	c := NewReasoningCache(nil, time.Minute)

	ctxA := NewReasoningContext("pregunta a")
	ctxB := NewReasoningContext("pregunta b")
	c.Store("pregunta a", ctxA)
	c.Store("pregunta b", ctxB)

	gotA, _ := c.Get("pregunta a")
	gotB, _ := c.Get("pregunta b")

	if gotA != ctxA || gotB != ctxB {
		t.Error("cada clave debería devolver su propio ReasoningContext, sin mezclarse")
	}
}

// TestReasoningCache_TTLExpira confirma que una entrada más vieja que
// el TTL vuelve a ser un miss, incluso sin que el EventLog haya
// cambiado.
func TestReasoningCache_TTLExpira(t *testing.T) {
	c := NewReasoningCache(nil, time.Millisecond)

	c.Store("x", NewReasoningContext("x"))
	time.Sleep(5 * time.Millisecond)

	if _, hit := c.Get("x"); hit {
		t.Fatal("una entrada más vieja que el TTL no debería seguir siendo un hit")
	}
}

// TestReasoningCache_EventLogCrecidoInvalidaTodo confirma el criterio
// central de invalidación: si el EventLog creció desde la última vez
// que se consultó el cache, TODAS las entradas se descartan, no solo
// la relacionada con el evento nuevo — porque no hay forma barata de
// saber cuál quedó desactualizada sin recalcularla.
func TestReasoningCache_EventLogCrecidoInvalidaTodo(t *testing.T) {
	log := &memoryintelligence.EventLog{}
	store := memoryintelligence.NewEventStore(log)
	c := NewReasoningCache(store, time.Minute)

	c.Store("a", NewReasoningContext("a"))
	c.Store("b", NewReasoningContext("b"))

	// Confirmar que ambas están cacheadas antes de que el log crezca.
	if _, hit := c.Get("a"); !hit {
		t.Fatal("esperaba que 'a' fuera un hit antes de que el EventLog creciera")
	}

	log.Events = append(log.Events, memoryintelligence.RawEvent{Action: "abrir_programa", Target: "blender"})

	if _, hit := c.Get("a"); hit {
		t.Error("'a' debería invalidarse cuando el EventLog crece, aunque el evento no tenga nada que ver con 'a'")
	}
	if _, hit := c.Get("b"); hit {
		t.Error("'b' también debería invalidarse — el crecimiento del EventLog vacía TODO el cache")
	}
}

// TestReasoningCache_SinEventLogNoRompe confirma que un EventLog nil
// (mismo criterio defensivo que el resto del proyecto) no hace panic —
// checkGrowth() lo trata como longitud 0 y simplemente nunca dispara
// la invalidación por crecimiento.
func TestReasoningCache_SinEventLogNoRompe(t *testing.T) {
	c := NewReasoningCache(nil, time.Minute)

	c.Store("x", NewReasoningContext("x"))
	if _, hit := c.Get("x"); !hit {
		t.Fatal("sin EventLog, una entrada recién guardada debería seguir siendo un hit")
	}
}

// TestReasoningCache_Invalidate confirma que Invalidate() vacía el
// cache a mano, sin esperar a TTL ni a que el EventLog crezca.
func TestReasoningCache_Invalidate(t *testing.T) {
	c := NewReasoningCache(nil, time.Minute)
	c.Store("x", NewReasoningContext("x"))

	c.Invalidate()

	if _, hit := c.Get("x"); hit {
		t.Fatal("después de Invalidate(), la entrada no debería seguir siendo un hit")
	}
}

// TestReasoningCache_Stats confirma que Hits/Misses/Size se llevan la
// cuenta correctamente a través de una secuencia realista de uso.
func TestReasoningCache_Stats(t *testing.T) {
	c := NewReasoningCache(nil, time.Minute)

	c.Get("x") // miss
	c.Store("x", NewReasoningContext("x"))
	c.Get("x") // hit
	c.Get("x") // hit
	c.Get("y") // miss

	stats := c.Stats()
	if stats.Hits != 2 {
		t.Errorf("esperaba Hits=2, obtuve %d", stats.Hits)
	}
	if stats.Misses != 2 {
		t.Errorf("esperaba Misses=2, obtuve %d", stats.Misses)
	}
	if stats.Size != 1 {
		t.Errorf("esperaba Size=1, obtuve %d", stats.Size)
	}
}
