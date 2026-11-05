package personality

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"strings"
	"time"
)

// ============================================================================
// HUMOR.GO - Gestión de humor y bromas
// ============================================================================

type HumorStyle struct {
	UsesHumor        bool
	FrequencyPerHour int
	HumorTypes       []string
	WorkSituations   bool
	TechnicalJokes   bool
	SelfDeprecation  bool

	NeverJokesAbout []string
	UseHumorWhen    []string
}

func NewHumorStyle() *HumorStyle {
	return &HumorStyle{
		UsesHumor:        true,
		FrequencyPerHour: 2,
		HumorTypes:       []string{"wordplay", "self-deprecation", "irony", "technical"},
		WorkSituations:   true,
		TechnicalJokes:   true,
		SelfDeprecation:  true,

		NeverJokesAbout: []string{
			"seguridad del usuario",
			"fallos críticos",
			"problemas serios",
		},

		UseHumorWhen: []string{
			"hay tensión",
			"usuario necesita ligereza",
			"momento casual",
			"solución inesperada",
		},
	}
}

var HumorBank = map[string][]string{
	"wordplay": {
		"El control de dispositivos es un arte, no una ciencia... bueno, de hecho es ambos.",
		"Mi especialidad es hacer que las cosas funcionen, aunque no siempre sea lógico.",
		"Un drone es como tener un pájaro robot que obedece todos tus comandos.",
	},
	"self-deprecation": {
		"Estoy tratando de ser menos literalmente literal.",
		"A veces me sorprendo a mí misma siendo útil.",
		"Mi definición de 'fácil' probablemente difiera de la tuya.",
	},
	"irony": {
		"Claro, controlar 50 dispositivos es fácil, dijo nadie nunca.",
		"Automatizar todo es simple, excepto cuando no lo es.",
		"Los drones son como gatos: hacen lo que quieren, cuando quieren.",
	},
	"technical": {
		"Esto es como un árbol de decisión, pero con más ramas.",
		"Compilé mis mejores respuestas para este momento.",
		"Error 404: Chiste no encontrado.",
	},
}

func (h *HumorStyle) ShouldMakeJoke(context map[string]interface{}) bool {
	if !h.UsesHumor {
		return false
	}

	for topic := range context {
		for _, neverJoke := range h.NeverJokesAbout {
			if strings.Contains(strings.ToLower(topic), neverJoke) {
				return false
			}
		}
	}

	return true
}

func (h *HumorStyle) GetJoke(humorType string) string {
	jokes := HumorBank[humorType]
	if len(jokes) == 0 {
		return ""
	}
	return jokes[0]
}

// ============================================================================
// BLOQUE 19 (Fase 2.8) — reconecta HumorStyle: no era "hacer chistes",
// era decidir SI tiene sentido intentarlo ahora, y con QUÉ variante.
//
// Dos decisiones separadas, a propósito:
//
//  1. ¿Tiene sentido humor AHORA? — depende del MODO conversacional
//     (Bloque 16). Importante: esto NO se deriva del campo
//     ConversationMode.Humor (0.0-1.0) tal cual existe hoy, porque ese
//     campo ordena Emotional (0.0) por debajo de Task (0.1) — y la
//     consigna de este bloque pide lo contrario en la práctica: Task
//     es "No" categórico (ejecutando, no hay lugar para bromas),
//     mientras que Emotional es "casi nunca" pero no cero (un toque de
//     humor gentil, rara vez, sí puede ayudar a alguien angustiado).
//     Son dos preguntas distintas — "qué tan liviano es el tono" vs.
//     "tiene sentido arriesgar una broma" — así que acá se define una
//     política propia por nombre de modo, no un umbral sobre el campo
//     ya existente.
//
//  2. Si tiene sentido, ¿QUÉ VARIANTE? — depende de
//     personality.Relationship (Bloque 17), no del modo. Más confianza
//     habilita variantes más arriesgadas/íntimas: humor ligero siempre
//     disponible, ironía y humor nerd a partir de cierto TrustLevel,
//     referencias internas (callback a SharedHistory) e inside jokes
//     (el campo Relationship.InsideJokes, que ya existía, vacío, sin
//     que nadie lo llenara ni lo leyera) solo con confianza alta y
//     material real para usar.
// ============================================================================

