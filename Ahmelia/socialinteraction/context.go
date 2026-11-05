package socialinteraction

import "strings"

// B96.38.2 — current social context only; no historical ownership.
type ContextBuilder struct{}

func NewContextBuilder() *ContextBuilder { return &ContextBuilder{} }
func (b *ContextBuilder) Build(input string, channel string) Context {
	return Context{ParticipantCount: 1, Channel: channel, Activity: "conversation", Formality: inferFormality(input), Public: false, Group: false, Purpose: classifyKind(input)}
}
func inferFormality(s string) Formality {
	l := strings.ToLower(s)
	if strings.Contains(l, "señor") || strings.Contains(l, "señora") || strings.Contains(l, "usted") {
		return Formal
	}
	if strings.Contains(l, "bro") || strings.Contains(l, "man") || strings.Contains(l, "jaja") {
		return Casual
	}
	return Neutral
}
func classifyKind(s string) InteractionKind {
	l := strings.ToLower(strings.TrimSpace(s))
	social := hasAny(l, "hola", "buenos días", "buenas", "gracias", "adiós", "chao", "nos vemos", "perdón", "felicidades", "jaja", "¿cómo estás", "como estas", "qué tal", "que tal")
	task := hasAny(l, "abre ", "crea ", "ejecuta ", "haz ", "puedes abrir", "puedes crear")
	if social && task {
		return Mixed
	}
	if social {
		return Social
	}
	if task {
		return Task
	}
	return Informational
}
func hasAny(s string, xs ...string) bool {
	for _, x := range xs {
		if strings.Contains(s, x) {
			return true
		}
	}
	return false
}
