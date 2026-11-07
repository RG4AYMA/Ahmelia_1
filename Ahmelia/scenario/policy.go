package scenario

import (
	"time"

	"ahmelia/restriction"
)

// Decision is the deterministic policy result for one scene point. It is not
// an action and it does not authorize anything.
type Decision struct {
	Kind        string
	Reason      string
	Score       float64
	Constraints []string
	Evidence    []string
}

const (
	DecisionSilence            = "silence"
	DecisionWorkInitiative     = "work_initiative"
	DecisionRecreationProposal = "recreation_proposal"
)

// DecideWorkInitiative closes work initiative while a work restriction is
// active. Without a restriction the previous behavior is unchanged.
func DecideWorkInitiative(store *restriction.Store, now time.Time) Decision {
	for _, r := range store.Active(now) {
		if r.Scope == restriction.ScopeWorkInitiative {
			return Decision{
				Kind:        DecisionSilence,
				Reason:      "work_restriction_active",
				Constraints: []string{"work_initiative_blocked"},
				Evidence:    []string{"restriction:" + r.ID},
			}
		}
	}
	return Decision{Kind: DecisionWorkInitiative, Reason: "no_active_restriction"}
}

// ProposalWindowOpen reports whether a recreation proposal is allowed now.
// It is a read-only check; recording the proposal is ProposeRecreation's job.
func (o *OwnerSim) ProposalWindowOpen(now time.Time, window time.Duration) bool {
	if o == nil || o.state == nil {
		return false
	}
	return o.state.ProposalWindowOpen(now, window)
}

// ProposeRecreation allows at most one optional proposal per window, and only
// while the owner is explicitly receptive. It never proposes work.
func (o *OwnerSim) ProposeRecreation(now time.Time, window time.Duration) Decision {
	if window <= 0 {
		window = 2 * time.Hour
	}
	if _, ok := o.ActivePain(now); ok {
		return Decision{
			Kind:        DecisionSilence,
			Reason:      "owner_state_sensitive",
			Constraints: []string{"no_optional_proposals_while_sensitive"},
		}
	}
	rec := o.Receptivity(now)
	if rec.Level != "high" {
		return Decision{
			Kind:        DecisionSilence,
			Reason:      "receptivity_" + rec.Level,
			Constraints: []string{"recreation_requires_high_receptivity"},
			Evidence:    rec.Evidence,
		}
	}

	if o == nil || o.state == nil || !o.state.ProposalWindowOpen(now, window) {
		return Decision{
			Kind:        DecisionSilence,
			Reason:      "proposal_window_active",
			Constraints: []string{"one_recreation_proposal_per_window"},
		}
	}
	_ = o.state.MarkProposal(now)

	return Decision{
		Kind:        DecisionRecreationProposal,
		Reason:      "receptivity_high",
		Constraints: []string{"optional", "evidence_bound"},
		Evidence:    rec.Evidence,
	}
}
