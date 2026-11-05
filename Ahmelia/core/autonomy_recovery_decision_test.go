package core

import (
	"testing"

	"ahmelia/autonomy"
	"ahmelia/planning"
)

func TestB913ResumeDecision_PendingRunnableWork(t *testing.T) {
	planner, _, adapter := newB912Runtime(t)
	goal := planning.NewGoal("Resume", "continuar", planning.PriorityHigh, "ahmelia")
	planner.RegisterGoal(goal)
	plan, err := adapter.EnsurePlan(goal)
	if err != nil {
		t.Fatalf("EnsurePlan: %v", err)
	}

	analyzer, _ := NewRecoveryAnalyzer(planner, adapter)
	analysis := analyzer.Analyze(goal)
	assessment := NewRecoveryDecider().Decide(analysis)

	if assessment.Decision != RecoveryResume {
		t.Fatalf("Decision = %q, quería resume", assessment.Decision)
	}
	if assessment.Reason != RecoveryReasonPendingWork {
		t.Fatalf("Reason = %q, quería pending_work", assessment.Reason)
	}
	if assessment.Candidate.Goal != goal || assessment.Candidate.Plan != plan {
		t.Fatal("la decisión debe conservar identidad runtime")
	}
}

func TestB913ResumeDecision_CompletedIsIdle(t *testing.T) {
	goal := planning.NewGoal("Terminado", "", planning.PriorityMedium, "ahmelia")
	goal.Complete()

	planner := planning.NewPlanner()
	planner.RegisterGoal(goal)
	adapter := newB913AdapterForPlanner(t, planner)

	analyzer, _ := NewRecoveryAnalyzer(planner, adapter)
	got := NewRecoveryDecider().Decide(analyzer.Analyze(goal))

	if got.Decision != RecoveryIdle || got.Reason != RecoveryReasonCompleted {
		t.Fatalf("assessment = %+v, quería idle/completed", got)
	}
}

func TestB913ResumeDecision_BlockedRemainsBlocked(t *testing.T) {
	goal := planning.NewGoal("Bloqueado", "", planning.PriorityHigh, "ahmelia")
	goal.SetBlocked("capacidad no resoluble")

	planner := planning.NewPlanner()
	planner.RegisterGoal(goal)
	adapter := newB913AdapterForPlanner(t, planner)

	analyzer, _ := NewRecoveryAnalyzer(planner, adapter)
	got := NewRecoveryDecider().Decide(analyzer.Analyze(goal))

	if got.Decision != RecoveryBlocked || got.Reason != RecoveryReasonGoalBlocked {
		t.Fatalf("assessment = %+v, quería blocked/goal_blocked", got)
	}
}

func TestB913ResumeDecision_PausedIsIdle(t *testing.T) {
	goal := planning.NewGoal("Pausado", "", planning.PriorityMedium, "user")
	goal.Status = planning.GoalPaused

	planner := planning.NewPlanner()
	planner.RegisterGoal(goal)
	adapter := newB913AdapterForPlanner(t, planner)

	analyzer, _ := NewRecoveryAnalyzer(planner, adapter)
	got := NewRecoveryDecider().Decide(analyzer.Analyze(goal))

	if got.Decision != RecoveryIdle {
		t.Fatalf("Decision = %q, quería idle", got.Decision)
	}
}

func TestB913ResumeDecision_NoPlanNeedsUser(t *testing.T) {
	planner, _, adapter := newB912Runtime(t)
	goal := planning.NewGoal("Sin plan", "", planning.PriorityHigh, "user")
	planner.RegisterGoal(goal)

	analyzer, _ := NewRecoveryAnalyzer(planner, adapter)
	got := NewRecoveryDecider().Decide(analyzer.Analyze(goal))

	if got.Decision != RecoveryNeedsUser || got.Reason != RecoveryReasonNoPlan {
		t.Fatalf("assessment = %+v, quería needs_user/no_plan", got)
	}
}

func TestB913ResumeDecision_NoExecutableTaskNeedsUser(t *testing.T) {
	planner, _, adapter := newB912Runtime(t)
	goal := planning.NewGoal("Sin trabajo ejecutable", "", planning.PriorityHigh, "user")
	planner.RegisterGoal(goal)

	blocker := planning.NewTask(goal.ID, "bloqueadora", "", 1, 0)
	blocker.Status = planning.TaskBlocked
	pending := planning.NewTask(goal.ID, "dependiente", "", 2, 0)
	pending.Dependencies = []string{blocker.ID}
	plan := &planning.Plan{
		ID: "plan-no-exec", GoalID: goal.ID,
		Tasks: []*planning.Task{blocker, pending}, Feasible: true,
	}
	planner.RegisterTask(blocker)
	planner.RegisterTask(pending)
	adapter.LoadState(map[string]*planning.Plan{goal.ID: plan}, nil)

	analyzer, _ := NewRecoveryAnalyzer(planner, adapter)
	got := NewRecoveryDecider().Decide(analyzer.Analyze(goal))

	if got.Decision != RecoveryNeedsUser || got.Reason != RecoveryReasonNoExecutableTask {
		t.Fatalf("assessment = %+v, quería needs_user/no_executable_task", got)
	}
}

func TestB913ResumeDecision_DoesNotMutateRuntime(t *testing.T) {
	planner, _, adapter := newB912Runtime(t)
	goal := planning.NewGoal("Inmutable", "", planning.PriorityHigh, "ahmelia")
	planner.RegisterGoal(goal)
	plan, err := adapter.EnsurePlan(goal)
	if err != nil {
		t.Fatalf("EnsurePlan: %v", err)
	}
	beforeGoal := goal.Status
	beforePlan := adapter.Plans()[goal.ID]

	analyzer, _ := NewRecoveryAnalyzer(planner, adapter)
	got := NewRecoveryDecider().Decide(analyzer.Analyze(goal))
	if !got.Valid() {
		t.Fatalf("assessment inválida: %+v", got)
	}

	if goal.Status != beforeGoal || adapter.Plans()[goal.ID] != beforePlan || beforePlan != plan {
		t.Fatal("B91.3 modificó el runtime")
	}
}

func TestB913ResumeDecision_NilIsConservative(t *testing.T) {
	got := NewRecoveryDecider().Decide(nil)
	if got.Decision != RecoveryNeedsUser || got.Reason != RecoveryReasonUnknown {
		t.Fatalf("assessment = %+v, quería needs_user/unknown", got)
	}
	if got.Valid() {
		t.Fatal("un assessment nil no debe ser estructuralmente válido")
	}
}

func newB913AdapterForPlanner(t *testing.T, planner *planning.Planner) *autonomy.GoalPlanningAdapter {
	t.Helper()
	state := autonomy.New()
	monitor := autonomy.NewGoalMonitor(state, 0)
	adapter, err := autonomy.NewGoalPlanningAdapter(state, monitor, planner, b912TestMapper{}, nil, nil)
	if err != nil {
		t.Fatalf("NewGoalPlanningAdapter: %v", err)
	}
	return adapter
}
