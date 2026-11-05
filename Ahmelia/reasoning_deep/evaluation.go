package reasoning_deep

import (
	"fmt"
	"time"
)

// EvaluationCriteria son los criterios para validar una decisión
type EvaluationCriteria struct {
	Name          string
	Description   string
	Weight        float64      // 0.0-1.0 (importancia)
	IsMet         bool
	Evidence      string
	Confidence    float64
}

// Evaluation es la validación completa de una decisión
type Evaluation struct {
	DecisionID    string
	DecisionName  string
	Criteria      []*EvaluationCriteria
	OverallScore  float64      // 0.0-1.0
	IsValid       bool
	Issues        []string
	Concerns      []string
	Recommendations []string
	EvaluatedAt   time.Time
}

// Evaluator valida decisiones antes de implementarlas
type Evaluator struct {
	evaluations map[string]*Evaluation
}

// NewEvaluator crea un nuevo evaluador
func NewEvaluator() *Evaluator {
	return &Evaluator{
		evaluations: make(map[string]*Evaluation),
	}
}

// Evaluate valida una decisión contra múltiples criterios
func (e *Evaluator) Evaluate(decisionID, decisionName string, criteria []*EvaluationCriteria) *Evaluation {
	evaluation := &Evaluation{
		DecisionID:   decisionID,
		DecisionName: decisionName,
		Criteria:     criteria,
		EvaluatedAt:  time.Now(),
		Issues:       []string{},
		Concerns:     []string{},
		Recommendations: []string{},
	}

	// Calcular score general
	evaluation.calculateScore()

	// Validar
	evaluation.IsValid = evaluation.OverallScore >= 0.65

	// Identificar problemas
	evaluation.identifyIssues()

	// Generar recomendaciones
	evaluation.generateRecommendations()

	e.evaluations[decisionID] = evaluation
	return evaluation
}

// calculateScore calcula el score ponderado
func (e *Evaluation) calculateScore() {
	if len(e.Criteria) == 0 {
		e.OverallScore = 0.5
		return
	}

	var weightedSum, totalWeight float64

	for _, c := range e.Criteria {
		score := 0.0
		if c.IsMet {
			score = 1.0
		}

		// Ajustar por confianza
		score = score * c.Confidence

		weightedSum += score * c.Weight
		totalWeight += c.Weight
	}

	if totalWeight > 0 {
		e.OverallScore = weightedSum / totalWeight
	}
}

// identifyIssues identifica criterios no cumplidos
func (e *Evaluation) identifyIssues() {
	for _, c := range e.Criteria {
		if !c.IsMet {
			e.Issues = append(e.Issues, fmt.Sprintf(
				"❌ %s: %s",
				c.Name, c.Evidence,
			))

			// Si es importante, es una preocupación
			if c.Weight >= 0.3 {
				e.Concerns = append(e.Concerns, fmt.Sprintf(
					"Criterio importante no cumplido: %s",
					c.Name,
				))
			}
		}
	}
}

// generateRecommendations genera recomendaciones basadas en la evaluación
func (e *Evaluation) generateRecommendations() {
	if e.OverallScore < 0.5 {
		e.Recommendations = append(e.Recommendations,
			"⚠️  Score muy bajo. Reconsiderar esta decisión.")
	}

	if len(e.Concerns) > 0 {
		e.Recommendations = append(e.Recommendations,
			fmt.Sprintf("⚠️  Hay %d criterios importantes no cumplidos.", len(e.Concerns)))
	}

	if e.OverallScore >= 0.8 {
		e.Recommendations = append(e.Recommendations,
			"✅ Esta decisión cumple con los criterios. Proceder con confianza.")
	}

	if e.OverallScore >= 0.65 && e.OverallScore < 0.8 {
		e.Recommendations = append(e.Recommendations,
			"✓ Cumple criterios, pero con reservas. Monitorearprogreso.")
	}
}

// GetReport retorna un reporte de evaluación
func (e *Evaluation) GetReport() string {
	report := fmt.Sprintf("EVALUACIÓN: %s\n\n", e.DecisionName)

	// Score general
	report += fmt.Sprintf("Score general: %.2f/1.0 ", e.OverallScore)
	if e.IsValid {
		report += "✅ VÁLIDA\n"
	} else {
		report += "❌ NO VÁLIDA\n"
	}

	report += "\nCRITERIOS EVALUADOS:\n"
	for _, c := range e.Criteria {
		status := "❌"
		if c.IsMet {
			status = "✅"
		}

		report += fmt.Sprintf("%s %s (peso: %.0f%%, confianza: %.0f%%)\n",
			status, c.Name, c.Weight*100, c.Confidence*100)
		report += fmt.Sprintf("   %s\n", c.Evidence)
	}

	if len(e.Issues) > 0 {
		report += "\nPROBLEMAS IDENTIFICADOS:\n"
		for _, issue := range e.Issues {
			report += fmt.Sprintf("  %s\n", issue)
		}
	}

	if len(e.Concerns) > 0 {
		report += "\nPREOCUPACIONES:\n"
		for _, concern := range e.Concerns {
			report += fmt.Sprintf("  ⚠️  %s\n", concern)
		}
	}

	if len(e.Recommendations) > 0 {
		report += "\nRECOMENDACIONES:\n"
		for _, rec := range e.Recommendations {
			report += fmt.Sprintf("  %s\n", rec)
		}
	}

	return report
}

// GetSimplifiedReport retorna solo lo esencial
func (e *Evaluation) GetSimplifiedReport() string {
	return fmt.Sprintf(
		"Validé '%s': %.0f%% cumple criterios.%s%s",
		e.DecisionName,
		e.OverallScore*100,
		func() string {
			if len(e.Concerns) > 0 {
				return fmt.Sprintf(" Preocupaciones: %d", len(e.Concerns))
			}
			return ""
		}(),
		func() string {
			if !e.IsValid {
				return " ⚠️  NO RECOMENDADA"
			}
			return ""
		}(),
	)
}

// CanProceed verifica si puede procederse con la decisión
func (e *Evaluation) CanProceed() bool {
	return e.IsValid && len(e.Concerns) == 0
}

// ShouldReview verifica si necesita revisión
func (e *Evaluation) ShouldReview() bool {
	return !e.CanProceed() || (e.OverallScore < 0.8)
}
