package language

import (
	"fmt"
	"strings"

	"ahmelia/cognition"
	"ahmelia/interaction"
	"ahmelia/personality"
)

// ============================================================================
// RESPONSE_COMPOSER.GO — Bloque 34 (Response Composer 2.0)
// ============================================================================
//
// Middleware decorador entre LanguageGenerator (que crea texto base) y
// la respuesta final enviada al usuario. Toma 6 decisiones sobre CÓMO
// presentar el contenido ya generado, sin generar contenido nuevo.
//
// A diferencia de LanguageGenerator (que produce frases), ResponseComposer
// decide qué de todo lo disponible entra en ESTA respuesta puntual:
// recorta/extiende (Detail), agrega humor si corresponde (Humor),
// menciona contexto reciente (MentionMemory), termina con pregunta
// (AskFollowUp), o desafía si es apropiado (ChallengeUser).
//
// BLOQUE 34 — NOVEDAD: ReasoningContext Integration
// Antes (Bloque 20), ResponseComposer solo veía:
//   - ConversationMode (tono del turno)
//   - ContextAdapter (preferencias del usuario)
//   - HumorStyle (humor de Ahmelia)
//   - WorkingMemory (memoria de corto plazo)
//   - ConversationFlow (progresión de tema)
//   - Relationship (relación usuario-Ahmelia)
//
// Ahora (Bloque 34), TAMBIÉN ve ReasoningContext (de reasonAboutQuestion):
//   - Si Cognitive Mind se activó (rc.Activated)
//   - Qué calculó (GoalAnalysis, TaskPlan, Decision, etc.)
//   - Por qué se activó o no (rc.ReasoningNeed)
// Esto permite tomar decisiones MÁS INTELIGENTES sobre decoraciones.
// Ejemplo: si Cognitive Mind apenas se activó, puede agregar pista
// sobre por qué la respuesta fue diferente esta vez. Si Decision.Confidence
// es baja, puede sugerir alternativas.
//
// Patrón: Conocimiento → LanguageGenerator → texto base → ResponseComposer 2.0 → respuesta final
//
// Filosofía: cada responsabilidad está separada.
//   - LanguageGenerator: ¿QUÉ digo?
//   - ResponseComposer 2.0: ¿CÓMO lo presento? (ahora: con qué decoraciones basadas en razonamiento)
//   - No hay que tocar uno para mejorar el otro.

// Bloque 35 (Fase 2.8) — umbrales para aplicar Tone de verdad en
// ComposeResponse. Elegidos mirando el rango real de los seis
// ConversationMode que existen hoy (cognition/conversation_mode.go),
// no números arbitrarios: Warmth va de 0.4 (Task) a 0.9 (Emotional) —
// 0.7 separa a Emotional/Reflection (cálidos) del resto. Initiative va
// de 0.2 (Task) a 0.8 (Brainstorming) — 0.5 separa a Task/Reflection/
// Emotional (más pasivos) de Discussion/Learning/Brainstorming (más
// proactivos).
const (
	highWarmthThreshold          = 0.7
	proactiveInitiativeThreshold = 0.5
)

// warmClosers son las frases de cierre cálido que Bloque 35 agrega
// cuando el registro lo pide. Un solo elemento por ahora, mismo
// criterio que generateChallenge() (más abajo) ya usaba para su lista
// de un solo challenge real: variar de verdad necesitaría una fuente
// de aleatoriedad que este composer no tiene todavía — inventar varias
// frases para elegir entre ellas con un criterio falso (por ejemplo,
// según el largo del texto) sería más ruido que valor.
var warmClosers = []string{
	"Acá estoy si necesitás algo más.",
}

