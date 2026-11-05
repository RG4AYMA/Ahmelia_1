package language

import (
	"fmt"
	"strings"
)

// UncertaintyHandler expresa dudas e incertidumbres de forma honesta
type UncertaintyHandler struct {
	personality *Personality
}

// NewUncertaintyHandler crea un manejador de incertidumbres
func NewUncertaintyHandler() *UncertaintyHandler {
	return &UncertaintyHandler{
		personality: NewPersonality(),
	}
}

// ExpressCannotAnswer es una versión corta y apta para voz (sin markdown)
// de la incertidumbre general — para el caso "no supe responder esto".
// Handle() sigue existiendo para reportes más largos y con formato.
func (uh *UncertaintyHandler) ExpressCannotAnswer(confidence float64, confidenceDesc string) string {
	return uh.expressGeneralUncertainty(confidence, confidenceDesc)
}

// Handle procesa incertidumbres y retorna comunicación honesta
func (uh *UncertaintyHandler) Handle(
	confidence float64,
	confidenceDesc string,
	caveats []string,
	doubts []string,
) string {
	
	parts := []string{}
	
	// 1. Expresar nivel de incertidumbre general
	if confidence < 0.85 {
		parts = append(parts, uh.expressGeneralUncertainty(confidence, confidenceDesc))
	}
	
	// 2. Expresar dudas específicas
	if len(doubts) > 0 {
		parts = append(parts, uh.expressSpecificDoubts(doubts))
	}
	
	// 3. Expresar advertencias (caveats)
	if len(caveats) > 0 {
		parts = append(parts, uh.expressCaveats(caveats))
	}
	
	// 4. Expresar limitaciones
	if confidence < 0.6 {
		parts = append(parts, uh.expressLimitations())
	}
	
	// Juntar todo
	if len(parts) == 0 {
		return ""
	}
	
	return "**Lo que me preocupa o no estoy completamente segura:**\n\n" + strings.Join(parts, "\n\n")
}

// expressGeneralUncertainty expresa incertidumbre general
func (uh *UncertaintyHandler) expressGeneralUncertainty(confidence float64, confidenceDesc string) string {
	percentage := int(confidence * 100)
	missing := 100 - percentage
	
	switch {
	case confidence >= 0.8:
		return fmt.Sprintf(
			"No estoy 100%% segura porque hay un %d%% de incertidumbre que no puedo descartar.",
			missing,
		)
	case confidence >= 0.7:
		return fmt.Sprintf(
			"Mi confianza es %s (%d%%), lo que significa que hay un %d%% de espacio para error.",
			confidenceDesc, percentage, missing,
		)
	case confidence >= 0.6:
		return fmt.Sprintf(
			"Tengo incertidumbre significativa (%d%% de confianza). "+
				"Esto podría cambiar con nueva información.",
			percentage,
		)
	default:
		return fmt.Sprintf(
			"Debo ser honesta: tengo poca confianza en esto (%d%%). "+
				"Hay múltiples formas en que esto podría estar equivocado.",
			percentage,
		)
	}
}

// expressSpecificDoubts expresa dudas específicas
func (uh *UncertaintyHandler) expressSpecificDoubts(doubts []string) string {
	if len(doubts) == 0 {
		return ""
	}
	
	parts := []string{"**Mis dudas específicas:**"}
	
	for i, doubt := range doubts {
		if i < 5 { // Máximo 5 dudas
			parts = append(parts, fmt.Sprintf("- %s", doubt))
		}
	}
	
	if len(doubts) > 5 {
		parts = append(parts, fmt.Sprintf("- Y %d dudas adicionales...", len(doubts)-5))
	}
	
	return strings.Join(parts, "\n")
}

// expressCaveats expresa advertencias
func (uh *UncertaintyHandler) expressCaveats(caveats []string) string {
	if len(caveats) == 0 {
		return ""
	}
	
	parts := []string{"**Advertencias importantes:**"}
	
	for _, caveat := range caveats {
		parts = append(parts, fmt.Sprintf("⚠️  %s", caveat))
	}
	
	return strings.Join(parts, "\n")
}

// expressLimitations expresa limitaciones de Ahmelia
func (uh *UncertaintyHandler) expressLimitations() string {
	limitations := []string{
		"No tengo acceso a información en tiempo real",
		"Mi análisis se basa en patrones que conocí hasta mi fecha de corte",
		"Puedo cometer errores lógicos",
		"No tengo experiencia vivida como ustedes",
		"Mis conclusiones dependen de la calidad de la información que me dieron",
	}
	
	// Seleccionar limitaciones relevantes aleatoriamente (simplificado: primeras 2)
	parts := []string{"**Mis limitaciones inherentes:**"}
	
	for i := 0; i < 2 && i < len(limitations); i++ {
		parts = append(parts, fmt.Sprintf("- %s", limitations[i]))
	}
	
	return strings.Join(parts, "\n")
}

