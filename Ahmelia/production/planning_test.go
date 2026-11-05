package production

import (
	"testing"
	"time"

	"ahmelia/planning"
)

func testPlanningGoal() *planning.Goal {
	return &planning.Goal{ID: "goal-b95-5", Name: "B95.5 test goal", Priority: planning.PriorityMedium}
}

func TestPlanningServiceReusesExistingPlannerAndPlan(t *testing.T) {
	planner := planning.NewPlanner()
	goal := testPlanningGoal()
	planner.RegisterGoal(goal)

	plan := planner.GeneratePlan(goal)
	service, err := NewPlanningService(planner)
	if err != nil {
		t.Fatal(err)
	}

	assessment, err := service.Assess(plan)
	if err != nil {
		t.Fatal(err)
	}
	if assessment.PlanID != plan.ID || assessment.GoalID != goal.ID {
		t.Fatal("assessment lost plan identity")
	}
	if assessment.TaskCount != len(plan.Tasks) {
		t.Fatal("unexpected task count")
	}
	if assessment.CriticalPath <= 0 {
		t.Fatal("expected critical path")
	}
	if assessment.NextRunnableTaskID == "" {
		t.Fatal("expected runnable first task")
	}
	if got := service.Planner(); got != planner {
		t.Fatal("planner identity was not preserved")
	}
}

func TestPlanningServiceDoesNotMutatePlan(t *testing.T) {
	planner := planning.NewPlanner()
	goal := testPlanningGoal()
	planner.RegisterGoal(goal)
	plan := planner.GeneratePlan(goal)
	beforeEstimate := plan.TotalEstimate
	beforeFeasible := plan.Feasible
	beforeIssues := append([]string(nil), plan.Issues...)

	service, _ := NewPlanningService(planner)
	if _, err := service.Assess(plan); err != nil {
		t.Fatal(err)
	}
	if plan.TotalEstimate != beforeEstimate || plan.Feasible != beforeFeasible {
		t.Fatal("assessment mutated plan")
	}
	if len(plan.Issues) != len(beforeIssues) {
		t.Fatal("assessment mutated issues")
	}
}

func TestPlanningServiceAnalyzeDelegates(t *testing.T) {
	planner := planning.NewPlanner()
	goal := testPlanningGoal()
	planner.RegisterGoal(goal)
	plan := planner.GeneratePlan(goal)
	service, _ := NewPlanningService(planner)
	text, err := service.Analyze(plan)
	if err != nil {
		t.Fatal(err)
	}
	if text == "" {
		t.Fatal("expected analysis")
	}
}

func TestPlanningServiceRejectsInvalidDependencies(t *testing.T) {
	if _, err := NewPlanningService(nil); err == nil {
		t.Fatal("expected nil planner rejection")
	}
	planner := planning.NewPlanner()
	service, _ := NewPlanningService(planner)
	if _, err := service.Assess(nil); err == nil {
		t.Fatal("expected nil plan rejection")
	}
	if _, err := service.Analyze(nil); err == nil {
		t.Fatal("expected nil plan rejection")
	}
	_ = time.Second
}
