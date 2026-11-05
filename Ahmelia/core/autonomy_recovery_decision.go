package core

// RecoveryDecider is B91.3: it converts the facts produced by B91.2 into
// exactly one recovery decision. It is deliberately read-only: deciding
// whether to resume does not start execution, mutate Goals/Tasks, or touch
// AutonomousState.
type RecoveryDecider struct{}

// NewRecoveryDecider creates the B91.3 decision component.
func NewRecoveryDecider() *RecoveryDecider {
	return &RecoveryDecider{}
}

// Decide maps a B91.2 analysis to the recovery decision defined by B91.1.
//
// Policy:
//   - completed Goals are idle: there is nothing to resume;
//   - blocked Goals remain blocked;
//   - paused Goals wait instead of being resumed automatically;
//   - pending runnable work is safe to resume;
//   - missing/infeasible/non-executable Plans require user intervention;
//   - unknown/invalid input is conservative and requires the user.
func (d *RecoveryDecider) Decide(analysis *RecoveryAnalysis) *RecoveryAssessment {
	if analysis == nil || analysis.Candidate == nil || analysis.Candidate.Goal == nil {
		return NewRecoveryAssessment(nil, RecoveryNeedsUser, RecoveryReasonUnknown)
	}

	goal := analysis.Candidate.Goal

	switch {
	case goal.IsComplete():
		return NewRecoveryAssessment(analysis.Candidate, RecoveryIdle, RecoveryReasonCompleted)
	case goal.IsBlocked():
		return NewRecoveryAssessment(analysis.Candidate, RecoveryBlocked, RecoveryReasonGoalBlocked)
	case goal.Status == planningGoalPaused:
		return NewRecoveryAssessment(analysis.Candidate, RecoveryIdle, RecoveryReasonPendingWork)
	case analysis.Reason == RecoveryReasonPendingWork &&
		analysis.PlanResolvable &&
		analysis.PendingRunnable > 0:
		return NewRecoveryAssessment(analysis.Candidate, RecoveryResume, RecoveryReasonPendingWork)
	case analysis.Reason == RecoveryReasonNoPlan:
		return NewRecoveryAssessment(analysis.Candidate, RecoveryNeedsUser, RecoveryReasonNoPlan)
	case analysis.Reason == RecoveryReasonNoExecutableTask:
		return NewRecoveryAssessment(analysis.Candidate, RecoveryNeedsUser, RecoveryReasonNoExecutableTask)
	case analysis.Reason == RecoveryReasonRequiresUser:
		return NewRecoveryAssessment(analysis.Candidate, RecoveryNeedsUser, RecoveryReasonRequiresUser)
	case analysis.Reason == RecoveryReasonCompleted:
		return NewRecoveryAssessment(analysis.Candidate, RecoveryIdle, RecoveryReasonCompleted)
	default:
		return NewRecoveryAssessment(analysis.Candidate, RecoveryNeedsUser, RecoveryReasonUnknown)
	}
}

// Keep the planning dependency local to this file without exposing it in the
// public API.
const planningGoalPaused = "paused"
