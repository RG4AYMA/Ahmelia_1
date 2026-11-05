package interaction

import (
	"regexp"
	"strings"

	"ahmelia/ownership"
)

// CorrectionInfo is the structured representation of a user repairing,
// retracting, negating, or replacing previously understood meaning.
//
// It deliberately does not execute anything. A correction changes the
// semantic interpretation of the current turn and gives downstream layers
// enough information to avoid acting on the meaning the user just rejected.
type CorrectionInfo struct {
	IsCorrection bool
	IsNegation   bool
	Target       string
	Replacement  string
	Reason       string
	Confidence   float64
}

// AnalyzeCorrection identifies explicit conversational repairs. Ordinary
// negative statements such as "no funciona" are NOT corrections: they report
// a state. Likewise "no abras Chrome" is a control instruction handled by
// the normal pause/stop precedence. A correction requires evidence that the
// user is replacing or rejecting an interpretation.
func AnalyzeCorrection(raw, normalized string) CorrectionInfo {
	n := strings.TrimSpace(normalized)
	if n == "" {
		return CorrectionInfo{}
	}

	// Strong replacement forms. Capture the rejected and intended meanings
	// when the user explicitly provides both.
	replacementPatterns := []*regexp.Regexp{
		regexp.MustCompile(`^(?:no[,.]?\s+)?(?:me refer[ií]a|quise decir|quer[ií]a decir|quiero decir)\s+(?:a\s+)?(.+)$`),
		regexp.MustCompile(`^no\s+(.+?)\s*,?\s*(?:sino|si no)\s+(?:a\s+)?(.+)$`),
		regexp.MustCompile(`^(.+?)\s+(?:no era|no es)\s+(?:a\s+)?(.+)$`),
		regexp.MustCompile(`^(?:en realidad|en realidad,)\s+(?:a\s+)?(.+)$`),
	}
	for i, p := range replacementPatterns {
		m := p.FindStringSubmatch(n)
		if len(m) == 0 {
			continue
		}
		if i == 1 {
			return CorrectionInfo{
				IsCorrection: true,
				IsNegation:   true,
				Target:       strings.TrimSpace(m[1]),
				Replacement:  strings.TrimSpace(m[2]),
				Reason:       "explicit replacement: sino",
				Confidence:   0.98,
			}
		}
		if i == 2 {
			return CorrectionInfo{
				IsCorrection: true,
				IsNegation:   true,
				Target:       strings.TrimSpace(m[1]),
				Replacement:  strings.TrimSpace(m[2]),
				Reason:       "explicit rejection of prior meaning",
				Confidence:   0.96,
			}
		}
		return CorrectionInfo{
			IsCorrection: true,
			Replacement:  strings.TrimSpace(m[1]),
			Reason:       "explicit conversational repair",
			Confidence:   0.98,
		}
	}

	// Compound reformulations are common when a user first rejects the current
	// direction and then states the replacement in the same turn. The linguistic
	// structure is intentionally generic: rejection marker + explicit
	// reformulation cue + replacement. It does not identify any capability or
	// domain.
	compoundPatterns := []*regexp.Regexp{
		regexp.MustCompile(`^no\b.*\b(?:cambia(?:r)?|cambiemos|prefiero(?:\s+que)?|quiero\s+que)\s*(?:el|la)?\s*[^:]*:\s*(?:quiero\s+que|prefiero\s+que)\s+(.+)$`),
		regexp.MustCompile(`^no\b.*\b(?:cambia(?:r)?|cambiemos)\b.*?\b(?:quiero|prefiero)\s+que\s+(.+)$`),
	}
	for _, p := range compoundPatterns {
		m := p.FindStringSubmatch(n)
		if len(m) == 2 {
			replacement := strings.TrimSpace(strings.Trim(m[1], " .!?¿¡,;:"))
			if replacement != "" {
				return CorrectionInfo{
					IsCorrection: true,
					IsNegation:   true,
					Replacement:  replacement,
					Reason:       "compound reformulation after rejected direction",
					Confidence:   0.96,
				}
			}
		}
	}

	// Ownership corrections repair who a fact belongs to, not merely wording.
	// Example: "pero es mi color favorito, no el tuyo" rejects an assistant
	// statement that attributed a user fact to Ahmelia. Keep this explicit so
	// the whole chain can repair ownership before response generation/memory.
	ownershipPatterns := []*regexp.Regexp{
		regexp.MustCompile(`^(?:pero[,.]?\s+)?(?:es|son)\s+(?:mi|mis|mio|mío)\s+(.+?)\s*,?\s*no\s+(?:el|la|los|las)?\s*(?:tuyo|tuya|tuyos|tuyas)$`),
	}
	for _, p := range ownershipPatterns {
		m := p.FindStringSubmatch(n)
		if len(m) == 0 {
			continue
		}
		return CorrectionInfo{
			IsCorrection: true,
			IsNegation:   true,
			Replacement:  "mi " + strings.TrimSpace(m[1]),
			Reason:       "ownership correction: user-owned fact was attributed to self",
			Confidence:   0.99,
		}
	}

	// Short forms are common in speech and should remain explicit rather than
	// inferred from every occurrence of "no".
	for _, prefix := range []string{
		"no, me referia a ", "no, me refería a ",
		"no, queria decir ", "no, quería decir ",
		"no era eso, ", "no es eso, ",
		"that's not what i meant, ", "i meant ",
	} {
		if strings.HasPrefix(n, prefix) {
			replacement := strings.TrimSpace(strings.TrimPrefix(n, prefix))
			if replacement != "" {
				return CorrectionInfo{
					IsCorrection: true,
					IsNegation:   true,
					Replacement:  replacement,
					Reason:       "explicit spoken correction",
					Confidence:   0.97,
				}
			}
		}
	}

	return CorrectionInfo{}
}