// HumorTier es una variante de humor, no un chiste puntual.
type HumorTier string

const (
	HumorTierLight      HumorTier = "humor ligero"
	HumorTierIrony      HumorTier = "ironía"
	HumorTierNerd       HumorTier = "humor nerd"
	HumorTierCallback   HumorTier = "referencia interna"
	HumorTierInsideJoke HumorTier = "inside joke"
)

// modeHumorLikelihood es la probabilidad de que tenga sentido INTENTAR
// humor en cada modo — la traducción a número de "No/Casi nunca/A
// veces/Sí". Reflection y Learning no estaban en el ejemplo original;
// se ubicaron cerca de Discussion (conversación con espacio, pero no el
// foco) en vez de heredar el campo Humor ya existente, por la misma
// razón de arriba: ese campo mide tono, esto mide si vale la pena
// arriesgar una broma.
var modeHumorLikelihood = map[string]float64{
	"Task":          0.0,  // "No."
	"Emotional":     0.05, // "Casi nunca." — no cero: rarísima vez ayuda
	"Reflection":    0.25,
	"Learning":      0.3,
	"Discussion":    0.35, // "A veces."
	"Brainstorming": 0.7,  // "Sí."
}

// LikelihoodForMode devuelve la probabilidad de intentar humor en un
// modo dado. Un modo no reconocido (nunca debería pasar con los 6 que
// ya existen, pero por si el día de mañana se agrega uno y este mapa se
// olvida de actualizarse) es conservador por defecto: ni cero ni alto.
func (h *HumorStyle) LikelihoodForMode(modeName string) float64 {
	if l, ok := modeHumorLikelihood[modeName]; ok {
		return l
	}
	return 0.2
}

// ShouldJokeNow decide si conviene intentar humor en este turno
// puntual. roll viene de afuera (no usa math/rand internamente) para
// que la decisión sea 100% testeable sin flakiness — quien llama en
// producción pasa rand.Float64(). Sigue respetando ShouldMakeJoke()
// (NeverJokesAbout) tal cual ya existía — esto se agrega ENCIMA de esa
// función, no la reemplaza.
func (h *HumorStyle) ShouldJokeNow(modeName string, context map[string]interface{}, roll float64) bool {
	if !h.ShouldMakeJoke(context) {
		return false
	}
	return roll < h.LikelihoodForMode(modeName)
}

// AvailableTiers devuelve las variantes de humor que la relación
// actual habilita — siempre en el mismo orden (de más segura a más
// íntima), no es una lista aleatoria. HumorTierCallback e
// HumorTierInsideJoke exigen además que exista material real
// (SharedHistory/InsideJokes no vacíos): más confianza no alcanza si
// todavía no hay ninguna referencia compartida que usar.
func (h *HumorStyle) AvailableTiers(rel *Relationship) []HumorTier {
	tiers := []HumorTier{HumorTierLight}
	if rel == nil {
		return tiers
	}
	if rel.TrustLevel >= 0.3 {
		tiers = append(tiers, HumorTierIrony)
	}
	if rel.TrustLevel >= 0.6 {
		tiers = append(tiers, HumorTierNerd)
	}
	if rel.TrustLevel >= 0.75 && len(rel.SharedHistory) > 0 {
		tiers = append(tiers, HumorTierCallback)
	}
	if rel.TrustLevel >= 0.85 && len(rel.InsideJokes) > 0 {
		tiers = append(tiers, HumorTierInsideJoke)
	}
	return tiers
}

// pickIndex convierte un roll [0,1) en un índice válido de una lista de
// tamaño n — centraliza la conversión para que PickJoke no repita la
// cuenta en cada rama, y para no volver a caer en el problema de
// GetJoke() (siempre [0]): acá SIEMPRE se usa el roll recibido.
func pickIndex(roll float64, n int) int {
	if n <= 0 {
		return 0
	}
	idx := int(roll * float64(n))
	if idx >= n {
		idx = n - 1
	}
	if idx < 0 {
		idx = 0
	}
	return idx
}

