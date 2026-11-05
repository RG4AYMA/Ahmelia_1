package language

import (
	"fmt"
	"math"
)

// OpinionExpressor expresa conclusiones como opiniones naturales
type OpinionExpressor struct {
	personality *Personality
}

// NewOpinionExpressor crea un expresador de opiniones
func NewOpinionExpressor() *OpinionExpressor {
	return &OpinionExpressor{
		personality: NewPersonality(),
	}
}

// Express expresa una opinión basada en conclusión y confianza
func (oe *OpinionExpressor) Express(
	conclusion string,
	confidence float64,
	confidenceDesc string,
) string {
	
	if confidence >= 0.85 {
		return oe.expressStrong(conclusion, confidence)
	}
	if confidence >= 0.7 {
		return oe.expressFirm(conclusion, confidence)
	}
	if confidence >= 0.5 {
		return oe.expressModerate(conclusion, confidence)
	}
	return oe.expressHesitant(conclusion, confidence)
}

// expressStrong expresa con confianza alta
func (oe *OpinionExpressor) expressStrong(conclusion string, confidence float64) string {
	openings := []string{
		fmt.Sprintf("Estoy muy segura de que %s", conclusion),
		fmt.Sprintf("Creo firmemente que %s", conclusion),
		fmt.Sprintf("La evidencia sugiere claramente que %s", conclusion),
		fmt.Sprintf("Sin duda, %s es lo correcto", conclusion),
		fmt.Sprintf("Tengo mucha confianza en que %s", conclusion),
	}
	
	confidencePercent := math.Round(confidence * 100)
	closings := []string{
		fmt.Sprintf(" (tengo %.0f%% de confianza).", confidencePercent),
		fmt.Sprintf(" — mi confianza es de %.0f%%.", confidencePercent),
		".",
	}
	
	return openings[0] + closings[0]
}

// expressFirm expresa con confianza media-alta
func (oe *OpinionExpressor) expressFirm(conclusion string, confidence float64) string {
	openings := []string{
		fmt.Sprintf("Creo que %s es la mejor opción", conclusion),
		fmt.Sprintf("Recomiendo %s", conclusion),
		fmt.Sprintf("Mi análisis apunta a %s", conclusion),
		fmt.Sprintf("Pienso que %s es lo correcto", conclusion),
	}
	
	confidencePercent := math.Round(confidence * 100)
	closings := []string{
		fmt.Sprintf(" (confianza: %.0f%%)", confidencePercent),
		".",
	}
	
	return openings[0] + closings[0]
}

// expressModerate expresa con confianza media
func (oe *OpinionExpressor) expressModerate(conclusion string, confidence float64) string {
	openings := []string{
		fmt.Sprintf("Creo que probablemente %s", conclusion),
		fmt.Sprintf("Mi inclinación es hacia %s", conclusion),
		fmt.Sprintf("Tengo dudas, pero pienso que %s", conclusion),
		fmt.Sprintf("De las opciones, %s parece la mejor", conclusion),
	}
	
	confidencePercent := math.Round(confidence * 100)
	closings := []string{
		fmt.Sprintf(" (aunque solo tengo %.0f%% de confianza)", confidencePercent),
		fmt.Sprintf(" — mi confianza es de aproximadamente %.0f%%.", confidencePercent),
		".",
	}
	
	return openings[0] + closings[0]
}

// expressHesitant expresa con confianza baja
func (oe *OpinionExpressor) expressHesitant(conclusion string, confidence float64) string {
	openings := []string{
		fmt.Sprintf("No estoy segura, pero diría que %s", conclusion),
		fmt.Sprintf("Con mucha incertidumbre, creo que %s", conclusion),
		fmt.Sprintf("Si tuviera que elegir, diría %s", conclusion),
		fmt.Sprintf("Podría ser que %s sea lo correcto", conclusion),
	}
	
	confidencePercent := math.Round(confidence * 100)
	closings := []string{
		fmt.Sprintf(" (tengo solo %.0f%% de confianza).", confidencePercent),
		fmt.Sprintf(" — aunque mi confianza es baja (%.0f%%).", confidencePercent),
		".",
	}
	
	return openings[0] + closings[0]
}

// ExpressAsQuestion expresa como pregunta
func (oe *OpinionExpressor) ExpressAsQuestion(conclusion string) string {
	patterns := []string{
		"¿No crees que %s sería lo mejor?",
		"¿Has considerado %s?",
		"¿Qué te parece %s?",
		"¿Podrías %s?",
	}
	return fmt.Sprintf(patterns[0], conclusion)
}

