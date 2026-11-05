package socialinteraction

import "strings"

type IntentDetector struct{}

func NewIntentDetector() *IntentDetector { return &IntentDetector{} }
func (d *IntentDetector) Detect(input string, signals []Signal) SocialIntent {
	l := strings.ToLower(input)
	switch {
	case hasAny(l, "hola", "buenos días", "buenas"):
		return Greeting
	case hasAny(l, "adiós", "chao", "nos vemos", "hasta luego"):
		return Farewell
	case hasAny(l, "gracias", "te agradezco"):
		return Thanking
	case hasAny(l, "perdón", "lo siento"):
		return Apology
	case hasAny(l, "felicidades", "felicitaciones"):
		return Congratulation
	case hasAny(l, "¿cómo estás", "como estas", "qué tal", "que tal"):
		return CheckIn
	case hasAny(l, "jaja", "jajaja", "😂"):
		return Humor
	case hasAny(l, "me siento mal", "estoy triste", "necesito apoyo"):
		return EmotionalSupportRequest
	case hasAny(l, "ven conmigo", "quieres venir", "¿vienes"):
		return SocialInvitation
	case hasAny(l, "ok", "vale", "entiendo", "de acuerdo"):
		return Acknowledgment
	}
	if len(signals) > 0 {
		return SocialComment
	}
	return UnknownIntent
}