// IsCorrection reports whether the already-parsed semantic request represents
// a correction. It exists so legacy stateful branches can consume the shared
// SemanticRequest contract without importing or re-running the old
// comprehension classifier.

// HasConcreteCorrectionReplacement reports whether a correction replacement
// contains enough semantic substance to become an executable repair target.
// Purely deictic/meta-language such as "eso", "otra cosa" or "lo que quería"
// can reject the previous meaning, but cannot safely select a new materialized
// objective. Those turns must remain conversational/clarifying until a concrete
// replacement is supplied.
func HasConcreteCorrectionReplacement(replacement string) bool {
	n := strings.TrimSpace(normalize(replacement))
	n = strings.Trim(n, " .!?¿¡,;:")
	if n == "" {
		return false
	}
	for _, vague := range []string{
		"eso", "aquello", "esa cosa", "esa otra cosa", "otra cosa",
		"algo distinto", "algo diferente", "lo mismo",
		"lo que queria", "lo que quería", "lo que quiero",
		"lo que quise decir", "como queria", "como quería",
		"como dije", "como antes",
	} {
		if n == normalize(vague) {
			return false
		}
	}
	return true
}

func IsCorrection(r SemanticRequest) bool {
	return r.Correction.IsCorrection || CanonicalActs(r).Has(ActCorrection)
}

// ApplyCorrection produces the semantic surface that should be considered
// after the user repairs their meaning. Raw remains untouched for audit and
// conversational memory. This is intentionally conservative: when there is
// no replacement, the original turn remains the source of truth.
func ApplyCorrection(r SemanticRequest, previous string) SemanticRequest {
	if !r.Correction.IsCorrection || strings.TrimSpace(r.Correction.Replacement) == "" {
		return r
	}
	replacement := strings.TrimSpace(r.Correction.Replacement)
	r.Objective.Text = replacement
	// Re-establish ownership on the repaired semantic surface exactly once.
	// This prevents a correction from preserving an empty/wrong subject.
	resolution := ownership.Resolve(replacement)
	r.Subject = UserObservationSubject(resolution.Subject)
	r.SubjectConfidence = resolution.Confidence
	r.SubjectEvidence = append([]ownership.Evidence(nil), resolution.Evidence...)
	if r.Subject == "" {
		r.Subject = UserObservationSubjectUser
		r.SubjectConfidence = 0.50
	}
	r.Action = inferAction(normalizeForClassification(replacement))
	r.RequestedAction = requestedActionFor(r.Kind, normalizeForClassification(replacement))
	r.Entities = extractEntities(normalizeForClassification(replacement))
	r.Topic = inferConversationTopic(normalize(replacement))
	r.Confidence = r.Correction.Confidence

	// A correction is still a correction at the interaction boundary; the
	// downstream layer can inspect CorrectionTarget/Replacement instead of
	// pretending the repaired text was the original utterance.
	r.SpeechAct = SpeechCorrect
	if strings.TrimSpace(previous) != "" {
		r.Correction.Target = strings.TrimSpace(previous)
	}
	return r
}
