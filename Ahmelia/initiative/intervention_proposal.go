package initiative

import (
	"errors"
	"strings"
	"time"
)

// InterventionProposal is the semantic hand-off from Initiative to Dialogue.
// Initiative owns only the authorization to intervene and the non-linguistic
// action. It never supplies a finished sentence through this contract.
// Dialogue decides the conversational movement; Language owns realization.
type InterventionProposal struct {
	ID          string                 `json:"id"`
	DecisionID  string                 `json:"decision_id"`
	Action      Action                 `json:"action"`
	ContentMode ContentMode            `json:"content_mode"`
	Category    ActionCategory         `json:"category"`
	Source      Source                 `json:"source"`
	Reason      Reason                 `json:"reason"`
	Target      string                 `json:"target,omitempty"`
	Topic       string                 `json:"topic,omitempty"`
	Context     string                 `json:"context,omitempty"`
	Priority    Priority               `json:"priority"`
	Urgency     Urgency                `json:"urgency"`
	Constraints []InitiativeConstraint `json:"constraints,omitempty"`
	Confidence  float64                `json:"confidence"`
	CreatedAt   time.Time              `json:"created_at"`
	ExpiresAt   time.Time              `json:"expires_at,omitempty"`
}

// BuildInterventionProposal converts an authorized action contract into a
// semantic proposal. It cannot turn WAIT/DEFER into INTERVENE and it never
// accepts or transports candidate.Question as response content.
func BuildInterventionProposal(contract ActionContract, opportunity InitiativeOpportunity, topic string) (InterventionProposal, error) {
	if contract.Decision != Intervene {
		return InterventionProposal{}, errors.New("intervention proposal requires INTERVENE")
	}
	if err := contract.Valid(); err != nil {
		return InterventionProposal{}, err
	}
	if !contract.Action.Valid() {
		return InterventionProposal{}, errors.New("intervention proposal requires a valid action")
	}

	proposal := InterventionProposal{
		ID:          "proposal-" + contract.ID,
		DecisionID:  contract.DecisionID,
		Action:      contract.Action,
		ContentMode: contract.Spec.ContentMode,
		Category:    contract.Spec.Category,
		Source:      contract.Source,
		Reason:      contract.Reason,
		Target:      strings.TrimSpace(contract.Target),
		Topic:       strings.TrimSpace(topic),
		Context:     strings.TrimSpace(opportunity.Context),
		Priority:    contract.Priority,
		Urgency:     contract.Urgency,
		Constraints: append([]InitiativeConstraint(nil), contract.Constraints...),
		Confidence:  contract.Confidence,
		CreatedAt:   contract.CreatedAt,
		ExpiresAt:   contract.ExpiresAt,
	}
	if proposal.Topic == "" {
		proposal.Topic = strings.TrimSpace(opportunity.Target)
	}
	return proposal, nil
}

func (p InterventionProposal) Valid() bool {
	return strings.TrimSpace(p.ID) != "" &&
		strings.TrimSpace(p.DecisionID) != "" &&
		p.Action.Valid() &&
		p.ContentMode != "" &&
		p.Category != "" &&
		!p.CreatedAt.IsZero()
}
