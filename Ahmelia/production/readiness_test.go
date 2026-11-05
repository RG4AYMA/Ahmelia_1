package production

import (
	"context"
	"testing"

	"ahmelia/autonomy"
	"ahmelia/capability"
	"ahmelia/tools"
)

type readinessEnv struct{}

func (readinessEnv) Name() string { return "test" }

type readinessExec struct{}

func (readinessExec) Execute(_ context.Context, a tools.Action, _ tools.Environment) tools.Result {
	return tools.Result{ActionID: a.ID, Success: true}
}

type readinessTool struct{}

func (readinessTool) ID() tools.ID                  { return "filesystem" }
func (readinessTool) Capabilities() []capability.ID { return []capability.ID{"filesystem.create_file"} }
func (readinessTool) Executor() tools.Executor      { return readinessExec{} }

func testCapability() capability.Capability {
	return capability.Capability{
		ID: "filesystem.create_file", Domain: capability.DomainLocal,
		Description: "create a file", Action: "create_file",
		Permission: capability.PermissionAutomatic,
	}
}

func TestAuditReadyAndExtensibleWithoutCoreChanges(t *testing.T) {
	caps := capability.NewRegistry()
	if err := caps.Register(testCapability()); err != nil {
		t.Fatal(err)
	}
	tr := tools.NewRegistry()
	if err := tr.Register(readinessTool{}); err != nil {
		t.Fatal(err)
	}

	report := Audit(caps, tr)
	if !report.Ready {
		t.Fatalf("expected ready report: %+v", report)
	}
	if len(report.Findings) != 0 {
		t.Fatalf("unexpected findings: %+v", report.Findings)
	}

	// Extensibility contract: a new tool is connected only through its
	// capability declaration + Tool registration; no planner/cycle mutation.
	runner := tools.NewRunner(tr)
	result := runner.Execute(context.Background(), tools.Action{
		ID: "a1", CapabilityID: "filesystem.create_file", Name: "create_file",
	}, readinessEnv{})
	if !result.Success {
		t.Fatalf("expected tool execution success: %+v", result)
	}
}

func TestAuditRejectsInvalidCapabilityAndNilRegistries(t *testing.T) {
	report := Audit(nil, tools.NewRegistry())
	if report.Ready {
		t.Fatal("nil capability registry must not be ready")
	}

	caps := capability.NewRegistry()
	if err := caps.Register(capability.Capability{
		ID: "bad", Domain: capability.DomainLocal, Action: "x", Permission: "invalid",
	}); err == nil {
		t.Fatal("registry should reject invalid permission")
	}

	if autonomy.CanExecute(autonomy.AuthorityRecommend) {
		t.Fatal("recommend must never authorize execution")
	}
}

func TestAuditWarnsButDoesNotFailForUncoveredFutureCapability(t *testing.T) {
	caps := capability.NewRegistry()
	if err := caps.Register(testCapability()); err != nil {
		t.Fatal(err)
	}
	tr := tools.NewRegistry()
	report := Audit(caps, tr)
	if !report.Ready {
		t.Fatalf("uncovered capability should be a warning: %+v", report)
	}
	found := false
	for _, f := range report.Findings {
		if f.ID == "CAP-UNCOVERED-filesystem.create_file" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected uncovered-capability warning")
	}
}
