package programming

import "testing"

func verificationProject() CodeProject { return CodeProject{ID: "p1", Root: "."} }

func TestCodeVerifierBuildAndTestsPass(t *testing.T) {
	e := NewCodeVerifier().VerifyCode(CodeVerificationRequest{
		Project: verificationProject(),
		Build:   &Build{ID: "b1", ProjectID: "p1", Command: "build", Status: BuildSuccess, ExitCode: 0},
		Tests:   &TestRun{ID: "t1", ProjectID: "p1", Command: "test", Status: TestPassed, ExitCode: 0},
	})
	if !e.Success {
		t.Fatalf("expected success: %#v", e)
	}
	if e.ObjectiveStatus != "unassessed" {
		t.Fatalf("build/tests must not mark semantic objective assessed: %q", e.ObjectiveStatus)
	}
}

func TestCodeVerifierBuildFailureFails(t *testing.T) {
	e := NewCodeVerifier().VerifyCode(CodeVerificationRequest{
		Project: verificationProject(),
		Build:   &Build{ID: "b1", ProjectID: "p1", Command: "build", Status: BuildFailed, ExitCode: 1},
	})
	if e.Success {
		t.Fatal("expected failure")
	}
}

func TestCodeVerifierDoesNotInventFileEvidence(t *testing.T) {
	e := NewCodeVerifier().VerifyCode(CodeVerificationRequest{Project: verificationProject(), ExpectedFiles: []string{"main.go"}})
	if e.Success {
		t.Fatal("expected unresolved file evidence to fail closed")
	}
}
