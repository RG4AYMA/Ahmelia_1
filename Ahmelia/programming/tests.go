package programming

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// B96.9 — Test Execution
//
// Test execution is a programming-domain adapter. It runs the project's
// declared test command and returns a formal TestRun. Production/B95 remains
// responsible for authorization, safety, observation and verification.

type TestRequest struct {
	ID         string
	Project    CodeProject
	Command    string
	WorkingDir string
	Timeout    time.Duration
}

func (r TestRequest) Validate() error {
	if r.ID == "" {
		return fmt.Errorf("programming test request: ID vacío")
	}
	if err := r.Project.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(r.Command) == "" {
		return fmt.Errorf("programming test request %q: Command vacío", r.ID)
	}
	wd := r.WorkingDir
	if wd == "" {
		wd = r.Project.Root
	}
	if !pathWithin(wd, r.Project.Root) {
		return fmt.Errorf("programming test request %q: working directory fuera del proyecto", r.ID)
	}
	return nil
}

type TestExecutor interface {
	RunTests(context.Context, TestRequest) TestRun
}

type OSTestExecutor struct{}

func NewOSTestExecutor() *OSTestExecutor { return &OSTestExecutor{} }

func (e *OSTestExecutor) RunTests(ctx context.Context, req TestRequest) TestRun {
	started := time.Now().UTC()
	result := TestRun{ID: req.ID, ProjectID: req.Project.ID, Command: req.Command, Status: TestUnknown, StartedAt: started}
	if err := req.Validate(); err != nil {
		result.Status = TestFailed
		result.Failures = []string{err.Error()}
		result.FinishedAt = time.Now().UTC()
		return result
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if req.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, req.Timeout)
		defer cancel()
	}
	wd := req.WorkingDir
	if wd == "" {
		wd = req.Project.Root
	}
	cmd := testShellCommand(ctx, req.Command)
	cmd.Dir = wd
	output, err := cmd.CombinedOutput()
	result.Output = string(output)
	result.FinishedAt = time.Now().UTC()
	if err == nil {
		result.Status = TestPassed
		result.ExitCode = 0
		return result
	}

	result.Status = TestFailed
	result.ExitCode = testExitCode(err)
	if result.ExitCode == 0 && ctx.Err() != nil {
		result.ExitCode = -1
	}
	result.Failures = classifyTestFailures(result.Output)
	if len(result.Failures) == 0 {
		result.Failures = []string{err.Error()}
	}
	return result
}

func testShellCommand(ctx context.Context, command string) *exec.Cmd {
	if runtime.GOOS == "windows" {
		return exec.CommandContext(ctx, "cmd", "/C", command)
	}
	return exec.CommandContext(ctx, "sh", "-c", command)
}

func testExitCode(err error) int {
	if ee, ok := err.(*exec.ExitError); ok {
		return ee.ExitCode()
	}
	return 0
}

func classifyTestFailures(output string) []string {
	var failures []string
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		lower := strings.ToLower(line)
		if strings.Contains(lower, "fail") ||
			strings.Contains(lower, "error") ||
			strings.Contains(lower, "panic") ||
			strings.Contains(lower, "assert") ||
			strings.Contains(lower, "failed") ||
			strings.Contains(lower, "fatal") {
			failures = append(failures, line)
		}
	}
	return failures
}
