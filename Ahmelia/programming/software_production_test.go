package programming

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type loopDependencyPreparerStub struct {
	called bool
	result DependencyResult
}

func (s *loopDependencyPreparerStub) Prepare(_ context.Context, req DependencyRequest) DependencyResult {
	s.called = true
	if s.result.ID == "" {
		s.result.ID = req.ID
		s.result.ProjectID = req.Project.ID
		s.result.Operation = req.Operation
	}
	return s.result
}

type loopBuildStub struct {
	called bool
	result Build
}

func (s *loopBuildStub) Build(_ context.Context, _ BuildRequest) Build {
	s.called = true
	return s.result
}

type loopTestStub struct {
	called bool
	result TestRun
}

func (s *loopTestStub) RunTests(_ context.Context, _ TestRequest) TestRun {
	s.called = true
	return s.result
}

type loopVerifyStub struct {
	called bool
	result CodeVerificationEvidence
}

func (s *loopVerifyStub) VerifyCode(_ CodeVerificationRequest) CodeVerificationEvidence {
	s.called = true
	return s.result
}

func loopProject(t *testing.T) CodeProject {
	t.Helper()
	return CodeProject{ID: "p1", Name: "demo", Root: t.TempDir(), Language: "go"}
}

func TestSoftwareProductionLoopPreparesGoDependenciesBeforeBuild(t *testing.T) {
	p := loopProject(t)
	if err := os.WriteFile(filepath.Join(p.Root, "go.mod"), []byte("module example.test/p\n\ngo 1.23.2\n"), 0644); err != nil {
		t.Fatal(err)
	}
	ops, _ := NewOSSourceFileOperations()
	editor, _ := NewMultiFileEditor(ops)
	deps := &loopDependencyPreparerStub{result: DependencyResult{Success: true}}
	b := &loopBuildStub{result: Build{ID: "b", ProjectID: p.ID, Command: "build", Status: BuildSuccess}}
	te := &loopTestStub{result: TestRun{ID: "t", ProjectID: p.ID, Command: "test", Status: TestPassed}}
	v := &loopVerifyStub{result: CodeVerificationEvidence{ProjectID: p.ID, Success: true}}
	loop, _ := NewSoftwareProductionLoop(editor, b, te, v)
	loop.DependencyPreparer = deps
	res, err := loop.Run(context.Background(), SoftwareProductionRequest{ID: "prod-dependencies", Project: p, Build: &BuildRequest{ID: "b", Project: p, Command: "build"}, Verification: CodeVerificationRequest{Project: p}})
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != SoftwareProductionSucceeded || !deps.called || res.DependencyPreparation == nil || !res.DependencyPreparation.Success || !b.called {
		t.Fatalf("dependency preparation did not converge before build: result=%+v deps=%+v buildCalled=%v", res, deps, b.called)
	}
}

func TestSoftwareProductionLoopStopsWhenDependencyPreparationFails(t *testing.T) {
	p := loopProject(t)
	if err := os.WriteFile(filepath.Join(p.Root, "go.mod"), []byte("module example.test/p\n\ngo 1.23.2\n"), 0644); err != nil {
		t.Fatal(err)
	}
	ops, _ := NewOSSourceFileOperations()
	editor, _ := NewMultiFileEditor(ops)
	deps := &loopDependencyPreparerStub{result: DependencyResult{Success: false, Errors: []string{"resolution denied"}}}
	b := &loopBuildStub{result: Build{ID: "b", ProjectID: p.ID, Command: "build", Status: BuildSuccess}}
	te := &loopTestStub{result: TestRun{ID: "t", ProjectID: p.ID, Command: "test", Status: TestPassed}}
	v := &loopVerifyStub{result: CodeVerificationEvidence{ProjectID: p.ID, Success: true}}
	loop, _ := NewSoftwareProductionLoop(editor, b, te, v)
	loop.DependencyPreparer = deps
	res, err := loop.Run(context.Background(), SoftwareProductionRequest{ID: "prod-dependencies-failed", Project: p, Build: &BuildRequest{ID: "b", Project: p, Command: "build"}, Verification: CodeVerificationRequest{Project: p}})
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != SoftwareProductionFailed || res.FailedStage != "dependencies" || !strings.Contains(res.FailureReason, "resolution denied") {
		t.Fatalf("dependency failure was not preserved: %+v", res)
	}
	if b.called || te.called || v.called {
		t.Fatal("build, tests, and verification must not run after dependency preparation fails")
	}
}

