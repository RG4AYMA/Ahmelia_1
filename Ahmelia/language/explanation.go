package language

import (
	"fmt"
	"strings"
)

// Explanation maneja explicaciones paso a paso
type Explanation struct {
	Topic      string
	Steps      []string
	Evidence   []string
	Conclusion string
	Caveats    []string
}

// ExplanationEngine genera explicaciones
type ExplanationEngine struct {
	currentTopic string
}

// NewExplanationEngine crea un nuevo motor de explicaciones
func NewExplanationEngine() *ExplanationEngine {
	return &ExplanationEngine{}
}

// ExplainReasoning explica el razonamiento paso a paso
func (ee *ExplanationEngine) ExplainReasoning(conclusion string, evidence []string) *Explanation {
	explanation := &Explanation{
		Topic:      "Razonamiento",
		Conclusion: conclusion,
		Evidence:   evidence,
	}

	// Construir pasos
	explanation.Steps = []string{
		"Recopilé los datos disponibles",
		"Analicé desde múltiples perspectivas",
		"Evaluté factibilidad y riesgos",
		"Llegué a una conclusión fundamentada",
	}

	return explanation
}

// ExplainDecision explica una decisión
func (ee *ExplanationEngine) ExplainDecision(decision string, alternatives []string, reasoning string) *Explanation {
	steps := []string{
		fmt.Sprintf("Consideré %d alternativas diferentes", len(alternatives)),
		"Analicé pros y contras de cada una",
		"Evaluté el balance de riesgos y beneficios",
		fmt.Sprintf("La mejor opción resultó ser: %s", decision),
	}

	return &Explanation{
		Topic:      "Decisión",
		Steps:      steps,
		Conclusion: fmt.Sprintf("Elegí %s porque %s", decision, reasoning),
	}
}

// ExplainAlternatives explica alternativas disponibles
func (ee *ExplanationEngine) ExplainAlternatives(main string, alternatives []string) string {
	result := fmt.Sprintf("La opción principal es: %s\n\nAlternativas consideradas:\n", main)

	for i, alt := range alternatives {
		result += fmt.Sprintf("%d. %s\n", i+1, alt)
	}

	return result
}

// BuildDetailedExplanation construye una explicación detallada
func (ee *ExplanationEngine) BuildDetailedExplanation(topic string, steps []string, conclusion string) string {
	var result strings.Builder

	result.WriteString(fmt.Sprintf("EXPLICACIÓN: %s\n\n", topic))
	result.WriteString("Pasos del análisis:\n\n")

	for i, step := range steps {
		result.WriteString(fmt.Sprintf("%d. %s\n", i+1, step))
	}

	result.WriteString(fmt.Sprintf("\nConclusión:\n%s\n", conclusion))

	return result.String()
}

// FormatExplanation formatea una explicación para presentación
func (e *Explanation) FormatExplanation() string {
	var result strings.Builder

	result.WriteString(fmt.Sprintf("### %s\n\n", e.Topic))

	if len(e.Steps) > 0 {
		result.WriteString("**Pasos:**\n")
		for i, step := range e.Steps {
			result.WriteString(fmt.Sprintf("%d. %s\n", i+1, step))
		}
		result.WriteString("\n")
	}

	if len(e.Evidence) > 0 {
		result.WriteString("**Evidencia:**\n")
		for _, ev := range e.Evidence {
			result.WriteString(fmt.Sprintf("- %s\n", ev))
		}
		result.WriteString("\n")
	}

	if e.Conclusion != "" {
		result.WriteString(fmt.Sprintf("**Conclusión:**\n%s\n", e.Conclusion))
	}

	if len(e.Caveats) > 0 {
		result.WriteString("\n**Consideraciones:**\n")
		for _, caveat := range e.Caveats {
			result.WriteString(fmt.Sprintf("- ⚠️  %s\n", caveat))
		}
	}

	return result.String()
}

// SimpleExplanation retorna una versión simplificada
func (e *Explanation) SimpleExplanation() string {
	if e.Conclusion == "" {
		return "Sin conclusión disponible"
	}
	return e.Conclusion
}

// AddCaveat añade una consideración o limitación
func (e *Explanation) AddCaveat(caveat string) {
	e.Caveats = append(e.Caveats, caveat)
}

// GetStepCount retorna el número de pasos
func (e *Explanation) GetStepCount() int {
	return len(e.Steps)
}

// GetEvidenceCount retorna el número de evidencias
func (e *Explanation) GetEvidenceCount() int {
	return len(e.Evidence)
}