// ResponsePlan agrupa las 6 decisiones decorativas (todos los campos
// tienen productor real ya existente)
type ResponsePlan struct {
	// Expression is the language-independent communication contract produced by B96.22.15.
	Expression interaction.ExpressionProfile
	// Tone: parámetros de conversación del turno actual
	Formality  float64 // 0-1 (ConversationMode.Formality)
	Warmth     float64 // 0-1 (ConversationMode.Warmth)
	Initiative float64 // 0-1 (ConversationMode.Initiative)

	// Detail: adaptación al usuario
	PreferredLength string // "brief", "detailed", "comprehensive" (ContextAdapter.PreferredLength)

	// Humor: si incluir una broma y cuál
	ShouldIncludeHumor bool
	JokeText           string                // "" si no hay
	JokeTier           personality.HumorTier // "wordplay", "self-deprecation", etc

	// MentionMemory: contexto reciente para recordar
	HasRecentMemory  bool
	RecentMemoryText string // "" si no hay

	// AskFollowUp: preguntas sugeridas para continuar la conversación
	HasFollowUp         bool
	FollowUpQuestions   []string // máximo 3
	SelectedFollowUpIdx int      // cuál elegir (0, 1, o 2)

	// ChallengeUser: si desafiar y con qué tono
	ShouldChallenge bool
	ChallengeText   string // "" si no hay
}

// ResponseComposer es el orquestador de las 6 decisiones (Bloque 20)
// + lectura de ReasoningContext para enriquecer decoraciones (Bloque 34)
type ResponseComposer struct {
	conversationMode     *cognition.ConversationMode
	contextAdapter       *ContextAdapter
	humorStyle           *personality.HumorStyle
	workingMemory        *cognition.WorkingMemory
	conversationFlow     *ConversationFlow
	relationship         *personality.Relationship
	reasoningContext     *cognition.ReasoningContext   // Bloque 34: para leer qué calculó Cognitive Mind
	expressionProfile    interaction.ExpressionProfile // B96.22.15 — how to express, separate from what to say
	expressionProfileSet bool                          // preserves compatibility for callers that have not adopted B96.22.15 yet
}

// NewResponseComposer crea un nuevo compositor
func NewResponseComposer(
	mode *cognition.ConversationMode,
	context *ContextAdapter,
	humor *personality.HumorStyle,
	memory *cognition.WorkingMemory,
	flow *ConversationFlow,
	rel *personality.Relationship,
) *ResponseComposer {
	if mode == nil {
		mode = &cognition.ConversationMode{
			Formality:  0.5,
			Warmth:     0.5,
			Initiative: 0.5,
		}
	}
	if context == nil {
		context = NewContextAdapter()
	}
	if humor == nil {
		humor = personality.NewHumorStyle()
	}
	if rel == nil {
		rel = personality.NewRelationship()
	}

	return &ResponseComposer{
		conversationMode: mode,
		contextAdapter:   context,
		humorStyle:       humor,
		workingMemory:    memory,
		conversationFlow: flow,
		relationship:     rel,
		reasoningContext: nil, // Se inicializa en nil; SetReasoningContext lo asigna (Bloque 34)
	}
}

// SetMode actualiza el ConversationMode vigente para este turno.
// ConversationModeManager no expone un getter (no tiene Current()),
// así que Engine llama a esto justo después de mode := e.conversationMode.Compute(...),
// mismo criterio ya usado en Bloque 18 para ContextAdapter.PreferredLength.
func (rc *ResponseComposer) SetMode(mode *cognition.ConversationMode) {
	if mode != nil {
		rc.conversationMode = mode
	}
}

// SetReasoningContext asigna el ReasoningContext (Bloque 34).
// reasonAboutSemanticRequest() lo llena y luego lo pasa aquí para que
// ResponseComposer pueda tomar decisiones más inteligentes sobre
// decoraciones basadas en lo que Cognitive Mind calculó.
func (rc *ResponseComposer) SetReasoningContext(ctx *cognition.ReasoningContext) {
	if ctx != nil {
		rc.reasoningContext = ctx
	}
}

