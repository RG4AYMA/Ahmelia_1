package cognition

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// ============================================================================
// PIPELINE.GO — Bloque 11, Fase 2.6 (Integración Cognitiva)
// ============================================================================
//
// El roadmap (documento "FASE 2.5/2.8", sección "2. Cognitive Pipeline")
// describe ocho etapas por las que debería pasar cada input:
//
//   Escuchar → Comprender → Construir contexto → Razonar →
//   Elegir estrategia → Ejecutar → Aprender → Guardar experiencia
//
// Esas ocho etapas YA existen en core/engine.go — Process() escucha y
// comprende (classifier + intent), consulta el Context Cache (Bloque 10),
// razona vía knowledgeEngine, y execute() elige estrategia (Bloque 7/8,
// AdaptStrategy), ejecuta, y en su defer aprende (learningState) y guarda
// experiencia (episode/eventLog). El problema, igual que con Timeline o
// Goals antes de que alguien los cableara: estaba todo repartido, sin un
// solo lugar que dijera "en qué etapa está Ahmelia ahora" ni un registro
// de por dónde pasó cada input.
//
// Pipeline no reemplaza ese flujo ni le agrega pasos nuevos — es un
// observador liviano: core/engine.go marca en qué etapa entra en cada
// punto de decisión que ya existía, y Pipeline guarda el trace. Mismo
// principio que Context Builder con Fase 2.5: no reimplementa nada, solo
// hace visible lo que ya estaba pasando.

// Stage es una de las ocho etapas del pipeline cognitivo, tal como las
// nombra el roadmap (en español, porque así es como Ahmelia las explica).
type Stage string

const (
	StageListen     Stage = "Escuchar"
	StageComprehend Stage = "Comprender"
	StageContext    Stage = "Construir contexto"
	StageReason     Stage = "Razonar"
	StageStrategy   Stage = "Elegir estrategia"
	StageExecute    Stage = "Ejecutar"
	StageLearn      Stage = "Aprender"
	StageExperience Stage = "Guardar experiencia"
)

// StageEvent es un paso registrado: qué etapa, cuándo, y un detalle corto
// (qué input, qué acción) para que el trace sea legible y no solo una
// lista de nombres repetidos.
type StageEvent struct {
	Stage  Stage
	At     time.Time
	Detail string
}

// Pipeline acumula el trace de etapas de la sesión completa, acotado a
// maxHistory para no crecer sin límite en sesiones largas — mismo
// criterio que ActionHistory (selfknowledge/action_history.go): importa
// lo reciente, no todo desde el arranque.
type Pipeline struct {
	mu         sync.Mutex
	history    []StageEvent
	maxHistory int
}

// DefaultPipelineHistory acota el trace a las últimas ~6 pasadas
// completas (8 etapas c/u, cuando el flujo las recorre todas) sin gastar
// memoria de más.
const DefaultPipelineHistory = 50

// NewPipeline arranca un Pipeline vacío — llamar UNA vez en NewEngine(),
// igual que Context Builder/Cache.
func NewPipeline() *Pipeline {
	return &Pipeline{maxHistory: DefaultPipelineHistory}
}

// Enter registra que el Engine entró a una etapa. Barato a propósito
// (un append + un recorte) porque se llama en el camino caliente de
// Process()/execute(), no solo cuando alguien pregunta por el pipeline.
func (p *Pipeline) Enter(stage Stage, detail string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.history = append(p.history, StageEvent{Stage: stage, At: time.Now(), Detail: detail})
	if len(p.history) > p.maxHistory {
		p.history = p.history[len(p.history)-p.maxHistory:]
	}
}

// Current devuelve la última etapa registrada, o "" si el pipeline nunca
// se usó todavía (sesión recién arrancada, sin ningún input procesado).
func (p *Pipeline) Current() Stage {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.history) == 0 {
		return ""
	}
	return p.history[len(p.history)-1].Stage
}

// Trace devuelve los últimos n eventos registrados (todas las etapas,
// de todas las pasadas), del más viejo al más nuevo. n<=0 devuelve todo
// el historial disponible.
func (p *Pipeline) Trace(n int) []StageEvent {
	p.mu.Lock()
	defer p.mu.Unlock()
	if n <= 0 || n > len(p.history) {
		n = len(p.history)
	}
	out := make([]StageEvent, n)
	copy(out, p.history[len(p.history)-n:])
	return out
}

// LastPass devuelve solo las etapas de la pasada MÁS RECIENTE — desde el
// último StageListen registrado hasta el final del historial. Un input
// no siempre recorre las ocho etapas (una corrección o un saludo se
// resuelven en dos o tres), así que esto es "lo que de verdad pasó con
// el último input", no una plantilla fija de ocho pasos.
func (p *Pipeline) LastPass() []StageEvent {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.history) == 0 {
		return nil
	}
	start := 0
	for i := len(p.history) - 1; i >= 0; i-- {
		if p.history[i].Stage == StageListen {
			start = i
			break
		}
	}
	out := make([]StageEvent, len(p.history)-start)
	copy(out, p.history[start:])
	return out
}

// Narrative arma la respuesta a "¿en qué etapa estás?" / "¿cómo procesas
// lo que digo?" — la pasada más reciente, como una cadena de flechas, más
// el detalle de la etapa actual.
func (p *Pipeline) Narrative() string {
	pass := p.LastPass()
	if len(pass) == 0 {
		return "Todavía no procesé nada en esta sesión — el pipeline cognitivo arranca con la primera frase que me digas."
	}

	names := make([]string, len(pass))
	for i, ev := range pass {
		names[i] = string(ev.Stage)
	}

	last := pass[len(pass)-1]
	detail := ""
	if last.Detail != "" {
		detail = fmt.Sprintf(" (%s)", last.Detail)
	}

	return fmt.Sprintf(
		"Mi última pasada por el pipeline cognitivo fue: %s. Ahora mismo estoy en \"%s\"%s.",
		strings.Join(names, " → "), last.Stage, detail,
	)
}

// pipelineTriggers detecta preguntas sobre el pipeline cognitivo en sí
// (metacognición sobre el PROCESO, no sobre un resultado) — distinto de
// "estado mental" (Capa 5.5, MentalAgendaV2), que es sobre foco/dudas/
// preocupaciones, no sobre por qué etapas pasó el procesamiento.
var pipelineTriggers = []string{
	"pipeline cognitivo",
	"en que etapa estas",
	"en que etapa vas",
	"que etapa del pipeline",
	"como procesas lo que digo",
	"como piensas paso a paso",
	"muestrame el pipeline",
	"como funciona tu pipeline",
	"que hiciste paso a paso",
}

// ParsePipelineQuery detecta si la pregunta pide ver el trace del
// pipeline cognitivo.
func ParsePipelineQuery(normalizedText string) bool {
	for _, trigger := range pipelineTriggers {
		if strings.Contains(normalizedText, trigger) {
			return true
		}
	}
	return false
}