func TestSoftwareProductionLoopSuccess(t *testing.T) {
	p := loopProject(t)
	ops, err := NewOSSourceFileOperations()
	if err != nil {
		t.Fatal(err)
	}
	editor, err := NewMultiFileEditor(ops)
	if err != nil {
		t.Fatal(err)
	}
	b := &loopBuildStub{result: Build{ID: "b", ProjectID: p.ID, Command: "build", Status: BuildSuccess}}
	te := &loopTestStub{result: TestRun{ID: "t", ProjectID: p.ID, Command: "test", Status: TestPassed}}
	v := &loopVerifyStub{result: CodeVerificationEvidence{ProjectID: p.ID, Success: true}}
	loop, err := NewSoftwareProductionLoop(editor, b, te, v)
	if err != nil {
		t.Fatal(err)
	}

	res, err := loop.Run(context.Background(), SoftwareProductionRequest{
		ID: "prod-1", Project: p,
		Build:        &BuildRequest{ID: "b", Project: p, Command: "build"},
		Tests:        &TestRequest{ID: "t", Project: p, Command: "test"},
		Verification: CodeVerificationRequest{Project: p},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != SoftwareProductionSucceeded {
		t.Fatalf("status=%s", res.Status)
	}
	if !b.called || !te.called || !v.called {
		t.Fatal("expected all stages")
	}
}

func TestSoftwareProductionLoopStopsAfterBuildFailure(t *testing.T) {
	p := loopProject(t)
	ops, _ := NewOSSourceFileOperations()
	editor, _ := NewMultiFileEditor(ops)
	b := &loopBuildStub{result: Build{ID: "b", ProjectID: p.ID, Command: "build", Status: BuildFailed, ExitCode: 1}}
	te := &loopTestStub{result: TestRun{ID: "t", ProjectID: p.ID, Command: "test", Status: TestPassed}}
	v := &loopVerifyStub{result: CodeVerificationEvidence{ProjectID: p.ID, Success: true}}
	loop, _ := NewSoftwareProductionLoop(editor, b, te, v)
	res, err := loop.Run(context.Background(), SoftwareProductionRequest{
		ID: "prod-2", Project: p, Build: &BuildRequest{ID: "b", Project: p, Command: "build"},
		Tests: &TestRequest{ID: "t", Project: p, Command: "test"}, Verification: CodeVerificationRequest{Project: p},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != SoftwareProductionFailed || res.FailedStage != "build" {
		t.Fatalf("unexpected result: %+v", res)
	}
	if te.called || v.called {
		t.Fatal("later stages must not run")
	}
}

func TestSoftwareProductionLoopCancelledBeforeExecution(t *testing.T) {
	p := loopProject(t)
	ops, _ := NewOSSourceFileOperations()
	editor, _ := NewMultiFileEditor(ops)
	b := &loopBuildStub{result: Build{ID: "b", ProjectID: p.ID, Command: "build", Status: BuildSuccess}}
	te := &loopTestStub{result: TestRun{ID: "t", ProjectID: p.ID, Command: "test", Status: TestPassed}}
	v := &loopVerifyStub{result: CodeVerificationEvidence{ProjectID: p.ID, Success: true}}
	loop, _ := NewSoftwareProductionLoop(editor, b, te, v)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	res, err := loop.Run(ctx, SoftwareProductionRequest{ID: "prod-3", Project: p, Verification: CodeVerificationRequest{Project: p}})
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != SoftwareProductionCancelled {
		t.Fatalf("status=%s", res.Status)
	}
	if b.called || te.called || v.called {
		t.Fatal("cancelled run must not execute")
	}
}

func TestSoftwareProductionLoopBuildFailureConvergesToStructuredDiagnostic(t *testing.T) {
	p := loopProject(t)
	ops, _ := NewOSSourceFileOperations()
	editor, _ := NewMultiFileEditor(ops)
	b := &loopBuildStub{result: Build{
		ID: "b", ProjectID: p.ID, Command: "build", Status: BuildFailed, ExitCode: 1,
		Output: "main.go:10:5: undefined: missingValue\n",
	}}
	te := &loopTestStub{result: TestRun{ID: "t", ProjectID: p.ID, Command: "test", Status: TestPassed}}
	v := &loopVerifyStub{result: CodeVerificationEvidence{ProjectID: p.ID, Success: true}}
	loop, _ := NewSoftwareProductionLoop(editor, b, te, v)
	res, err := loop.Run(context.Background(), SoftwareProductionRequest{
		ID: "prod-evidence-build", Project: p,
		Build:        &BuildRequest{ID: "b", Project: p, Command: "build"},
		Verification: CodeVerificationRequest{Project: p},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.ErrorAnalysis == nil || len(res.ErrorAnalysis.Diagnostics) != 1 {
		t.Fatalf("missing structured build failure evidence: %+v", res.ErrorAnalysis)
	}
	if got := res.ErrorAnalysis.Diagnostics[0].ErrorType; got != "undefined_symbol" {
		t.Fatalf("diagnostic type=%q", got)
	}
	if !strings.Contains(res.FailureReason, "main.go:10") {
		t.Fatalf("failure reason lost diagnostic location: %q", res.FailureReason)
	}
}

func TestSoftwareProductionLoopVerificationFailurePreservesCheckEvidence(t *testing.T) {
	p := loopProject(t)
	ops, _ := NewOSSourceFileOperations()
	editor, _ := NewMultiFileEditor(ops)
	b := &loopBuildStub{result: Build{ID: "b", ProjectID: p.ID, Command: "build", Status: BuildSuccess}}
	te := &loopTestStub{result: TestRun{ID: "t", ProjectID: p.ID, Command: "test", Status: TestPassed}}
	v := &loopVerifyStub{result: CodeVerificationEvidence{ProjectID: p.ID, Success: false, Checks: []CodeVerificationCheck{{Name: "expected_file:main.go", Success: false, Reason: "file missing"}}}}
	loop, _ := NewSoftwareProductionLoop(editor, b, te, v)
	res, err := loop.Run(context.Background(), SoftwareProductionRequest{
		ID: "prod-evidence-verify", Project: p,
		Build: &BuildRequest{ID: "b", Project: p, Command: "build"},
		Tests: &TestRequest{ID: "t", Project: p, Command: "test"}, Verification: CodeVerificationRequest{Project: p},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != SoftwareProductionFailed || res.FailedStage != "verify" {
		t.Fatalf("unexpected verification result: %+v", res)
	}
	if !strings.Contains(res.FailureReason, "expected_file:main.go") || !strings.Contains(res.FailureReason, "file missing") {
		t.Fatalf("verification evidence lost in failure reason: %q", res.FailureReason)
	}
}
