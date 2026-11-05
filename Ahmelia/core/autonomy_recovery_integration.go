package core

import (
	"errors"
	"strings"
	"time"

	"ahmelia/autonomy"
)

// RecoveryEvent is the immutable B91.4 hand-off from Recovery to the rest of
// Ahmelia. It describes what recovery decided without executing the Goal.
type RecoveryEvent struct {
	ID        string           `json:"id"`
	GoalID    string           `json:"goal_id"`
	GoalName  string           `json:"goal_name"`
	Decision  RecoveryDecision `json:"decision"`
	Reason    RecoveryReason   `json:"reason"`
	Timestamp time.Time        `json:"timestamp"`
}

// RecoveryIntegrationResult contains the event plus the two existing pipeline
// representations that consume it: Initiative and Conversation.
type RecoveryIntegrationResult struct {
	Event        RecoveryEvent
	Initiative   autonomy.InitiativeDecision
	Conversation autonomy.ConversationDecision
}

// RecoveryIntegration connects B91.3 to the existing Initiative and
// Conversation components. It is intentionally side-effect free with respect
// to Goals/Tasks: integration translates state; it does not resume execution.
type RecoveryIntegration struct {
	initiative   *autonomy.InitiativeEngine
	conversation *autonomy.ConversationManager
}

// NewRecoveryIntegration requires the existing Initiative and Conversation
// components. Both are injected so B91.4 does not create duplicate engines.
func NewRecoveryIntegration(
	initiative *autonomy.InitiativeEngine,
	conversation *autonomy.ConversationManager,
) (*RecoveryIntegration, error) {
	if initiative == nil {
		return nil, errors.New("core: recovery integration: initiative engine no configurado")
	}
	if conversation == nil {
		return nil, errors.New("core: recovery integration: conversation manager no configurado")
	}
	return &RecoveryIntegration{initiative: initiative, conversation: conversation}, nil
}

// Integrate translates one B91.3 assessment into a RecoveryEvent and feeds the
// corresponding pure decision into Initiative and Conversation.
func (r *RecoveryIntegration) Integrate(assessment *RecoveryAssessment) (*RecoveryIntegrationResult, error) {
	if r == nil || r.initiative == nil || r.conversation == nil {
		return nil, errors.New("core: recovery integration: dependencia no configurada")
	}
	if assessment == nil || assessment.Candidate == nil || assessment.Candidate.Goal == nil {
		return nil, errors.New("core: recovery integration: assessment invalido")
	}
	if !assessment.Decision.IsValid() || !assessment.Reason.IsValid() {
		return nil, errors.New("core: recovery integration: assessment invalido")
	}

	goal := assessment.Candidate.Goal
	event := RecoveryEvent{
		ID:        recoveryIntegrationID(),
		GoalID:    goal.ID,
		GoalName:  strings.TrimSpace(goal.Name),
		Decision:  assessment.Decision,
		Reason:    assessment.Reason,
		Timestamp: time.Now(),
	}

	action, risk := recoveryInitiativeMapping(assessment.Decision)
	initiativeDecision := autonomy.NewInitiativeDecisionFromRecovery(
		action,
		"Recuperación: "+recoveryDecisionText(assessment.Decision),
		goal.ID,
		risk,
	)
	conversationDecision := r.conversation.EvaluateStrategy(initiativeDecision)

	return &RecoveryIntegrationResult{
		Event:        event,
		Initiative:   initiativeDecision,
		Conversation: conversationDecision,
	}, nil
}

func recoveryInitiativeMapping(decision RecoveryDecision) (autonomy.DecisionAction, autonomy.RiskLevel) {
	switch decision {
	case RecoveryResume:
		return autonomy.ActionResumeGoal, autonomy.RiskLow
	case RecoveryBlocked:
		return autonomy.ActionInformUser, autonomy.RiskLow
	case RecoveryNeedsUser:
		return autonomy.ActionAskUser, autonomy.RiskMedium
	case RecoveryIdle:
		return autonomy.ActionNoAction, autonomy.RiskLow
	default:
		return autonomy.ActionAskUser, autonomy.RiskMedium
	}
}

func recoveryDecisionText(decision RecoveryDecision) string {
	switch decision {
	case RecoveryResume:
		return "objetivo listo para reanudar"
	case RecoveryIdle:
		return "objetivo sin trabajo que reanudar"
	case RecoveryBlocked:
		return "objetivo bloqueado"
	case RecoveryNeedsUser:
		return "se requiere intervención del usuario"
	default:
		return "estado de recuperación desconocido"
	}
}

func recoveryIntegrationID() string {
	return "recovery-" + time.Now().UTC().Format("20060102T150405.000000000")
}
