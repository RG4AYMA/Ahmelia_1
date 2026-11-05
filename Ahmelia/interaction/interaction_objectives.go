package interaction

import (
	"strings"
	"time"
)

// InteractionObjective is the durable semantic representation of an objective
// currently pursued through interaction. It is not a work task and does not
// authorize or execute any capability.
type InteractionObjective struct {
	ID         string
	Topic      string
	Text       string
	Status     string // active | open | completed | abandoned
	SourceTurn uint64
	Decision   string
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// ObjectiveCandidate is a proposed objective extracted from a semantic turn.
// Detection is intentionally separate from activation so interpretation does
// not silently create or replace user goals.
type ObjectiveCandidate struct {
	IsObjective bool
	Topic       string
	Text        string
	Confidence  float64
	Reason      string
	SourceTurn  uint64
}

func DetectObjective(req SemanticRequest, turnID uint64) ObjectiveCandidate {
	text := strings.TrimSpace(req.Objective.Text)
	acts := CanonicalActs(req)
	if text == "" || acts.Has(ActQuestion) || acts.Has(ActSocial) {
		return ObjectiveCandidate{}
	}
	if !acts.Has(ActCommand) && !acts.Has(ActContinuation) && !acts.Has(ActProposal) && !acts.Has(ActDecision) {
		return ObjectiveCandidate{}
	}
	confidence := req.Confidence
	if confidence <= 0 {
		confidence = 0.80
	}
	if confidence > 1 {
		confidence = 1
	}
	return ObjectiveCandidate{IsObjective: true, Topic: strings.TrimSpace(req.Topic), Text: text,
		Confidence: confidence, Reason: "semantic request contains an explicit interaction objective", SourceTurn: turnID}
}

// ApplyObjective activates an already-accepted objective. It is explicit so
// callers can place cognition, authority, and user-confirmation policy before
// durable state mutation.
func ApplyObjective(state *ConversationState, candidate ObjectiveCandidate, decision string) bool {
	if state == nil || !candidate.IsObjective || strings.TrimSpace(candidate.Text) == "" {
		return false
	}
	state.AddObjective(candidate.Topic, candidate.Text, candidate.SourceTurn, decision)
	return true
}
