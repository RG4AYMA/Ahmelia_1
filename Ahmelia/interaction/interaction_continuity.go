package interaction

import (
	"sort"
	"strings"
	"time"
)

// InteractionContinuity is a derived read model that answers whether the
// current human↔Ahmelia interaction can continue coherently from its existing
// state. It deliberately does not own history or persistence.
//
// ConversationState remains authoritative for turns, threads, decisions,
// commitments and objectives. Work remains authoritative for operational
// lifecycle. This type only assembles the minimum continuity anchors needed by
// an interaction consumer.
type InteractionContinuity struct {
	SessionID         string
	Topic             string
	Objective         string
	ThreadID          string
	WorkID            string
	Language          string
	LastTurnID        uint64
	LastUserInput     string
	LastAssistant     string
	RecentTurns       []TurnRecord
	RecentDecisions   []ConversationDecision
	OpenCommitments   []ConversationCommitment
	ActiveObjectiveID string
	HasContinuity     bool
	ContinuityReason  string
	UpdatedAt         time.Time
}

// Continuity returns a detached projection of the state required to keep an
// interaction coherent. It does not infer new meaning and does not mutate
// conversation, work, memory or cognition state.
func (s *InteractionState) Continuity() InteractionContinuity {
	if s == nil {
		return InteractionContinuity{}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return continuityFromSnapshot(s.snapshot)
}

func continuityFromSnapshot(s InteractionStateSnapshot) InteractionContinuity {
	c := InteractionContinuity{
		SessionID:     strings.TrimSpace(s.SessionID),
		Topic:         strings.TrimSpace(s.Focus.Topic),
		Objective:     strings.TrimSpace(s.Focus.Objective),
		ThreadID:      strings.TrimSpace(s.Focus.ThreadID),
		WorkID:        strings.TrimSpace(s.Focus.WorkID),
		Language:      strings.TrimSpace(s.Conversation.Language),
		LastUserInput: strings.TrimSpace(s.Conversation.LastUserInput),
		LastAssistant: strings.TrimSpace(s.Conversation.LastAssistant),
		UpdatedAt:     s.Metadata.UpdatedAt,
	}

	if n := len(s.Conversation.Turns); n > 0 {
		last := s.Conversation.Turns[n-1]
		c.LastTurnID = last.ID
		start := n - 8
		if start < 0 {
			start = 0
		}
		c.RecentTurns = append([]TurnRecord(nil), s.Conversation.Turns[start:]...)
	}

	// Decisions and commitments are already bounded by ConversationState.
	// Keep only items relevant to the active interaction lane when a topic is
	// available, while preserving untied entries because they can still be
	// part of the current interaction context.
	c.RecentDecisions = relevantDecisions(s.Conversation.Decisions, c.Topic)
	c.OpenCommitments = openCommitments(s.Conversation.Commitments)

	if objective, ok := activeObjective(s.Conversation.Objectives); ok {
		c.ActiveObjectiveID = objective.ID
		if c.Objective == "" {
			c.Objective = strings.TrimSpace(objective.Text)
		}
	}

	switch {
	case c.SessionID == "":
		c.ContinuityReason = "interaction session is unavailable"
	case c.LastTurnID == 0 && c.ThreadID == "":
		c.ContinuityReason = "no prior interaction anchor is available"
	case c.ThreadID != "":
		c.HasContinuity = true
		c.ContinuityReason = "active interaction thread and semantic history are available"
	case c.LastTurnID > 0:
		c.HasContinuity = true
		c.ContinuityReason = "semantic turn history is available for continuation"
	default:
		c.ContinuityReason = "interaction has no resumable semantic anchor"
	}
	return c
}

func activeObjective(objectives []InteractionObjective) (InteractionObjective, bool) {
	for i := len(objectives) - 1; i >= 0; i-- {
		if objectives[i].Status == "active" {
			return objectives[i], true
		}
	}
	return InteractionObjective{}, false
}

func relevantDecisions(decisions []ConversationDecision, topic string) []ConversationDecision {
	if len(decisions) == 0 {
		return nil
	}
	topic = strings.ToLower(strings.TrimSpace(topic))
	out := make([]ConversationDecision, 0, len(decisions))
	for _, d := range decisions {
		dt := strings.ToLower(strings.TrimSpace(d.Topic))
		if topic == "" || dt == "" || dt == topic || strings.Contains(dt, topic) || strings.Contains(topic, dt) {
			out = append(out, d)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].SourceTurn == out[j].SourceTurn {
			return out[i].At.Before(out[j].At)
		}
		return out[i].SourceTurn < out[j].SourceTurn
	})
	if len(out) > 16 {
		out = out[len(out)-16:]
	}
	return append([]ConversationDecision(nil), out...)
}

func openCommitments(commitments []ConversationCommitment) []ConversationCommitment {
	if len(commitments) == 0 {
		return nil
	}
	out := make([]ConversationCommitment, 0, len(commitments))
	for _, c := range commitments {
		if strings.EqualFold(strings.TrimSpace(c.Status), "open") {
			out = append(out, c)
		}
	}
	if len(out) > 16 {
		out = out[len(out)-16:]
	}
	return append([]ConversationCommitment(nil), out...)
}