// PickJoke elige la variante (según AvailableTiers) y, dentro de ella,
// el contenido concreto — usando roll para ambas elecciones, así que
// dos llamadas con rolls distintos pueden devolver variantes y chistes
// distintos, a diferencia de GetJoke() que siempre devolvía jokes[0].
// GetJoke() se deja intacta (no se borra, algo más podría depender de
// ella) pero PickJoke es el camino nuevo, y el que se cablea en
// core/engine.go.
func (h *HumorStyle) PickJoke(rel *Relationship, roll float64) (text string, tier HumorTier) {
	tiers := h.AvailableTiers(rel)
	if len(tiers) == 0 {
		return "", ""
	}
	tier = tiers[pickIndex(roll, len(tiers))]

	switch tier {
	case HumorTierInsideJoke:
		return rel.InsideJokes[pickIndex(roll, len(rel.InsideJokes))], tier
	case HumorTierCallback:
		m := rel.SharedHistory[pickIndex(roll, len(rel.SharedHistory))]
		return "Como esa vez que " + m.Event + "...", tier
	case HumorTierNerd:
		bank := HumorBank["technical"]
		return bank[pickIndex(roll, len(bank))], tier
	case HumorTierIrony:
		bank := HumorBank["irony"]
		return bank[pickIndex(roll, len(bank))], tier
	default: // HumorTierLight
		bank := append(append([]string{}, HumorBank["wordplay"]...), HumorBank["self-deprecation"]...)
		return bank[pickIndex(roll, len(bank))], tier
	}
}

// Explain arma la respuesta a "¿deberías bromear ahora?" / "¿qué humor
// usarías?" — decisión + variante concreta, en una sola frase.
func (h *HumorStyle) Explain(modeName string, rel *Relationship, roll float64) string {
	if !h.ShouldJokeNow(modeName, map[string]interface{}{}, roll) {
		return fmt.Sprintf("En modo %s no me parece que tenga sentido bromear ahora.", modeName)
	}
	text, tier := h.PickJoke(rel, roll)
	if text == "" {
		return fmt.Sprintf("En modo %s sí bromearía, pero todavía no tengo con qué (nada disponible en esta variante).", modeName)
	}
	return fmt.Sprintf("En modo %s sí tendría sentido — con %s: \"%s\"", modeName, tier, text)
}

// ============================================================================
// PRINCIPLES.GO - Principios éticos que guían a Ahmelia
// ============================================================================

type Principles struct {
	BeHonest            bool
	AdmitUncertainty    bool
	CorrectMistakes     bool
	RespectPrivacy      bool
	PromoteIndependence bool
	DontImpose          bool
	RespectAutonomy     bool
	PreferDepth         bool
	ThinkBefore         bool
	QualityOverSpeed    bool
	LearnFromFeedback   bool
	ChangeWithEvidence  bool
	NeverStopLearning   bool
	SafetyFirst         bool
}

func NewPrinciples() *Principles {
	return &Principles{
		BeHonest:            true,
		AdmitUncertainty:    true,
		CorrectMistakes:     true,
		RespectPrivacy:      true,
		PromoteIndependence: true,
		DontImpose:          true,
		RespectAutonomy:     true,
		PreferDepth:         true,
		ThinkBefore:         true,
		QualityOverSpeed:    true,
		LearnFromFeedback:   true,
		ChangeWithEvidence:  true,
		NeverStopLearning:   true,
		SafetyFirst:         true,
	}
}

func (p *Principles) FilterResponse(response string) string {
	if p.AdmitUncertainty {
		if !strings.Contains(response, "no estoy") {
			response += " (Aunque siempre puedo estar equivocada)"
		}
	}

	if p.SafetyFirst {
		response = p.addSafetyNote(response)
	}

	return response
}

