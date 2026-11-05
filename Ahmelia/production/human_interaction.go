package production

import (
	"context"
	"fmt"

	"ahmelia/autonomy"
)

// B95.14 — Human Interaction.
// Production does not create a second conversation/initiative system. It
// adapts the existing autonomy ConversationManager and InitiativeDecision
// contracts so production can pause, ask, inform or defer without executing
// through the human-interaction path.

type HumanInteraction struct {
	conversation *autonomy.ConversationManager
}

func NewHumanInteraction(conversation *autonomy.ConversationManager) (*HumanInteraction, error) {
	if conversation == nil {
		return nil, fmt.Errorf("production: conversation manager nil")
	}
	return &HumanInteraction{conversation: conversation}, nil
}

func (h *HumanInteraction) Evaluate(ctx context.Context, decision autonomy.InitiativeDecision) (autonomy.ConversationDecision, error) {
	if h == nil || h.conversation == nil {
		return autonomy.ConversationDecision{}, fmt.Errorf("production: human interaction unavailable")
	}
	if ctx == nil {
		return autonomy.ConversationDecision{}, fmt.Errorf("production: context nil")
	}
	if err := ctx.Err(); err != nil {
		return autonomy.ConversationDecision{}, err
	}
	return h.conversation.EvaluateStrategy(decision), nil
}

// NeedsHumanResponse reports whether the initiative decision is explicitly
// directed at human interaction. It never grants execution authority.
func NeedsHumanResponse(decision autonomy.InitiativeDecision) bool {
	switch decision.Action {
	case autonomy.ActionAskUser, autonomy.ActionInformUser, autonomy.ActionSuggest:
		return true
	default:
		return false
	}
}

// CanExecuteAfterInteraction is deliberately conservative: human interaction
// itself never becomes execution authority. An explicit capability approval
// must still pass through B94.3/AuthorizationGate.
func CanExecuteAfterInteraction(decision autonomy.InitiativeDecision) bool {
	return autonomy.AuthorityForInitiativeAction(decision.Action) == autonomy.AuthorityExecute
}
