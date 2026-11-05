package integration

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"ahmelia/autonomy"
	"ahmelia/execution"
	"ahmelia/planning"
)

// GoalPlanCompiler materializes the canonical Goal -> Plan into the existing
// execution-workflow representation. It is a projection only: planning.Goal,
// planning.Plan and planning.Task remain the authorities for identity and
// lifecycle; TaskGraph remains the scheduling authority.
type GoalPlanCompiler struct {
	planner *autonomy.GoalPlanningAdapter
}

// CompiledGoalWorkflow is an ephemeral execution projection. Actions are
// kept outside TaskExecutionSpec because autonomy.ActionRequest belongs to the
// autonomous planning boundary, not to the generic execution fabric.
type CompiledGoalWorkflow struct {
	GoalID  string
	PlanID  string
	Specs   []execution.TaskExecutionSpec
	Actions map[string]autonomy.ActionRequest
}

func NewGoalPlanCompiler(planner *autonomy.GoalPlanningAdapter) (*GoalPlanCompiler, error) {
	if planner == nil {
		return nil, fmt.Errorf("integration: goal plan compiler nil planner")
	}
	return &GoalPlanCompiler{planner: planner}, nil
}

// Compile obtains the one canonical Plan for the Goal and translates every
// Task through the same GoalActionMapper used by the autonomous cycle. No
// task is marked started and nothing is executed here. If any task cannot be
// mapped, compilation fails without returning a partial workflow.
func (c *GoalPlanCompiler) Compile(ctx context.Context, goal *planning.Goal) (CompiledGoalWorkflow, error) {
	if c == nil || c.planner == nil {
		return CompiledGoalWorkflow{}, fmt.Errorf("integration: goal plan compiler unavailable")
	}
	if goal == nil {
		return CompiledGoalWorkflow{}, fmt.Errorf("integration: goal plan compiler nil goal")
	}
	if err := ctx.Err(); err != nil {
		return CompiledGoalWorkflow{}, err
	}

	plan, err := c.planner.EnsurePlanContext(ctx, goal)
	if err != nil {
		return CompiledGoalWorkflow{}, err
	}
	if plan == nil || !plan.Feasible {
		return CompiledGoalWorkflow{}, fmt.Errorf("integration: goal %q has no feasible plan", goal.ID)
	}
	if len(plan.Tasks) == 0 {
		return CompiledGoalWorkflow{}, fmt.Errorf("integration: plan %q has no tasks", plan.ID)
	}

	out := CompiledGoalWorkflow{
		GoalID:  goal.ID,
		PlanID:  plan.ID,
		Specs:   make([]execution.TaskExecutionSpec, 0, len(plan.Tasks)),
		Actions: make(map[string]autonomy.ActionRequest, len(plan.Tasks)),
	}

	seen := make(map[string]struct{}, len(plan.Tasks))
	for _, task := range plan.Tasks {
		if task == nil {
			return CompiledGoalWorkflow{}, fmt.Errorf("integration: plan %q contains nil task", plan.ID)
		}
		if task.GoalID != goal.ID {
			return CompiledGoalWorkflow{}, fmt.Errorf("integration: task %q belongs to Goal %q, not %q", task.ID, task.GoalID, goal.ID)
		}
		if _, exists := seen[task.ID]; exists {
			return CompiledGoalWorkflow{}, fmt.Errorf("integration: plan %q contains duplicate task %q", plan.ID, task.ID)
		}
		seen[task.ID] = struct{}{}

		action, err := c.planner.MapTaskForWorkflow(ctx, goal, task)
		if err != nil {
			return CompiledGoalWorkflow{}, fmt.Errorf("integration: map task %q: %w", task.ID, err)
		}
		if action.ID == "" {
			action.ID = task.ID
		}
		if action.ID != task.ID {
			return CompiledGoalWorkflow{}, fmt.Errorf("integration: mapped task %q produced action ID %q", task.ID, action.ID)
		}

		out.Actions[task.ID] = action
		out.Specs = append(out.Specs, execution.TaskExecutionSpec{
			Task:                 task,
			CapabilityID:         action.Action,
			Operation:            action.Action,
			Timeout:              5 * time.Minute,
			ExecutionFingerprint: fingerprintAction(action),
		})
	}

	return out, nil
}

// CompileWithMapper keeps the canonical Goal/Plan authority while allowing a
// domain capability mapper to project planned Tasks into domain-specific
// ActionRequests. It does not create another planner or Task lifecycle.
func (c *GoalPlanCompiler) CompileWithMapper(ctx context.Context, goal *planning.Goal, mapper autonomy.GoalActionMapper) (CompiledGoalWorkflow, error) {
	if c == nil || c.planner == nil {
		return CompiledGoalWorkflow{}, fmt.Errorf("integration: goal plan compiler unavailable")
	}
	if mapper == nil {
		return CompiledGoalWorkflow{}, fmt.Errorf("integration: goal plan compiler mapper nil")
	}
	if goal == nil {
		return CompiledGoalWorkflow{}, fmt.Errorf("integration: goal plan compiler nil goal")
	}
	if err := ctx.Err(); err != nil {
		return CompiledGoalWorkflow{}, err
	}
	plan, err := c.planner.EnsurePlanContext(ctx, goal)
	if err != nil {
		return CompiledGoalWorkflow{}, err
	}
	if plan == nil || !plan.Feasible || len(plan.Tasks) == 0 {
		return CompiledGoalWorkflow{}, fmt.Errorf("integration: goal %q has no feasible executable tasks", goal.ID)
	}
	out := CompiledGoalWorkflow{GoalID: goal.ID, PlanID: plan.ID, Specs: make([]execution.TaskExecutionSpec, 0, len(plan.Tasks)), Actions: make(map[string]autonomy.ActionRequest, len(plan.Tasks))}
	seen := make(map[string]struct{}, len(plan.Tasks))
	for _, task := range plan.Tasks {
		if task == nil || task.GoalID != goal.ID {
			return CompiledGoalWorkflow{}, fmt.Errorf("integration: plan %q contains task outside goal %q", plan.ID, goal.ID)
		}
		if _, ok := seen[task.ID]; ok {
			return CompiledGoalWorkflow{}, fmt.Errorf("integration: plan %q contains duplicate task %q", plan.ID, task.ID)
		}
		seen[task.ID] = struct{}{}
		action, err := mapper.MapTask(ctx, goal, task)
		if err != nil {
			return CompiledGoalWorkflow{}, fmt.Errorf("integration: map task %q: %w", task.ID, err)
		}
		if action.ID == "" {
			action.ID = task.ID
		}
		if action.ID != task.ID {
			return CompiledGoalWorkflow{}, fmt.Errorf("integration: mapped task %q produced action ID %q", task.ID, action.ID)
		}
		out.Actions[task.ID] = action
		out.Specs = append(out.Specs, execution.TaskExecutionSpec{Task: task, CapabilityID: action.Action, Operation: action.Action, Timeout: 5 * time.Minute, ExecutionFingerprint: fingerprintAction(action)})
	}
	return out, nil
}

// fingerprintAction binds checkpoint recovery to the concrete mapped operation.
// A task ID may remain stable while its target or parameters change; resuming
// such a checkpoint would otherwise skip work from a different plan meaning.
func fingerprintAction(action autonomy.ActionRequest) string {
	raw, err := json.Marshal(struct {
		ID         string
		Action     string
		Target     string
		Parameters map[string]string
	}{ID: action.ID, Action: action.Action, Target: action.Target, Parameters: action.Parameters})
	if err != nil {
		return "invalid"
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