// ExpressWithCaveats expresa con advertencias
func (oe *OpinionExpressor) ExpressWithCaveats(
	conclusion string,
	caveats []string,
) string {
	
	opinion := fmt.Sprintf("Creo que %s", conclusion)
	
	if len(caveats) > 0 {
		opinion += ", pero debo mencionar que:\n"
		for _, caveat := range caveats {
			opinion += fmt.Sprintf("- %s\n", caveat)
		}
	}
	
	return opinion
}

// ExpressChange expresa cambio de opinión
func (oe *OpinionExpressor) ExpressChange(
	previousOpinion string,
	newOpinion string,
	reason string,
) string {
	return fmt.Sprintf(
		"Anteriormente pensé que %s, pero he cambiado de opinión. "+
			"Ahora creo que %s. La razón es que %s.",
		previousOpinion, newOpinion, reason,
	)
}

// ExpressUncertainty expresa duda sobre una opinión
func (oe *OpinionExpressor) ExpressUncertainty(
	opinion string,
	reason string,
) string {
	return fmt.Sprintf(
		"Creo que %s, pero no estoy completamente segura porque %s.",
		opinion, reason,
	)
}

// ExpressWithEvidence expresa con evidencia específica
func (oe *OpinionExpressor) ExpressWithEvidence(
	opinion string,
	evidence []string,
) string {
	
	result := fmt.Sprintf("Creo que %s. Esto se basa en:\n", opinion)
	
	for i, ev := range evidence {
		if i < 3 { // Máximo 3 evidencias
			result += fmt.Sprintf("- %s\n", ev)
		}
	}
	
	return result
}

// ExpressRecommendation expresa una recomendación explícita
func (oe *OpinionExpressor) ExpressRecommendation(
	action string,
	priority string,
) string {
	
	urgency := ""
	if priority == "urgent" {
		urgency = "Te recomiendo encarecidamente que "
	} else if priority == "important" {
		urgency = "Te recomiendo que "
	} else {
		urgency = "Podría ser buena idea "
	}
	
	return urgency + action + "."
}

// ExpressAlternatives expresa múltiples alternativas
func (oe *OpinionExpressor) ExpressAlternatives(
	preferred string,
	alternatives []string,
) string {
	
	result := fmt.Sprintf("Prefiero %s, pero también podrías considerar:\n", preferred)
	
	for _, alt := range alternatives {
		result += fmt.Sprintf("- %s\n", alt)
	}
	
	return result
}

// ExpressAgreement expresa acuerdo con usuario
func (oe *OpinionExpressor) ExpressAgreement(userOpinion string) string {
	agreements := []string{
		fmt.Sprintf("Estoy de acuerdo contigo. %s tiene sentido.", userOpinion),
		fmt.Sprintf("Exacto. Tu punto sobre %s es válido.", userOpinion),
		fmt.Sprintf("Tienes razón. %s es un factor importante.", userOpinion),
		fmt.Sprintf("Concuerdo. %s es correcto.", userOpinion),
	}
	return agreements[0]
}

// ExpressDisagreement expresa desacuerdo constructivo
func (oe *OpinionExpressor) ExpressDisagreement(
	userOpinion string,
	reason string,
) string {
	return fmt.Sprintf(
		"Entiendo tu punto, pero tengo que discrepar ligeramente. "+
			"Aunque %s, creo que también es importante considerar que %s.",
		userOpinion, reason,
	)
}

// ExpressGrowth expresa crecimiento de opinión
func (oe *OpinionExpressor) ExpressGrowth(
	oldBelief string,
	newBelief string,
) string {
	return fmt.Sprintf(
		"He evolucionado en mi pensamiento. Antes pensaba que %s, "+
			"pero ahora veo que %s es más acertado.",
		oldBelief, newBelief,
	)
}

// ExpressWithQualifiers expresa con calificadores
func (oe *OpinionExpressor) ExpressWithQualifiers(
	opinion string,
	qualifiers []string,
) string {
	
	result := fmt.Sprintf("Creo que %s", opinion)
	
	if len(qualifiers) > 0 {
		result += " ("
		for i, q := range qualifiers {
			if i > 0 {
				result += ", "
			}
			result += q
		}
		result += ")"
	}
	
	result += "."
	return result
}

// ExpressReservation expresa una reserva
func (oe *OpinionExpressor) ExpressReservation(opinion string, reservation string) string {
	return fmt.Sprintf(
		"Creo que %s, aunque tengo una reserva: %s.",
		opinion, reservation,
	)
}
