package production

import (
	"context"
	"testing"

	"ahmelia/autonomy"
	"ahmelia/capability"
	"ahmelia/planning"
	"ahmelia/tools"
)

type gatewayEnv struct{}

func (gatewayEnv) Name() string { return "b95.9-test" }

type gatewayExecutor struct{ calls int }

func (e *gatewayExecutor) Execute(_ context.Context, a tools.Action, _ tools.Environment) tools.Result {
	e.calls++
	return tools.Result{ActionID: a.ID, Success: true, Output: "executed"}
}

type gatewayTool struct{ exec *gatewayExecutor }

func (t gatewayTool) ID() tools.ID                  { return "filesystem-tool" }
func (t gatewayTool) Capabilities() []capability.ID { return []capability.ID{"filesystem.create_file"} }
func (t gatewayTool) Executor() tools.Executor      { return t.exec }

func gatewayBinding(t *testing.T) (CapabilityBinding, *gatewayExecutor, *capability.Registry, *tools.Runner) {
	t.Helper()
	caps := capability.NewRegistry()
	c := capability.Capability{
		ID: "filesystem.create_file", Domain: capability.DomainLocal,
		Action: "create_file", Permission: capability.PermissionAutomatic,
	}
	if err := caps.Register(c); err != nil {
		t.Fatal(err)
	}

	exec := &gatewayExecutor{}
	tr := tools.NewRegistry()
	if err := tr.Register(gatewayTool{exec}); err != nil {
		t.Fatal(err)
	}

	binder, err := NewCapabilityBinder(caps, tr)
	if err != nil {
		t.Fatal(err)
	}
	task := planning.NewTask("goal-95-9", "create file", "", 1, 0)
	binding, err := binder.Bind(task, c.ID, "action-95-9", nil)
	if err != nil {
		t.Fatal(err)
	}
	return binding, exec, caps, tools.NewRunner(tr)
}

func TestExecutionGatewayExecutesOnlyAfterAuthorization(t *testing.T) {
	binding, exec, caps, runner := gatewayBinding(t)
	gate, err := NewAuthorizationGate(caps)
	if err != nil {
		t.Fatal(err)
	}
	auth, err := gate.AuthorizeBinding(binding, autonomy.ApprovalNone)
	if err != nil {
		t.Fatal(err)
	}
	// automatic capability is authorized without external approval.
	if !auth.CanExecute() {
		t.Fatalf("expected authorization: %+v", auth)
	}

	gw, _ := NewExecutionGateway(runner)
	out := gw.ExecuteAuthorized(context.Background(), binding, auth, gatewayEnv{})
	if out.Result.Err != nil || !out.Result.Success {
		t.Fatalf("execution failed: %+v", out.Result)
	}
	if exec.calls != 1 {
		t.Fatalf("executor calls=%d, expected 1", exec.calls)
	}
}

func TestExecutionGatewayRejectsDeniedAuthorizationWithoutCallingExecutor(t *testing.T) {
	binding, exec, caps, runner := gatewayBinding(t)
	gate, _ := NewAuthorizationGate(caps)
	denied := AuthorizationResult{
		CapabilityID: binding.Capability.ID,
		Decision: autonomy.CapabilityAuthorization{
			CapabilityID: binding.Capability.ID,
			Authority:    autonomy.AuthorityRecommend,
			Allowed:      false,
			Reason:       "denied",
		},
	}
	_ = gate
	gw, _ := NewExecutionGateway(runner)
	out := gw.ExecuteAuthorized(context.Background(), binding, denied, gatewayEnv{})
	if out.Result.Err == nil {
		t.Fatal("expected authorization rejection")
	}
	if exec.calls != 0 {
		t.Fatalf("executor was called %d times", exec.calls)
	}
}

func TestExecutionGatewayRejectsMismatchedAuthorization(t *testing.T) {
	binding, exec, _, runner := gatewayBinding(t)
	gw, _ := NewExecutionGateway(runner)
	auth := AuthorizationResult{
		CapabilityID: "another.capability",
		Decision: autonomy.CapabilityAuthorization{
			CapabilityID: "another.capability",
			Authority:    autonomy.AuthorityExecute,
			Allowed:      true,
		},
	}
	out := gw.ExecuteAuthorized(context.Background(), binding, auth, gatewayEnv{})
	if out.Result.Err == nil {
		t.Fatal("expected capability mismatch")
	}
	if exec.calls != 0 {
		t.Fatal("executor must not run on mismatch")
	}
}

func TestExecutionGatewayRejectsNilEnvironment(t *testing.T) {
	binding, exec, caps, runner := gatewayBinding(t)
	gate, _ := NewAuthorizationGate(caps)
	auth, _ := gate.AuthorizeBinding(binding, autonomy.ApprovalNone)
	gw, _ := NewExecutionGateway(runner)
	out := gw.ExecuteAuthorized(context.Background(), binding, auth, nil)
	if out.Result.Err == nil {
		t.Fatal("expected nil environment rejection")
	}
	if exec.calls != 0 {
		t.Fatal("executor must not run without environment")
	}
}
