package language

import (
	"fmt"
	"strings"
)

// conversationSummaryTriggers detecta pedidos del resumen de TODA la
// sesión — distinto de ParseWorkingMemoryQuery (cognition, Bloque 13),
// que tiene TTL propio (5 min) y responde "qué sigue vigente ahora".
// Esto es el historial completo de temas/decisiones desde que arrancó
// la conversación, sin vencer nunca.
var conversationSummaryTriggers = []string{
	"resumen de la conversacion",
	"resumen de lo que hablamos",
	"que hemos hablado",
	"de que temas hablamos",
	"resumime la charla",
}

// ParseConversationSummaryQuery detecta si la pregunta pide el resumen
// completo de la sesión (ConversationFlow.CreateSummary()).
func ParseConversationSummaryQuery(normalizedText string) bool {
	for _, trigger := range conversationSummaryTriggers {
		if strings.Contains(normalizedText, trigger) {
			return true
		}
	}
	return false
}

// ConversationFlow mantiene coherencia conversacional y continuidad
type ConversationFlow struct {
	currentTopic      string
	previousDecisions []string
	topicsDiscussed   []string
	turnNumber        int
}

// NewConversationFlow crea un gestor de flujo conversacional
func NewConversationFlow() *ConversationFlow {
	return &ConversationFlow{
		currentTopic:      "",
		previousDecisions: []string{},
		topicsDiscussed:   []string{},
		turnNumber:        0,
	}
}

// RecordTopic registra un nuevo tema
func (cf *ConversationFlow) RecordTopic(topic string) {
	cf.currentTopic = topic
	cf.turnNumber++

	if !cf.topicExists(topic) {
		cf.topicsDiscussed = append(cf.topicsDiscussed, topic)
	}
}

// RecordDecision registra una decisión tomada
func (cf *ConversationFlow) RecordDecision(decision string) {
	cf.previousDecisions = append(cf.previousDecisions, decision)
}

// Maintain mantiene continuidad con conversaciones previas
func (cf *ConversationFlow) Maintain(currentResponse string) string {
	if len(cf.previousDecisions) > 0 {
		prefix := cf.createConnectorToPrevious()
		return prefix + "\n\n" + currentResponse
	}

	return currentResponse
}

// createConnectorToPrevious crea conectores a decisiones previas
func (cf *ConversationFlow) createConnectorToPrevious() string {
	if len(cf.previousDecisions) == 0 {
		return ""
	}

	lastDecision := cf.previousDecisions[len(cf.previousDecisions)-1]

	connectors := []string{
		fmt.Sprintf("Recordando lo que decidimos antes sobre %s,", lastDecision),
		fmt.Sprintf("Continuando con nuestro análisis de %s,", lastDecision),
		fmt.Sprintf("Basándome en lo que hablamos sobre %s,", lastDecision),
		fmt.Sprintf("Como comentamos antes sobre %s,", lastDecision),
	}

	return cf.selectVariant(connectors)
}

// MaintainCoherence mantiene coherencia en múltiples turnos
func (cf *ConversationFlow) MaintainCoherence(currentPoint string) string {
	if len(cf.topicsDiscussed) == 0 {
		return currentPoint
	}

	if cf.isSameTopic(currentPoint) {
		return cf.createContinuation(currentPoint)
	}

	return cf.createTopicShift(currentPoint)
}

// isSameTopic verifica si estamos en el mismo tema
func (cf *ConversationFlow) isSameTopic(current string) bool {
	if len(cf.topicsDiscussed) == 0 {
		return true
	}

	keywords := []string{"classifier", "refactor", "architecture", "performance", "reasoner", "memory"}

	for _, keyword := range keywords {
		if strings.Contains(strings.ToLower(current), keyword) {
			for _, prev := range cf.topicsDiscussed {
				if strings.Contains(strings.ToLower(prev), keyword) {
					return true
				}
			}
		}
	}

	return false
}

// createContinuation crea una respuesta de continuación
func (cf *ConversationFlow) createContinuation(currentPoint string) string {
	continuations := []string{
		"Expandiendo lo anterior,",
		"Como mencioné,",
		"Continuando con esto,",
		"Para profundizar en ello,",
	}

	return cf.selectVariant(continuations) + " " + currentPoint
}

// createTopicShift crea una transición a un nuevo tema
func (cf *ConversationFlow) createTopicShift(currentPoint string) string {
	shifts := []string{
		"Ahora, sobre un tema diferente,",
		"Cambiando de perspectiva,",
		"Pasando a otro aspecto,",
		"Considerando ahora,",
	}

	return cf.selectVariant(shifts) + " " + currentPoint
}

// selectVariant exists only for legacy compatibility APIs. The canonical
// conversational path is B96.39 + Language generation and never calls these
// template methods to select a movement. When an older caller still uses one,
// variants rotate with conversation state instead of silently pinning the first variant.
func (cf *ConversationFlow) selectVariant(variants []string) string {
	if len(variants) == 0 {
		return ""
	}
	idx := cf.turnNumber % len(variants)
	return variants[idx]
}

// topicExists verifica si un tema ya fue tratado
func (cf *ConversationFlow) topicExists(topic string) bool {
	for _, t := range cf.topicsDiscussed {
		if strings.ToLower(t) == strings.ToLower(topic) {
			return true
		}
	}
	return false
}

