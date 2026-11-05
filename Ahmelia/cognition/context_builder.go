package cognition

import (
	"strings"
	"time"

	"ahmelia/comprehension"
	"ahmelia/environment"
	"ahmelia/memory"
	memoryintelligence "ahmelia/memory_intelligence"
	"ahmelia/planning"
	"ahmelia/reasoning_deep"
)

// ============================================================================
// CONTEXT_BUILDER.GO — Bloque 9, Fase 2.6 (Integración Cognitiva)
// ============================================================================
//
// Hasta acá cada módulo de Fase 2.5 (World Model, Timeline, Habits,
// Experience, Insights, Reflection, Learning, Mental State, Conversation
// Context) se consultaba por separado, cada uno con su propio ParseXQuery
// en core/engine.go — el usuario tenía que preguntar por partes ("qué
// hicimos", "qué aprendiste", "cómo te va") y nunca había un único punto
// que los juntara.
//
// Context Builder es ese punto. No reemplaza a ninguno de los módulos —
// los consulta a todos, en un solo lugar, y arma un UnifiedContext. Esto
// es exactamente lo que propuso el roadmap (documento "FASE 2.5/2.8"):
// el "director de orquesta" que se para ANTES del Reasoner, para que este
// no tenga que ir a buscar diez fuentes distintas cada vez que necesita
// decidir algo.
//
//   Usuario → Context Builder → Reasoner → Respuesta
//
// Diseño: Builder guarda REFERENCIAS a lo que ya vive en core/engine.go
// (World, EventLog, LongMem, Agenda, Mental, Convo, Learning) — no
// duplica ningún estado, no re-implementa ninguna fuente. Solo orquesta.

// UnifiedContext es el contexto consolidado de un instante dado. Cada
// campo es el resumen que YA sabe producir su módulo de origen — Context
// Builder no interpreta nada nuevo, solo los junta. Campos vacíos
// significan "esa fuente no tenía nada que decir ahora", no un error.
type UnifiedContext struct {
	BuiltAt time.Time

	World        string // environment.Manager.Describe() — qué está abierto AHORA
	Timeline     string // memory_intelligence.DescribeTimeline() — episodios recientes
	Habits       string // memory_intelligence.DescribeHabits()
	Insights     string // memory_intelligence.DescribeInsights()
	Reflection   string // memory_intelligence.DescribeReflections() — cómo le está yendo
	Learning     string // memory_intelligence.LearningState.DescribeLearning() — qué adaptó
	Goals        string // planning.AgendaManager.GetQuickStatus()
	MentalState  string // reasoning_deep.MentalAgendaV2.GetQuickMentalState()
	Conversation string // comprehension.ConversationContext.Summary()
}

// IsEmpty es true si ninguna fuente tenía nada que aportar — típico en la
// primera sesión, antes de que haya EventLog/episodios acumulados.
func (uc *UnifiedContext) IsEmpty() bool {
	return uc.World == "" && uc.Timeline == "" && uc.Habits == "" &&
		uc.Insights == "" && uc.Reflection == "" && uc.Learning == "" &&
		uc.Goals == "" && uc.MentalState == "" && uc.Conversation == ""
}

// Narrative arma el resumen legible de TODO el contexto en un solo texto
// — la respuesta a "dame el panorama completo" / "contexto completo".
// Solo incluye las secciones que tienen contenido real: si Habits está
// vacío (todavía no hay 3+ días de patrón), no aparece "Hábitos: " seguido
// de nada — eso se leería como que Ahmelia no entendió, no como que no
// hay dato.
func (uc *UnifiedContext) Narrative() string {
	if uc.IsEmpty() {
		return "Todavía no tengo contexto acumulado — esto se va llenando a medida que uses Ahmelia."
	}

	var parts []string
	add := func(label, content string) {
		if content != "" {
			parts = append(parts, label+": "+content)
		}
	}

	add("Ahora mismo", uc.World)
	add("Conversación", uc.Conversation)
	add("Objetivos", uc.Goals)
	add("Estado", uc.MentalState)
	add("Cronología reciente", uc.Timeline)
	add("Hábitos", uc.Habits)
	add("Aprendizajes", uc.Learning)
	add("Reflexión", uc.Reflection)
	add("Insights", uc.Insights)

	return strings.Join(parts, ". ")
}

