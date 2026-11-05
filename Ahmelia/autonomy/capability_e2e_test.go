package autonomy

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"ahmelia/capability"
	"ahmelia/tools"
)

type e2eTool struct {
	id       tools.ID
	caps     []capability.ID
	executor tools.Executor
}

func (t e2eTool) ID() tools.ID                  { return t.id }
func (t e2eTool) Capabilities() []capability.ID { return t.caps }
func (t e2eTool) Executor() tools.Executor      { return t.executor }

type e2eEnv struct{ root string }

func (e e2eEnv) Name() string { return "filesystem-e2e" }

type createFileExecutor struct{}

func (createFileExecutor) Execute(_ context.Context, a tools.Action, env tools.Environment) tools.Result {
	e, ok := env.(e2eEnv)
	if !ok {
		return tools.Result{ActionID: a.ID, Err: os.ErrInvalid}
	}
	name := a.Inputs["path"]
	if name == "" {
		return tools.Result{ActionID: a.ID, Err: os.ErrInvalid}
	}
	path := filepath.Join(e.root, filepath.FromSlash(name))
	if err := os.WriteFile(path, []byte("enemy"), 0644); err != nil {
		return tools.Result{ActionID: a.ID, Err: err}
	}
	return tools.Result{ActionID: a.ID, Success: true, Output: path}
}

type fileObserver struct{}

func (fileObserver) Observe(_ context.Context, a tools.Action, _ tools.Result) (tools.Observation, error) {
	// The verifier receives the environment-independent observation through Details.
	return tools.Observation{Confirmed: true, Details: a.Inputs["path"]}, nil
}

type fileVerifier struct{ root string }

func (v fileVerifier) Verify(_ context.Context, a tools.Action, _ tools.Result, o tools.Observation) tools.Verification {
	path := filepath.Join(v.root, filepath.FromSlash(o.Details))
	_, err := os.Stat(path)
	return tools.Verification{Success: err == nil, Reason: "filesystem state verified", Observation: o}
}

type e2eBuilder struct{}

func (e2eBuilder) BuildAction(_ context.Context, _ string, d capability.DiscoveryResult) (tools.Action, error) {
	return tools.Action{ID: "e2e-create-file", CapabilityID: d.Capability.ID, Name: d.Capability.Action, Inputs: map[string]string{"path": "enemy.cs"}}, nil
}

func TestCapabilityE2E_ProducesAndVerifiesRealFilesystemState(t *testing.T) {
	root := t.TempDir()
	caps := capability.NewRegistry()
	caps.MustRegister(capability.Capability{
		ID: "filesystem.create_file", Domain: capability.DomainLocal, Action: "create_file",
		Description: "crear un archivo en el filesystem", Permission: capability.PermissionAutomatic,
	})

	tr := tools.NewRegistry()
	if err := tr.Register(e2eTool{id: "filesystem", caps: []capability.ID{"filesystem.create_file"}, executor: createFileExecutor{}}); err != nil {
		t.Fatal(err)
	}
	vr := tools.NewVerifiedRunner(tools.NewRunner(tr), fileObserver{}, fileVerifier{root: root}, 0)
	e2e := NewCapabilityE2E(caps, vr, e2eBuilder{}, e2eEnv{root: root}, nil, 0)

	result, err := e2e.Execute(context.Background(), "crear archivo enemigo", ApprovalNone)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Verified.Success {
		t.Fatalf("verification failed: %+v", result.Verified)
	}
	if _, err := os.Stat(filepath.Join(root, "enemy.cs")); err != nil {
		t.Fatalf("real filesystem artifact missing: %v", err)
	}
}

func TestCapabilityE2E_AskFirstCannotExecuteWithoutApproval(t *testing.T) {
	caps := capability.NewRegistry()
	caps.MustRegister(capability.Capability{ID: "filesystem.create_file", Domain: capability.DomainLocal, Action: "create_file", Description: "crear archivo", Permission: capability.PermissionAskFirst})
	e2e := NewCapabilityE2E(caps, nil, e2eBuilder{}, e2eEnv{root: t.TempDir()}, nil, 0)
	_, err := e2e.Execute(context.Background(), "crear archivo", ApprovalNone)
	if err == nil {
		t.Fatal("expected authorization failure")
	}
}
