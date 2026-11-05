package language

import (
	"fmt"
	"strings"
)

// ResponseBuilder arma la respuesta final con estructura lógica
type ResponseBuilder struct {
	useNumbering bool
	useBullets   bool
	maxLength    int
}

// NewResponseBuilder crea un constructor de respuestas
func NewResponseBuilder() *ResponseBuilder {
	return &ResponseBuilder{
		useNumbering: true,
		useBullets:   true,
		maxLength:    2000,
	}
}

// ResponseContent es la estructura de una respuesta completa
type ResponseContent struct {
	Hook            string   // Enganche inicial
	MainPoint       string   // Punto principal
	Recommendation  string   // Mi recomendación
	Explanation     string   // Explicación
	Uncertainties   string   // Qué me preocupa
	Considerations  string   // Qué tener en cuenta
	CallToAction    string   // Próximo paso
	Closing         string   // Cierre cálido
}

// Build construye una respuesta básica
func (rb *ResponseBuilder) Build(
	mainOpinion string,
	explanation string,
	uncertainties string,
) string {

	response := &ResponseContent{
		Hook:           "He considerado esto cuidadosamente.",
		MainPoint:      mainOpinion,
		Explanation:    explanation,
		Uncertainties:  uncertainties,
		CallToAction:   "¿Cómo te parece?",
		Closing:        "¿Quieres que profundice en algún aspecto?",
	}

	return rb.formatResponse(response)
}

// BuildWithConfidence construye respuesta con nivel de confianza
func (rb *ResponseBuilder) BuildWithConfidence(
	mainOpinion string,
	explanation string,
	confidence float64,
) string {

	confidenceStr := rb.getConfidenceDescription(confidence)

	hook := fmt.Sprintf("Estoy %s en esta respuesta.", confidenceStr)

	response := &ResponseContent{
		Hook:           hook,
		MainPoint:      mainOpinion,
		Explanation:    explanation,
		CallToAction:   "¿Necesitas más detalles?",
		Closing:        "¿Hay algo más que quieras saber?",
	}

	return rb.formatResponse(response)
}

// BuildDetailed construye una respuesta muy detallada
func (rb *ResponseBuilder) BuildDetailed(
	topic string,
	mainOpinion string,
	explanation string,
	alternatives []string,
	confidence float64,
) string {

	parts := []string{
		fmt.Sprintf("## %s", topic),
		"",
		"### Mi Análisis",
		mainOpinion,
		"",
		"### Explicación",
		explanation,
		"",
	}

	// Agregar alternativas si existen
	if len(alternatives) > 0 {
		parts = append(parts, "### Alternativas Consideradas")
		for i, alt := range alternatives {
			parts = append(parts, fmt.Sprintf("%d. %s", i+1, alt))
		}
		parts = append(parts, "")
	}

	// Agregar confianza
	confidenceStr := rb.getConfidenceDescription(confidence)
	parts = append(parts, fmt.Sprintf("### Confianza: %s (%.0f%%)", confidenceStr, confidence*100))
	parts = append(parts, "")

	parts = append(parts, "### Próximos Pasos")
	parts = append(parts, rb.formatNextSteps())

	return strings.Join(parts, "\n")
}

// formatResponse arma la respuesta final
func (rb *ResponseBuilder) formatResponse(r *ResponseContent) string {
	parts := []string{}

	if r.Hook != "" {
		parts = append(parts, r.Hook)
		parts = append(parts, "")
	}

	if r.MainPoint != "" {
		parts = append(parts, r.MainPoint)
		parts = append(parts, "")
	}

	if r.Explanation != "" {
		parts = append(parts, "**Explicación:**")
		parts = append(parts, r.Explanation)
		parts = append(parts, "")
	}

	if r.Uncertainties != "" {
		parts = append(parts, "**Consideraciones:**")
		parts = append(parts, r.Uncertainties)
		parts = append(parts, "")
	}

	if r.Considerations != "" {
		parts = append(parts, "**Lo importante:**")
		parts = append(parts, r.Considerations)
		parts = append(parts, "")
	}

	if r.CallToAction != "" {
		parts = append(parts, r.CallToAction)
		parts = append(parts, "")
	}

	if r.Closing != "" {
		parts = append(parts, r.Closing)
	}

	return strings.Join(parts, "\n")
}

// formatNextSteps formatea próximos pasos
func (rb *ResponseBuilder) formatNextSteps() string {
	steps := []string{
		"1. Validar en tu contexto específico",
		"2. Identificar posibles problemas",
		"3. Hacer pruebas pequeñas",
		"4. Medir y ajustar",
	}

	return strings.Join(steps, "\n")
}

// getConfidenceDescription retorna descripción de confianza
func (rb *ResponseBuilder) getConfidenceDescription(confidence float64) string {
	switch {
	case confidence >= 0.9:
		return "muy segura"
	case confidence >= 0.8:
		return "segura"
	case confidence >= 0.7:
		return "bastante segura"
	case confidence >= 0.6:
		return "moderadamente segura"
	case confidence >= 0.5:
		return "poco segura"
	default:
		return "muy insegura"
	}
}

// FormatAsMarkdown formatea la respuesta como markdown
func (r *ResponseContent) FormatAsMarkdown() string {
	var md strings.Builder

	if r.Hook != "" {
		md.WriteString("> " + r.Hook + "\n\n")
	}

	if r.MainPoint != "" {
		md.WriteString("**" + r.MainPoint + "**\n\n")
	}

	if r.Explanation != "" {
		md.WriteString(r.Explanation + "\n\n")
	}

	if r.Uncertainties != "" {
		md.WriteString("⚠️ " + r.Uncertainties + "\n\n")
	}

	if r.CallToAction != "" {
		md.WriteString("_" + r.CallToAction + "_\n")
	}

	return md.String()
}

// SetMaxLength establece la longitud máxima
func (rb *ResponseBuilder) SetMaxLength(length int) {
	rb.maxLength = length
}

// TruncateIfNeeded trunca la respuesta si es demasiado larga
func (rb *ResponseBuilder) TruncateIfNeeded(response string) string {
	if len(response) > rb.maxLength {
		return response[:rb.maxLength] + "..."
	}
	return response
}