// ============================================================================
// MÉTODOS PRIVADOS PARA EXTRAER DECORACIONES DEL REASONINGCONTEXT (Bloque 34)
// ============================================================================

// cognitiveMindInsight devuelve un texto que explique que Cognitive Mind
// se activó y por qué. Usa ReasoningNeed.Score y Signals para calibrar
// la explicación de acuerdo a la complejidad.
func (rc *ResponseComposer) cognitiveMindInsight() string {
	if rc.reasoningContext == nil || !rc.reasoningContext.Activated {
		return ""
	}

	// Si Cognitive Mind se activó, explicar por qué brevemente
	if rc.reasoningContext.ReasoningNeed != nil {
		need := rc.reasoningContext.ReasoningNeed
		signalCount := len(need.Signals)

		// Múltiples señales + puntaje alto = necesita análisis profundo
		if signalCount >= 3 && need.Score >= 4 {
			return "Esto requiere análisis más profundo. "
		}
		// Dominio específico detectado = necesita plan
		if signalCount >= 2 && need.Score >= 2 {
			return "Esto necesita un plan. "
		}
		// Algo activó pero no es lo más complejo
		if need.Score >= 2 {
			return "Esto involucra varios aspectos. "
		}
	}

	return "Después de razonar sobre eso... "
}

// decisionConfidenceHint devuelve un texto sobre la certeza del Decision
// basado en Verdict, RiskCount y NoLeadGaps. Si hay riesgos o gaps sin
// pista, la confianza es menor.
func (rc *ResponseComposer) decisionConfidenceHint() string {
	if rc.reasoningContext == nil || !rc.reasoningContext.Activated {
		return ""
	}

	decision := rc.reasoningContext.Decision
	if decision == nil {
		return ""
	}

	// Usar Verdict para calibrar confianza
	// VerdictSeekInfo = máxima incertidumbre (hay riesgos Y gaps sin pista)
	if decision.Verdict == "pedir mas informacion antes de empezar" {
		return "Aunque debo ser honesto: esto tiene bastantes puntos grises y convendría investigar más. "
	}

	// VerdictAdjust = confianza media (hay riesgos O gaps sin pista, pero no ambos)
	if decision.Verdict == "ajustar el plan" {
		if decision.RiskCount > 0 && decision.NoLeadGaps == 0 {
			return "Mi confianza en esto no es absoluta — hay algunos requisitos en riesgo, pero el plan tiene pistas claras. "
		}
		if decision.NoLeadGaps > 0 && decision.RiskCount == 0 {
			return "Hay algunos gaps sin pista clara, pero es posible arrancar con cuidado. "
		}
		return "Conviene ajustar expectativas antes de arrancar. "
	}

	// VerdictProceed = máxima confianza (sin riesgos ni gaps sin pista)
	return ""
}

// SetExpressionProfile installs the B96.22.15 expression contract for the
// current turn. It contains no generated language and does not select a
// capability; it only gives the composer a stable communication boundary.
func (rc *ResponseComposer) SetExpressionProfile(profile interaction.ExpressionProfile) {
	rc.expressionProfile = profile
	rc.expressionProfileSet = true
}

