package production

import (
	"fmt"
	"time"

	"ahmelia/planning"
)

// PlanningService is the production-facing planning boundary. It reuses the
// existing planning.Planner and planning.Plan instead of introducing a second
// planner or plan model.
type PlanningService struct {
	planner *planning.Planner
}

// NewPlanningService adapts the existing Planner for Production.
func NewPlanningService(planner *planning.Planner) (*PlanningService, error) {
	if planner == nil {
		return nil, fmt.Errorf("production: planning service planner nil")
	}
	return &PlanningService{planner: planner}, nil
}

// Planner returns the exact planner owned by the planning subsystem.
func (s *PlanningService) Planner() *planning.Planner {
	if s == nil {
		return nil
	}
	return s.planner
}

// Assess describes an existing plan without creating a second plan or
// mutating its tasks. Planning remains the owner of plan generation and
// dependency semantics.
type PlanningAssessment struct {
	PlanID             string
	GoalID             string
	Feasible           bool
	Issues             []string
	TaskCount          int
	TotalEstimate      time.Duration
	CriticalPath       time.Duration
	NextRunnableTaskID string
}

// Assess evaluates the plan using the existing Planner and TaskSystem.
func (s *PlanningService) Assess(plan *planning.Plan) (*PlanningAssessment, error) {
	if s == nil || s.planner == nil {
		return nil, fmt.Errorf("production: planning service unavailable")
	}
	if plan == nil || plan.ID == "" || plan.GoalID == "" {
		return nil, fmt.Errorf("production: invalid plan")
	}

	taskSystem, err := NewTaskSystem(plan)
	if err != nil {
		return nil, fmt.Errorf("production: task system: %w", err)
	}

	assessment := &PlanningAssessment{
		PlanID:        plan.ID,
		GoalID:        plan.GoalID,
		Feasible:      plan.Feasible,
		Issues:        append([]string(nil), plan.Issues...),
		TaskCount:     len(plan.Tasks),
		TotalEstimate: plan.TotalEstimate,
		CriticalPath:  s.planner.EstimateCriticalPath(plan),
	}
	if next := taskSystem.NextRunnable(); next != nil {
		assessment.NextRunnableTaskID = next.ID
	}
	return assessment, nil
}

// Analyze delegates the human-readable plan analysis to the existing Planner.
// It is intentionally read-only from Production's point of view.
func (s *PlanningService) Analyze(plan *planning.Plan) (string, error) {
	if s == nil || s.planner == nil {
		return "", fmt.Errorf("production: planning service unavailable")
	}
	if plan == nil || plan.ID == "" || plan.GoalID == "" {
		return "", fmt.Errorf("production: invalid plan")
	}
	return s.planner.AnalyzePlan(plan), nil
}
