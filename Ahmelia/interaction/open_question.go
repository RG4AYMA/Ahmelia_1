package interaction

import (
	"strings"
	"time"
	"unicode"
)

// OpenQuestion is the durable record of a question Ahmelia asked and is still
// waiting to be answered. It is the only door through which a personal answer
// can enter the user model: an utterance is admitted as an answer only if it
// is a verified response to one of these questions. A question Ahmelia never
// asked can never open this door, so a user's own question cannot fabricate
// data no matter how it gets classified downstream.
type OpenQuestion struct {
	ID           uint64             `json:"id"`
	SessionID    string             `json:"session_id,omitempty"`
	Kind         string             `json:"kind,omitempty"`
	Category     string             `json:"category,omitempty"`
	Key          string             `json:"key,omitempty"`
	Question     string             `json:"question,omitempty"`
	AskedTurn    uint64             `json:"asked_turn,omitempty"`
	AskedAt      time.Time          `json:"asked_at,omitempty"`
	Status       OpenQuestionStatus `json:"status"`
	AnsweredTurn uint64             `json:"answered_turn,omitempty"`
	ClosedAt     time.Time          `json:"closed_at,omitempty"`
}

type OpenQuestionStatus string

const (
	OpenQuestionOpen      OpenQuestionStatus = "open"
	OpenQuestionAnswered  OpenQuestionStatus = "answered"
	OpenQuestionCancelled OpenQuestionStatus = "cancelled"
	OpenQuestionExpired   OpenQuestionStatus = "expired"
)

// OpenQuestionKindPersonalValue marks a question that asks the user for a
// personal value (preference or attribute). Domain-specific questions
// (goal, capability, knowledge) keep their own normalization contracts and
// are not registered here yet.
const OpenQuestionKindPersonalValue = "personal_value"

// Freshness window of an open question. Same session, close in time and close
// in turns: outside this envelope the question is no longer meaningful as the
// anchor of an answer and expires instead of admitting data.
const (
	OpenQuestionMaxAge                 = 12 * time.Hour
	OpenQuestionMaxTurnDistance uint64 = 6
)

// SocialDetector is the seam through which the runtime injects the canonical
// social-intent layer without making interaction import a domain authority:
// the B96.32.15 audit allows interaction only the standard library plus the
// audited leaf utilities (shared, ownership). A nil detector cannot attest
// that the utterance is not social, and verification fails closed rather
// than admitting a value the layer never saw.
type SocialDetector interface {
	IsSocial(text string) bool
}

// VerifyAnswerToOpenQuestion decides whether the current turn is a valid
// answer to Ahmelia's open question. Every signal is a deterministic rule over
// existing semantic classifications (communicative acts + the canonical
// social-intent layer injected by the runtime); no model, and no list of
// scene phrases, takes part.
//
// The rules are deliberately biased towards rejection: rejecting a true answer
// only leaves the question open, while admitting a non-answer would fabricate
// user data. A question-shaped utterance is never an answer (this covers the
// user's own questions and question-shaped interjections); orders, social
// acts, confirmations, corrections and negated preferences are never values.
func VerifyAnswerToOpenQuestion(q OpenQuestion, req SemanticRequest, now time.Time, social SocialDetector) (PersonalAnswer, bool) {
	if q.Status != OpenQuestionOpen || strings.TrimSpace(q.Key) == "" {
		return PersonalAnswer{}, false
	}
	text := strings.TrimSpace(req.Raw)
	if text == "" {
		return PersonalAnswer{}, false
	}

	// Question-shaped utterances are not values, even when they arrive right
	// after Ahmelia's question.
	if isLikelyQuestion(text) {
		return PersonalAnswer{}, false
	}

	acts := CanonicalActs(req)

	// Hard rejects: acts that make a value answer impossible. ActFeedback is
	// checked after these because "gracias" also carries feedback evidence.
	for _, reject := range []CommunicativeAct{
		ActQuestion, ActOpinionRequest, ActCommand, ActControl,
		ActSocial, ActRejection, ActCorrection, ActConfirmation,
		ActAuthorizationInquiry,
	} {
		if acts.Has(reject) {
			return PersonalAnswer{}, false
		}
	}

	// Social intents already classified by the canonical social layer
	// (acknowledgements, apologies, thanks, greetings) are never values; a
	// missing layer fails closed.
	if social == nil || social.IsSocial(text) {
		return PersonalAnswer{}, false
	}

	n := normalizeForClassification(req.Normalized)
	if n == "" {
		n = normalizeForClassification(text)
	}

	// Explicit first-person declarations of the same shape the declaration
	// path owns ("mi X es Y"): admitting them here as well would duplicate the
	// write. They are matched by the existing semantic attribute resolver with
	// their own extracted value.
	if attribute, ok := ResolveSemanticAttribute(req); ok && attribute.HasValue && attribute.Subject == UserObservationSubjectUser {
		return PersonalAnswer{}, false
	}

	// Preference/declaration answers: the user states a value with a
	// first-person predicate ("me gusta X", "prefiero X"). Confidence is
	// derived from the act, not from word counting.
	if acts.Has(ActFeedback) || acts.Has(ActDeclaration) {
		confidence := .86
		if acts.Has(ActFeedback) {
			// "no me gusta X" is not a value; "me gusta" with no object is
			// an incomplete predicate and carries nothing.
			if strings.HasPrefix(n, "no ") {
				return PersonalAnswer{}, false
			}
			if n == "me gusta" || n == "me gustan" || strings.HasSuffix(n, " gusta") || strings.HasSuffix(n, " gustan") {
				return PersonalAnswer{}, false
			}
			confidence = .90
		}
		return PersonalAnswer{OK: true, Category: q.Category, Key: q.Key, Value: text, Confidence: confidence}, true
	}

	// Compact answers: a short value-shaped utterance ("café", "la gaseosa de
	// piña") with no act evidence other than continuity or insufficient
	// evidence. Anything the semantic layer already classified as a non-value
	// act (information, proposal, exploration, anaphora...) is not a value.
	for _, ev := range acts.Evidence {
		switch ev.Act {
		case ActUnknown, ActContinuation, ActReference:
			// neutral for a compact value
		default:
			return PersonalAnswer{}, false
		}
	}
	if !hasContentToken(n, 4) {
		return PersonalAnswer{}, false
	}
	return PersonalAnswer{OK: true, Category: q.Category, Key: q.Key, Value: text, Confidence: .82}, true
}

// hasContentToken reports whether the normalized text contains a run of at
// least min letters/digits. It rejects value-less noise ("ok", "no sé") from
// the compact path without counting words.
func hasContentToken(n string, min int) bool {
	count := 0
	for _, r := range n {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			count++
			if count >= min {
				return true
			}
			continue
		}
		count = 0
	}
	return false
}
