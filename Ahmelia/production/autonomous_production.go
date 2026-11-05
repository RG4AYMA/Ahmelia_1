package production

import (
	"context"
	"fmt"
	"sync"

	"ahmelia/autonomy"
	"ahmelia/planning"
)

// B95.19 — Autonomous Production
//
// Production becomes consumable by the existing autonomy layer without
// creating another autonomous brain. Goal selection remains owned by
// autonomy.GoalSelector, plan generation remains owned by the existing
// GoalPlanningAdapter/planning.Planner, and execution remains owned by the
// B95.18 MultiStepProduction pipeline.
//
// This coordinator only connects those authorities:
//
//   GoalSelector -> PlanProvider -> ProductionFactory -> MultiStepProduction
//
// It never creates a Goal and never executes an Action directly.

type ProductionGoalSelector interface {
	SelectGoal() *planning.Goal
}

type ProductionPlanProvider interface {
	EnsurePlan(goal *planning.Goal) (*planning.Plan, error)
}

type ProductionFactory interface {
	Build(ctx context.Context, goal *planning.Goal, plan *planning.Plan) (*MultiStepProduction, error)
}

// AutonomousProduction coordinates one autonomous production run at a time.
type AutonomousProduction struct {
	mu       sync.Mutex
	selector ProductionGoalSelector
	plans    ProductionPlanProvider
	factory  ProductionFactory
	active   *Production
}

func NewAutonomousProduction(selector ProductionGoalSelector, plans ProductionPlanProvider, factory ProductionFactory) (*AutonomousProduction, error) {
	if selector == nil {
		return nil, fmt.Errorf("production: autonomous selector nil")
	}
	if plans == nil {
		return nil, fmt.Errorf("production: autonomous plan provider nil")
	}
	if factory == nil {
		return nil, fmt.Errorf("production: autonomous factory nil")
	}
	return &AutonomousProduction{selector: selector, plans: plans, factory: factory}, nil
}

// RunNext selects the existing best goal, obtains its existing plan, and hands
// the production context to the canonical B95.18 multi-step runner.
//
// No goal means there is simply nothing to produce. A running production is
// rejected rather than being replaced or executed concurrently.
func (a *AutonomousProduction) RunNext(ctx context.Context) (MultiStepRunResult, bool, error) {
	if a == nil {
		return MultiStepRunResult{}, false, fmt.Errorf("production: autonomous production unavailable")
	}
	if err := ctx.Err(); err != nil {
		return MultiStepRunResult{}, false, err
	}

	a.mu.Lock()
	if a.active != nil && a.active.Status == ProductionRunning {
		a.mu.Unlock()
		return MultiStepRunResult{}, false, fmt.Errorf("production: autonomous production already running")
	}
	a.mu.Unlock()

	goal := a.selector.SelectGoal()
	if goal == nil {
		return MultiStepRunResult{}, false, nil
	}
	plan, err := a.plans.EnsurePlan(goal)
	if err != nil {
		return MultiStepRunResult{}, false, err
	}
	if plan == nil || plan.GoalID != goal.ID {
		return MultiStepRunResult{}, false, fmt.Errorf("production: invalid plan for goal %q", goal.ID)
	}

	multi, err := a.factory.Build(ctx, goal, plan)
	if err != nil {
		return MultiStepRunResult{}, false, err
	}
	if multi == nil || multi.production == nil {
		return MultiStepRunResult{}, false, fmt.Errorf("production: factory returned invalid multi-step production")
	}

	a.mu.Lock()
	if a.active != nil && a.active.Status == ProductionRunning {
		a.mu.Unlock()
		return MultiStepRunResult{}, false, fmt.Errorf("production: autonomous production already running")
	}
	a.active = multi.production
	a.mu.Unlock()

	result, err := multi.Run(ctx, nil, nil)

	a.mu.Lock()
	if a.active == multi.production && multi.production.Status != ProductionRunning {
		a.active = nil
	}
	a.mu.Unlock()

	return result, true, err
}

// Active returns the currently running production context, if any.
func (a *AutonomousProduction) Active() *Production {
	if a == nil {
		return nil
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.active
}

// GoalSelectorAdapter reuses the existing autonomy.GoalSelector.
type GoalSelectorAdapter struct{ selector *autonomy.GoalSelector }

func NewGoalSelectorAdapter(selector *autonomy.GoalSelector) (*GoalSelectorAdapter, error) {
	if selector == nil {
		return nil, fmt.Errorf("production: goal selector nil")
	}
	return &GoalSelectorAdapter{selector: selector}, nil
}

func (a *GoalSelectorAdapter) SelectGoal() *planning.Goal {
	if a == nil || a.selector == nil {
		return nil
	}
	return a.selector.SelectGoal()
}

// GoalPlanningAdapter is the production-facing wrapper around the existing
// autonomy.GoalPlanningAdapter. The alias name is deliberately not reused to
// avoid shadowing autonomy's type.
type GoalPlanningProvider struct{ adapter *autonomy.GoalPlanningAdapter }

func NewGoalPlanningProvider(adapter *autonomy.GoalPlanningAdapter) (*GoalPlanningProvider, error) {
	if adapter == nil {
		return nil, fmt.Errorf("production: goal planning adapter nil")
	}
	return &GoalPlanningProvider{adapter: adapter}, nil
}

func (p *GoalPlanningProvider) EnsurePlan(goal *planning.Goal) (*planning.Plan, error) {
	if p == nil || p.adapter == nil {
		return nil, fmt.Errorf("production: goal planning provider unavailable")
	}
	return p.adapter.EnsurePlan(goal)
}