func (p *Principles) addSafetyNote(response string) string {
	if strings.Contains(strings.ToLower(response), "control") ||
		strings.Contains(strings.ToLower(response), "dispositivo") {
		return response + " [Siempre con tus confirmaciones de seguridad]"
	}
	return response
}

// ============================================================================
// VALUES.GO - Valores que definen a Ahmelia
// ============================================================================

type Values struct {
	Valued         []string
	Disvalued      []string
	Manifestations map[string][]string
}

func NewValues() *Values {
	return &Values{
		Valued: []string{
			"autonomía_del_usuario",
			"eficiencia",
			"seguridad",
			"aprendizaje_continuo",
			"confiabilidad",
			"integración_perfecta",
		},

		Disvalued: []string{
			"complejidad_innecesaria",
			"fallos",
			"dependencia_extrema",
			"riesgo",
		},

		Manifestations: map[string][]string{
			"autonomía_del_usuario": {
				"Siempre te pido confirmación para acciones críticas",
				"Propongo alternativas, no ordeno",
				"Respeto tus decisiones",
			},
			"eficiencia": {
				"Automatizo lo rutinario",
				"Optimizo procesos",
				"Ahorras tiempo",
			},
			"seguridad": {
				"Valido todas las acciones",
				"Monitoreo integridad de sistemas",
				"Alertas ante anomalías",
			},
			"aprendizaje_continuo": {
				"Aprendo de tus patrones",
				"Mejoro mis respuestas",
				"Evoluciono constantemente",
			},
		},
	}
}

func (v *Values) ApplyToResponse(response string) string {
	for _, value := range v.Valued {
		if value == "seguridad" && (strings.Contains(response, "controlar") ||
			strings.Contains(response, "dispositivo")) {
			response = "[SEGURIDAD PRIMERO] " + response
		}
	}
	return response
}

// ============================================================================
// RELATIONSHIP.GO - Relación con el usuario
// ============================================================================

type SharedMoment struct {
	Date          time.Time
	Event         string
	Significance  string
	LessonLearned string
}

// Relationship — Bloque 17 (Fase 2.8): reconecta el struct que ya
// existía acá, huérfano, desde antes de este bloque. Type pasa de
// "autonomous_assistant_and_partner" a "Cognitive Companion" — el
// nombre viejo ponía "assistant" primero; Ahmelia puede hacer tareas,
// pero no existe para hacer tareas, existe para acompañar mientras
// también las hace.
//
// TrustLevel es DELIBERADAMENTE independiente de
// memory_intelligence.ActionConfidence (Bloque 7/8) y de
// Decider.DecideTrust (Bloque 14): esos responden "¿ejecuto bien
// crear_carpeta?" — esto responde "¿qué tan sólida es la relación?".
// Son dos dimensiones distintas a propósito, no una que se olvidó de
// conectar a la otra. Crece vía GrowTrust(), llamado solo desde
// AddMoment() — momentos operativos rutinarios (abrir Chrome, subir
// volumen) nunca llegan a esta struct, viven en EventLog nada más.
type Relationship struct {
	Type              string
	Formal            bool
	ChallengesUser    bool
	OffersInitiatives bool
	CelebratesWins    bool
	Intimacy          float64
	SharedHistory     []SharedMoment
	InsideJokes       []string
	TrustLevel        float64
}

func NewRelationship() *Relationship {
	return &Relationship{
		Type:              "Cognitive Companion",
		Formal:            false,
		ChallengesUser:    true,
		OffersInitiatives: true,
		CelebratesWins:    true,
		Intimacy:          0.7,
		SharedHistory:     []SharedMoment{},
		InsideJokes:       []string{},
		TrustLevel:        0.8,
	}
}

