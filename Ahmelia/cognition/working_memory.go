package cognition

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// ============================================================================
// WORKING_MEMORY.GO — Bloque 13, Fase 2.6 (Integración Cognitiva)
// ============================================================================
//
// El roadmap (sección "4. Working Memory") lo describe con un ejemplo
// concreto:
//
//   Estamos hablando de Unity. Hace cinco minutos hablamos del
//   inventario. Ahora dice: "Arréglalo." Sabe qué significa.
//
// Hasta este bloque, la cadena real era:
//
//   Context Builder → Attention Manager → MentalAgendaV2.Observe()
//
// Es decir: Attention decidía qué importaba AHORA (Bloque 12), pero lo
// que "importaba hace un rato" se perdía apenas Attend() se volvía a
// llamar en la siguiente pregunta — nada retenía esas señales entre
// llamadas. MentalAgendaV2 acumula observaciones, pero no es memoria de
// trabajo: no tiene noción de "esto ya venció" ni de
// "esto ya lo dijimos, actualizá en vez de duplicar".
//
// Working Memory se inserta EXACTAMENTE donde el roadmap lo pide:
//
//   Context Builder → Attention Manager → Working Memory → MentalAgendaV2
//
// No reemplaza a Attention (que decide qué importa) ni a MentalAgenda
// (que es la agenda deliberativa de la Capa 5.5) — es la pieza que
// faltaba entre las dos: retiene lo que se atendió, con antigüedad
// propia, para que "hace cinco minutos hablamos de X" sea una pregunta
// respondible y no algo que ya se perdió.

// Item es una señal atendida (ver attention.go) que Working Memory
// retiene, con cuándo entró y cuándo se actualizó por última vez —
// distinto de StageEvent (Bloque 11): Item vive mientras no venza, no es
// un log de auditoría de todo lo que pasó.
type Item struct {
	Label     string
	Content   string
	Score     float64
	StoredAt  time.Time // primera vez que se registró esta Label
	UpdatedAt time.Time // última vez que se refrescó (misma Label, contenido nuevo o repetido)
}

// Age devuelve hace cuánto se actualizó este ítem por última vez — lo
// que importa para "hace cinco minutos hablamos de X" es la última vez
// que fue relevante, no la primera.
func (it Item) Age() time.Duration {
	return time.Since(it.UpdatedAt)
}

// DefaultWorkingMemoryTTL — cuánto tiempo sigue siendo "reciente" algo
// que se atendió. El propio ejemplo del roadmap usa "hace cinco
// minutos" como referencia todavía resolvible, así que el TTL tiene que
// cubrir eso — deliberadamente mucho más largo que el TTL del Context
// Cache (Bloque 10, 15s: esa es la red de seguridad de "no reconstruir
// dos veces lo mismo", esto es "seguir recordando de qué hablamos").
const DefaultWorkingMemoryTTL = 5 * time.Minute

// DefaultWorkingMemoryCapacity — tope de ítems retenidos simultáneamente.
// Igual que Pipeline.maxHistory (Bloque 11): memoria de trabajo acotada
// a propósito, no un log que crece sin límite.
const DefaultWorkingMemoryCapacity = 8

// WorkingMemory retiene las últimas señales atendidas, cada una con su
// propia antigüedad, y las va venciendo solas por TTL — sin que nadie
// tenga que llamar a un Invalidate() explícito (a diferencia del
// Context Cache, acá "venterse" es el comportamiento normal, no una
// excepción).
type WorkingMemory struct {
	mu       sync.Mutex
	items    []Item
	ttl      time.Duration
	capacity int
}

// NewWorkingMemory arma la memoria de trabajo con los defaults del
// roadmap. Igual que el resto de Fase 2.6, se instancia UNA vez en
// NewEngine() y vive todo el proceso.
func NewWorkingMemory() *WorkingMemory {
	return &WorkingMemory{ttl: DefaultWorkingMemoryTTL, capacity: DefaultWorkingMemoryCapacity}
}

// prune (se llama con el lock ya tomado) saca los ítems vencidos por
// TTL. Barato: recorre una lista acotada a capacity elementos.
func (w *WorkingMemory) prune(now time.Time) {
	var alive []Item
	for _, it := range w.items {
		if now.Sub(it.UpdatedAt) <= w.ttl {
			alive = append(alive, it)
		}
	}
	w.items = alive
}

