package programming

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// B96.8 — Build & Compilation
//
// Build is a programming-domain execution adapter. It delegates process
// execution to the operating system while keeping Production/B95 as the
// authority for authorization, safety and verification.

type BuildRequest struct {
	ID         string
	Project    CodeProject
	Command    string
	WorkingDir string
	Timeout    time.Duration
}

func (r BuildRequest) Validate() error {
	if r.ID == "" {
		return fmt.Errorf("programming build request: ID vacío")
	}
	if err := r.Project.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(r.Command) == "" {
		return fmt.Errorf("programming build request %q: Command vacío", r.ID)
	}
	wd := r.WorkingDir
	if wd == "" {
		wd = r.Project.Root
	}
	if !pathWithin(wd, r.Project.Root) {
		return fmt.Errorf("programming build request %q: working directory fuera del proyecto", r.ID)
	}
	return nil
}

type BuildExecutor interface {
	Build(context.Context, BuildRequest) Build
}

type OSBuildExecutor struct{}

func NewOSBuildExecutor() *OSBuildExecutor { return &OSBuildExecutor{} }

func (e *OSBuildExecutor) Build(ctx context.Context, req BuildRequest) Build {
	started := time.Now().UTC()
	result := Build{ID: req.ID, ProjectID: req.Project.ID, Command: req.Command, Status: BuildUnknown, StartedAt: started}
	if err := req.Validate(); err != nil {
		result.Status = BuildFailed
		result.Errors = []string{err.Error()}
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
	cmd := shellCommand(ctx, req.Command)
	cmd.Dir = wd
	output, err := cmd.CombinedOutput()
	result.Output = string(output)
	result.FinishedAt = time.Now().UTC()
	if err == nil {
		result.Status = BuildSuccess
		result.ExitCode = 0
	} else {
		result.Status = BuildFailed
		result.ExitCode = exitCode(err)
		if result.ExitCode == 0 && ctx.Err() != nil {
			result.ExitCode = -1
		}
		result.Errors = classifyBuildLines(result.Output, false)
		if len(result.Errors) == 0 {
			result.Errors = []string{err.Error()}
		}
	}
	// Warnings are extracted independently so successful builds can expose
	// compiler warnings without turning them into failures.
	result.Warnings = classifyBuildLines(result.Output, true)
	return result
}

func shellCommand(ctx context.Context, command string) *exec.Cmd {
	if runtime.GOOS == "windows" {
		return exec.CommandContext(ctx, "cmd", "/C", command)
	}
	return exec.CommandContext(ctx, "sh", "-c", command)
}

func exitCode(err error) int {
	if ee, ok := err.(*exec.ExitError); ok {
		return ee.ExitCode()
	}
	return 0
}

func classifyBuildLines(output string, warnings bool) []string {
	var out []string
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		lower := strings.ToLower(line)
		isWarning := strings.Contains(lower, "warning") || strings.Contains(lower, "advertencia")
		if warnings && isWarning {
			out = append(out, line)
		}
		if !warnings && !isWarning {
			out = append(out, line)
		}
	}
	return out
}

func pathWithin(path, root string) bool {
	absPath, err1 := filepath.Abs(path)
	absRoot, err2 := filepath.Abs(root)
	if err1 != nil || err2 != nil {
		return false
	}
	rel, err := filepath.Rel(absRoot, absPath)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))) && !filepath.IsAbs(rel)
}
