package core

import (
	"context"
	"strings"
	"testing"

	"ahmelia/interaction"
	"ahmelia/planning"
	"ahmelia/production"
)

func newProgrammingWorkflowOwnershipFixture(t *testing.T) (*Engine, *planning.Goal, production.WorkSession, *planning.Plan) {
	isolateTestWorkspace(t)
	t.Helper()
	engine := NewEngine(nil, nil, nil, nil, nil)
	engine.workStore = NewPersistentWorkStore(production.NewWorkStore(t.TempDir() + "/work_sessions.json"))
	goal := planning.NewGoal("crear programa de prueba", "produce", planning.PriorityMedium, "test")
	if err := engine.goalStore.Register(goal); err != nil {
		t.Fatal(err)
	}
	plan, err := engine.goalPlanningAdapter.EnsurePlan(goal)
	if err != nil {
		t.Fatal(err)
	}
	work, err := engine.workStore.Begin(interaction.SemanticRequest{Raw: "crear programa de prueba", Objective: interaction.Objective{Text: "crear programa de prueba"}})
	if err != nil {
		t.Fatal(err)
	}
	return engine, goal, work, plan
}

func TestRunProgrammingGoalRejectsWorkOwnedByDifferentGoal(t *testing.T) {
	engine, goal, work, plan := newProgrammingWorkflowOwnershipFixture(t)
	if err := engine.workStore.AttachPlanning(work.ID, "another-goal", plan.ID); err != nil {
		t.Fatal(err)
	}
	_, err := engine.RunProgrammingGoal(context.Background(), goal.ID, work.ID, "project-test")
	if err == nil || !strings.Contains(err.Error(), "belongs to Goal") {
		t.Fatalf("expected ownership rejection before execution, got %v", err)
	}
}

func TestRunProgrammingGoalRejectsWorkLinkedToDifferentPlan(t *testing.T) {
	engine, goal, work, plan := newProgrammingWorkflowOwnershipFixture(t)
	if err := engine.workStore.AttachPlanning(work.ID, goal.ID, "stale-plan-id"); err != nil {
		t.Fatal(err)
	}
	_, err := engine.RunProgrammingGoal(context.Background(), goal.ID, work.ID, "project-test")
	if err == nil || !strings.Contains(err.Error(), "does not match canonical Goal plan") {
		t.Fatalf("expected canonical plan rejection before execution (canonical=%s), got %v", plan.ID, err)
	}
}

func TestRunProgrammingGoalRejectsMissingWorkOwner(t *testing.T) {
	engine, goal, _, _ := newProgrammingWorkflowOwnershipFixture(t)
	_, err := engine.RunProgrammingGoal(context.Background(), goal.ID, "missing-work", "project-test")
	if err == nil || !strings.Contains(err.Error(), "work session \"missing-work\" not found") {
		t.Fatalf("expected missing work owner rejection before execution, got %v", err)
	}
}