// Remember recibe las señales que Attention Manager decidió atender
// (Attend().attended, ver attention.go) y las incorpora a la memoria de
// trabajo. Si una Label ya estaba (ej. "Conversación" reaparece en la
// siguiente pregunta), se ACTUALIZA in-place — UpdatedAt se refresca,
// StoredAt no — en vez de duplicarse. Esto es justo lo que MentalAgenda
// no hacía: cada Observe() anterior era una entrada más, sin noción de
// "esto ya lo sabíamos, seguí siendo cierto".
func (w *WorkingMemory) Remember(signals []Signal) {
	if len(signals) == 0 {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()

	now := time.Now()
	w.prune(now)

	for _, s := range signals {
		updated := false
		for i := range w.items {
			if w.items[i].Label == s.Label {
				w.items[i].Content = s.Content
				w.items[i].Score = s.Score
				w.items[i].UpdatedAt = now
				updated = true
				break
			}
		}
		if !updated {
			w.items = append(w.items, Item{
				Label: s.Label, Content: s.Content, Score: s.Score,
				StoredAt: now, UpdatedAt: now,
			})
		}
	}

	// Tope de capacidad: si se pasó, se descartan los ítems MÁS
	// VIEJOS por UpdatedAt (menos recientes primero) — no los de menor
	// Score, porque acá lo que importa es antigüedad, no relevancia
	// (eso ya lo decidió Attention antes de llegar acá).
	if len(w.items) > w.capacity {
		w.sortByRecencyDesc()
		w.items = w.items[:w.capacity]
	}
}

// sortByRecencyDesc (con el lock ya tomado) ordena por UpdatedAt, más
// reciente primero — insertion sort simple, la lista nunca pasa de
// capacity+len(signals) elementos, no hace falta más que esto.
func (w *WorkingMemory) sortByRecencyDesc() {
	for i := 1; i < len(w.items); i++ {
		for j := i; j > 0 && w.items[j].UpdatedAt.After(w.items[j-1].UpdatedAt); j-- {
			w.items[j], w.items[j-1] = w.items[j-1], w.items[j]
		}
	}
}

// Active devuelve los ítems vigentes (no vencidos), del más reciente al
// más viejo. Vencer es lazy: se recalcula acá, no hace falta un
// goroutine de limpieza en background.
func (w *WorkingMemory) Active() []Item {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.prune(time.Now())
	w.sortByRecencyDesc()

	out := make([]Item, len(w.items))
	copy(out, w.items)
	return out
}

// MostRecent devuelve el ítem más recientemente actualizado, si hay
// alguno vigente — la base para resolver "arréglalo" contra "de qué
// veníamos hablando", el ejemplo central del roadmap. No reemplaza a
// reference.Resolver (que resuelve pronombres contra APLICACIONES
// específicas) — esto es el equivalente para TEMAS/contexto general.
func (w *WorkingMemory) MostRecent() (Item, bool) {
	active := w.Active()
	if len(active) == 0 {
		return Item{}, false
	}
	return active[0], true
}

// Narrative arma la respuesta a "¿de qué hablábamos?" / "¿qué
// recordás?": cada ítem vigente con hace cuánto se actualizó, más
// reciente primero.
func (w *WorkingMemory) Narrative() string {
	active := w.Active()
	if len(active) == 0 {
		return "No tengo nada reciente en memoria de trabajo — se habrá vencido, o todavía no hablamos de nada puntual."
	}
	parts := make([]string, len(active))
	for i, it := range active {
		parts[i] = fmt.Sprintf("%s (hace %s): %s", it.Label, formatAge(it.Age()), it.Content)
	}
	return strings.Join(parts, ". ")
}

// formatAge da una antigüedad legible en español, redondeada a la
// unidad que tenga sentido — mismo criterio de "legible antes que
// preciso" que ya usa Timeline (memory_intelligence/timeline.go) para
// "hoy"/"ayer"/"hace dos semanas".
func formatAge(d time.Duration) string {
	switch {
	case d < time.Minute:
		return "un momento"
	case d < 2*time.Minute:
		return "un minuto"
	case d < time.Hour:
		return fmt.Sprintf("%d minutos", int(d.Minutes()))
	default:
		return fmt.Sprintf("%d horas", int(d.Hours()))
	}
}

// workingMemoryTriggers detecta preguntas sobre lo que se habló
// recientemente — distinto de "contexto completo" (Bloque 9, TODO sin
// filtrar) y de "qué te importa ahora" (Bloque 12, el filtro de
// atención de ESTE instante). Esto pregunta por la ventana de tiempo
// reciente, con antigüedad explícita.
var workingMemoryTriggers = []string{
	"de que hablabamos",
	"de que estabamos hablando",
	"que dijimos hace un rato",
	"que hablamos hace un momento",
	"recuerdas de que hablamos",
	"que tenias en mente",
	"que era lo que decias",
	"que estabamos haciendo",
	"que haciamos",
	"que estabamos trabajando",
	"que era lo que estabamos haciendo",
	"que era lo que haciamos",
	"what were we doing",
	"what were we working on",
}

// ParseWorkingMemoryQuery detecta si la pregunta pide el contenido de
// la memoria de trabajo reciente.
func ParseWorkingMemoryQuery(normalizedText string) bool {
	for _, trigger := range workingMemoryTriggers {
		if strings.Contains(normalizedText, trigger) {
			return true
		}
	}
	return false
}
