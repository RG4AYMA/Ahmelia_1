package cognition

import (
	"strings"

	"ahmelia/reasoning"
)

// ============================================================================
// CONVERSATION_MODE.GO — Bloque 16, Fase 2.8 (arranca la fase, no reconecta nada)
// ============================================================================
//
// Decisión de arquitectura acordada antes de escribir esto: Identidad,
// Modo Conversacional y Perfil de Interacción son tres conceptos
// distintos que no se mezclan.
//
//   Identidad          → Compañera (siempre, no cambia acá)
//   Modo Conversacional → Task/Discussion/Reflection/Learning/
//                         Brainstorming/Emotional (este bloque)
//   Perfil de Interacción → Privado/Público/... (Security Profile,
//                         bloque futuro — a propósito no existe todavía)
//
// El Modo Conversacional nace ANTES que el resto de la respuesta —
// afecta cuánto contexto se consulta, si se puede bromear, si se
// puede iniciar tema. No es un filtro al final, es una condición de
// toda la conversación. Por eso vive en cognition/, mismo lugar que
// Pipeline/Attention/Internal State: es otro observador que va a
// consultarse ANTES de generar lenguaje, no una fuente de contenido.
//
// Ningún ModeDetector inventa lógica nueva si ya existe algo que
// aprovechar: Reflection reutiliza tal cual los triggers de
// Pipeline/Attention/Working Memory/Context Builder (Bloques 9/11/12/
// 13) y de Trust (Bloque 14) — son, literalmente, preguntas sobre el
// propio proceso o pasado de Ahmelia, que es exactamente lo que
// Reflection significa.

// ConversationMode es el resultado de Compute(): los seis parámetros
// que el resto del pipeline (todavía sin construir, bloques futuros)
// va a consultar para decidir cómo generar la respuesta.
type ConversationMode struct {
	Name             string
	Formality        float64
	Warmth           float64
	Initiative       float64
	Humor            float64
	DetailLevel      float64
	CanChallengeUser bool
	CanBringTopics   bool
}

var modeProfiles = map[string]ConversationMode{
	"Task": {
		Name: "Task", Formality: 0.3, Warmth: 0.4, Initiative: 0.2, Humor: 0.1, DetailLevel: 0.3,
		CanChallengeUser: false, CanBringTopics: false,
	},
	"Discussion": {
		Name: "Discussion", Formality: 0.4, Warmth: 0.5, Initiative: 0.6, Humor: 0.3, DetailLevel: 0.7,
		CanChallengeUser: true, CanBringTopics: true,
	},
	"Reflection": {
		Name: "Reflection", Formality: 0.3, Warmth: 0.7, Initiative: 0.4, Humor: 0.2, DetailLevel: 0.6,
		CanChallengeUser: true, CanBringTopics: false,
	},
	"Learning": {
		Name: "Learning", Formality: 0.3, Warmth: 0.6, Initiative: 0.5, Humor: 0.3, DetailLevel: 0.8,
		CanChallengeUser: false, CanBringTopics: true,
	},
	"Emotional": {
		Name: "Emotional", Formality: 0.1, Warmth: 0.9, Initiative: 0.3, Humor: 0.0, DetailLevel: 0.4,
		CanChallengeUser: false, CanBringTopics: false,
	},
	"Brainstorming": {
		Name: "Brainstorming", Formality: 0.2, Warmth: 0.6, Initiative: 0.8, Humor: 0.5, DetailLevel: 0.5,
		CanChallengeUser: true, CanBringTopics: true,
	},
}

// ConversationModeContext junta las señales que los ModeDetector
// necesitan. Se arma UNA vez por Process(), en core/engine.go, con lo
// que ya está disponible en ese punto — no dispara ninguna consulta
// nueva. Crece por acá, no en la firma de Compute(), a propósito: el
// roadmap ya anticipa Relationship (Bloque 17+) y Security Profile
// (bloque futuro) sumándose a esta misma struct sin romper nada que
// ya llame a Compute().
type ConversationModeContext struct {
	Input              string // normalizado (shared.Normalize)
	ActionResolved     bool   // el clasificador ya resolvió una acción ejecutable
	WaitingForLearning bool   // e.sess.WaitingForLearning()
	CurrentTopic       string // e.context.GetCurrentTopic(), reservado para detectores futuros
}

// ModeScore es lo que cada detector devuelve: no solo el número, la
// razón. Es la diferencia entre "0.7" y algo que Ahmelia puede citar
// si se le pregunta "¿por qué estás en modo Reflection?" — mismo
// espíritu que Pipeline.Narrative() o Decider.SummarizeDecision().
type ModeScore struct {
	Mode   string
	Score  float64
	Reason string
}

// ModeDetector es la interfaz que cada modo implementa. Agregar un
// detector nuevo, o reescribir uno existente (ej. Emotional, cuando
// haga falta algo más que keywords), no toca a los demás — ese es el
// punto entero de no meter los seis scores adentro de Compute().
type ModeDetector interface {
	Score(ctx ConversationModeContext) ModeScore
}

