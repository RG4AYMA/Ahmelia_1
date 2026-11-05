package production

import (
	"context"
	"testing"

	"ahmelia/autonomy"
	"ahmelia/capability"
	"ahmelia/planning"
	"ahmelia/tools"
)

func TestMultiStepRunsAllTasksInDependencyOrder(t *testing.T) {
	goal := &planning.Goal{ID: "g", Name: "multi-step"}
	t1 := planning.NewTask(goal.ID, "one", "", 1, 0)
	t2 := planning.NewTask(goal.ID, "two", "", 2, 0)
	t2.Dependencies = []string{t1.ID}
	plan := &planning.Plan{ID: "p", GoalID: goal.ID, Tasks: []*planning.Task{t1, t2}}

	prod, err := NewProduction("prod", goal)
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

	caps := capability.NewRegistry()
	c := capability.Capability{ID: "create_file", Domain: capability.DomainLocal, Description: "create", Action: "create_file", Permission: capability.PermissionAutomatic}
	if err := caps.Register(c); err != nil {
		t.Fatal(err)
	}
	tr := tools.NewRegistry()
	if err := tr.Register(loopTool{}); err != nil {
		t.Fatal(err)
	}
	binder, err := NewCapabilityBinder(caps, tr)
	if err != nil {
		t.Fatal(err)
	}
	b1, err := binder.Bind(t1, c.ID, "a1", nil)
	if err != nil {
		t.Fatal(err)
	}
	b2, err := binder.Bind(t2, c.ID, "a2", nil)
	if err != nil {
		t.Fatal(err)
	}
	provider, err := NewStaticBindingProvider([]CapabilityBinding{b1, b2})
	if err != nil {
		t.Fatal(err)
	}

	gate, err := NewAuthorizationGate(caps)
	if err != nil {
		t.Fatal(err)
	}
	gateway, err := NewExecutionGateway(tools.NewRunner(tr))
	if err != nil {
		t.Fatal(err)
	}
	loop, err := NewProductionLoop(ProductionLoopConfig{
		Production: prod, TaskSystem: ts, Gateway: gateway, Authorization: gate,
		Observer: loopObserver{}, Verifier: mustVerificationService(t), Delivery: NewResultDelivery(),
	})
	if err != nil {
		t.Fatal(err)
	}
	multi, err := NewMultiStepProduction(prod, ts, loop, provider)
	if err != nil {
		t.Fatal(err)
	}

	out, err := multi.Run(context.Background(), map[string]autonomy.Approval{"a1": autonomy.ApprovalNone, "a2": autonomy.ApprovalNone}, loopEnv{})
	if err != nil {
		t.Fatal(err)
	}
	if !out.Completed || len(out.Steps) != 2 {
		t.Fatalf("unexpected result: %+v", out)
	}
	if t1.Status != planning.TaskCompleted || t2.Status != planning.TaskCompleted {
		t.Fatalf("tasks not complete: %q %q", t1.Status, t2.Status)
	}
	if prod.Status != ProductionCompleted {
		t.Fatalf("production status=%q", prod.Status)
	}
}

func TestMultiStepStopsOnMissingBinding(t *testing.T) {
	goal := &planning.Goal{ID: "g"}
	task := planning.NewTask(goal.ID, "one", "", 1, 0)
	plan := &planning.Plan{ID: "p", GoalID: goal.ID, Tasks: []*planning.Task{task}}
	prod, _ := NewProduction("prod", goal)
	_ = prod.BindPlan(plan)
	ts, _ := NewTaskSystem(plan)
	provider, _ := NewStaticBindingProvider(nil)
	loop, _, _ := makeLoop(t)
	// Replace the loop's production/task owners so the test checks only the
	// multi-step binding boundary.
	loop.production = prod
	loop.taskSystem = ts
	multi, _ := NewMultiStepProduction(prod, ts, loop, provider)
	out, err := multi.Run(context.Background(), map[string]autonomy.Approval{"a1": autonomy.ApprovalNone, "a2": autonomy.ApprovalNone}, loopEnv{})
	if err != nil {
		t.Fatal(err)
	}
	if out.Completed || out.Failure == "" {
		t.Fatalf("expected binding failure: %+v", out)
	}
	if task.Status != planning.TaskPending {
		t.Fatalf("task changed unexpectedly: %q", task.Status)
	}
}

func mustVerificationService(t *testing.T) *VerificationService {
	t.Helper()
	v, err := NewVerificationService(funcVerifier{})
	if err != nil {
		t.Fatal(err)
	}
	return v
}

type funcVerifier struct{}

func (funcVerifier) Verify(_ context.Context, _ tools.Action, _ tools.Result, o tools.Observation) tools.Verification {
	return tools.Verification{Success: o.Confirmed, Reason: "verified", Observation: o}
}

func TestStaticBindingProviderRejectsDuplicate(t *testing.T) {
	task := planning.NewTask("g", "one", "", 1, 0)
	if _, err := NewStaticBindingProvider([]CapabilityBinding{{Task: task}, {Task: task}}); err == nil {
		t.Fatal("expected duplicate error")
	}
}

var _ = autonomy.ApprovalNone
