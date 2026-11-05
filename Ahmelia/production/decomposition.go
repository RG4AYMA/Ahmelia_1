package production

import (
	"ahmelia/planning"
	"fmt"
)

// GoalDecomposer is the production-facing contract for turning an existing
// Goal into an existing planning.Plan. Production owns the orchestration
// boundary, while the planning subsystem remains the owner of decomposition.
type GoalDecomposer interface {
	Decompose(*planning.Goal) (*planning.Plan, error)
}

// PlannerDecomposer adapts the existing planning.Planner without introducing
// another planner or another Task model.
type PlannerDecomposer struct{ planner *planning.Planner }

func NewPlannerDecomposer(planner *planning.Planner) (*PlannerDecomposer, error) {
	if planner == nil {
		return nil, fmt.Errorf("production: decomposer planner nil")
	}
	return &PlannerDecomposer{planner: planner}, nil
}

func (d *PlannerDecomposer) Decompose(goal *planning.Goal) (*planning.Plan, error) {
	if d == nil || d.planner == nil {
		return nil, fmt.Errorf("production: decomposer unavailable")
	}
	if goal == nil || goal.ID == "" {
		return nil, fmt.Errorf("production: goal inválido")
	}
	plan := d.planner.GeneratePlan(goal)
	if plan == nil {
		return nil, fmt.Errorf("production: planner returned nil plan")
	}
	if plan.GoalID != goal.ID {
		return nil, fmt.Errorf("production: plan belongs to goal %q, expected %q", plan.GoalID, goal.ID)
	}
	return plan, nil
}

// DecomposeAndBind performs the only production-specific step: it asks the
// existing decomposer for a plan and binds that exact plan to Production.
// It does not execute, start, or mutate tasks.
func DecomposeAndBind(p *Production, d GoalDecomposer) (*planning.Plan, error) {
	if p == nil {
		return nil, fmt.Errorf("production: nil")
	}
	if p.Goal == nil {
		return nil, fmt.Errorf("production: goal nil")
	}
	if d == nil {
		return nil, fmt.Errorf("production: decomposer nil")
	}
	plan, err := d.Decompose(p.Goal)
	if err != nil {
		return nil, fmt.Errorf("production: decompose: %w", err)
	}
	if plan == nil {
		return nil, fmt.Errorf("production: decomposer returned nil plan")
	}
	if err := p.BindPlan(plan); err != nil {
		return nil, err
	}
	return plan, nil
}