// Builder es el director de orquesta: sostiene referencias a las fuentes
// ya vivas en el Engine y las consulta bajo demanda. Ningún campo es
// dueño de su propio estado — todos apuntan a lo que ya existe.
//
// B93.3: EventLog y Learning pasaron a ser *EventStore/*LearningStore —
// dueño único compartido con la goroutine del ciclo autónomo. Build() ya
// no toca ningún slice/mapa directo, solo llama a los métodos del Store
// (que toman su propio lock).
type Builder struct {
	World      *environment.Manager
	EventStore *memoryintelligence.EventStore
	LongMem    *memory.LongMemory
	Agenda     *planning.AgendaManager
	Mental     *reasoning_deep.MentalAgendaV2
	Convo      *comprehension.ConversationContext
	Learning   *memoryintelligence.LearningStore
}

// NewBuilder arma el Context Builder a partir de lo que el Engine ya
// tiene instanciado — llamar UNA vez en NewEngine(), igual que las demás
// fuentes.
func NewBuilder(
	world *environment.Manager,
	eventStore *memoryintelligence.EventStore,
	longMem *memory.LongMemory,
	agenda *planning.AgendaManager,
	mental *reasoning_deep.MentalAgendaV2,
	convo *comprehension.ConversationContext,
	learning *memoryintelligence.LearningStore,
) *Builder {
	return &Builder{
		World:      world,
		EventStore: eventStore,
		LongMem:    longMem,
		Agenda:     agenda,
		Mental:     mental,
		Convo:      convo,
		Learning:   learning,
	}
}

// Build consulta todas las fuentes y arma el UnifiedContext del instante
// actual. Cada fuente se consulta de forma defensiva — que una falle o
// esté vacía (ej. World.Describe() sin nada abierto todavía) no debe
// tirar abajo a las demás.
func (b *Builder) Build() *UnifiedContext {
	uc := &UnifiedContext{BuiltAt: time.Now()}

	if b.World != nil {
		if desc, ok := b.World.Describe(); ok {
			uc.World = desc
		}
	}

	if b.LongMem != nil {
		// Últimas 24hs como ventana por defecto — mismo criterio que
		// ParseTimelineQuery usa para "hoy".
		from := time.Now().AddDate(0, 0, -1)
		to := time.Now()
		episodes := memoryintelligence.EpisodesInRange(b.LongMem, from, to)
		if len(episodes) > 0 {
			uc.Timeline = memoryintelligence.DescribeTimeline(episodes, "hoy")
		}
	}

	if b.EventStore != nil {
		habits := b.EventStore.DetectHabits(memoryintelligence.DefaultPatternThreshold, memoryintelligence.DefaultMinHabitDays)
		if len(habits) > 0 {
			uc.Habits = memoryintelligence.DescribeHabits(habits)
		}
		if insights := b.EventStore.GenerateInsights(); len(insights) > 0 {
			uc.Insights = memoryintelligence.DescribeInsights(insights)
		}
		if reflections := b.EventStore.GenerateReflections(); len(reflections) > 0 {
			uc.Reflection = memoryintelligence.DescribeReflections(reflections)
		}
	}

	if b.Learning != nil {
		uc.Learning = b.Learning.DescribeLearning()
	}

	if b.Agenda != nil {
		uc.Goals = b.Agenda.GetQuickStatus()
	}

	if b.Mental != nil {
		uc.MentalState = b.Mental.GetQuickMentalState()
	}

	if b.Convo != nil {
		uc.Conversation = b.Convo.Summary(3)
	}

	return uc
}

// contextTriggers detecta si el usuario está pidiendo el panorama
// COMPLETO (todas las fuentes juntas), no una de ellas en particular —
// mismo criterio que learningTriggers/ParseReflectionQuery: frases lo
// bastante específicas como para no competir con preguntas puntuales ya
// resueltas por Timeline/Habits/Insights/Reflection/Learning por
// separado.
var contextTriggers = []string{
	"contexto completo",
	"panorama completo",
	"panorama general",
	"resumen completo",
	"resumen general",
	"contexto unificado",
	"dame el contexto",
	"que sabes de todo esto",
	"que esta pasando en general",
	"ponme al dia de todo",
}

// ParseContextQuery detecta si la pregunta pide el panorama completo.
func ParseContextQuery(normalizedText string) bool {
	for _, trigger := range contextTriggers {
		if strings.Contains(normalizedText, trigger) {
			return true
		}
	}
	return false
}
