package production

import (
	"context"
	"testing"

	"ahmelia/autonomy"
	"ahmelia/capability"
	"ahmelia/planning"
	"ahmelia/tools"
)

type loopExecutor struct{}

func (loopExecutor) Execute(_ context.Context, a tools.Action, _ tools.Environment) tools.Result {
	return tools.Result{ActionID: a.ID, Success: true, Output: "created"}
}

type loopTool struct{}

func (loopTool) ID() tools.ID                  { return "loop-tool" }
func (loopTool) Capabilities() []capability.ID { return []capability.ID{"create_file"} }
func (loopTool) Executor() tools.Executor      { return loopExecutor{} }

type loopEnv struct{}

func (loopEnv) Name() string { return "test" }

type loopObserver struct{}

func (loopObserver) Observe(_ context.Context, _ tools.Action, _ tools.Result) (tools.Observation, error) {
	return tools.Observation{Confirmed: true, Details: "exists"}, nil
}

type loopVerifier struct{}

func (loopVerifier) Verify(_ context.Context, _ tools.Action, _ tools.Result, o tools.Observation) tools.Verification {
	return tools.Verification{Success: o.Confirmed, Reason: "verified", Observation: o}
}

func makeLoop(t *testing.T) (*ProductionLoop, CapabilityBinding, *planning.Task) {
	t.Helper()
	g := &planning.Goal{ID: "g1", Name: "create"}
	task := planning.NewTask(g.ID, "create file", "create", 1, 0)
	plan := &planning.Plan{ID: "p1", GoalID: g.ID, Tasks: []*planning.Task{task}}
	prod, _ := NewProduction("prod1", g)
	_ = prod.BindPlan(plan)
	ts, _ := NewTaskSystem(plan)
	cr := capability.NewRegistry()
	c := capability.Capability{ID: "create_file", Domain: capability.DomainLocal, Description: "create", Action: "create_file", Permission: capability.PermissionAutomatic}
	if err := cr.Register(c); err != nil {
		t.Fatal(err)
	}
	tr := tools.NewRegistry()
	if err := tr.Register(loopTool{}); err != nil {
		t.Fatal(err)
	}
	binder, _ := NewCapabilityBinder(cr, tr)
	binding, err := binder.Bind(task, c.ID, "a1", map[string]string{})
	if err != nil {
		t.Fatal(err)
	}
	auth, _ := NewAuthorizationGate(cr)
	gateway, _ := NewExecutionGateway(tools.NewRunner(tr))
	verifier, _ := NewVerificationService(loopVerifier{})
	loop, _ := NewProductionLoop(ProductionLoopConfig{Production: prod, TaskSystem: ts, Gateway: gateway, Authorization: auth, Observer: loopObserver{}, Verifier: verifier, Delivery: NewResultDelivery()})
	_ = autonomy.ApprovalNone
	return loop, binding, task
}

func TestProductionLoopEndToEnd(t *testing.T) {
	loop, binding, task := makeLoop(t)
	out, err := loop.Run(context.Background(), binding, autonomy.ApprovalNone, loopEnv{})
	if err != nil {
		t.Fatal(err)
	}
	if !out.Success || !out.Verification.Success || !out.Delivered.Success {
		t.Fatalf("unexpected result: %+v", out)
	}
	if task.Status != planning.TaskCompleted {
		t.Fatalf("task status=%q", task.Status)
	}
	if loop.production.Status != ProductionCompleted {
		t.Fatalf("production status=%q", loop.production.Status)
	}
}

func TestProductionLoopRequiresAuthorization(t *testing.T) {
	loop, binding, task := makeLoop(t)
	// Replace capability permission with ask_first through a fresh registry is
	// intentionally outside the loop; this test only proves a denied result
	// never reaches execution by using a gate whose registry requires approval.
	cr := capability.NewRegistry()
	c := capability.Capability{ID: "create_file", Domain: capability.DomainLocal, Description: "create", Action: "create_file", Permission: capability.PermissionAskFirst}
	_ = cr.Register(c)
	gate, _ := NewAuthorizationGate(cr)
	loop.authorization = gate
	out, err := loop.Run(context.Background(), binding, autonomy.ApprovalNone, loopEnv{})
	if err == nil {
		t.Fatal("expected unauthorized error")
	}
	if task.Status != planning.TaskPending {
		t.Fatalf("task should remain in progress, got %q", task.Status)
	}
	_ = out
}
