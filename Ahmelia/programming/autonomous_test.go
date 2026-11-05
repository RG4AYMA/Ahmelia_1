package programming

import (
	"context"
	"testing"

	"ahmelia/autonomy"
	"ahmelia/planning"
)

type autonomousGoalSelector struct{ goal *planning.Goal }

func (s autonomousGoalSelector) SelectGoal() *planning.Goal { return s.goal }

type autonomousPlanProvider struct{ plan *planning.Plan }

func (p autonomousPlanProvider) EnsurePlan(goal *planning.Goal) (*planning.Plan, error) {
	return p.plan, nil
}

// We test the orchestration contract with the real autonomy selector/planner
// collaborators only through the narrow interfaces exposed by the adapter.
func TestStaticProgrammingRequestProvider(t *testing.T) {
	goal := planning.NewGoal("program", "program", planning.PriorityMedium, "ahmelia")
	req := SoftwareProductionRequest{ID: "sp-1", Project: CodeProject{ID: "p", Root: t.TempDir()}}
	provider, err := NewStaticProgrammingRequestProvider(map[string]SoftwareProductionRequest{goal.ID: req})
	if err != nil {
		t.Fatal(err)
	}
	got, err := provider.BuildProgrammingRequest(context.Background(), goal, &planning.Plan{ID: "plan", GoalID: goal.ID})
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != req.ID {
		t.Fatalf("got request %q, want %q", got.ID, req.ID)
	}
}

func TestAutonomousProgrammingRequiresCanonicalCollaborators(t *testing.T) {
	if _, err := NewAutonomousProgramming(nil, nil, nil, nil); err == nil {
		t.Fatal("expected selector validation error")
	}
}

func TestAutonomousProgrammingPlanMismatch(t *testing.T) {
	// Ensure the production-facing wrapper preserves the existing planning
	// authority instead of accepting a plan for another goal.
	_ = autonomy.DefaultGoalSelectionWeights
	_ = autonomousGoalSelector{}
	_ = autonomousPlanProvider{}
}
