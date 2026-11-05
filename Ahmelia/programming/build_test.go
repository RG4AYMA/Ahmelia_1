package programming

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestOSBuildExecutor_SuccessCapturesOutput(t *testing.T) {
	root := t.TempDir()
	project := CodeProject{ID: "p", Root: root}
	command := "echo build-ok"
	if runtime.GOOS == "windows" {
		command = "echo build-ok"
	}
	got := NewOSBuildExecutor().Build(context.Background(), BuildRequest{ID: "b1", Project: project, Command: command})
	if got.Status != BuildSuccess || got.ExitCode != 0 {
		t.Fatalf("unexpected result: %+v", got)
	}
	if got.Output == "" {
		t.Fatal("expected output")
	}
	if got.FinishedAt.Before(got.StartedAt) {
		t.Fatal("invalid timestamps")
	}
}

func TestOSBuildExecutor_FailureCapturesExitCodeAndErrors(t *testing.T) {
	root := t.TempDir()
	project := CodeProject{ID: "p", Root: root}
	command := "exit 7"
	if runtime.GOOS == "windows" {
		command = "exit /b 7"
	}
	got := NewOSBuildExecutor().Build(context.Background(), BuildRequest{ID: "b2", Project: project, Command: command})
	if got.Status != BuildFailed {
		t.Fatalf("expected failed, got %+v", got)
	}
	if got.ExitCode != 7 {
		t.Fatalf("expected exit code 7, got %d", got.ExitCode)
	}
	if len(got.Errors) == 0 {
		t.Fatal("expected errors")
	}
}

func TestBuildRequestRejectsWorkingDirectoryOutsideProject(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	project := CodeProject{ID: "p", Root: root}
	got := NewOSBuildExecutor().Build(context.Background(), BuildRequest{ID: "b3", Project: project, WorkingDir: outside, Command: "echo x"})
	if got.Status != BuildFailed || len(got.Errors) == 0 {
		t.Fatalf("expected validation failure, got %+v", got)
	}
}

func TestOSBuildExecutor_Timeout(t *testing.T) {
	root := t.TempDir()
	project := CodeProject{ID: "p", Root: root}
	command := "sleep 2"
	if runtime.GOOS == "windows" {
		command = "ping 127.0.0.1 -n 3 > nul"
	}
	got := NewOSBuildExecutor().Build(context.Background(), BuildRequest{ID: "b4", Project: project, Command: command, Timeout: 50 * time.Millisecond})
	if got.Status != BuildFailed {
		t.Fatalf("expected timeout failure, got %+v", got)
	}
}

func TestBuildDoesNotEscapeProjectWithRelativePath(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	project := CodeProject{ID: "p", Root: root}
	got := NewOSBuildExecutor().Build(context.Background(), BuildRequest{ID: "b5", Project: project, WorkingDir: filepath.Join(root, "src"), Command: "echo ok"})
	if got.Status != BuildSuccess {
		t.Fatalf("expected success, got %+v", got)
	}
}
