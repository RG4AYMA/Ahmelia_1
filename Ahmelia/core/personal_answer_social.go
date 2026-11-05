package core

import "ahmelia/socialinteraction"

// personalAnswerSocialDetector adapts the canonical social-intent layer
// (socialinteraction) to the seam that interaction.VerifyAnswerToOpenQuestion
// consumes. The B96.32.15 architecture audit keeps interaction free of
// internal domain dependencies, so the runtime injects the social layer
// instead of interaction importing it: greetings, thanks, small talk and
// acknowledgements remain the social layer's call and can never become
// personal values. The detector is stateless and is queried over the raw
// utterance.
type personalAnswerSocialDetector struct {
	detector *socialinteraction.IntentDetector
}

func (d personalAnswerSocialDetector) IsSocial(text string) bool {
	return d.detector.Detect(text, nil) != socialinteraction.UnknownIntent
}

var answerSocialDetector = personalAnswerSocialDetector{detector: socialinteraction.NewIntentDetector()}
