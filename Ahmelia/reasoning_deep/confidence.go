package reasoning_deep

import (
	"fmt"
	"time"
)

// ConfidenceSource es una fuente de información para calcular confianza
type ConfidenceSource struct {
	Name        string
	Type        string           // "evidence", "consistency", "expertise", "recency"
	Weight      float64          // 0.0-1.0 (importancia)
	Value       float64          // 0.0-1.0 (calidad)
	Timestamp   time.Time
}

// ConfidenceCalculator calcula confianza dinámica
type ConfidenceCalculator struct {
	sources map[string][]*ConfidenceSource
}

// NewConfidenceCalculator crea un nuevo calculador de confianza
func NewConfidenceCalculator() *ConfidenceCalculator {
	return &ConfidenceCalculator{
		sources: make(map[string][]*ConfidenceSource),
	}
}

// AddSource añade una fuente de evidencia
func (cc *ConfidenceCalculator) AddSource(topicID string, source *ConfidenceSource) {
	if cc.sources[topicID] == nil {
		cc.sources[topicID] = []*ConfidenceSource{}
	}
	source.Timestamp = time.Now()
	cc.sources[topicID] = append(cc.sources[topicID], source)
}

// CalculateConfidence calcula la confianza general sobre un tema
func (cc *ConfidenceCalculator) CalculateConfidence(topicID string) float64 {
	sources := cc.sources[topicID]
	if len(sources) == 0 {
		return 0.0
	}

	var weightedSum, totalWeight float64

	for _, source := range sources {
		// Aplicar recency: información más reciente vale más
		ageMinutes := time.Since(source.Timestamp).Minutes()
		recencyFactor := 1.0
		if ageMinutes > 60 {
			recencyFactor = 0.9
		}
		if ageMinutes > 1440 { // 1 día
			recencyFactor = 0.7
		}
		if ageMinutes > 10080 { // 1 semana
			recencyFactor = 0.5
		}

		adjustedValue := source.Value * recencyFactor

		weightedSum += adjustedValue * source.Weight
		totalWeight += source.Weight
	}

	if totalWeight == 0 {
		return 0.5
	}

	confidence := weightedSum / totalWeight

	// Penalidad por conflictos
	conflictPenalty := cc.detectConflicts(sources)
	confidence = confidence - conflictPenalty

	if confidence < 0.0 {
		confidence = 0.0
	}
	if confidence > 1.0 {
		confidence = 1.0
	}

	return confidence
}

// detectConflicts detecta si hay fuentes contradictorias
func (cc *ConfidenceCalculator) detectConflicts(sources []*ConfidenceSource) float64 {
	if len(sources) < 2 {
		return 0.0
	}

	// Calcular varianza de valores
	var sum, sumSq float64
	for _, s := range sources {
		sum += s.Value
		sumSq += s.Value * s.Value
	}

	mean := sum / float64(len(sources))
	variance := (sumSq / float64(len(sources))) - (mean * mean)

	// Alta varianza = conflicto
	// Máximo penalidad: 0.3
	penalty := variance * 0.3
	if penalty > 0.3 {
		penalty = 0.3
	}

	return penalty
}

// GetConfidenceReport retorna un reporte de confianza
func (cc *ConfidenceCalculator) GetConfidenceReport(topicID string) string {
	confidence := cc.CalculateConfidence(topicID)
	sources := cc.sources[topicID]

	report := fmt.Sprintf("ANÁLISIS DE CONFIANZA: %s\n\n", topicID)
	report += fmt.Sprintf("Confianza general: %.0f%%\n\n", confidence*100)

	if len(sources) == 0 {
		report += "Sin fuentes de información\n"
		return report
	}

	report += fmt.Sprintf("Basado en %d fuente(s):\n", len(sources))
	for i, source := range sources {
		age := time.Since(source.Timestamp)
		ageStr := fmt.Sprintf("%.0f min", age.Minutes())
		if age.Hours() >= 1 {
			ageStr = fmt.Sprintf("%.1f horas", age.Hours())
		}
		if age.Hours() >= 24 {
			ageStr = fmt.Sprintf("%.1f días", age.Hours()/24)
		}

		report += fmt.Sprintf(
			"%d. %s (%s): valor=%.0f%%, peso=%.0f%%, antigüedad=%s\n",
			i+1, source.Name, source.Type, source.Value*100, source.Weight*100, ageStr,
		)
	}

	// Detectar conflictos
	conflict := cc.detectConflicts(sources)
	if conflict > 0.1 {
		report += fmt.Sprintf("\n⚠️  Conflicto detectado entre fuentes (penalidad: %.0f%%)\n", conflict*100)
	}

	return report
}

// GetConfidenceLevel retorna nivel cualitativo
func (cc *ConfidenceCalculator) GetConfidenceLevel(topicID string) string {
	confidence := cc.CalculateConfidence(topicID)

	switch {
	case confidence < 0.2:
		return "muy baja"
	case confidence < 0.4:
		return "baja"
	case confidence < 0.6:
		return "media"
	case confidence < 0.8:
		return "alta"
	default:
		return "muy alta"
	}
}

// GetConfidenceStatement retorna una frase sobre la confianza
func (cc *ConfidenceCalculator) GetConfidenceStatement(topicID string) string {
	confidence := cc.CalculateConfidence(topicID)
	level := cc.GetConfidenceLevel(topicID)
	sourceCount := len(cc.sources[topicID])

	return fmt.Sprintf(
		"Confianza %s (%.0f%%) basada en %d fuente(s)",
		level, confidence*100, sourceCount,
	)
}

// ShouldExpressUncertainty verifica si debería expresar dudas
func (cc *ConfidenceCalculator) ShouldExpressUncertainty(topicID string) bool {
	confidence := cc.CalculateConfidence(topicID)
	return confidence < 0.7
}

// NeedMoreEvidence verifica si necesita más datos
func (cc *ConfidenceCalculator) NeedMoreEvidence(topicID string) bool {
	sources := cc.sources[topicID]
	confidence := cc.CalculateConfidence(topicID)

	// Necesita más evidencia si:
	// - Pocas fuentes (< 2)
	// - Baja confianza (< 0.5)
	// - Hay conflictos
	return len(sources) < 2 || confidence < 0.5 || cc.detectConflicts(sources) > 0.15
}

// AskForClarification retorna preguntas que necesita responder
func (cc *ConfidenceCalculator) AskForClarification(topicID string) []string {
	sources := cc.sources[topicID]
	questions := []string{}

	if len(sources) == 0 {
		questions = append(questions, "¿Tienes información sobre este tema?")
	}

	if len(sources) == 1 {
		questions = append(questions, "Necesito más perspectivas. ¿Hay otra forma de verlo?")
	}

	conflict := cc.detectConflicts(sources)
	if conflict > 0.1 {
		questions = append(questions, "Detecto información contradictoria. ¿Cuál es la más precisa?")
	}

	return questions
}

// UpdateConfidence actualiza basado en feedback del usuario
func (cc *ConfidenceCalculator) UpdateConfidence(topicID string, feedback string, wasCorrect bool) {
	var value float64 = 0.7
	if !wasCorrect {
		value = 0.3
	}

	source := &ConfidenceSource{
		Name:      fmt.Sprintf("Feedback: %s", feedback),
		Type:      "feedback",
		Weight:    0.5, // Feedback tiene peso medio
		Value:     value,
		Timestamp: time.Now(),
	}

	cc.AddSource(topicID, source)
}
