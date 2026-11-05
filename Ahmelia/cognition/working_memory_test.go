package cognition

import (
	"strings"
	"testing"
	"time"
)

// TestRemember_GuardaYEsRecuperable confirma el caso base: una señal
// atendida queda disponible en Active() y como MostRecent().
func TestRemember_GuardaYEsRecuperable(t *testing.T) {
	w := NewWorkingMemory()
	w.Remember([]Signal{{Label: "Conversación", Content: "hablando de Unity", Score: 0.7}})

	active := w.Active()
	if len(active) != 1 || active[0].Content != "hablando de Unity" {
		t.Fatalf("Active() = %+v, esperaba 1 ítem con el contenido guardado", active)
	}

	recent, ok := w.MostRecent()
	if !ok || recent.Label != "Conversación" {
		t.Fatalf("MostRecent() = %+v, ok=%v, esperaba la señal recién guardada", recent, ok)
	}
}

// TestRemember_ActualizaEnVezDeDuplicar confirma la diferencia central
// con el Observe() crudo de MentalAgenda que reemplaza: la misma Label
// que reaparece actualiza el ítem existente, no crea uno nuevo.
func TestRemember_ActualizaEnVezDeDuplicar(t *testing.T) {
	w := NewWorkingMemory()
	w.Remember([]Signal{{Label: "Conversación", Content: "hablando de Unity", Score: 0.7}})
	w.Remember([]Signal{{Label: "Conversación", Content: "hablando del inventario", Score: 0.75}})

	active := w.Active()
	if len(active) != 1 {
		t.Fatalf("Active() tiene %d ítems, esperaba 1 (misma Label debe actualizar, no duplicar)", len(active))
	}
	if active[0].Content != "hablando del inventario" {
		t.Fatalf("Active()[0].Content = %q, esperaba el contenido más reciente", active[0].Content)
	}
}

// TestActive_DescartaVencidos confirma que un ítem con TTL corto deja de
// aparecer en Active() una vez pasado el tiempo — el comportamiento
// central de "memoria temporal" que distingue esto de una simple lista.
func TestActive_DescartaVencidos(t *testing.T) {
	w := NewWorkingMemory()
	w.ttl = 10 * time.Millisecond
	w.Remember([]Signal{{Label: "Conversación", Content: "hablando de Unity", Score: 0.7}})

	time.Sleep(20 * time.Millisecond)

	if active := w.Active(); len(active) != 0 {
		t.Fatalf("Active() después del TTL = %+v, esperaba lista vacía (vencido)", active)
	}
	if _, ok := w.MostRecent(); ok {
		t.Fatal("MostRecent() después del TTL debería devolver ok=false")
	}
}

// TestRemember_RespetaCapacidadDescartandoLoMasViejo confirma el tope
// de capacidad: al superarlo, se descartan los ítems menos recientes,
// no los de menor score (eso ya lo decidió Attention antes).
func TestRemember_RespetaCapacidadDescartandoLoMasViejo(t *testing.T) {
	w := NewWorkingMemory()
	w.capacity = 2

	w.Remember([]Signal{{Label: "A", Content: "primero", Score: 0.9}})
	time.Sleep(2 * time.Millisecond)
	w.Remember([]Signal{{Label: "B", Content: "segundo", Score: 0.5}})
	time.Sleep(2 * time.Millisecond)
	w.Remember([]Signal{{Label: "C", Content: "tercero", Score: 0.5}})

	active := w.Active()
	if len(active) != 2 {
		t.Fatalf("Active() tiene %d ítems, esperaba 2 (capacity=2)", len(active))
	}
	for _, it := range active {
		if it.Label == "A" {
			t.Fatalf("Label 'A' (la más vieja) debería haber sido descartada, quedó: %+v", active)
		}
	}
}

// TestNarrative_IncluyeAntiguedad confirma que Narrative() menciona hace
// cuánto se actualizó cada ítem, no solo el contenido — es la diferencia
// entre esto y Context Builder.Narrative().
func TestNarrative_IncluyeAntiguedad(t *testing.T) {
	w := NewWorkingMemory()
	w.Remember([]Signal{{Label: "Conversación", Content: "hablando de Unity", Score: 0.7}})

	n := w.Narrative()
	if !strings.Contains(n, "hablando de Unity") || !strings.Contains(n, "hace") {
		t.Fatalf("Narrative() = %q, esperaba contenido + referencia de antigüedad ('hace ...')", n)
	}
}

// TestNarrative_VacioEsExplicito confirma el mensaje cuando no hay nada
// vigente — mismo criterio que UnifiedContext/Pipeline.
func TestNarrative_VacioEsExplicito(t *testing.T) {
	w := NewWorkingMemory()
	n := w.Narrative()
	if !strings.Contains(n, "No tengo nada reciente") {
		t.Fatalf("Narrative() vacío = %q, esperaba mensaje explícito", n)
	}
}

// TestParseWorkingMemoryQuery_DetectaVariantesSinPisarOtrosTriggers
// confirma el trigger y que no choca con Attention (Bloque 12) o
// Context (Bloque 9).
func TestParseWorkingMemoryQuery_DetectaVariantesSinPisarOtrosTriggers(t *testing.T) {
	positive := []string{
		"de que hablabamos",
		"de que estabamos hablando",
		"recuerdas de que hablamos",
	}
	for _, q := range positive {
		if !ParseWorkingMemoryQuery(q) {
			t.Errorf("ParseWorkingMemoryQuery(%q) = false, esperaba true", q)
		}
	}

	negative := []string{
		"que es importante ahora",
		"contexto completo",
		"abre chrome",
	}
	for _, q := range negative {
		if ParseWorkingMemoryQuery(q) {
			t.Errorf("ParseWorkingMemoryQuery(%q) = true, esperaba false (choca con otro trigger)", q)
		}
	}
}

func TestParseWorkingMemoryQuery_CurrentContextVariants(t *testing.T) {
	for _, q := range []string{
		"que estabamos haciendo antes",
		"que era lo que estabamos haciendo antes",
		"que haciamos antes",
	} {
		if !ParseWorkingMemoryQuery(q) {
			t.Fatalf("ParseWorkingMemoryQuery(%q) = false", q)
		}
	}
}
