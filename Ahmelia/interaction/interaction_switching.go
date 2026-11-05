package interaction

import "strings"

// InteractionSwitchDecision is the explicit interaction-layer projection of
// a focus change. It describes which semantic lane is active without taking
// ownership of cognition, work execution, memory, or capabilities.
type InteractionSwitchDecision struct {
	Relation      ThreadRelation
	PreviousID    string
	CurrentID     string
	PreviousTopic string
	CurrentTopic  string
	Confidence    float64
	Reason        string
}

// InteractionSwitcher is the single coordination entry point for changing
// interaction focus. ConversationState remains authoritative for thread
// lifecycle; ThreadRouter supplies semantic routing rules.
type InteractionSwitcher struct {
	router *ThreadRouter
}

func NewInteractionSwitcher() *InteractionSwitcher {
	return &InteractionSwitcher{router: NewThreadRouter()}
}

// Switch routes a semantic request to an interaction lane and returns the
// resulting focus transition. A switch preserves the previous lane as open;
// it never deletes history and never starts/stops operational work.
func (s *InteractionSwitcher) Switch(state *ConversationState, req SemanticRequest) InteractionSwitchDecision {
	if state == nil {
		return InteractionSwitchDecision{Relation: ThreadUnknown, Confidence: 0.2, Reason: "conversation state unavailable"}
	}
	if s == nil || s.router == nil {
		s = NewInteractionSwitcher()
	}

	before, hasBefore := state.ActiveThread()
	route := s.router.Route(state, req)

	out := InteractionSwitchDecision{
		Relation:   route.Relation,
		PreviousID: route.PreviousID,
		CurrentID:  route.Thread.ID,
		Confidence: route.Confidence,
		Reason:     route.Reason,
	}
	if hasBefore {
		out.PreviousTopic = strings.TrimSpace(before.Topic)
	}
	out.CurrentTopic = strings.TrimSpace(route.Thread.Topic)
	return out
}

// SwitchTopic is a small explicit API for callers that already have a
// semantic topic and want to resume or create a lane without inventing raw
// conversational text. It remains domain-neutral.
func (s *InteractionSwitcher) SwitchTopic(state *ConversationState, topic, objective string) InteractionSwitchDecision {
	topic = strings.TrimSpace(topic)
	if topic == "" {
		return InteractionSwitchDecision{Relation: ThreadUnknown, Confidence: 0.1, Reason: "empty interaction topic"}
	}
	raw := "cambiemos de tema, hablemos de " + topic
	return s.Switch(state, SemanticRequest{
		Kind:       KindConversation,
		Topic:      topic,
		Objective:  Objective{Text: strings.TrimSpace(objective)},
		Raw:        raw,
		Normalized: strings.ToLower(raw),
	})
}
