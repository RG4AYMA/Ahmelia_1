package core

import "ahmelia/planning"

// RecoveryDecision is the outcome that the recovery phase may eventually
// choose for a recovery candidate. B91.1 only defines the model; B91.2/B91.3
// are responsible for producing and choosing the decision.
type RecoveryDecision string

const (
	RecoveryResume    RecoveryDecision = "resume"
	RecoveryIdle      RecoveryDecision = "idle"
	RecoveryBlocked   RecoveryDecision = "blocked"
	RecoveryNeedsUser RecoveryDecision = "needs_user"
)

// IsValid reports whether d is one of the defined recovery decisions.
func (d RecoveryDecision) IsValid() bool {
	switch d {
	case RecoveryResume, RecoveryIdle, RecoveryBlocked, RecoveryNeedsUser:
		return true
	default:
		return false
	}
}

// RecoveryReason describes the evidence behind a recovery assessment.
// Reasons are deliberately typed so later recovery logic does not need to
// interpret free-form strings.
type RecoveryReason string

const (
	RecoveryReasonPendingWork      RecoveryReason = "pending_work"
	RecoveryReasonCompleted        RecoveryReason = "completed"
	RecoveryReasonGoalBlocked      RecoveryReason = "goal_blocked"
	RecoveryReasonNoPlan           RecoveryReason = "no_plan"
	RecoveryReasonNoExecutableTask RecoveryReason = "no_executable_task"
	RecoveryReasonRequiresUser     RecoveryReason = "requires_user"
	RecoveryReasonUnknown          RecoveryReason = "unknown"
)

// IsValid reports whether r is one of the defined recovery reasons.
func (r RecoveryReason) IsValid() bool {
	switch r {
	case RecoveryReasonPendingWork,
		RecoveryReasonCompleted,
		RecoveryReasonGoalBlocked,
		RecoveryReasonNoPlan,
		RecoveryReasonNoExecutableTask,
		RecoveryReasonRequiresUser,
		RecoveryReasonUnknown:
		return true
	default:
		return false
	}
}

// RecoveryPriority is the priority used to order recovery candidates.
// It intentionally reuses planning.GoalPriority so recovery cannot invent a
// second, conflicting priority scale for the same Goal.
type RecoveryPriority = planning.GoalPriority

const (
	RecoveryPriorityLow      = planning.PriorityLow
	RecoveryPriorityMedium   = planning.PriorityMedium
	RecoveryPriorityHigh     = planning.PriorityHigh
	RecoveryPriorityCritical = planning.PriorityCritical
	RecoveryPriorityUrgent   = planning.PriorityUrgent
)

// RecoveryCandidate is the immutable identity/context that B91 recovery
// evaluates. Goal and Plan/Tasks are references to the runtime objects
// already reconciled by B90. They are not copied.
type RecoveryCandidate struct {
	Goal         *planning.Goal
	Plan         *planning.Plan
	PendingTasks []*planning.Task
	Priority     RecoveryPriority
}

// NewRecoveryCandidate creates a candidate while preserving runtime
// identity. The supplied task slice is copied so callers cannot mutate the
// candidate's slice container, while the Task pointers remain identical.
func NewRecoveryCandidate(goal *planning.Goal, plan *planning.Plan, pendingTasks []*planning.Task) *RecoveryCandidate {
	var priority RecoveryPriority
	if goal != nil {
		priority = goal.Priority
	}

	tasks := append([]*planning.Task(nil), pendingTasks...)
	return &RecoveryCandidate{
		Goal:         goal,
		Plan:         plan,
		PendingTasks: tasks,
		Priority:     priority,
	}
}

// RecoveryAssessment is the result of analyzing one candidate. B91.1 keeps
// this as data only; it does not infer or mutate runtime state.
type RecoveryAssessment struct {
	Candidate *RecoveryCandidate
	Decision  RecoveryDecision
	Reason    RecoveryReason
}

// NewRecoveryAssessment creates an assessment from an existing candidate.
func NewRecoveryAssessment(candidate *RecoveryCandidate, decision RecoveryDecision, reason RecoveryReason) *RecoveryAssessment {
	return &RecoveryAssessment{
		Candidate: candidate,
		Decision:  decision,
		Reason:    reason,
	}
}

// Valid validates the structural contract of an assessment without changing
// any Goal, Plan, or Task.
func (a *RecoveryAssessment) Valid() bool {
	if a == nil || a.Candidate == nil || a.Candidate.Goal == nil {
		return false
	}
	if !a.Decision.IsValid() || !a.Reason.IsValid() {
		return false
	}
	if a.Candidate.Plan != nil && a.Candidate.Plan.GoalID != a.Candidate.Goal.ID {
		return false
	}
	for _, task := range a.Candidate.PendingTasks {
		if task == nil || task.GoalID != a.Candidate.Goal.ID {
			return false
		}
	}
	return true
}
