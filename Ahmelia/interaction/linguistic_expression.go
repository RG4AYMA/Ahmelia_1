package interaction

import "strings"

// ExpressionProfile describes HOW a response should be communicated without
// containing the response itself. It is intentionally language-agnostic:
// language is selected separately from meaning, and the same profile can be
// rendered by text, voice, UI, or a future embodied interface.
type ExpressionProfile struct {
	Language           string
	Formality          float64
	Warmth             float64
	Directness         float64
	Detail             string
	AskFollowUp        bool
	AcknowledgeFirst   bool
	PreserveQuestion   bool
	PreserveFormatting bool
}

// BuildExpressionProfile converts communicative intent and semantic meaning
// into a stable expression contract. It does not generate language.
func BuildExpressionProfile(intent ResponseIntent, meaning SemanticMeaning, formality, warmth, initiative float64, detail string) ExpressionProfile {
	language := strings.TrimSpace(intent.Language)
	if language == "" {
		language = strings.TrimSpace(meaning.Language)
	}

	p := ExpressionProfile{
		Language:           language,
		Formality:          clamp01(formality),
		Warmth:             clamp01(warmth),
		Directness:         0.65,
		Detail:             detail,
		AskFollowUp:        intent.Kind == ResponseIntentAsk || intent.Kind == ResponseIntentInvite || intent.NeedsUserInput,
		AcknowledgeFirst:   intent.Kind == ResponseIntentAcknowledge || intent.Kind == ResponseIntentResume,
		PreserveQuestion:   intent.AddressesQuestion,
		PreserveFormatting: true,
	}

	// A direct answer should be clearer and less decorative than an open
	// social/collaborative turn. These are expression parameters, not canned
	// sentences.
	switch intent.Kind {
	case ResponseIntentAnswer, ResponseIntentClarify:
		p.Directness = 0.80
	case ResponseIntentPropose, ResponseIntentInvite:
		p.Directness = 0.55
	case ResponseIntentAcknowledge:
		p.Directness = 0.70
	}

	// High initiative permits a follow-up, but it never forces one when the
	// response is already resolving a question that still needs processing.
	if initiative < 0.35 && intent.Kind != ResponseIntentAsk {
		p.AskFollowUp = false
	}

	return p
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}
