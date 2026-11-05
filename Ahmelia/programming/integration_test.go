package programming

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	memory_intelligence "ahmelia/memory_intelligence"
	"ahmelia/production"
)

func TestProgrammingIntegrationBehavioralE2EProducesObservableVerifiedWorkspace(t *testing.T) {
	root := t.TempDir()
	project := CodeProject{ID: "behavioral-e2e", Name: "behavioral-e2e", Root: root, Language: "Go"}
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module behavioral-e2e\n\ngo 1.23.2\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	fake := integrationWorkspaceSource{}
	pw, err := production.NewWorkspace("behavioral-e2e-workspace", fake)
	if err != nil {
		t.Fatal(err)
	}
	ws, err := NewProgrammingWorkspace(pw, project)
	if err != nil {
		t.Fatal(err)
	}
	ops, err := NewOSSourceFileOperations(ws)
	if err != nil {
		t.Fatal(err)
	}
	editor, err := NewMultiFileEditor(ops)
	if err != nil {
		t.Fatal(err)
	}

	plan := MultiFilePlan{
		ID:        "behavioral-change",
		Project:   project,
		CreatedAt: time.Now(),
		Changes: []MultiFileChange{
			{ID: "create-main", Operation: SourceCreate, File: SourceFile{Path: "main.go", Language: "Go", Role: RoleEntrypoint, ProjectID: project.ID}, Content: "package main\n\nfunc main() { println(Answer()) }\n\nfunc Answer() int { return 42 }\n"},
			{ID: "create-test", Operation: SourceCreate, File: sourceTestFile(project.ID), Content: "package main\n\nimport \"testing\"\n\nfunc TestAnswer(t *testing.T) { if Answer() != 42 { t.Fatalf(\"unexpected answer\") } }\n"},
		},
	}

	build := BuildRequest{ID: "behavioral-build", Project: project, Command: "go build ./...", Timeout: 30 * time.Second}
	tests := TestRequest{ID: "behavioral-test", Project: project, Command: "go test ./...", Timeout: 30 * time.Second}
	req := SoftwareProductionRequest{ID: "behavioral-e2e-request", Project: project, Changes: &plan, Build: &build, Tests: &tests, Verification: CodeVerificationRequest{Project: project}}

	loop, err := NewSoftwareProductionLoop(editor, NewOSBuildExecutor(), NewOSTestExecutor(), NewCodeVerifier())
	if err != nil {
		t.Fatal(err)
	}
	integration, err := NewProgrammingIntegration(loop)
	if err != nil {
		t.Fatal(err)
	}

	result, err := integration.Run(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != SoftwareProductionSucceeded || !result.Production.Verification.Success {
		t.Fatalf("behavioral E2E did not verify: status=%s verification=%+v failure=%s", result.Status, result.Production.Verification, result.Production.FailureReason)
	}
	for _, path := range []string{"main.go", "main_test.go"} {
		if _, err := os.Stat(filepath.Join(root, path)); err != nil {
			t.Fatalf("observable workspace is missing %s: %v", path, err)
		}
	}
	if result.Production.Build == nil || result.Production.Build.ExitCode != 0 {
		t.Fatalf("build evidence is not successful: %+v", result.Production.Build)
	}
	if result.Production.Tests == nil || result.Production.Tests.ExitCode != 0 {
		t.Fatalf("test evidence is not successful: %+v", result.Production.Tests)
	}

	// The behavioral assertion is deliberately external to internal status:
	// a verified result must correspond to a real workspace containing the
	// produced source and a build/test process that actually passed.
}

func TestProgrammingIntegrationBehavioralE2ERejectsBrokenWorkspace(t *testing.T) {
	root := t.TempDir()
	project := CodeProject{ID: "behavioral-e2e-failure", Name: "behavioral-e2e-failure", Root: root, Language: "Go"}
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module behavioral-e2e-failure\n\ngo 1.23.2\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	fake := integrationWorkspaceSource{}
	pw, err := production.NewWorkspace("behavioral-e2e-failure-workspace", fake)
	if err != nil {
		t.Fatal(err)
	}
	ws, err := NewProgrammingWorkspace(pw, project)
	if err != nil {
		t.Fatal(err)
	}
	ops, err := NewOSSourceFileOperations(ws)
	if err != nil {
		t.Fatal(err)
	}
	editor, err := NewMultiFileEditor(ops)
	if err != nil {
		t.Fatal(err)
	}

	plan := MultiFilePlan{ID: "broken-change", Project: project, CreatedAt: time.Now(), Changes: []MultiFileChange{
		{ID: "create-broken", Operation: SourceCreate, File: SourceFile{Path: "main.go", Language: "Go", Role: RoleEntrypoint, ProjectID: project.ID}, Content: "package main\nfunc main( {\n"},
	}}
	req := SoftwareProductionRequest{
		ID: "behavioral-e2e-failure-request", Project: project, Changes: &plan,
		Build:        &BuildRequest{ID: "broken-build", Project: project, Command: "go build ./...", Timeout: 30 * time.Second},
		Tests:        &TestRequest{ID: "broken-test", Project: project, Command: "go test ./...", Timeout: 30 * time.Second},
		Verification: CodeVerificationRequest{Project: project},
	}
	loop, err := NewSoftwareProductionLoop(editor, NewOSBuildExecutor(), NewOSTestExecutor(), NewCodeVerifier())
	if err != nil {
		t.Fatal(err)
	}
	integration, err := NewProgrammingIntegration(loop)
	if err != nil {
		t.Fatal(err)
	}
	result, err := integration.Run(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != SoftwareProductionFailed {
		t.Fatalf("broken workspace escaped as success: %+v", result)
	}
	if result.Production.Verification.Success {
		t.Fatal("broken workspace was reported as verified")
	}
	if result.Production.FailedStage != "build" {
		t.Fatalf("failure stage=%q, want build", result.Production.FailedStage)
	}
}

func sourceTestFile(projectID string) SourceFile {
	return SourceFile{Path: "main_test.go", Language: "Go", Role: RoleTest, ProjectID: projectID}
}

func TestProgrammingIntegrationRealGoProduction(t *testing.T) {
	root := t.TempDir()
	project := CodeProject{ID: "integration-project", Name: "integration", Root: root, Language: "Go"}
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module integration\n\ngo 1.23.2\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	fake := integrationWorkspaceSource{}
	pw, err := production.NewWorkspace("integration-workspace", fake)
	if err != nil {
		t.Fatal(err)
	}
	ws, err := NewProgrammingWorkspace(pw, project)
	if err != nil {
		t.Fatal(err)
	}
	ops, err := NewOSSourceFileOperations(ws)
	if err != nil {
		t.Fatal(err)
	}
	editor, err := NewMultiFileEditor(ops)
	if err != nil {
		t.Fatal(err)
	}

	plan := MultiFilePlan{ID: "change-1", Project: project, CreatedAt: time.Now(), Changes: []MultiFileChange{
		{ID: "create-main", Operation: SourceCreate, File: SourceFile{Path: "main.go", Language: "Go", Role: RoleEntrypoint, ProjectID: project.ID}, Content: "package main\n\nfunc main() {}\n"},
		{ID: "create-test", Operation: SourceCreate, File: SourceFile{Path: "main_test.go", Language: "Go", Role: RoleTest, ProjectID: project.ID}, Content: "package main\n\nimport \"testing\"\n\nfunc TestMainCompiles(t *testing.T) {}\n"},
	}}

	build := BuildRequest{ID: "build-1", Project: project, Command: "go build ./...", Timeout: 30 * time.Second}
	tests := TestRequest{ID: "test-1", Project: project, Command: "go test ./...", Timeout: 30 * time.Second}
	req := SoftwareProductionRequest{ID: "integration-1", Project: project, Changes: &plan, Build: &build, Tests: &tests, Verification: CodeVerificationRequest{Project: project}}

	loop, err := NewSoftwareProductionLoop(editor, NewOSBuildExecutor(), NewOSTestExecutor(), NewCodeVerifier())
	if err != nil {
		t.Fatal(err)
	}
	integration, err := NewProgrammingIntegration(loop)
	if err != nil {
		t.Fatal(err)
	}

	result, err := integration.Run(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != SoftwareProductionSucceeded {
		t.Fatalf("expected success, got %s: %s", result.Status, result.Production.FailureReason)
	}
	if result.Production.Build == nil || result.Production.Build.Status != BuildSuccess {
		t.Fatal("build stage did not succeed")
	}
	if result.Production.Tests == nil || result.Production.Tests.Status != TestPassed {
		t.Fatal("test stage did not succeed")
	}
	if !result.Production.Verification.Success {
		t.Fatal("verification did not succeed")
	}
}

type integrationWorkspaceSource struct{}

func (integrationWorkspaceSource) Snapshot() (memory_intelligence.WorkspaceSnapshot, error) {
	return memory_intelligence.WorkspaceSnapshot{}, nil
}
