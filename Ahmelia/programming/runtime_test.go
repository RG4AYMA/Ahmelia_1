package programming

import (
	"context"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestOSRuntimeExecutor_StartAndObserveCompletion(t *testing.T) {
	root := t.TempDir()
	project := CodeProject{ID: "p", Root: root}
	command := "echo runtime-ok"
	got, err := NewOSRuntimeExecutor().Start(context.Background(), RuntimeRequest{ID: "r1", Project: project, Command: command})
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != RuntimeRunning {
		t.Fatalf("expected running, got %+v", got)
	}
}

func TestOSRuntimeExecutor_GetCapturesOutputAndExitCode(t *testing.T) {
	root := t.TempDir()
	project := CodeProject{ID: "p", Root: root}
	exec := NewOSRuntimeExecutor()
	command := "echo runtime-ok"
	got, err := exec.Start(context.Background(), RuntimeRequest{ID: "r2", Project: project, Command: command})
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		state, ok := exec.Get(got.ID)
		if ok && state.Status != RuntimeRunning {
			if state.Status != RuntimeStopped || state.ExitCode == nil || *state.ExitCode != 0 {
				t.Fatalf("unexpected final runtime: %+v", state)
			}
			if !strings.Contains(state.Output, "runtime-ok") {
				t.Fatalf("expected output, got %q", state.Output)
			}
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("runtime did not finish")
}

func TestOSRuntimeExecutor_StopTerminatesProcess(t *testing.T) {
	root := t.TempDir()
	project := CodeProject{ID: "p", Root: root}
	command := "sleep 2"
	if runtime.GOOS == "windows" {
		command = "ping 127.0.0.1 -n 5 > nul"
	}
	exec := NewOSRuntimeExecutor()
	_, err := exec.Start(context.Background(), RuntimeRequest{ID: "r3", Project: project, Command: command})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := exec.Stop("r3"); err != nil {
		t.Fatal(err)
	}
}

func TestRuntimeRequestRejectsWorkingDirectoryOutsideProject(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	project := CodeProject{ID: "p", Root: root}
	_, err := NewOSRuntimeExecutor().Start(context.Background(), RuntimeRequest{ID: "r4", Project: project, WorkingDir: outside, Command: "echo x"})
	if err == nil {
		t.Fatal("expected validation error")
	}
}

func TestRuntimeRequestValidation(t *testing.T) {
	root := t.TempDir()
	project := CodeProject{ID: "p", Root: root}
	_, err := NewOSRuntimeExecutor().Start(context.Background(), RuntimeRequest{ID: "", Project: project, Command: "echo x"})
	if err == nil {
		t.Fatal("expected validation error")
	}
}
