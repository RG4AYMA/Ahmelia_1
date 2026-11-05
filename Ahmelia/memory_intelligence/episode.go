package memory_intelligence

import (
	"fmt"
	"strings"
	"time"

	"ahmelia/environment"
	"ahmelia/memory"
)

// ============================================================================
// EPISODE.GO — memoria episódica (Nivel 1 + parte real del Nivel 3)
// ============================================================================
//
// Por qué esto no podía ser solo "usar lo que ya existe": ya había un
// volcado de sesión (learning/experience.go: DumpContextToMemory), pero
// tiene dos límites reales que lo hacen insuficiente para un "episodio":
//
//  1. Solo guarda las últimas 10 interacciones (mismo límite que
//     comprehension.ConversationContext, porque lee de ahí) — ese límite
//     tiene sentido para resolución de referencias inmediatas ("ábrelo"),
//     pero NO alcanza para resumir una sesión de trabajo completa.
//  2. Guarda cada interacción como su propia entrada con
//     key = action+"_"+target. Eso significa que "abre unity" en dos
//     sesiones distintas usa la MISMA key — la segunda sesión sobre-
//     escribe silenciosamente el recuerdo de la primera.
//
// Episode no reemplaza ese volcado (sigue sirviendo para lo que servía:
// contexto reciente al reiniciar). Esto es una capa nueva, en paralelo:
// acumula TODA la sesión (sin límite de 10) y al final la resume en UNA
// sola entrada con key única por sesión (timestamp de inicio), así que
// las sesiones no se pisan entre sí.
//
// Los números que reporta son solo los que de verdad se pueden contar
// con datos que existen: duración real, cantidad de comandos, apps/
// targets distintos tocados, errores. NO reporta "% de avance" ni
// "archivos modificados" — no hay ninguna fuente de datos detrás de esas
// dos cifras hoy; incluirlas sería inventar un número, no medirlo.

// EpisodeEntry es una acción ejecutada durante la sesión actual.
type EpisodeEntry struct {
	Action    string
	Target    string
	Success   bool
	Timestamp time.Time
}

// Episode acumula la sesión completa, desde que arranca Ahmelia hasta
// que se cierra. No persiste nada por sí solo — Summarize() es lo único
// que escribe a memory.LongMemory, y solo al final.
type Episode struct {
	Start   time.Time
	entries []EpisodeEntry
}

// NewEpisode arranca un episodio nuevo. Se crea una vez por ejecución de
// Ahmelia (en NewEngine), no una vez por comando.
func NewEpisode() *Episode {
	return &Episode{Start: time.Now()}
}

// Record agrega una acción ejecutada con éxito o no. Se llama desde el
// mismo defer de core/engine.go que ya llama a memory.RecordUsage —
// mismo momento, propósito distinto: RecordUsage guarda CADA comando
// para poder recordarlo por nombre después; Record acumula la sesión
// completa para poder resumirla al cerrar.
func (e *Episode) Record(action, target string, success bool) {
	if action == "" || action == "desconocido" {
		return
	}
	e.entries = append(e.entries, EpisodeEntry{
		Action:    action,
		Target:    target,
		Success:   success,
		Timestamp: time.Now(),
	})
}

// Summarize arma un resumen legible de la sesión y lo persiste como una
// única entrada KnowContext con key única (episodio_<timestamp de
// inicio>), evitando el problema de sobreescritura que tiene
// DumpContextToMemory. world es opcional (puede ir nil) — si hay una
// app activa con Project conocido (VSCode vía title_parser.go, por
// ejemplo), se menciona el proyecto — es el Nivel 2 (objetivo/tema de
// la sesión), usando un dato que YA existe, no uno nuevo.
func (e *Episode) Summarize(long *memory.LongMemory, world *environment.Manager) (string, bool) {
	if len(e.entries) == 0 {
		return "", false
	}

	duration := time.Since(e.Start)
	errors := 0
	seenTargets := make(map[string]bool)
	var targets []string
	for _, entry := range e.entries {
		if !entry.Success {
			errors++
		}
		if entry.Target != "" && !seenTargets[entry.Target] {
			seenTargets[entry.Target] = true
			targets = append(targets, entry.Target)
		}
	}

	summary := fmt.Sprintf("Sesión de %s: %d comando(s)", formatDuration(duration), len(e.entries))
	if len(targets) > 0 {
		shown := targets
		if len(shown) > 6 {
			shown = shown[:6]
		}
		summary += ", tocaste: " + strings.Join(shown, ", ")
	}
	if errors > 0 {
		summary += fmt.Sprintf(", %d con error", errors)
	}
	if world != nil {
		if app, ok := world.ActiveApp(); ok && app.Project != "" {
			summary += fmt.Sprintf(". Trabajaste en el proyecto %s", app.Project)
		}
	}

	key := fmt.Sprintf("episodio_%d", e.Start.Unix())
	long.Data[key] = &memory.Knowledge{
		ID:         key,
		Type:       memory.KnowContext,
		Input:      summary,
		Importance: 0.4,
		Source:     memory.SourceLearned,
		CreatedAt:  e.Start,
		LastUsed:   time.Now(),
		Tags:       []string{"episodio"},
	}
	long.Save()

	return summary, true
}

// formatDuration da una duración legible ("15 minutos", "2 horas 10
// minutos") en vez de un time.Duration crudo.
func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return "menos de un minuto"
	}
	mins := int(d.Minutes())
	if mins < 60 {
		return fmt.Sprintf("%d minutos", mins)
	}
	hours := mins / 60
	remaining := mins % 60
	if remaining == 0 {
		return fmt.Sprintf("%d horas", hours)
	}
	return fmt.Sprintf("%d horas %d minutos", hours, remaining)
}