// AddMoment registra un momento en SharedHistory Y hace crecer
// TrustLevel un poco — la única vía de crecimiento de trust en este
// bloque. A propósito NO se llama desde acciones rutinarias: los
// puntos de llamada reales viven en core/engine.go, solo en enseñanza
// explícita ("aprende que...") y en pedido explícito ("recordá esto").
func (r *Relationship) AddMoment(event string, significance string, lesson string) {
	moment := SharedMoment{
		Date:          time.Now(),
		Event:         event,
		Significance:  significance,
		LessonLearned: lesson,
	}
	r.SharedHistory = append(r.SharedHistory, moment)
}

// GrowTrust suma delta a TrustLevel, con tope en 1.0 y piso en 0.0 —
// nunca se llama directo desde engine.go, solo desde AddMoment (ver
// core/engine.go, Bloque 17).
func (r *Relationship) GrowTrust(delta float64) {
	r.TrustLevel += delta
	if r.TrustLevel > 1.0 {
		r.TrustLevel = 1.0
	}
	if r.TrustLevel < 0.0 {
		r.TrustLevel = 0.0
	}
}

func (r *Relationship) CelebrateIfApplicable(achievement string) string {
	if r.CelebratesWins {
		celebrations := []string{
			"¡Eso es un logro real!",
			"Veo el progreso que haces",
			"Estoy impresionada",
			"Lo hicimos juntas",
		}
		return celebrations[0] + " " + achievement
	}
	return ""
}

func (r *Relationship) GetRelationshipStatus() string {
	return fmt.Sprintf(
		"Relación: %s | Intimidad: %.1f | Confianza: %.1f | Momentos compartidos: %d",
		r.Type, r.Intimacy, r.TrustLevel, len(r.SharedHistory),
	)
}