// CreatePlan genera el ResponsePlan para este turno
func (rc *ResponseComposer) CreatePlan(currentTopic string) ResponsePlan {
	plan := ResponsePlan{
		Expression: rc.expressionProfile,
		// 1. Tone
		Formality:  rc.conversationMode.Formality,
		Warmth:     rc.conversationMode.Warmth,
		Initiative: rc.conversationMode.Initiative,

		// 2. Detail
		PreferredLength: rc.contextAdapter.PreferredLength,

		// 3. Humor
		ShouldIncludeHumor: false,
		JokeText:           "",
		JokeTier:           "",

		// 4. MentionMemory
		HasRecentMemory:  false,
		RecentMemoryText: "",

		// 5. AskFollowUp
		HasFollowUp:         false,
		FollowUpQuestions:   []string{},
		SelectedFollowUpIdx: 0,

		// 6. ChallengeUser
		ShouldChallenge: false,
		ChallengeText:   "",
	}

	// Decidir si incluir humor (usa HumorStyle.ShouldJokeNow)
	context := map[string]interface{}{
		"topic": currentTopic,
	}
	roll := 0.5 // roll neutral (podría ser aleatorio)
	if rc.humorStyle.ShouldJokeNow(rc.conversationMode.Name, context, roll) {
		jokeText, jokeTier := rc.humorStyle.PickJoke(rc.relationship, roll)
		plan.ShouldIncludeHumor = true
		plan.JokeText = jokeText
		plan.JokeTier = jokeTier
	}

	// Decidir si mencionar memoria reciente
	if rc.workingMemory != nil {
		if recent, ok := rc.workingMemory.MostRecent(); ok {
			plan.HasRecentMemory = true
			plan.RecentMemoryText = fmt.Sprintf("Hace poco hablábamos de %s", recent.Content)
		}
	}

	// Decidir si incluir pregunta de seguimiento
	if rc.conversationFlow != nil && strings.TrimSpace(currentTopic) != "" &&
		strings.EqualFold(strings.TrimSpace(rc.conversationFlow.CurrentTopic()), strings.TrimSpace(currentTopic)) {
		followUps := rc.conversationFlow.CreateFollowUpQuestions(currentTopic)
		if len(followUps) > 0 {
			plan.HasFollowUp = true
			plan.FollowUpQuestions = followUps
			plan.SelectedFollowUpIdx = 0 // elegir la primera por defecto
		}
	}

	// Decidir si desafiar al usuario
	if rc.conversationMode.CanChallengeUser && rc.relationship.ChallengesUser {
		plan.ShouldChallenge = true
		plan.ChallengeText = rc.generateChallenge()
	}

	return plan
}

// generateChallenge crea un desafío amigable al usuario
func (rc *ResponseComposer) generateChallenge() string {
	challenges := []string{
		"¿Estás seguro de eso? Te veo un poco dudoso.",
		"Mmm, ¿pero consideraste esto...?",
		"Interesante punto, pero ¿qué pasa si...?",
	}
	if len(challenges) > 0 {
		return challenges[0] // En Bloque 21, esto sería más inteligente
	}
	return ""
}

