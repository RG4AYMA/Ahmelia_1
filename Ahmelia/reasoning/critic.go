package reasoning

import (
	"fmt"
)

// CriticalReview es la evaluación crítica de una acción
type CriticalReview struct {
	Action           string
	WasSuccessful    bool
	WasOptimal       bool   // ¿Fue la mejor opción?
	Certainty        float64 // Qué tan seguro estaba
	PostAnalysis     string
	Lessons          []string
	WouldChangeIt    bool
	RecommendedNext  string
	InternalDoubt    string // El pensamiento interno
}

// Critic se evalúa a sí mismo después de actuar
type Critic struct {
	analyzer *Analyzer
}

// NewCritic crea un crítico
func NewCritic(analyzer *Analyzer) *Critic {
	return &Critic{analyzer: analyzer}
}

/* =======================
   REVISAR CRÍTICAMENTE UNA ACCIÓN
   Después de ejecutarla, preguntarse:
   ¿Hice bien? ¿Pude hacerlo mejor?
======================= */

func (c *Critic) ReviewAction(action string, success bool, confidence float64) CriticalReview {
	review := CriticalReview{
		Action:        action,
		WasSuccessful: success,
		Certainty:     confidence,
	}

	// Analizar si fue óptima
	patterns := c.analyzer.AnalyzeSuccessPatterns()
	var bestPerformer string
	var bestRate float64

	for _, p := range patterns {
		if p.SuccessRate > bestRate {
			bestRate = p.SuccessRate
			bestPerformer = p.Action
		}
	}

	// ¿Fue óptima?
	if action == bestPerformer || bestRate <= 0 {
		review.WasOptimal = true
		review.PostAnalysis = "Elegí la acción más confiable disponible"
	} else {
		review.WasOptimal = false
		review.PostAnalysis = fmt.Sprintf("No era óptima. '%s' hubiera sido mejor (%.0f%% vs %.0f%%)",
			bestPerformer, bestRate, c.getSuccessRateFor(action))
	}

	// Lecciones
	review.Lessons = c.extractLessons(action, success, confidence)

	// ¿Lo cambiaría?
	review.WouldChangeIt = !success || !review.WasOptimal

	// Recomendación para la próxima
	review.RecommendedNext = c.recommendNext(action, success)

	// Thoughts internos (lo que piensa pero no dice)
	review.InternalDoubt = c.generateInternalDoubt(action, success, confidence)

	return review
}

/* =======================
   EXTRAER LECCIONES
   "¿Qué aprendí?"
======================= */

func (c *Critic) extractLessons(action string, success bool, confidence float64) []string {
	lessons := []string{}

	if success {
		if confidence >= 0.8 {
			lessons = append(lessons, "Mi confianza fue justificada. Aprendí a reconocer este patrón")
		} else if confidence >= 0.5 {
			lessons = append(lessons, "Funcionó a pesar de la duda. Aumentar confianza en este tipo de comando")
		}
	} else {
		if confidence >= 0.7 {
			lessons = append(lessons, "Fallé a pesar de estar seguro. Necesito revisar mis criterios")
		} else {
			lessons = append(lessons, "Falló como esperaba. Necesito aprender a detectar esto antes")
		}
	}

	// Detección de patrones de error
	failures := c.analyzer.AnalyzeFailurePatterns()
	for _, f := range failures {
		if f.Action == action && f.Count >= 2 {
			lessons = append(lessons,
				fmt.Sprintf("Patrón detectado: '%s' falla recurrentemente. Causa: %s", action, f.Reason),
			)
		}
	}

	return lessons
}

/* =======================
   RECOMENDAR PRÓXIMA ACCIÓN
======================= */

func (c *Critic) recommendNext(action string, success bool) string {
	if success {
		return "Mantener esta aproximación. Funciona"
	}

	// Si falló, alternativas
	patterns := c.analyzer.AnalyzeSuccessPatterns()
	if len(patterns) > 0 {
		return fmt.Sprintf("Próxima vez, probar '%s' que tiene mejor historial", patterns[0].Action)
	}

	return "Recopilar más datos antes de decidir"
}

/* =======================
   GENERAR PENSAMIENTOS INTERNOS
   Esto NO se muestra, pero afecta la respuesta
======================= */

func (c *Critic) generateInternalDoubt(action string, success bool, confidence float64) string {
	doubtLevel := ""

	if confidence < 0.3 {
		doubtLevel = "Estoy muy inseguro de esta decisión"
	} else if confidence < 0.6 {
		doubtLevel = "Tengo dudas sobre esta acción"
	} else if confidence < 0.8 {
		doubtLevel = "Bastante seguro pero no 100%"
	} else {
		doubtLevel = "Muy seguro de esta decisión"
	}

	// Añadir duda post-evento
	if !success && confidence >= 0.7 {
		return fmt.Sprintf(
			"%s. Pero falló inesperadamente. Mi modelo de confianza puede estar mal calibrado.",
			doubtLevel,
		)
	}

	if success && confidence < 0.5 {
		return fmt.Sprintf(
			"%s. Pero funcionó. Quizás soy demasiado conservador.",
			doubtLevel,
		)
	}

	return doubtLevel
}

/* =======================
   EVALUAR CALIBRACIÓN
   ¿Mis niveles de confianza coinciden con la realidad?
======================= */

func (c *Critic) EvaluateCalibration() map[string]float64 {
	_ = c.analyzer.history.GetLast(20)

	// Simplificar: asumir que el analyzer tiene estos datos
	successPatterns := c.analyzer.AnalyzeSuccessPatterns()

	// Calcular precisión: qué tan bien mis predicciones matchean la realidad
	accuracy := 0.0
	if len(successPatterns) > 0 {
		totalRate := 0.0
		for _, p := range successPatterns {
			totalRate += p.SuccessRate
		}
		accuracy = totalRate / float64(len(successPatterns)) / 100
	}

	return map[string]float64{
		"accuracy":             accuracy,
		"calibration_quality":  0.75, // Placeholder
		"needs_recalibration":  0.5,  // Placeholder
	}
}

/* =======================
   HELPERS
======================= */

func (c *Critic) getSuccessRateFor(action string) float64 {
	patterns := c.analyzer.AnalyzeSuccessPatterns()
	for _, p := range patterns {
		if p.Action == action {
			return p.SuccessRate
		}
	}
	return 0.0
}