// modePriority resuelve empates exactos entre detectores — Emotional
// NO entra en esta lista porque tiene su propia regla, por encima de
// esta prioridad (ver Compute): un empate con Emotional lo gana
// Emotional siempre, no importa el orden de acá.
var modePriority = []string{"Task", "Learning", "Reflection", "Brainstorming", "Discussion"}

// ---------------------------------------------------------------------------
// Detectores
// ---------------------------------------------------------------------------

// taskDetector — señal binaria real: el clasificador ya resolvió una
// acción ejecutable. Sin ambigüedad de objetivo cuando esto pasa.
type taskDetector struct{}

func (taskDetector) Score(ctx ConversationModeContext) ModeScore {
	if ctx.ActionResolved {
		return ModeScore{"Task", 1.0, "acción resuelta por el clasificador"}
	}
	return ModeScore{"Task", 0.0, ""}
}

// learningDetector — señal fuerte si la sesión ya está esperando
// confirmación de aprendizaje (Bloque WaitingForLearning existente);
// si no, patrones de objetivo "quiero que sepas/enseñarte".
type learningDetector struct{}

var learningGoalPatterns = []string{
	"enseñame", "ensename", "aprende que", "quiero explicarte", "quiero que sepas",
}

func (learningDetector) Score(ctx ConversationModeContext) ModeScore {
	if ctx.WaitingForLearning {
		return ModeScore{"Learning", 1.0, "sesión esperando confirmación de aprendizaje"}
	}
	for _, p := range learningGoalPatterns {
		if strings.Contains(ctx.Input, p) {
			return ModeScore{"Learning", 0.7, "patrón de objetivo: enseñar a Ahmelia"}
		}
	}
	return ModeScore{"Learning", 0.0, ""}
}

// reflectionDetector — reutiliza tal cual los triggers meta-cognitivos
// que ya existen (Bloques 9/11/12/13/14): son, literalmente,
// preguntas sobre el propio proceso o pasado de Ahmelia. Suma
// patrones de objetivo retrospectivo propios de este bloque
// (decisiones/aprendizaje/errores/evolución) que esos triggers no
// cubrían.
type reflectionDetector struct{}

var reflectionGoalPatterns = []string{
	"que aprendiste de", "como cambio", "como has cambiado", "en que te equivocaste",
	"por que decidiste", "que error cometiste",
}

func (reflectionDetector) Score(ctx ConversationModeContext) ModeScore {
	switch {
	case ParsePipelineQuery(ctx.Input):
		return ModeScore{"Reflection", 0.9, "meta-pregunta sobre el pipeline (Bloque 11)"}
	case ParseAttentionQuery(ctx.Input):
		return ModeScore{"Reflection", 0.9, "meta-pregunta sobre atención (Bloque 12)"}
	case ParseWorkingMemoryQuery(ctx.Input):
		return ModeScore{"Reflection", 0.9, "meta-pregunta sobre memoria de trabajo (Bloque 13)"}
	case ParseContextQuery(ctx.Input):
		return ModeScore{"Reflection", 0.9, "meta-pregunta sobre contexto (Bloque 9)"}
	case reasoning.ParseTrustQuery(ctx.Input):
		return ModeScore{"Reflection", 0.9, "meta-pregunta sobre confianza (Bloque 14)"}
	}
	for _, p := range reflectionGoalPatterns {
		if strings.Contains(ctx.Input, p) {
			return ModeScore{"Reflection", 0.6, "patrón de objetivo retrospectivo"}
		}
	}
	return ModeScore{"Reflection", 0.0, ""}
}

// brainstormDetector — objetivo "quiero muchas respuestas posibles",
// no una palabra suelta como "ideas".
type brainstormDetector struct{}

var brainstormGoalPatterns = []string{
	"dame formas", "dame ideas", "dame alternativas", "dame opciones",
	"cuales son mis opciones", "inventemos", "se te ocurre algo para",
	"formas de organizar", "maneras de resolver",
}

func (brainstormDetector) Score(ctx ConversationModeContext) ModeScore {
	for _, p := range brainstormGoalPatterns {
		if strings.Contains(ctx.Input, p) {
			return ModeScore{"Brainstorming", 0.7, "objetivo: generar muchas posibilidades"}
		}
	}
	return ModeScore{"Brainstorming", 0.0, ""}
}

// discussionDetector — objetivo "quiero tu mejor respuesta, una sola".
// También el default: si nada más superó 0.5, Discussion gana con
// 0.35 — una conversación humana por defecto es Discussion, no Task.
type discussionDetector struct{}

var discussionGoalPatterns = []string{
	"que opinas de", "que pensas de", "cual elegirias", "estas de acuerdo con",
	"crees que", "que te parece",
}

func (discussionDetector) Score(ctx ConversationModeContext) ModeScore {
	for _, p := range discussionGoalPatterns {
		if strings.Contains(ctx.Input, p) {
			return ModeScore{"Discussion", 0.6, "objetivo: compartir una opinión concreta"}
		}
	}
	return ModeScore{"Discussion", 0.35, "default — ninguna otra señal superó el umbral"}
}

