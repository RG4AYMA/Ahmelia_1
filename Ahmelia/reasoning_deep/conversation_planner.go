package reasoning_deep

import (
	"fmt"
	"time"
)

// InformationGap representa lo que nos falta saber
type InformationGap struct {
	Question string
	Priority int     // 1-5
	Impact   string  // "alta", "media", "baja"
}

// ConversationPlan es un plan sobre cómo responder
type ConversationPlan struct {
	ID              string
	UserInput       string
	UserIntent      string
	InformationGaps []InformationGap
	
	// Decisiones de estrategia
	ShouldAskFirst  bool         // ¿Preguntar antes de responder?
	ShouldExplain   bool         // ¿Explicar el razonamiento?
	ShouldGiveExample bool       // ¿Dar ejemplos?
	ShouldRecommend bool         // ¿Hacer recomendación?
	ShouldExpressDoubt bool      // ¿Expresar dudas?
	
	// Orden de respuesta
	ResponseOrder   []string    // ["pregunta", "explicación", "ejemplo", "recomendación"]
	
	// Confianza
	Confidence      float64
	ReadinessLevel  string      // "ready_to_respond", "need_info", "uncertain"
	
	PlannedAt       time.Time
}

// ConversationPlanner planifica cómo responder inteligentemente
type ConversationPlanner struct {
	plans map[string]*ConversationPlan
}

// NewConversationPlanner crea un nuevo planificador de conversación
func NewConversationPlanner() *ConversationPlanner {
	return &ConversationPlanner{
		plans: make(map[string]*ConversationPlan),
	}
}

// PlanResponse planifica cómo responder a una pregunta
func (cp *ConversationPlanner) PlanResponse(
	userInput string,
	userIntent string,
	confidence float64,
	informationGaps []InformationGap,
) *ConversationPlan {
	plan := &ConversationPlan{
		ID:              generatePlanID(),
		UserInput:       userInput,
		UserIntent:      userIntent,
		InformationGaps: informationGaps,
		Confidence:      confidence,
		PlannedAt:       time.Now(),
	}

	// Decidir estrategia basada en confianza y gaps
	cp.decideStrategy(plan)

	// Planificar orden de respuesta
	cp.planResponseOrder(plan)

	// Determinar nivel de preparación
	cp.determineReadiness(plan)

	cp.plans[plan.ID] = plan
	return plan
}

// decideStrategy decide la estrategia de respuesta
func (cp *ConversationPlanner) decideStrategy(plan *ConversationPlan) {
	// Si tengo muchos gaps, preguntar primero
	if len(plan.InformationGaps) >= 3 {
		plan.ShouldAskFirst = true
		plan.ShouldExplain = false
	}

	// Si tengo baja confianza, expresar dudas
	if plan.Confidence < 0.6 {
		plan.ShouldExpressDoubt = true
	}

	// Si tengo confianza alta, explicar
	if plan.Confidence >= 0.7 {
		plan.ShouldExplain = true
		plan.ShouldGiveExample = true
	}

	// Si es recomendación y confianza es alta, recomendar
	if plan.UserIntent == "ask_for_advice" && plan.Confidence >= 0.8 {
		plan.ShouldRecommend = true
	}
}

// planResponseOrder planifica el orden de la respuesta
func (cp *ConversationPlanner) planResponseOrder(plan *ConversationPlan) {
	order := []string{}

	if plan.ShouldAskFirst {
		order = append(order, "pregunta")
	}

	if plan.ShouldExpressDoubt {
		order = append(order, "expresar_duda")
	}

	order = append(order, "respuesta_principal")

	if plan.ShouldExplain {
		order = append(order, "explicacion")
	}

	if plan.ShouldGiveExample {
		order = append(order, "ejemplo")
	}

	if plan.ShouldRecommend {
		order = append(order, "recomendacion")
	}

	plan.ResponseOrder = order
}

// determineReadiness determina si está lista para responder
func (cp *ConversationPlanner) determineReadiness(plan *ConversationPlan) {
	if plan.ShouldAskFirst {
		plan.ReadinessLevel = "need_info"
	} else if plan.Confidence < 0.5 {
		plan.ReadinessLevel = "uncertain"
	} else {
		plan.ReadinessLevel = "ready_to_respond"
	}
}

