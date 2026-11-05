package reasoning_deep

import (
	"fmt"
	"time"
)

// Conclusion representa una conclusión fundamentada
type Conclusion struct {
	ID              string
	Topic           string
	Statement       string           // La conclusión en texto
	Evidence        []string         // Evidencia que la respalda
	Perspectives    []string         // Perspectivas analizadas
	Confidence      float64          // 0.0-1.0
	ConfidenceLevel string           // "muy baja", "baja", "media", "alta", "muy alta"
	Limitations     []string         // Lo que no sé
	Alternatives    []string         // Otras conclusiones posibles
	TimeToReview    time.Duration    // Cuándo revisar esta conclusión
	ConcludedAt     time.Time
}

// ConclusionBuilder construye conclusiones fundamentadas
type ConclusionBuilder struct {
	conclusions map[string]*Conclusion
}

// NewConclusionBuilder crea un nuevo constructor de conclusiones
func NewConclusionBuilder() *ConclusionBuilder {
	return &ConclusionBuilder{
		conclusions: make(map[string]*Conclusion),
	}
}

// BuildConclusion construye una conclusión desde análisis
func (cb *ConclusionBuilder) BuildConclusion(
	topic string,
	statement string,
	evidence []string,
	perspectives []string,
	confidence float64,
	limitations []string,
	alternatives []string,
) *Conclusion {
	conclusion := &Conclusion{
		ID:           generateConclusionID(),
		Topic:        topic,
		Statement:    statement,
		Evidence:     evidence,
		Perspectives: perspectives,
		Confidence:   confidence,
		Limitations:  limitations,
		Alternatives: alternatives,
		ConcludedAt:  time.Now(),
	}

	// Determinar nivel de confianza
	conclusion.setConfidenceLevel()

	// Determinar cuándo revisar
	conclusion.setReviewTime()

	cb.conclusions[conclusion.ID] = conclusion
	return conclusion
}

// setConfidenceLevel asigna el nivel basado en score
func (c *Conclusion) setConfidenceLevel() {
	switch {
	case c.Confidence < 0.2:
		c.ConfidenceLevel = "muy baja"
	case c.Confidence < 0.4:
		c.ConfidenceLevel = "baja"
	case c.Confidence < 0.6:
		c.ConfidenceLevel = "media"
	case c.Confidence < 0.8:
		c.ConfidenceLevel = "alta"
	default:
		c.ConfidenceLevel = "muy alta"
	}
}

// setReviewTime determina cuándo revisar la conclusión
func (c *Conclusion) setReviewTime() {
	switch c.ConfidenceLevel {
	case "muy baja":
		c.TimeToReview = 1 * time.Hour      // Pronto
	case "baja":
		c.TimeToReview = 4 * time.Hour
	case "media":
		c.TimeToReview = 12 * time.Hour
	case "alta":
		c.TimeToReview = 1 * 24 * time.Hour // 1 día
	default:
		c.TimeToReview = 7 * 24 * time.Hour // 1 semana
	}
}

// GetStatement retorna la conclusión formateada
func (c *Conclusion) GetStatement() string {
	return c.Statement
}

// GetQualifiedStatement retorna la conclusión con calificadores
func (c *Conclusion) GetQualifiedStatement() string {
	qualifier := ""

	switch c.ConfidenceLevel {
	case "muy baja":
		qualifier = "Tengo poca confianza en que: "
	case "baja":
		qualifier = "Es posible que: "
	case "media":
		qualifier = "Considero que: "
	case "alta":
		qualifier = "Estoy bastante seguro de que: "
	default:
		qualifier = "Estoy muy seguro de que: "
	}

	return qualifier + c.Statement
}

// GetFullReport retorna un reporte completo
func (c *Conclusion) GetFullReport() string {
	report := fmt.Sprintf("CONCLUSIÓN: %s\n\n", c.Topic)

	// Declaración principal
	report += fmt.Sprintf("📌 %s\n", c.GetQualifiedStatement())
	report += fmt.Sprintf("Confianza: %s (%.0f%%)\n\n", c.ConfidenceLevel, c.Confidence*100)

	// Evidencia
	if len(c.Evidence) > 0 {
		report += "EVIDENCIA:\n"
		for _, e := range c.Evidence {
			report += fmt.Sprintf("  • %s\n", e)
		}
		report += "\n"
	}

	// Perspectivas consideradas
	if len(c.Perspectives) > 0 {
		report += "PERSPECTIVAS ANALIZADAS:\n"
		for _, p := range c.Perspectives {
			report += fmt.Sprintf("  • %s\n", p)
		}
		report += "\n"
	}

	// Limitaciones
	if len(c.Limitations) > 0 {
		report += "LIMITACIONES (Lo que no sé):\n"
		for _, l := range c.Limitations {
			report += fmt.Sprintf("  • %s\n", l)
		}
		report += "\n"
	}

	// Alternativas
	if len(c.Alternatives) > 0 {
		report += "ALTERNATIVAS CONSIDERADAS:\n"
		for _, a := range c.Alternatives {
			report += fmt.Sprintf("  • %s\n", a)
		}
		report += "\n"
	}

	// Recomendación
	report += fmt.Sprintf("Revisar esta conclusión en: %v\n", c.TimeToReview)

	return report
}

// GetCompactReport retorna un reporte corto
func (c *Conclusion) GetCompactReport() string {
	report := c.GetQualifiedStatement()

	if len(c.Evidence) > 0 {
		report += fmt.Sprintf(" (basado en: %d evidencias)", len(c.Evidence))
	}

	if len(c.Limitations) > 0 {
		report += fmt.Sprintf(" [pero no sé: %s]", c.Limitations[0])
	}

	return report
}

// IsStable verifica si la conclusión es estable
func (c *Conclusion) IsStable() bool {
	return c.Confidence >= 0.65 && len(c.Limitations) <= 2
}

// NeedsReview verifica si necesita revisión urgente
func (c *Conclusion) NeedsReview() bool {
	return time.Since(c.ConcludedAt) > c.TimeToReview
}

// AddEvidence añade más evidencia
func (c *Conclusion) AddEvidence(evidence string) {
	c.Evidence = append(c.Evidence, evidence)
	// Aumentar confianza ligeramente
	c.Confidence = c.Confidence + 0.05
	if c.Confidence > 1.0 {
		c.Confidence = 1.0
	}
	c.setConfidenceLevel()
}

// ChallengeConclusion reduce la confianza cuando se cuestiona
func (c *Conclusion) ChallengeConclusion(reason string) {
	c.Confidence = c.Confidence - 0.15
	if c.Confidence < 0.0 {
		c.Confidence = 0.0
	}
	c.Limitations = append(c.Limitations, reason)
	c.setConfidenceLevel()
	c.setReviewTime()
}

// helper
func generateConclusionID() string {
	return fmt.Sprintf("conc-%d", time.Now().UnixNano())
}
