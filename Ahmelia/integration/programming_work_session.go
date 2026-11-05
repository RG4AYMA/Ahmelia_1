package integration

import (
	"context"
	"fmt"
	"strings"

	"ahmelia/autonomy"
	"ahmelia/execution"
	"ahmelia/planning"
)

// ProgrammingWorkSession is the domain entry point for a real programming
// work session. It does not own planning, execution or verification; it
// composes the canonical GoalExecutionRunner and therefore keeps the same
// Goal -> Plan -> Workflow -> Execution -> Verification path.
type ProgrammingWorkSession struct {
	runner *GoalExecutionRunner
}

func NewProgrammingWorkSession(runner *GoalExecutionRunner) (*ProgrammingWorkSession, error) {
	if runner == nil {
		return nil, fmt.Errorf("integration: programming work session runner nil")
	}
	return &ProgrammingWorkSession{runner: runner}, nil
}

// Run executes one programming Goal using the canonical execution path.
// projectID is carried as execution context, not used to create a second
// project/artifact authority.
func (s *ProgrammingWorkSession) Run(ctx context.Context, goal *planning.Goal, workID, projectID string) (execution.TaskGraphResult, error) {
	if s == nil || s.runner == nil {
		return execution.TaskGraphResult{}, fmt.Errorf("integration: programming work session unavailable")
	}
	if goal == nil {
		return execution.TaskGraphResult{}, fmt.Errorf("integration: programming work session nil goal")
	}
	if strings.TrimSpace(workID) == "" {
		return execution.TaskGraphResult{}, fmt.Errorf("integration: programming work session empty workID")
	}
	if strings.TrimSpace(projectID) == "" {
		return execution.TaskGraphResult{}, fmt.Errorf("integration: programming work session empty projectID")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	ctx = context.WithValue(ctx, ProgrammingProjectContextKey{}, strings.TrimSpace(projectID))
	ctx = context.WithValue(ctx, ProgrammingWorkIDContextKey{}, strings.TrimSpace(workID))
	return s.runner.Run(ctx, goal, workID)
}

// ProgrammingGoalActionMapper adapts a planned programming task into the
// existing provider-neutral production operation. The physical provider is
// still selected by Engine's single execution authority.
type ProgrammingGoalActionMapper struct{}

func (ProgrammingGoalActionMapper) MapTask(ctx context.Context, goal *planning.Goal, task *planning.Task) (autonomy.ActionRequest, error) {
	if err := ctx.Err(); err != nil {
		return autonomy.ActionRequest{}, err
	}
	if goal == nil || task == nil {
		return autonomy.ActionRequest{}, fmt.Errorf("integration: programming mapper nil goal/task")
	}
	if task.GoalID != goal.ID {
		return autonomy.ActionRequest{}, fmt.Errorf("integration: programming task %q belongs to Goal %q", task.ID, task.GoalID)
	}
	raw := strings.TrimSpace(task.Description)
	if raw == "" {
		raw = strings.TrimSpace(task.Title)
	}
	if raw == "" {
		return autonomy.ActionRequest{}, fmt.Errorf("integration: programming task %q has no objective", task.ID)
	}
	params := map[string]string{"raw": raw}
	if projectID, ok := ctx.Value(ProgrammingProjectContextKey{}).(string); ok && projectID != "" {
		params["project_id"] = projectID
	}
	// A planned task must execute inside the WorkSession that owns its Goal
	// and Plan. Without this handoff, the Engine authority can create/attach a
	// different WorkSession for each task, splitting causal history from the
	// canonical workflow owner.
	if workID, ok := ctx.Value(ProgrammingWorkIDContextKey{}).(string); ok && strings.TrimSpace(workID) != "" {
		params["work_id"] = strings.TrimSpace(workID)
	}
	return autonomy.ActionRequest{ID: task.ID, Action: "produce", Target: raw, Parameters: params}, nil
}

// ProgrammingProjectContextKey carries the Core-owned project identity into a programming action mapper.
type ProgrammingProjectContextKey struct{}

// ProgrammingWorkIDContextKey carries the canonical WorkSession identity
// through plan compilation to the existing Engine execution authority.
type ProgrammingWorkIDContextKey struct{}

// compatibility alias for package-local tests and existing callers.
type programmingProjectIDKey = ProgrammingProjectContextKey
