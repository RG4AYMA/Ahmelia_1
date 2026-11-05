package initiative

import (
	"errors"
	"time"
)

// B96.37.6 — Initiative Action Selection & Decision Finalization.
// This layer does not decide whether Ahmelia should take a turn. B96.37.2
// remains the sole decision authority. It selects the non-linguistic action
// required to make an INTERVENE decision contract complete and validates the
// resulting contract before it can leave the initiative pipeline.
type ActionSelector struct{}

func NewActionSelector() *ActionSelector { return &ActionSelector{} }

func (s *ActionSelector) Select(op InitiativeOpportunity, decision Decision) Action {
	if decision != Intervene {
		return ""
	}
	switch op.Reason {
	case ReasonClarificationNeeded:
		return ActionAsk
	case ReasonPendingTask:
		return ActionRemind
	case ReasonUnfinishedCommitment:
		return ActionRemind
	case ReasonUsefulInformation:
		return ActionShareDiscovery
	case ReasonRelevantDiscovery:
		return ActionShareDiscovery
	case ReasonPotentialProblem:
		return ActionWarn
	case ReasonWarning:
		return ActionWarn
	case ReasonNextStepAvailable:
		return ActionSuggest
	case ReasonConversationContinue:
		return ActionContinue
	case ReasonPausedThreadAvailable:
		return ActionResume
	case ReasonAlternativeAvailable:
		return ActionPresentPossibility
	case ReasonNewPossibility:
		return ActionPropose
	case ReasonRelevantComment:
		return ActionComment
	case ReasonTopicRedirection:
		return ActionRedirect
	case ReasonContradictsAuthoritativeEvidence:
		return ActionDisagree
	default:
		return ActionContribute
	}
}

// FinalizeDecision completes an already-authorized decision without changing
// WAIT/DEFER/INTERVENE semantics. It is intentionally deterministic and
// contains no content generation or execution.
func (s *ActionSelector) FinalizeDecision(d InitiativeDecision, now time.Time) (InitiativeDecision, error) {
	if now.IsZero() {
		now = time.Now()
	}
	if d.CreatedAt.IsZero() {
		d.CreatedAt = now
	}
	if d.Decision == Intervene && !d.Action.Valid() {
		d.Action = s.Select(d.Opportunity, d.Decision)
	}
	d.Confidence = clamp(d.Confidence)
	if d.Provenance.Component == "" {
		d.Provenance.Component = "initiative.B96.37.6"
	}
	if d.Provenance.Subsystem == "" {
		d.Provenance.Subsystem = "B96.37.6"
	}
	if d.Provenance.CreatedAt.IsZero() {
		d.Provenance.CreatedAt = d.CreatedAt
	}
	if err := ValidateDecision(d); err != nil {
		return d, errors.Join(errors.New("B96.37.6 decision finalization failed"), err)
	}
	return d, nil
}
