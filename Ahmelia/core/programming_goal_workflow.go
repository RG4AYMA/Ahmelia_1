package core

import (
	"context"
	"fmt"
	"strings"

	"ahmelia/execution"
	"ahmelia/integration"
)

// RunProgrammingGoal is the Core-owned entry point for a programming Goal.
// Goal/Plan remain owned by GoalStore/GoalPlanningAdapter; the Programming
// mapper only projects the canonical Plan into provider-neutral actions.
func (e *Engine) RunProgrammingGoal(ctx context.Context, goalID, workID, projectID string) (execution.TaskGraphResult, error) {
	if e == nil || e.goalPlanCompiler == nil || e.goalExecutionRunner == nil || e.goalStore == nil {
		return execution.TaskGraphResult{}, fmt.Errorf("core: programming goal workflow unavailable")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	goalID = strings.TrimSpace(goalID)
	workID = strings.TrimSpace(workID)
	projectID = strings.TrimSpace(projectID)
	if goalID == "" || workID == "" || projectID == "" {
		return execution.TaskGraphResult{}, fmt.Errorf("core: programming goal requires goalID, workID and projectID")
	}
	goal, ok := e.goalStore.Get(goalID)
	if !ok || goal == nil {
		return execution.TaskGraphResult{}, fmt.Errorf("core: programming goal %q not found", goalID)
	}
	if e.workStore == nil {
		return execution.TaskGraphResult{}, fmt.Errorf("core: programming work store unavailable")
	}
	work, ok := e.workStore.Get(workID)
	if !ok {
		return execution.TaskGraphResult{}, fmt.Errorf("core: programming work session %q not found", workID)
	}
	if strings.TrimSpace(work.GoalID) == "" || work.GoalID != goalID {
		return execution.TaskGraphResult{}, fmt.Errorf("core: programming work session %q belongs to Goal %q, not %q", workID, work.GoalID, goalID)
	}
	plan, err := e.goalPlanningAdapter.EnsurePlanContext(ctx, goal)
	if err != nil {
		return execution.TaskGraphResult{}, fmt.Errorf("core: programming goal %q plan unavailable: %w", goalID, err)
	}
	if plan == nil || strings.TrimSpace(work.PlanID) == "" || work.PlanID != plan.ID {
		actualPlanID := ""
		if plan != nil {
			actualPlanID = plan.ID
		}
		return execution.TaskGraphResult{}, fmt.Errorf("core: programming work session %q plan %q does not match canonical Goal plan %q", workID, work.PlanID, actualPlanID)
	}

	ctx = context.WithValue(ctx, integration.ProgrammingProjectContextKey{}, projectID)
	ctx = context.WithValue(ctx, integration.ProgrammingWorkIDContextKey{}, workID)
	compiled, err := e.goalPlanCompiler.CompileWithMapper(ctx, goal, integration.ProgrammingGoalActionMapper{})
	if err != nil {
		return execution.TaskGraphResult{}, err
	}
	return e.goalExecutionRunner.RunCompiled(ctx, goal, workID, compiled)
}