// emotionalDetector — heurístico simple a propósito (no hay análisis
// emocional real del USUARIO en ningún lado del proyecto todavía;
// personality/emotion.go es el estado emocional de AHMELIA, no el del
// usuario). Cubre expresión de sentir Y de valoración ("me preocupa",
// "estoy harto"), no solo "me siento X".
type emotionalDetector struct{}

var emotionalPatterns = []string{
	"me siento", "estoy cansado", "estoy agotado", "estoy frustrado", "estoy feliz",
	"me preocupa", "estoy nervioso", "estoy harto", "ya no puedo mas", "me alegra",
	"me da miedo", "estoy orgulloso", "me decepciono",
}

func (emotionalDetector) Score(ctx ConversationModeContext) ModeScore {
	for _, p := range emotionalPatterns {
		if strings.Contains(ctx.Input, p) {
			return ModeScore{"Emotional", 0.8, "expresión emocional detectada: \"" + p + "\""}
		}
	}
	return ModeScore{"Emotional", 0.0, ""}
}

// ---------------------------------------------------------------------------
// Manager
// ---------------------------------------------------------------------------

// ConversationModeManager sostiene los seis detectores y el último
// resultado — mismo criterio de "vive todo el proceso" que el resto
// de Fase 2.6.
type ConversationModeManager struct {
	detectors  []ModeDetector
	lastScores []ModeScore
	current    *ConversationMode
}

// NewConversationModeManager arma el Manager con los seis detectores.
func NewConversationModeManager() *ConversationModeManager {
	return &ConversationModeManager{
		detectors: []ModeDetector{
			taskDetector{}, learningDetector{}, reflectionDetector{},
			brainstormDetector{}, discussionDetector{}, emotionalDetector{},
		},
	}
}

// Compute puntúa los seis modos y elige el ganador. Regla de empate:
// si Emotional está entre los que empataron en el máximo, gana
// Emotional (una persona prioriza la emoción sobre la tarea: "estoy
// muy frustrado con Unity" responde primero como compañera, después
// como ingeniera). Si no, gana el primero de modePriority.
func (m *ConversationModeManager) Compute(ctx ConversationModeContext) *ConversationMode {
	scores := make([]ModeScore, 0, len(m.detectors))
	for _, d := range m.detectors {
		scores = append(scores, d.Score(ctx))
	}
	m.lastScores = scores

	best := 0.0
	for _, s := range scores {
		if s.Score > best {
			best = s.Score
		}
	}

	tied := make(map[string]bool)
	for _, s := range scores {
		if s.Score == best {
			tied[s.Mode] = true
		}
	}

	var winner string
	if tied["Emotional"] {
		winner = "Emotional"
	} else {
		for _, name := range modePriority {
			if tied[name] {
				winner = name
				break
			}
		}
	}
	if winner == "" {
		winner = "Discussion" // no debería pasar (Discussion siempre puntúa >=0.35), red de seguridad
	}

	profile := modeProfiles[winner]
	m.current = &profile
	return &profile
}

// LastScores devuelve los seis scores de la última llamada a Compute()
// — la base de "¿por qué estás en modo Reflection?".
func (m *ConversationModeManager) LastScores() []ModeScore {
	return m.lastScores
}

// CurrentName devuelve el nombre del último modo calculado, "" si
// Compute() todavía no corrió — Bloque 19 (HumorStyle) lo necesita para
// decidir sin volver a calcular el modo dentro de una rama de query
// (eso contaminaría el resultado con el texto de la meta-pregunta en
// sí, no con el turno real que se está evaluando).
func (m *ConversationModeManager) CurrentName() string {
	if m.current == nil {
		return ""
	}
	return m.current.Name
}

// Narrative arma la respuesta hablada de la meta-pregunta: el modo
// actual + la razón que lo ganó. Si todavía no se llamó a Compute() en
// ningún turno (arranque en frío), lo dice tal cual, sin inventar un
// modo por defecto.
func (m *ConversationModeManager) Narrative() string {
	if m.current == nil {
		return "Todavía no calculé ningún modo conversacional — es el primer turno."
	}
	var reason string
	for _, s := range m.lastScores {
		if s.Mode == m.current.Name {
			reason = s.Reason
			break
		}
	}
	if reason == "" {
		return "Modo actual: " + m.current.Name
	}
	return "Modo actual: " + m.current.Name + " — " + reason
}

var modeQueryTriggers = []string{
	"en que modo estas conversando", "que modo conversacional tenes",
	"por que estas en modo", "en que modo estas",
}

// ParseModeQuery detecta la meta-pregunta sobre el modo conversacional
// actual — distinta de ParseInternalStateQuery (Bloque 15, sobre
// disponibilidad: "¿puedo hablarte?") y de ParsePipelineQuery (Bloque
// 11, sobre el proceso técnico): esta es sobre CÓMO está conversando
// Ahmelia ahora mismo, no si está libre ni por qué etapa técnica pasa.
func ParseModeQuery(normalizedText string) bool {
	for _, trigger := range modeQueryTriggers {
		if strings.Contains(normalizedText, trigger) {
			return true
		}
	}
	return false
}
