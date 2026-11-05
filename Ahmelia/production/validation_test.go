package production

import (
	"context"
	"testing"

	"ahmelia/autonomy"
	"ahmelia/capability"
	"ahmelia/planning"
	"ahmelia/tools"
)

type validationExecutor struct{}

func (validationExecutor) Execute(_ context.Context, a tools.Action, _ tools.Environment) tools.Result {
	return tools.Result{ActionID: a.ID, Success: true, Output: "validated"}
}

type validationTool struct{}

func (validationTool) ID() tools.ID                  { return "validation-tool" }
func (validationTool) Capabilities() []capability.ID { return []capability.ID{"validation.create"} }
func (validationTool) Executor() tools.Executor      { return validationExecutor{} }

type validationEnv struct{}

func (validationEnv) Name() string { return "validation" }

type validationObserver struct{}

func (validationObserver) Observe(_ context.Context, _ tools.Action, _ tools.Result) (tools.Observation, error) {
	return tools.Observation{Confirmed: true, Details: "output observed"}, nil
}

type validationVerifier struct{}

func (validationVerifier) Verify(_ context.Context, _ tools.Action, _ tools.Result, o tools.Observation) tools.Verification {
	return tools.Verification{Success: o.Confirmed, Reason: "verified", Observation: o}
}

func newValidationFixture(t *testing.T) (*ProductionLoop, CapabilityBinding, *planning.Task) {
	t.Helper()
	goal := &planning.Goal{ID: "validation-goal", Name: "produce", Priority: planning.PriorityMedium}
	task := planning.NewTask(goal.ID, "create output", "create output", 1, 0)
	plan := &planning.Plan{ID: "validation-plan", GoalID: goal.ID, Tasks: []*planning.Task{task}}
	prod, err := NewProduction("validation-production", goal)
	if err != nil {
		t.Fatal(err)
	}
	if err := prod.BindPlan(plan); err != nil {
		t.Fatal(err)
	}
	ts, err := NewTaskSystem(plan)
	if err != nil {
		t.Fatal(err)
	}
	cr := capability.NewRegistry()
	if err := cr.Register(capability.Capability{ID: "validation.create", Domain: capability.DomainLocal, Description: "create", Action: "create", Permission: capability.PermissionAutomatic}); err != nil {
		t.Fatal(err)
	}
	tr := tools.NewRegistry()
	if err := tr.Register(validationTool{}); err != nil {
		t.Fatal(err)
	}
	binder, err := NewCapabilityBinder(cr, tr)
	if err != nil {
		t.Fatal(err)
	}
	binding, err := binder.Bind(task, "validation.create", "validation-action", nil)
	if err != nil {
		t.Fatal(err)
	}
	auth, err := NewAuthorizationGate(cr)
	if err != nil {
		t.Fatal(err)
	}
	gateway, err := NewExecutionGateway(tools.NewRunner(tr))
	if err != nil {
		t.Fatal(err)
	}
	verifier, err := NewVerificationService(validationVerifier{})
	if err != nil {
		t.Fatal(err)
	}
	loop, err := NewProductionLoop(ProductionLoopConfig{Production: prod, TaskSystem: ts, Gateway: gateway, Authorization: auth, Observer: validationObserver{}, Verifier: verifier, Delivery: NewResultDelivery()})
	if err != nil {
		t.Fatal(err)
	}
	return loop, binding, task
}

func TestB9520ValidationReportReady(t *testing.T) {
	loop, _, task := newValidationFixture(t)
	report := ValidateProduction(loop, loop.taskSystem, loop.delivery)
	if !report.Ready {
		t.Fatalf("expected ready report: %+v", report)
	}
	if err := report.Error(); err != nil {
		t.Fatal(err)
	}
	if task.IsCompleted() {
		t.Fatal("validation must not execute production")
	}
}

func TestB9520ProductionE2E(t *testing.T) {
	loop, binding, task := newValidationFixture(t)
	out, err := loop.Run(context.Background(), binding, autonomy.ApprovalNone, validationEnv{})
	if err != nil {
		t.Fatal(err)
	}
	if !out.Success || !out.Verification.Success || !out.Delivered.Success {
		t.Fatalf("unexpected E2E result: %+v", out)
	}
	if !task.IsCompleted() || loop.production.Status != ProductionCompleted {
		t.Fatalf("incomplete production: task=%q production=%q", task.Status, loop.production.Status)
	}
}

func TestB9520CancelledContextDoesNotExecute(t *testing.T) {
	loop, binding, task := newValidationFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := loop.Run(ctx, binding, autonomy.ApprovalNone, validationEnv{}); err == nil {
		t.Fatal("expected cancellation")
	}
	if task.Attempts != 0 || task.Status != planning.TaskPending {
		t.Fatalf("cancelled run changed task: %+v", task)
	}
}