// ComposeResponse decora un texto base con el ResponsePlan (+ decoraciones de RC si aplica)
func (rc *ResponseComposer) ComposeResponse(baseText string, plan ResponsePlan) string {
	// Copiar el texto base como inicio
	composed := baseText

	// 1. Ajustar longitud según PreferredLength
	composed = rc.contextAdapter.AdjustLength(composed)

	// Bloque 34: Agregar decoraciones inteligentes basadas en ReasoningContext
	// Estas van ANTES de humor/memory/challenge/followup, para que el flujo sea
	// más natural: primero la respuesta base, luego el insight sobre razonamiento,
	// luego las 6 decoraciones estándar.
	cognitiveInsight := rc.cognitiveMindInsight()
	if cognitiveInsight != "" {
		composed = fmt.Sprintf("%s %s", composed, cognitiveInsight)
	}

	decisionHint := rc.decisionConfidenceHint()
	if decisionHint != "" {
		composed = fmt.Sprintf("%s %s", composed, decisionHint)
	}

	// 2. Agregar humor si corresponde
	if plan.ShouldIncludeHumor && plan.JokeText != "" {
		composed = fmt.Sprintf("%s\n\n😄 %s [%s]", composed, plan.JokeText, plan.JokeTier)
	}

	// 3. Mencionar memoria reciente
	if plan.HasRecentMemory && plan.RecentMemoryText != "" {
		composed = fmt.Sprintf("%s\n\nPor cierto, %s.", composed, plan.RecentMemoryText)
	}

	// 4. Añadir desafío si corresponde
	if plan.ShouldChallenge && plan.ChallengeText != "" {
		composed = fmt.Sprintf("%s\n\n%s", composed, plan.ChallengeText)
	}

	// 5. Terminar con pregunta de seguimiento — Bloque 35 (Fase 2.8):
	// gateado por plan.Initiative, no solo por si ConversationFlow
	// tenía una pregunta lista. Initiative es uno de los tres números
	// de Tone que CreatePlan calcula desde Bloque 20 (junto con
	// Formality y Warmth) pero que ComposeResponse nunca había leído
	// — solo aparecían en Describe(), para debugging. Con Initiative
	// bajo (Task: 0.2, Reflection: 0.4, Emotional: 0.3) Ahmelia deja
	// que el usuario lleve la conversación en vez de arrastrarla con
	// una pregunta; con Initiative alto (Discussion: 0.6, Learning:
	// 0.5, Brainstorming: 0.8) sí toma la iniciativa de proponer por
	// dónde seguir.
	askedFollowUp := false
	if plan.HasFollowUp && (!rc.expressionProfileSet || plan.Expression.AskFollowUp) && len(plan.FollowUpQuestions) > 0 && plan.Initiative >= proactiveInitiativeThreshold {
		if plan.SelectedFollowUpIdx < len(plan.FollowUpQuestions) {
			composed = fmt.Sprintf("%s\n\n%s", composed, plan.FollowUpQuestions[plan.SelectedFollowUpIdx])
			askedFollowUp = true
		}
	}

	// 6. Cierre cálido — Bloque 35: el segundo número de Tone que se
	// vuelve real acá. Solo se agrega si el registro lo pide (Warmth
	// alto: Emotional 0.9, Reflection 0.7 — no Task 0.4 ni Discussion
	// 0.5) Y si la respuesta todavía no tiene ya un cierre — un
	// challenge o un follow-up ya cumplen ese rol de "no dejar la
	// respuesta seca"; sumar un tercer cierre encima sería saturarla,
	// no calidez.
	//
	// Formality (el tercer número de Tone) queda deliberadamente sin
	// usar todavía: los seis ConversationMode de Bloque 16 van de 0.1
	// a 0.4 de Formality — ninguno llega a ser realmente "formal". El
	// propio archivo de ConversationMode lo dice explícito: el Perfil
	// de Interacción (que sí traería un registro formal real) "a
	// propósito no existe todavía". Escribir una rama gateada por
	// Formality alta hoy sería una rama que nunca se ejecuta con datos
	// reales — construir antes de tener el caso de uso, justo lo que
	// este proyecto evita en cada bloque.
	if plan.Warmth >= highWarmthThreshold && !plan.ShouldChallenge && !askedFollowUp {
		composed = fmt.Sprintf("%s\n\n%s", composed, warmClosers[0])
	}

	return composed
}

// Describe devuelve una descripción del ResponsePlan (para debugging/testing)
func (plan ResponsePlan) Describe() string {
	desc := fmt.Sprintf(
		"ResponsePlan:\n  Tone: %.1f/%.1f/%.1f (Formality/Warmth/Initiative)\n  Detail: %s\n",
		plan.Formality, plan.Warmth, plan.Initiative, plan.PreferredLength,
	)

	if plan.ShouldIncludeHumor {
		desc += fmt.Sprintf("  Humor: %s [%s]\n", plan.JokeText, plan.JokeTier)
	}

	if plan.HasRecentMemory {
		desc += fmt.Sprintf("  Memory: %s\n", plan.RecentMemoryText)
	}

	if plan.HasFollowUp {
		desc += fmt.Sprintf("  FollowUp: %s\n", plan.FollowUpQuestions[plan.SelectedFollowUpIdx])
	}

	if plan.ShouldChallenge {
		desc += fmt.Sprintf("  Challenge: %s\n", plan.ChallengeText)
	}

	return desc
}
