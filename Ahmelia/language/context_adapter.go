package language

import (
	"fmt"
	"strings"
)

// contextAdapterTriggers detecta preguntas sobre cómo ContextAdapter
// está percibiendo al usuario ahora mismo (nivel técnico asumido, largo
// preferido, dominio) — metacognición sobre la ADAPTACIÓN de lenguaje,
// distinta de "en qué modo estás conversando" (Bloque 16, el tono/humor
// general del turno).
var contextAdapterTriggers = []string{
	"como me estas percibiendo",
	"como me ves",
	"que nivel tecnico crees que tengo",
	"como estas adaptando tus respuestas",
}

// ParseContextAdapterQuery detecta si la pregunta pide
// GetContextSummary().
func ParseContextAdapterQuery(normalizedText string) bool {
	for _, trigger := range contextAdapterTriggers {
		if strings.Contains(normalizedText, trigger) {
			return true
		}
	}
	return false
}

// ContextAdapter adapta respuestas al contexto del usuario
type ContextAdapter struct {
	UserTechLevel   int    // 1-5
	PreferredLength string // "brief", "detailed", "comprehensive"
	Domain          string // "architecture", "ops", etc
	UserName        string
}

// NewContextAdapter crea un nuevo adaptador
func NewContextAdapter() *ContextAdapter {
	return &ContextAdapter{
		UserTechLevel:   3,
		PreferredLength: "detailed",
		Domain:          "general",
		UserName:        "Adrian",
	}
}

// AdaptResponse adapta una respuesta al contexto del usuario
func (ca *ContextAdapter) AdaptResponse(baseResponse string) string {
	if ca.UserTechLevel <= 2 {
		return ca.simplifyForNonTechnical(baseResponse)
	} else if ca.UserTechLevel >= 4 {
		return ca.expandForTechnical(baseResponse)
	}
	return baseResponse
}

// simplifyForNonTechnical simplifica para usuarios no técnicos
func (ca *ContextAdapter) simplifyForNonTechnical(response string) string {
	return fmt.Sprintf("En términos simples: %s", response)
}

// expandForTechnical expande para usuarios técnicos
func (ca *ContextAdapter) expandForTechnical(response string) string {
	return fmt.Sprintf("Análisis técnico detallado:\n%s", response)
}

// AdjustLength ajusta el largo de la respuesta
func (ca *ContextAdapter) AdjustLength(response string) string {
	switch ca.PreferredLength {
	case "brief":
		if len(response) > 200 {
			return response[:200] + "..."
		}
	case "comprehensive":
		return response + "\n\nPara más detalles específicos, puedo profundizar en cualquier área."
	}
	return response
}

// GetContextSummary retorna resumen del contexto
func (ca *ContextAdapter) GetContextSummary() string {
	return fmt.Sprintf(
		"Usuario: %s | Nivel técnico: %d/5 | Preferencia: %s | Dominio: %s",
		ca.UserName,
		ca.UserTechLevel,
		ca.PreferredLength,
		ca.Domain,
	)
}

// ShouldMentionTechnicalDetails decide si mencionar detalles técnicos
func (ca *ContextAdapter) ShouldMentionTechnicalDetails() bool {
	return ca.UserTechLevel >= 3
}

// GetToneForContext retorna el tono apropiado
func (ca *ContextAdapter) GetToneForContext() string {
	if ca.PreferredLength == "brief" {
		return "directo"
	}
	return "conversacional"
}
