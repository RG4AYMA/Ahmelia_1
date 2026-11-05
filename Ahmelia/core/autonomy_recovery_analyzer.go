package core

import (
	"ahmelia/autonomy"
	"ahmelia/planning"
)

// RecoveryAnalysis is the factual diagnosis produced by B91.2 for one
// restored Goal. It deliberately contains no recovery decision and never
// mutates the runtime. B91.3 consumes these facts to choose Resume, Idle,
// Blocked or NeedsUser.
type RecoveryAnalysis struct {
	Candidate *RecoveryCandidate

	GoalStatus   planning.GoalStatus
	PlanExists   bool
	PlanFeasible bool
	PlanIssues   []string

	TaskCount       int
	PendingTasks    int
	InProgressTasks int
	CompletedTasks  int
	BlockedTasks    int
	CancelledTasks  int

	// PendingRunnable is the number of pending Tasks whose dependencies are
	// already satisfied. PendingTasks can be non-zero while this is zero.
	PendingRunnable int

	// WasInProgress is true when the restored state contains an in-progress
	// Task. B90.6 normally normalizes these to pending on load, but the
	// analyzer keeps the fact visible for callers that analyze a live
	// reconciled runtime or an older snapshot.
	WasInProgress bool

	// PlanResolvable is deliberately conservative: B91.2 can prove that a
	// plan is absent/infeasible or that it has no executable work, but it
	// does not invoke GoalActionMapper because mapping is an execution-side
	// operation and must not have side effects during recovery analysis.
	PlanResolvable bool

	Reason RecoveryReason
}

// RecoveryAnalyzer inspects the reconciled Planner/GoalPlanningAdapter state.
// It is read-only: Analyze and AnalyzeAll never create Plans/Tasks, change
// Goal/Task status, or call the action mapper.
type RecoveryAnalyzer struct {
	planner *planning.Planner
	adapter *autonomy.GoalPlanningAdapter
}

// NewRecoveryAnalyzer creates a B91.2 analyzer over the already reconciled
// runtime. Both dependencies are required because B91.2 evaluates the same
// Goal/Plan identity that B90 restored.
func NewRecoveryAnalyzer(planner *planning.Planner, adapter *autonomy.GoalPlanningAdapter) (*RecoveryAnalyzer, error) {
	if planner == nil {
		return nil, errRecoveryAnalyzer("planner no configurado")
	}
	if adapter == nil {
		return nil, errRecoveryAnalyzer("goal planning adapter no configurado")
	}
	return &RecoveryAnalyzer{planner: planner, adapter: adapter}, nil
}

type recoveryAnalyzerError string

func (e recoveryAnalyzerError) Error() string { return "core: recovery analyzer: " + string(e) }

func errRecoveryAnalyzer(reason string) error { return recoveryAnalyzerError(reason) }

// Analyze inspects one Goal and its cached Plan. If the adapter has no cached
// Plan for the Goal, that is an explicit recovery fact; the analyzer never
// generates a replacement Plan.
func (a *RecoveryAnalyzer) Analyze(goal *planning.Goal) *RecoveryAnalysis {
	if goal == nil {
		return nil
	}

	plans := a.adapter.Plans()
	plan := plans[goal.ID]
	return analyzeRecoveryCandidate(NewRecoveryCandidate(goal, plan, nil))
}

// AnalyzeAll analyzes every Goal registered in Planner. The returned slice
// has one independent analysis per Goal and uses the same runtime pointers
// exposed by Planner/GoalPlanningAdapter. Map ordering is intentionally not
// promised.
func (a *RecoveryAnalyzer) AnalyzeAll() []*RecoveryAnalysis {
	goals := a.planner.Goals()
	plans := a.adapter.Plans()

	out := make([]*RecoveryAnalysis, 0, len(goals))
	for _, goal := range goals {
		if goal == nil {
			continue
		}
		out = append(out, analyzeRecoveryCandidate(
			NewRecoveryCandidate(goal, plans[goal.ID], nil),
		))
	}
	return out
}

func analyzeRecoveryCandidate(candidate *RecoveryCandidate) *RecoveryAnalysis {
	analysis := &RecoveryAnalysis{
		Candidate: candidate,
	}
	if candidate == nil || candidate.Goal == nil {
		analysis.Reason = RecoveryReasonUnknown
		return analysis
	}

	goal := candidate.Goal
	analysis.GoalStatus = goal.Status
	analysis.PlanExists = candidate.Plan != nil

	if goal.IsBlocked() {
		analysis.Reason = RecoveryReasonGoalBlocked
		if candidate.Plan != nil {
			inspectRecoveryPlan(candidate.Plan, analysis)
		}
		return analysis
	}
	if goal.IsComplete() {
		analysis.Reason = RecoveryReasonCompleted
		if candidate.Plan != nil {
			inspectRecoveryPlan(candidate.Plan, analysis)
		}
		return analysis
	}
	if candidate.Plan == nil {
		analysis.Reason = RecoveryReasonNoPlan
		return analysis
	}

	inspectRecoveryPlan(candidate.Plan, analysis)

	switch {
	case !analysis.PlanFeasible:
		analysis.PlanResolvable = false
		analysis.Reason = RecoveryReasonNoExecutableTask
	case analysis.PendingTasks == 0:
		if analysis.CompletedTasks == analysis.TaskCount && analysis.TaskCount > 0 {
			analysis.Reason = RecoveryReasonCompleted
		} else if analysis.BlockedTasks > 0 {
			analysis.Reason = RecoveryReasonGoalBlocked
		} else {
			analysis.Reason = RecoveryReasonNoExecutableTask
		}
	case analysis.PendingRunnable > 0:
		analysis.PlanResolvable = true
		analysis.Reason = RecoveryReasonPendingWork
	default:
		analysis.PlanResolvable = false
		analysis.Reason = RecoveryReasonNoExecutableTask
	}

	return analysis
}

func inspectRecoveryPlan(plan *planning.Plan, analysis *RecoveryAnalysis) {
	if plan == nil {
		return
	}

	analysis.PlanFeasible = plan.Feasible
	analysis.PlanIssues = append([]string(nil), plan.Issues...)

	tasks := make(map[string]*planning.Task, len(plan.Tasks))
	for _, task := range plan.Tasks {
		if task == nil {
			continue
		}
		analysis.TaskCount++
		tasks[task.ID] = task
		if task.Status == planning.TaskPending {
			analysis.Candidate.PendingTasks = append(analysis.Candidate.PendingTasks, task)
		}

		switch task.Status {
		case planning.TaskPending:
			analysis.PendingTasks++
		case planning.TaskInProgress:
			analysis.InProgressTasks++
			analysis.WasInProgress = true
		case planning.TaskCompleted:
			analysis.CompletedTasks++
		case planning.TaskBlocked:
			analysis.BlockedTasks++
		case planning.TaskCancelled:
			analysis.CancelledTasks++
		}
	}

	for _, task := range plan.Tasks {
		if task == nil || task.Status != planning.TaskPending {
			continue
		}
		if task.CanStart(tasks) {
			analysis.PendingRunnable++
		}
	}
}