// GetResponsePrompt retorna las instrucciones de cómo responder
func (plan *ConversationPlan) GetResponsePrompt() string {
	prompt := fmt.Sprintf("Plan para responder a: '%s'\n\n", plan.UserInput)

	// Información que me falta
	if len(plan.InformationGaps) > 0 {
		prompt += "NECESITO SABER:\n"
		for i, gap := range plan.InformationGaps {
			prompt += fmt.Sprintf("%d. %s (impacto: %s)\n", i+1, gap.Question, gap.Impact)
		}
		prompt += "\n"
	}

	// Estrategia
	prompt += "ESTRATEGIA:\n"
	if plan.ShouldAskFirst {
		prompt += "  • Preguntar primero para aclarar\n"
	}
	if plan.ShouldExpressDoubt {
		prompt += "  • Expresar dudas y limitaciones\n"
	}
	if plan.ShouldExplain {
		prompt += "  • Explicar el razonamiento\n"
	}
	if plan.ShouldGiveExample {
		prompt += "  • Dar ejemplos concretos\n"
	}
	if plan.ShouldRecommend {
		prompt += "  • Hacer una recomendación clara\n"
	}

	// Orden
	prompt += fmt.Sprintf("\nORDEN DE RESPUESTA: %v\n", plan.ResponseOrder)

	// Readiness
	prompt += fmt.Sprintf("ESTADO: %s (confianza: %.0f%%)\n", plan.ReadinessLevel, plan.Confidence*100)

	return prompt
}

// CanRespond verifica si puede responder directamente
func (plan *ConversationPlan) CanRespond() bool {
	return plan.ReadinessLevel == "ready_to_respond"
}

// NeedsMoreInfo verifica si necesita más información
func (plan *ConversationPlan) NeedsMoreInfo() bool {
	return plan.ReadinessLevel == "need_info"
}

// HasDoubts verifica si tiene dudas
func (plan *ConversationPlan) HasDoubts() bool {
	return plan.ShouldExpressDoubt || plan.ReadinessLevel == "uncertain"
}

// BuildActualResponse construye la respuesta basada en el plan
func (plan *ConversationPlan) BuildActualResponse(
	mainAnswer string,
	explanation string,
	example string,
	recommendation string,
) string {
	response := ""

	// Seguir el orden planeado
	for _, step := range plan.ResponseOrder {
		switch step {
		case "pregunta":
			// Las preguntas se construyen separadamente
			continue

		case "expresar_duda":
			response += "⚠️  No tengo certeza completa, pero basándome en lo que sé:\n\n"

		case "respuesta_principal":
			response += mainAnswer
			if response != "" && !endsWithNewline(response) {
				response += "\n"
			}

		case "explicacion":
			if explanation != "" {
				response += fmt.Sprintf("\n📚 Explicación: %s", explanation)
				if !endsWithNewline(response) {
					response += "\n"
				}
			}

		case "ejemplo":
			if example != "" {
				response += fmt.Sprintf("\n📝 Ejemplo: %s", example)
				if !endsWithNewline(response) {
					response += "\n"
				}
			}

		case "recomendacion":
			if recommendation != "" {
				response += fmt.Sprintf("\n💡 Mi recomendación: %s", recommendation)
			}
		}
	}

	return response
}

// GetQuestionsToAsk retorna las preguntas que debería hacer
func (plan *ConversationPlan) GetQuestionsToAsk() []string {
	questions := []string{}

	if len(plan.InformationGaps) > 0 {
		for _, gap := range plan.InformationGaps {
			if gap.Priority >= 3 {
				questions = append(questions, gap.Question)
			}
		}
	}

	return questions
}

// helper functions
func generatePlanID() string {
	return fmt.Sprintf("plan-%d", time.Now().UnixNano())
}

func endsWithNewline(s string) bool {
	return len(s) > 0 && s[len(s)-1] == '\n'
}
