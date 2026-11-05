package interaction

import "strings"

// SemanticConcept is a language-neutral communicative concept. It describes
// meaning, not the language used to express it and not the capability that may
// eventually satisfy it.
type SemanticConcept string

const (
	ConceptUnknown      SemanticConcept = ""
	ConceptQuestion     SemanticConcept = "question"
	ConceptCommand      SemanticConcept = "command"
	ConceptCorrection   SemanticConcept = "correction"
	ConceptConfirmation SemanticConcept = "confirmation"
	ConceptRejection    SemanticConcept = "rejection"
	ConceptSuggestion   SemanticConcept = "suggestion"
	ConceptProposal     SemanticConcept = "proposal"
	ConceptContinuation SemanticConcept = "continuation"
	ConceptInterruption SemanticConcept = "interruption"
	ConceptReference    SemanticConcept = "reference"
	ConceptClarify      SemanticConcept = "clarification"
	ConceptOpinion      SemanticConcept = "opinion"
	ConceptPreference   SemanticConcept = "preference"
	ConceptObservation  SemanticConcept = "observation"
	ConceptEmotion      SemanticConcept = "emotion_signal"
	ConceptFeedback     SemanticConcept = "feedback"
	ConceptAgreement    SemanticConcept = "agreement"
	ConceptDisagreement SemanticConcept = "disagreement"
	ConceptInformation  SemanticConcept = "information"
	ConceptNarrative    SemanticConcept = "narrative"
	ConceptSocial       SemanticConcept = "social"
	ConceptPlanning     SemanticConcept = "planning"
	ConceptDecision     SemanticConcept = "decision"
	ConceptRequest      SemanticConcept = "request"
)

// SemanticMeaning is the canonical, language-independent interpretation of a
// human utterance. It intentionally keeps the original text separate from
// the concepts inferred from it.
type SemanticMeaning struct {
	Concepts       []SemanticConcept
	Primary        SemanticConcept
	Language       string
	Confidence     float64
	Ambiguous      bool
	Evidence       []string
	SourceText     string
	NormalizedText string
	Query          SemanticQueryType
	Attribute      string
}

// BuildSemanticMeaning converts the existing general interaction semantics
// into a language-neutral representation. It does not select a provider,
// execute work, or generate a response.
func BuildSemanticMeaning(r SemanticRequest) SemanticMeaning {
	m := SemanticMeaning{
		Language:       r.Language,
		Confidence:     r.Confidence,
		SourceText:     r.Raw,
		NormalizedText: r.Normalized,
	}

	add := func(c SemanticConcept, evidence string) {
		if c == ConceptUnknown {
			return
		}
		for _, existing := range m.Concepts {
			if existing == c {
				if evidence != "" {
					m.Evidence = append(m.Evidence, evidence)
				}
				return
			}
		}
		m.Concepts = append(m.Concepts, c)
		if evidence != "" {
			m.Evidence = append(m.Evidence, evidence)
		}
	}

	// CommunicativeActs is the sole interaction-level semantic authority.
	// Meaning is a language-neutral projection of that contract, never a second
	// classifier built from legacy Kind/SpeechAct fields.
	acts := CanonicalActs(r)
	for _, act := range append([]CommunicativeAct{acts.Primary}, acts.Secondary...) {
		switch act {
		case ActQuestion:
			add(ConceptQuestion, "communicative_act")
		case ActCommand:
			add(ConceptCommand, "communicative_act")
		case ActCorrection:
			add(ConceptCorrection, "communicative_act")
		case ActConfirmation:
			add(ConceptConfirmation, "communicative_act")
		case ActRejection:
			add(ConceptRejection, "communicative_act")
		case ActProposal:
			add(ConceptProposal, "communicative_act")
		case ActIdea:
			add(ConceptSuggestion, "communicative_act")
		case ActExploration:
			add(ConceptPlanning, "communicative_act")
		case ActContinuation:
			add(ConceptContinuation, "communicative_act")
		case ActReference, ActAnaphora:
			add(ConceptReference, "communicative_act")
		case ActSocial:
			add(ConceptSocial, "communicative_act")
		case ActControl:
			add(ConceptInterruption, "communicative_act")
		case ActLearning:
			add(ConceptInformation, "communicative_act")
		case ActFeedback:
			add(ConceptFeedback, "communicative_act")
		case ActObservation:
			add(ConceptObservation, "communicative_act")
		case ActDeclaration:
			add(ConceptInformation, "communicative_act")
		case ActDecision:
			add(ConceptDecision, "communicative_act")
		case ActOpinionRequest:
			add(ConceptOpinion, "communicative_act")
		}
	}

	if r.FollowUp || r.Continues {
		add(ConceptContinuation, "conversation_link")
	}
	if len(r.References) > 0 {
		add(ConceptReference, "reference_resolution")
	}
	if r.Correction.IsCorrection {
		add(ConceptCorrection, "correction_analysis")
	}
	if r.UserSignals.Fatigue > 0 || r.UserSignals.Frustration > 0 ||
		r.UserSignals.Positive > 0 || r.UserSignals.Urgency > 0 {
		add(ConceptEmotion, "user_state_signal")
	}

	if r.QuestionType != "" {
		add(ConceptQuestion, "question_type")
	}
	if strings.TrimSpace(r.Objective.Text) != "" && acts.Has(ActProposal) {
		add(ConceptPlanning, "objective")
	}

	if len(m.Concepts) > 0 {
		m.Primary = m.Concepts[0]
	}
	m.Ambiguous = r.Confidence > 0 && r.Confidence < 0.55
	return m
}

// SemanticallyEquivalent compares two utterances at the interaction boundary.
// It deliberately ignores wording and language while retaining the primary
// communicative concepts. This is useful for proving that Spanish and English
// inputs converge before cognition/work/capability routing.
func SemanticallyEquivalent(a, b SemanticMeaning) bool {
	if a.Primary != b.Primary {
		return false
	}
	if len(a.Concepts) != len(b.Concepts) {
		return false
	}
	for _, x := range a.Concepts {
		found := false
		for _, y := range b.Concepts {
			if x == y {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}
