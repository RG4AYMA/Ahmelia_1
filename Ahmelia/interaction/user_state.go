package interaction

import (
	"math"
	"strings"
	"time"
)

// UserState captures communication signals inferred from the current turn.
// These are interaction signals, not psychological or medical diagnoses.
type UserState struct {
	Frustration     float64         `json:"frustration"`
	Enthusiasm      float64         `json:"enthusiasm"`
	Confusion       float64         `json:"confusion"`
	Urgency         float64         `json:"urgency"`
	Engagement      float64         `json:"engagement"`
	Confidence      float64         `json:"confidence"`
	Humor           float64         `json:"humor"`
	InteractionNeed InteractionNeed `json:"interaction_need"`
	UpdatedAt       time.Time       `json:"updated_at"`
}

// UserStateAssessment contains the current signals and their change from the
// immediately preceding conversational state.
type UserStateAssessment struct {
	Current  UserState
	Previous UserState
	Delta    UserStateDelta
	Evidence []string
}

// UserStateDelta describes movement rather than only a static label.
type UserStateDelta struct {
	Frustration float64
	Enthusiasm  float64
	Confusion   float64
	Urgency     float64
	Engagement  float64
	Confidence  float64
	Humor       float64
}

type InteractionNeed string

const (
	NeedNone           InteractionNeed = ""
	NeedAnswer         InteractionNeed = "answer"
	NeedExplanation    InteractionNeed = "explanation"
	NeedConfirmation   InteractionNeed = "confirmation"
	NeedClarification  InteractionNeed = "clarification"
	NeedReassurance    InteractionNeed = "reassurance"
	NeedAssistance     InteractionNeed = "assistance"
	NeedProposal       InteractionNeed = "proposal"
	NeedContinuation   InteractionNeed = "continuation"
	NeedInterruption   InteractionNeed = "interruption"
	NeedCorrection     InteractionNeed = "correction"
	NeedAcknowledgment InteractionNeed = "acknowledgment"
	NeedConversation   InteractionNeed = "conversation"
)

// AnalyzeUserState derives only observable conversational signals from text
// and semantic request data. It intentionally does not claim to know the
// user's internal emotional state.
func AnalyzeUserState(req SemanticRequest, ctx ConversationSnapshot) UserStateAssessment {
	prev := ctx.UserState
	raw := strings.TrimSpace(req.Raw)
	lower := strings.ToLower(raw)

	s := UserState{
		Engagement: 0.55,
		Confidence: clampUserState01(req.Confidence),
		UpdatedAt:  time.Now(),
	}
	// SemanticRequest is the canonical extraction boundary for observable
	// language signals. Preserve those explicit values in the turn state so
	// later cognition does not have to re-parse a narrower set of phrases.
	s.Frustration = max(s.Frustration, clampUserState01(req.UserSignals.Frustration))
	s.Urgency = max(s.Urgency, clampUserState01(req.UserSignals.Urgency))
	s.Enthusiasm = max(s.Enthusiasm, clampUserState01(req.UserSignals.Positive))

	if raw != "" {
		s.Engagement = 0.70
	}
	if CanonicalActs(req).Has(ActSocial) || CanonicalActs(req).Has(ActProposal) {
		s.Engagement = 0.78
	}
	if CanonicalActs(req).Has(ActCorrection) {
		s.Engagement = 0.82
		s.Confidence = max(s.Confidence, 0.70)
	}
	if CanonicalActs(req).Has(ActQuestion) {
		s.InteractionNeed = NeedAnswer
	}
	if CanonicalActs(req).Has(ActProposal) {
		s.InteractionNeed = NeedProposal
	}
	if CanonicalActs(req).Has(ActCorrection) {
		s.InteractionNeed = NeedCorrection
	}
	if req.NeedsClarification {
		s.InteractionNeed = NeedClarification
	}

	if containsAny(lower, "no entiendo", "no entiendo por qué", "confundido", "confusa",
		"i don't understand", "i dont understand", "i'm confused", "im confused") {
		s.Confusion = 0.85
		s.InteractionNeed = NeedExplanation
	}
	if containsAny(lower, "no funciona", "otra vez", "llevo horas", "llevo mucho",
		"no sirve", "sigue sin funcionar", "doesn't work", "doesnt work",
		"again", "still not working") {
		s.Frustration = 0.70
		s.InteractionNeed = NeedAssistance
	}
	if containsAny(lower, "urgente", "rápido", "rapido", "ya", "ahora mismo",
		"asap", "urgent", "right now") {
		s.Urgency = 0.80
	}
	if containsAny(lower, "genial", "perfecto", "funciona", "lo logramos", "brutal",
		"awesome", "great", "perfect", "we did it") {
		s.Enthusiasm = 0.72
	}
	if strings.Contains(raw, "😂") || strings.Contains(raw, "🤣") ||
		containsAny(lower, "jajaja", "jaja", "jeje", "lol", "haha") {
		s.Humor = 0.80
	}
	if strings.Contains(raw, "!") {
		s.Engagement = max(s.Engagement, 0.78)
	}
	if strings.Contains(raw, "😭") || strings.Contains(raw, "😤") {
		s.Frustration = max(s.Frustration, 0.80)
	}

	// A direct correction is a communication need, not evidence of an
	// emotional condition.
	if CanonicalActs(req).Has(ActCorrection) {
		s.InteractionNeed = NeedCorrection
	}
	if req.FollowUp || req.Continues {
		s.InteractionNeed = NeedContinuation
	}

	return UserStateAssessment{
		Current:  s,
		Previous: prev,
		Delta: UserStateDelta{
			Frustration: s.Frustration - prev.Frustration,
			Enthusiasm:  s.Enthusiasm - prev.Enthusiasm,
			Confusion:   s.Confusion - prev.Confusion,
			Urgency:     s.Urgency - prev.Urgency,
			Engagement:  s.Engagement - prev.Engagement,
			Confidence:  s.Confidence - prev.Confidence,
			Humor:       s.Humor - prev.Humor,
		},
		Evidence: userStateEvidence(s, lower),
	}
}

func userStateEvidence(s UserState, lower string) []string {
	var out []string
	if s.Frustration >= 0.60 {
		out = append(out, "problem/frustration language")
	}
	if s.Confusion >= 0.60 {
		out = append(out, "confusion language")
	}
	if s.Urgency >= 0.60 {
		out = append(out, "urgency language")
	}
	if s.Enthusiasm >= 0.60 {
		out = append(out, "positive/enthusiastic language")
	}
	if s.Humor >= 0.60 {
		out = append(out, "humor markers")
	}
	if s.InteractionNeed != NeedNone {
		out = append(out, "semantic interaction need")
	}
	return out
}

func clampUserState01(v float64) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0
	}
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}