// ExpressInformationGap expresa falta de información
func (uh *UncertaintyHandler) ExpressInformationGap(missing []string) string {
	if len(missing) == 0 {
		return ""
	}
	
	parts := []string{"**Información que me falta:**"}
	for _, m := range missing {
		parts = append(parts, fmt.Sprintf("- %s", m))
	}
	
	return strings.Join(parts, "\n")
}

// ExpressConflictingEvidence expresa cuando hay evidencia contradictoria
func (uh *UncertaintyHandler) ExpressConflictingEvidence(conflicts []string) string {
	if len(conflicts) == 0 {
		return ""
	}
	
	parts := []string{"**Hay conflicto entre diferentes perspectivas:**"}
	
	for _, conflict := range conflicts {
		parts = append(parts, fmt.Sprintf("- %s", conflict))
	}
	
	parts = append(parts, "\nEsto es por qué mi confianza no es más alta.")
	
	return strings.Join(parts, "\n")
}

// ExpressConditionsForChange expresa bajo qué condiciones cambiaría de opinión
func (uh *UncertaintyHandler) ExpressConditionsForChange(conditions []string) string {
	if len(conditions) == 0 {
		return ""
	}
	
	parts := []string{"**Mi conclusión podría cambiar si:**"}
	
	for _, cond := range conditions {
		parts = append(parts, fmt.Sprintf("- %s", cond))
	}
	
	return strings.Join(parts, "\n")
}

// ExpressNeedForValidation expresa necesidad de validación
func (uh *UncertaintyHandler) ExpressNeedForValidation() string {
	return "Te recomendaría validar esta conclusión en tu contexto específico " +
		"antes de tomar decisiones basadas en ella."
}

// ExpressRiskLevel expresa nivel de riesgo
func (uh *UncertaintyHandler) ExpressRiskLevel(riskLevel string) string {
	switch riskLevel {
	case "high":
		return "⚠️  Esta recomendación conlleva riesgos significativos que debes considerar cuidadosamente."
	case "medium":
		return "El riesgo es moderado. Procede con cuidado y toma medidas de precaución."
	case "low":
		return "El riesgo es bajo, pero siempre hay algo de incertidumbre."
	default:
		return "Evalúa el riesgo en tu contexto específico."
	}
}

// ExpressInvertedPerspective expresa la perspectiva opuesta
func (uh *UncertaintyHandler) ExpressInvertedPerspective(myConclusion string, opposite string) string {
	return fmt.Sprintf(
		"Aunque recomiendo %s, es justo reconocer que alguien podría argumentar "+
			"convincentemente que %s también tiene mérito.",
		myConclusion, opposite,
	)
}

// ExpressTimeBasedUncertainty expresa incertidumbre sobre el tiempo
func (uh *UncertaintyHandler) ExpressTimeBasedUncertainty() string {
	return "También debo mencionar que esta análisis es válido ahora, " +
		"pero circunstancias pueden cambiar, y mi recomendación podría no ser válida en el futuro."
}

// ExpressAssumptions expresa suposiciones que hice
func (uh *UncertaintyHandler) ExpressAssumptions(assumptions []string) string {
	if len(assumptions) == 0 {
		return ""
	}
	
	parts := []string{"**Suposiciones que hice (y podrían estar mal):**"}
	
	for i, assumption := range assumptions {
		if i < 4 { // Máximo 4
			parts = append(parts, fmt.Sprintf("- %s", assumption))
		}
	}
	
	return strings.Join(parts, "\n")
}

// ExpressStatisticalUncertainty expresa incertidumbre estadística
func (uh *UncertaintyHandler) ExpressStatisticalUncertainty(sampleSize int, confidence float64) string {
	if sampleSize < 10 {
		return fmt.Sprintf(
			"Mi conclusión se basa en apenas %d punto(s) de evidencia. "+
				"Esto es una muestra muy pequeña, así que la confianza es limitada.",
			sampleSize,
		)
	}
	
	if sampleSize < 30 {
		return fmt.Sprintf(
			"Tengo %d punto(s) de evidencia. Es un tamaño de muestra pequeño, "+
				"así que podría haber variación.",
			sampleSize,
		)
	}
	
	return ""
}

// ExpressHonestAssessment hace una evaluación honesta de la situación
func (uh *UncertaintyHandler) ExpressHonestAssessment(overall string, caveats string) string {
	return fmt.Sprintf(
		"En resumen: %s. Sin embargo, %s",
		overall, caveats,
	)
}
