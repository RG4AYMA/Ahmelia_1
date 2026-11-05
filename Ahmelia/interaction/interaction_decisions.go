package interaction

import (
	"strings"
	"time"
)

// EstablishedInteractionDecision is the interaction-layer projection of a decision that
// has been explicitly established in the current interaction.
//
// It is a coordination/read model, not a second decision history. The
// durable historical record remains ConversationState.Decisions. Cognition
// remains responsible for deciding when a candidate is actually accepted.
type EstablishedInteractionDecision struct {
	Topic      string
	Decision   string
	SourceTurn uint64
	At         time.Time
	Confidence float64
}

// DecisionCandidate describes a possible user decision detected in the
// current semantic turn. Detection is deliberately separate from applying
// the decision so interpretation cannot silently become authority.
type DecisionCandidate struct {
	IsDecision bool
	Topic      string
	Decision   string
	Confidence float64
	Reason     string
	SourceTurn uint64
}

// ActiveDecision returns the most recent durable decision relevant to the
// active interaction. It never creates or mutates decision history.
func (s *InteractionState) ActiveDecision() EstablishedInteractionDecision {
	if s == nil {
		return EstablishedInteractionDecision{}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	if len(s.snapshot.Conversation.Decisions) == 0 {
		return EstablishedInteractionDecision{}
	}
	// Prefer a decision matching the active topic, otherwise use the newest
	// decision. ConversationState owns ordering and persistence.
	topic := strings.ToLower(strings.TrimSpace(s.snapshot.Focus.Topic))
	for i := len(s.snapshot.Conversation.Decisions) - 1; i >= 0; i-- {
		d := s.snapshot.Conversation.Decisions[i]
		if topic == "" || strings.EqualFold(strings.TrimSpace(d.Topic), topic) {
			return EstablishedInteractionDecision{
				Topic: strings.TrimSpace(d.Topic), Decision: strings.TrimSpace(d.Decision),
				SourceTurn: d.SourceTurn, At: d.At, Confidence: 1,
			}
		}
	}
	d := s.snapshot.Conversation.Decisions[len(s.snapshot.Conversation.Decisions)-1]
	return EstablishedInteractionDecision{
		Topic: strings.TrimSpace(d.Topic), Decision: strings.TrimSpace(d.Decision),
		SourceTurn: d.SourceTurn, At: d.At, Confidence: 1,
	}
}

// DetectDecision identifies explicit decision language without committing it
// to ConversationState. A caller may apply the candidate after its own
// authority/cognition policy accepts it.
func DetectDecision(req SemanticRequest, turnID uint64) DecisionCandidate {
	text := strings.ToLower(strings.TrimSpace(req.Normalized))
	if text == "" {
		text = strings.ToLower(strings.TrimSpace(req.Raw))
	}
	if text == "" {
		return DecisionCandidate{}
	}

	// Explicit confirmation/selection is stronger than a generic command.
	if CanonicalActs(req).Has(ActConfirmation) {
		return candidateFromRequest(req, turnID, 0.96, "explicit confirmation")
	}
	if CanonicalActs(req).Has(ActRejection) {
		return DecisionCandidate{
			IsDecision: true, Topic: strings.TrimSpace(req.Topic),
			Decision: "rejected", Confidence: 0.92, Reason: "explicit rejection",
			SourceTurn: turnID,
		}
	}

	markers := []struct {
		parts  []string
		conf   float64
		reason string
	}{
		{[]string{"decidimos ", "hemos decidido ", "ya decidimos "}, 0.97, "explicit decision statement"},
		{[]string{"vamos a ", "vamos con ", "nos quedamos con ", "elegimos "}, 0.93, "explicit selection commitment"},
		{[]string{"hagamos ", "usaremos ", "usamos "}, 0.90, "explicit chosen course"},
		{[]string{"the decision is ", "we decided ", "let's go with ", "we will use "}, 0.93, "explicit decision statement"},
	}
	for _, m := range markers {
		for _, marker := range m.parts {
			if strings.Contains(text, marker) {
				return candidateFromRequest(req, turnID, m.conf, m.reason)
			}
		}
	}

	return DecisionCandidate{}
}

func candidateFromRequest(req SemanticRequest, turnID uint64, confidence float64, reason string) DecisionCandidate {
	decision := strings.TrimSpace(req.Objective.Text)
	if decision == "" {
		decision = strings.TrimSpace(req.Raw)
	}
	return DecisionCandidate{
		IsDecision: true,
		Topic:      strings.TrimSpace(req.Topic),
		Decision:   decision,
		Confidence: confidence,
		Reason:     reason,
		SourceTurn: turnID,
	}
}

// ApplyDecision records an already-accepted decision in ConversationState.
// This method is intentionally explicit: merely detecting decision language
// must never mutate durable interaction state.
func ApplyDecision(state *ConversationState, candidate DecisionCandidate) bool {
	if state == nil || !candidate.IsDecision || strings.TrimSpace(candidate.Decision) == "" {
		return false
	}
	state.AddDecision(candidate.Topic, candidate.Decision, candidate.SourceTurn)
	return true
}
