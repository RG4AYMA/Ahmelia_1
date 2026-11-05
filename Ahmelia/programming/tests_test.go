package programming

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestOSTestExecutor_Pass(t *testing.T) {
	root := t.TempDir()
	project := CodeProject{ID: "p", Root: root}
	got := NewOSTestExecutor().RunTests(context.Background(), TestRequest{ID: "t1", Project: project, Command: "echo tests-ok"})
	if got.Status != TestPassed || got.ExitCode != 0 {
		t.Fatalf("unexpected result: %+v", got)
	}
	if got.Output == "" || got.FinishedAt.Before(got.StartedAt) {
		t.Fatalf("expected output and valid timestamps: %+v", got)
	}
}

func TestOSTestExecutor_FailureCapturesExitCodeAndFailures(t *testing.T) {
	root := t.TempDir()
	project := CodeProject{ID: "p", Root: root}
	command := "echo FAIL: expected failure; exit 4"
	if runtime.GOOS == "windows" {
		command = "echo FAIL: expected failure & exit /b 4"
	}
	got := NewOSTestExecutor().RunTests(context.Background(), TestRequest{ID: "t2", Project: project, Command: command})
	if got.Status != TestFailed || got.ExitCode != 4 {
		t.Fatalf("unexpected result: %+v", got)
	}
	if len(got.Failures) == 0 {
		t.Fatal("expected failure diagnostics")
	}
}

func TestTestRequestRejectsWorkingDirectoryOutsideProject(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	project := CodeProject{ID: "p", Root: root}
	got := NewOSTestExecutor().RunTests(context.Background(), TestRequest{ID: "t3", Project: project, WorkingDir: outside, Command: "echo x"})
	if got.Status != TestFailed || len(got.Failures) == 0 {
		t.Fatalf("expected validation failure, got %+v", got)
	}
}

func TestOSTestExecutor_Timeout(t *testing.T) {
	root := t.TempDir()
	project := CodeProject{ID: "p", Root: root}
	command := "sleep 2"
	if runtime.GOOS == "windows" {
		command = "ping 127.0.0.1 -n 3 > nul"
	}
	got := NewOSTestExecutor().RunTests(context.Background(), TestRequest{ID: "t4", Project: project, Command: command, Timeout: 50 * time.Millisecond})
	if got.Status != TestFailed {
		t.Fatalf("expected timeout failure, got %+v", got)
	}
}

func TestTestExecutor_AllowsProjectSubdirectory(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "tests")
	if err := os.Mkdir(src, 0o755); err != nil {
		t.Fatal(err)
	}
	project := CodeProject{ID: "p", Root: root}
	got := NewOSTestExecutor().RunTests(context.Background(), TestRequest{ID: "t5", Project: project, WorkingDir: src, Command: "echo ok"})
	if got.Status != TestPassed {
		t.Fatalf("expected success, got %+v", got)
	}
}

func TestOSTestExecutor_NilContextIsSafe(t *testing.T) {
	root := t.TempDir()
	project := CodeProject{ID: "p", Root: root}
	got := NewOSTestExecutor().RunTests(nil, TestRequest{ID: "t6", Project: project, Command: "echo ok"})
	if got.Status != TestPassed {
		t.Fatalf("expected success, got %+v", got)
	}
}