// CreateFollowUpQuestions crea preguntas de seguimiento
func (cf *ConversationFlow) CreateFollowUpQuestions(topic string) []string {
	return []string{
		fmt.Sprintf("¿Qué aspecto de %s quieres explorar más?", topic),
		fmt.Sprintf("¿Cómo te afecta personalmente %s?", topic),
		fmt.Sprintf("¿Tienes dudas sobre mi análisis de %s?", topic),
	}
}

// CurrentTopic devuelve el tema activo tipado — Bloque 72 (Autonomous
// State) es el segundo consumidor real de este dato después de
// el antiguo consumidor de tema, que hasta acá
// lo leía a mano desde GetCurrentState()["current_topic"].(string). Con
// dos callers reales, vale el getter tipado en vez de seguir
// duplicando esa type assertion — GetCurrentState() sigue existiendo
// tal cual para quien ya la usa (CreateSummary y afines).
func (cf *ConversationFlow) CurrentTopic() string {
	return cf.currentTopic
}

// GetCurrentState retorna el estado actual de la conversación
func (cf *ConversationFlow) GetCurrentState() map[string]interface{} {
	return map[string]interface{}{
		"turn_number":      cf.turnNumber,
		"current_topic":    cf.currentTopic,
		"topics_discussed": cf.topicsDiscussed,
		"decisions_made":   cf.previousDecisions,
		"decision_count":   len(cf.previousDecisions),
	}
}

// CreateSummary crea un resumen de la conversación
func (cf *ConversationFlow) CreateSummary() string {
	if len(cf.topicsDiscussed) == 0 && len(cf.previousDecisions) == 0 {
		return "No hay conversación registrada aún."
	}

	var summary strings.Builder

	if len(cf.topicsDiscussed) > 0 {
		summary.WriteString(fmt.Sprintf("Hemos cubierto %d tema(s):\n", len(cf.topicsDiscussed)))
		for _, topic := range cf.topicsDiscussed {
			summary.WriteString(fmt.Sprintf("- %s\n", topic))
		}
		summary.WriteString("\n")
	}

	if len(cf.previousDecisions) > 0 {
		summary.WriteString(fmt.Sprintf("Hemos tomado %d decisión(es):\n", len(cf.previousDecisions)))
		for _, decision := range cf.previousDecisions {
			summary.WriteString(fmt.Sprintf("- %s\n", decision))
		}
	}

	return summary.String()
}

// BuildOnPreviousWork construye sobre trabajo anterior
func (cf *ConversationFlow) BuildOnPreviousWork(currentAnalysis string) string {
	if len(cf.previousDecisions) == 0 {
		return currentAnalysis
	}

	return fmt.Sprintf(
		"Basándome en nuestras conversaciones previas, %s",
		currentAnalysis,
	)
}

// CreateBridgeToNextTopic crea un puente al siguiente tema
func (cf *ConversationFlow) CreateBridgeToNextTopic(nextTopic string) string {
	if cf.currentTopic == "" {
		return fmt.Sprintf("Ahora, sobre %s.", nextTopic)
	}

	bridges := []string{
		fmt.Sprintf("Esto nos lleva naturalmente a %s.", nextTopic),
		fmt.Sprintf("Ahora que hemos resuelto %s, podemos enfocarnos en %s.", cf.currentTopic, nextTopic),
		fmt.Sprintf("Con %s claro, veamos %s.", cf.currentTopic, nextTopic),
	}

	return cf.selectVariant(bridges)
}

// RemindOfCommitment recuerda compromisos previos
func (cf *ConversationFlow) RemindOfCommitment(previousPromise string) string {
	return fmt.Sprintf(
		"Como dije que haría, aquí está mi análisis sobre %s.",
		previousPromise,
	)
}

// AcknowledgeUserInput reconoce input del usuario
func (cf *ConversationFlow) AcknowledgeUserInput(userComment string) string {
	acknowledgments := []string{
		fmt.Sprintf("Gracias por eso. %s es un punto válido.", userComment),
		fmt.Sprintf("Entiendo. Has señalado algo importante: %s", userComment),
		fmt.Sprintf("Ese es un buen punto: %s", userComment),
	}

	return cf.selectVariant(acknowledgments)
}

// IntegrateUserFeedback integra feedback del usuario
func (cf *ConversationFlow) IntegrateUserFeedback(
	originalConclusion string,
	userFeedback string,
) string {

	return fmt.Sprintf(
		"Considerando tu feedback sobre %s, "+
			"ajustaré mi conclusión original de %s.",
		userFeedback, originalConclusion,
	)
}

// DetectChangeOfTopic detecta si cambiamos de tema
func (cf *ConversationFlow) DetectChangeOfTopic(newTopic string) bool {
	return cf.currentTopic != "" && cf.currentTopic != newTopic
}

// ResetCurrentTopic removes only the active focus. Discussed topics and
// previous decisions remain available for summaries/history, but no longer
// drive automatic follow-ups or topic-dependent prompts.
func (cf *ConversationFlow) ResetCurrentTopic() {
	if cf == nil {
		return
	}
	cf.currentTopic = ""
}

// ResetConversation reinicia la conversación
func (cf *ConversationFlow) ResetConversation() {
	cf.currentTopic = ""
	cf.previousDecisions = []string{}
	cf.topicsDiscussed = []string{}
	cf.turnNumber = 0
}
