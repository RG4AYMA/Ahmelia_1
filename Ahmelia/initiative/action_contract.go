package initiative

import (
	"errors"
	"fmt"
	"time"
)

// B96.37.7 — Initiative Action Contract.
//
// This layer defines the semantic contract of an already-selected initiative
// action. It does not decide whether Ahmelia may take a turn, generate text,
// execute work, or choose a capability. B96.37.2 owns authorization and
// B96.37.6 owns action selection/finalization.
type ActionCategory string

const (
	ActionCategoryClarification ActionCategory = "clarification"
	ActionCategoryInformation   ActionCategory = "information"
	ActionCategoryTask          ActionCategory = "task"
	ActionCategoryWarning       ActionCategory = "warning"
	ActionCategoryContinuity    ActionCategory = "continuity"
	ActionCategoryDirection     ActionCategory = "direction"
	ActionCategoryContribution  ActionCategory = "contribution"
)

type ContentMode string

const (
	ContentQuestion     ContentMode = "question"
	ContentSuggestion   ContentMode = "suggestion"
	ContentProposal     ContentMode = "proposal"
	ContentStatement    ContentMode = "statement"
	ContentReminder     ContentMode = "reminder"
	ContentWarning      ContentMode = "warning"
	ContentContinuation ContentMode = "continuation"
	ContentRedirection  ContentMode = "redirection"
	ContentDisagreement ContentMode = "disagreement"
)

// ActionSpec describes what an action semantically permits. It contains no
// generated language and no executor reference.
type ActionSpec struct {
	Action       Action         `json:"action"`
	Category     ActionCategory `json:"category"`
	ContentMode  ContentMode    `json:"content_mode"`
	NeedsContent bool           `json:"needs_content"`
}

func ActionSpecification(action Action) (ActionSpec, bool) {
	specs := map[Action]ActionSpec{
		ActionAsk:                {Action: ActionAsk, Category: ActionCategoryClarification, ContentMode: ContentQuestion, NeedsContent: true},
		ActionSuggest:            {Action: ActionSuggest, Category: ActionCategoryDirection, ContentMode: ContentSuggestion, NeedsContent: true},
		ActionPropose:            {Action: ActionPropose, Category: ActionCategoryDirection, ContentMode: ContentProposal, NeedsContent: true},
		ActionResume:             {Action: ActionResume, Category: ActionCategoryContinuity, ContentMode: ContentContinuation, NeedsContent: true},
		ActionComment:            {Action: ActionComment, Category: ActionCategoryContribution, ContentMode: ContentStatement, NeedsContent: true},
		ActionWarn:               {Action: ActionWarn, Category: ActionCategoryWarning, ContentMode: ContentWarning, NeedsContent: true},
		ActionContribute:         {Action: ActionContribute, Category: ActionCategoryContribution, ContentMode: ContentStatement, NeedsContent: true},
		ActionRemind:             {Action: ActionRemind, Category: ActionCategoryTask, ContentMode: ContentReminder, NeedsContent: true},
		ActionShareDiscovery:     {Action: ActionShareDiscovery, Category: ActionCategoryInformation, ContentMode: ContentStatement, NeedsContent: true},
		ActionPresentPossibility: {Action: ActionPresentPossibility, Category: ActionCategoryDirection, ContentMode: ContentProposal, NeedsContent: true},
		ActionContinue:           {Action: ActionContinue, Category: ActionCategoryContinuity, ContentMode: ContentContinuation, NeedsContent: true},
		ActionRedirect:           {Action: ActionRedirect, Category: ActionCategoryContinuity, ContentMode: ContentRedirection, NeedsContent: true},
		ActionDisagree:           {Action: ActionDisagree, Category: ActionCategoryDirection, ContentMode: ContentDisagreement, NeedsContent: true},
	}
	spec, ok := specs[action]
	return spec, ok
}

// ActionContract is the execution-boundary description of an authorized
// conversational action. It is data only: a downstream content producer or
// channel adapter may consume it, but this package never executes it.
type ActionContract struct {
	ID            string                 `json:"id,omitempty"`
	DecisionID    string                 `json:"decision_id"`
	Decision      Decision               `json:"decision"`
	Action        Action                 `json:"action,omitempty"`
	Spec          ActionSpec             `json:"spec,omitempty"`
	Source        Source                 `json:"source"`
	Reason        Reason                 `json:"reason"`
	Target        string                 `json:"target,omitempty"`
	Timing        Timing                 `json:"timing"`
	Priority      Priority               `json:"priority"`
	Urgency       Urgency                `json:"urgency"`
	Constraints   []InitiativeConstraint `json:"constraints,omitempty"`
	Confidence    float64                `json:"confidence"`
	CreatedAt     time.Time              `json:"created_at"`
	ExpiresAt     time.Time              `json:"expires_at,omitempty"`
	PolicyVersion string                 `json:"policy_version"`
}

// ActionContractBuilder converts a finalized decision into a validated action
// contract. It cannot turn WAIT/DEFER into an intervention.
type ActionContractBuilder struct{}

func NewActionContractBuilder() *ActionContractBuilder { return &ActionContractBuilder{} }

func (b *ActionContractBuilder) Build(d InitiativeDecision) (ActionContract, error) {
	if err := ValidateDecision(d); err != nil {
		return ActionContract{}, fmt.Errorf("B96.37.7 decision rejected: %w", err)
	}

	contract := ActionContract{
		ID: "action-" + d.ID, DecisionID: d.ID, Decision: d.Decision, Action: d.Action,
		Source: d.Source, Reason: d.Reason, Target: d.Target,
		Timing: d.Timing, Priority: d.Priority, Urgency: d.Urgency,
		Constraints: append([]InitiativeConstraint(nil), d.Constraints...),
		Confidence:  clamp(d.Confidence), CreatedAt: d.CreatedAt,
		ExpiresAt: d.ExpiresAt, PolicyVersion: d.PolicyVersion,
	}

	if d.Decision != Intervene {
		if d.Action != "" {
			return ActionContract{}, errors.New("B96.37.7 non-intervention cannot carry an action")
		}
		return contract, nil
	}

	spec, ok := ActionSpecification(d.Action)
	if !ok {
		return ActionContract{}, fmt.Errorf("B96.37.7 unsupported action %q", d.Action)
	}
	contract.Spec = spec
	if contract.CreatedAt.IsZero() {
		return ActionContract{}, errors.New("B96.37.7 intervention requires creation time")
	}
	if contract.DecisionID == "" {
		return ActionContract{}, errors.New("B96.37.7 intervention requires decision id")
	}
	return contract, nil
}

func (c ActionContract) Valid() error {
	if c.Decision == Intervene {
		if !c.Action.Valid() {
			return errors.New("intervention action is invalid")
		}
		if c.Spec.Action != c.Action {
			return errors.New("action specification does not match action")
		}
		if c.Spec.Category == "" || c.Spec.ContentMode == "" {
			return errors.New("intervention action specification is incomplete")
		}
	} else if c.Action != "" {
		return errors.New("WAIT/DEFER contract must not carry an action")
	}
	if c.ExpiresAt.IsZero() == false && !c.ExpiresAt.After(c.CreatedAt) {
		return errors.New("action contract expiration must be after creation")
	}
	return nil
}
