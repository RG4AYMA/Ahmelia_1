package language

import (
	"fmt"
	"regexp"
	"strings"

	"ahmelia/interaction"
	"ahmelia/shared"
)

// commitmentPattern matches Spanish future-commitment / promise morphology.
// It runs against shared.Normalize(text), which lower-cases and strips
// accents/basic punctuation, so every alternative below is written in
// unaccented, lower-case form on purpose. Do not add accented forms here —
// they will never match after normalization and will silently rot.
var commitmentPattern = regexp.MustCompile(`(?i)\b(voy a|vamos a|hare|haremos|hara|haran|trabajare|seguire trabajando|intentare|buscare la forma|me comprometo|prometo|lograre|lograremos|podre lograrlo|podremos lograrlo)\b`)

// violatesFeedbackInventionGuardrail reports whether response contains
// invented future-commitment language while req is classified as user
// feedback. This is the deterministic, non-LLM enforcement of the existing
// prompt rule at buildConversationalPrompt: "En feedback o crítica del
// usuario ... No prometas cambios futuros ni atribuyas trabajo interno que no
// esté respaldado por los hechos disponibles."
//
// Scope is deliberately narrow: it only fires when
// req.CommunicativeActs.Has(interaction.ActFeedback) is true. It must never
// fire for ResponseIntentPropose turns, where future/commitment language is the
// legitimate, authorized content (see B96.40.5's
// user->commits_to->ahmelia:future_state evidence path).
func violatesFeedbackInventionGuardrail(req interaction.SemanticRequest, response string) bool {
	if !req.CommunicativeActs.Has(interaction.ActFeedback) {
		return false
	}
	return commitmentPattern.MatchString(shared.Normalize(response))
}

// deterministicFeedbackAcknowledge is the last-resort, non-LLM fallback used
// only when the LLM violates the guardrail twice in a row (first generation
// and one retry). It contains zero invented content: it only ever echoes the
// user's own raw input back as an acknowledged observation.
func deterministicFeedbackAcknowledge(req interaction.SemanticRequest) string {
	observation := strings.TrimSpace(req.Raw)
	if observation == "" {
		return "Gracias por decírmelo, lo tengo en cuenta."
	}
	return fmt.Sprintf("Gracias por decírmelo. Tomo en cuenta tu observación: %q.", observation)
}