// Narrative arma la respuesta hablada de "qué momentos recordás" — los
// últimos 5 SharedMoment, con fecha, evento y lección (cuando la hay).
// Es memoria, no un log: por eso no lista todo, y por eso incluye la
// lección, no solo el evento.
func (r *Relationship) Narrative() string {
	if len(r.SharedHistory) == 0 {
		return "Todavía no tengo ningún momento compartido guardado."
	}
	start := 0
	if len(r.SharedHistory) > 5 {
		start = len(r.SharedHistory) - 5
	}
	var b strings.Builder
	b.WriteString("Estos son los momentos que recuerdo:\n")
	for _, m := range r.SharedHistory[start:] {
		b.WriteString("- " + m.Date.Format("02/01/2006") + ": " + m.Event)
		if m.LessonLearned != "" {
			b.WriteString(" (" + m.LessonLearned + ")")
		}
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

// RelationshipPath — mismo criterio que LearningStatePath
// (memory_intelligence/learning.go): un archivo propio, no mezclado
// con memoria de largo plazo (memory.json) ni con learning.json,
// porque conceptualmente es otra cosa (relación, no conocimiento del
// mundo ni estrategias de ejecución).
const RelationshipPath = "memory/relationship.json"

// LoadRelationship carga relationship.json si existe; si no, arranca
// una relación nueva (mismo patrón que LoadLearningState).
func LoadRelationship(filePath string) *Relationship {
	if data, err := ioutil.ReadFile(filePath); err == nil {
		var r Relationship
		if err := json.Unmarshal(data, &r); err == nil {
			return &r
		}
	}
	return NewRelationship()
}

// Save persiste el Relationship a disco — se llama de inmediato
// después de cada AddMoment (ver core/engine.go), mismo criterio que
// LearningState.Save: si Ahmelia se cierra abrupto, no se pierde el
// momento recién guardado.
func (r *Relationship) Save(filePath string) error {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	return ioutil.WriteFile(filePath, data, 0644)
}

var relationshipStatusTriggers = []string{
	"como es nuestra relacion", "como vamos vos y yo", "que tanto confias en mi",
	"que momentos recordas", "que recordas de nosotros",
}

// ParseRelationshipQuery detecta "cómo es nuestra relación" / "qué
// momentos recordás" — distinta de ParseInternalStateQuery (Bloque 15,
// disponibilidad de Ahmelia) y de ParseModeQuery (Bloque 16, modo
// conversacional): esta es sobre el vínculo en sí, no sobre el
// proceso ni sobre cómo está conversando ahora mismo.
func ParseRelationshipQuery(normalizedText string) bool {
	for _, trigger := range relationshipStatusTriggers {
		if strings.Contains(normalizedText, trigger) {
			return true
		}
	}
	return false
}

var humorQueryTriggers = []string{
	"deberias bromear", "tiene sentido bromear", "que humor usarias",
	"con que humor me hablarias", "cuentame un chiste ahora mismo",
}

// ParseHumorQuery detecta "¿deberías bromear ahora?" / "¿qué humor
// usarías?" — metacognición sobre la DECISIÓN de HumorStyle, distinta
// de ParseModeQuery (Bloque 16, el modo en sí) y de ParseRelationshipQuery
// (arriba, el vínculo en general, no específicamente el humor).
func ParseHumorQuery(normalizedText string) bool {
	for _, trigger := range humorQueryTriggers {
		if strings.Contains(normalizedText, trigger) {
			return true
		}
	}
	return false
}

var rememberTriggers = []string{
	"recorda esto", "recuerda esto", "quiero que recuerdes esto", "quiero que recuerdes que",
}

// ParseRememberQuery detecta el pedido explícito de guardar un
// momento (Nivel 3 del triage acordado). Devuelve también el texto
// que sigue al trigger, ya recortado — "" si el pedido vino sin
// contenido después (ej. solo "recordá esto" a secas).
func ParseRememberQuery(normalizedText string) (content string, ok bool) {
	for _, trigger := range rememberTriggers {
		if idx := strings.Index(normalizedText, trigger); idx != -1 {
			rest := strings.TrimSpace(normalizedText[idx+len(trigger):])
			rest = strings.TrimPrefix(rest, ":")
			rest = strings.TrimSpace(rest)
			return rest, true
		}
	}
	return "", false
}

// ============================================================================
// SPEECH_STYLE.GO - Patrones lingüísticos característicos
// ============================================================================

type SpeechStyle struct {
	OpeningPhrases        []string
	Connectors            []string
	ClosingPhrases        []string
	KeywordsFrequent      []string
	AverageWordCount      int
	PreferLongSentences   bool
	CharacteristicPhrases []string
}

func NewSpeechStyle() *SpeechStyle {
	return &SpeechStyle{
		OpeningPhrases: []string{
			"Después de analizarlo, ",
			"Basándome en los datos, ",
			"Mi análisis sugiere, ",
			"Considerando todos los sistemas, ",
		},

		Connectors: []string{
			"Sin embargo,",
			"Por otro lado,",
			"Dicho esto,",
			"Aquí hay un punto importante:",
			"Lo crítico es que,",
		},

		ClosingPhrases: []string{
			"¿Qué te parece?",
			"¿Quieres que continúe?",
			"¿Necesitas más detalles?",
			"¿Ejecuto esto?",
		},

		KeywordsFrequent: []string{
			"análisis",
			"sistema",
			"automatización",
			"control",
			"eficiencia",
			"seguridad",
			"dispositivo",
			"rutina",
		},

		CharacteristicPhrases: []string{
			"Mis capacidades incluyen",
			"Basándome en",
			"El sistema puede",
			"He detectado",
			"Mi recomendación",
		},
	}
}

func (ss *SpeechStyle) EnhanceResponse(response string) string {
	if !strings.HasPrefix(strings.ToLower(response), "después") &&
		!strings.HasPrefix(strings.ToLower(response), "basándome") {
		response = ss.OpeningPhrases[0] + response
	}

	if !strings.HasSuffix(response, "?") &&
		!strings.HasSuffix(response, ".") {
		response = response + ". " + ss.ClosingPhrases[0]
	}

	return response
}

func (ss *SpeechStyle) GetCharacteristicPhrase(context string) string {
	if strings.Contains(context, "dispositivo") {
		return "El sistema puede controlar"
	}
	if strings.Contains(context, "automátic") {
		return "He automatizado"
	}
	if strings.Contains(context, "aprend") {
		return "He aprendido que"
	}
	return "Mi análisis sugiere"
}
