package programming

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// B96.15 — Version Control.
//
// Version control is a programming-domain adapter over Git. It provides
// repository state and explicit checkpoint/change operations while B95 keeps
// authority over authorization, safety, recovery and execution policy.

type VersionControlOperation string

const (
	VCStatus  VersionControlOperation = "status"
	VCDiff    VersionControlOperation = "diff"
	VCCommit  VersionControlOperation = "commit"
	VCBranch  VersionControlOperation = "branch"
	VCHistory VersionControlOperation = "history"
	VCRevert  VersionControlOperation = "revert"
)

type VersionControlRequest struct {
	ID        string
	Project   CodeProject
	Operation VersionControlOperation
	Message   string
	Branch    string
	Commit    string
	Timeout   time.Duration
}

func (r VersionControlRequest) Validate() error {
	if strings.TrimSpace(r.ID) == "" {
		return fmt.Errorf("programming version control request: ID vacío")
	}
	if err := r.Project.Validate(); err != nil {
		return err
	}
	switch r.Operation {
	case VCStatus, VCDiff, VCCommit, VCBranch, VCHistory, VCRevert:
	default:
		return fmt.Errorf("programming version control request %q: operación inválida", r.ID)
	}
	if r.Operation == VCCommit && strings.TrimSpace(r.Message) == "" {
		return fmt.Errorf("programming version control request %q: commit message vacío", r.ID)
	}
	if r.Operation == VCBranch && strings.TrimSpace(r.Branch) == "" {
		return fmt.Errorf("programming version control request %q: branch vacío", r.ID)
	}
	if r.Operation == VCRevert && strings.TrimSpace(r.Commit) == "" {
		return fmt.Errorf("programming version control request %q: commit vacío", r.ID)
	}
	return nil
}

type VersionControlResult struct {
	ID           string
	ProjectID    string
	Operation    VersionControlOperation
	Success      bool
	ExitCode     int
	Output       string
	Errors       []string
	Branch       string
	Commit       string
	ChangedFiles []string
	StartedAt    time.Time
	FinishedAt   time.Time
}

type VersionControl interface {
	Status(context.Context, VersionControlRequest) VersionControlResult
	Diff(context.Context, VersionControlRequest) VersionControlResult
	Commit(context.Context, VersionControlRequest) VersionControlResult
	Branch(context.Context, VersionControlRequest) VersionControlResult
	History(context.Context, VersionControlRequest) VersionControlResult
	Revert(context.Context, VersionControlRequest) VersionControlResult
}

type GitVersionControl struct{}

func NewGitVersionControl() *GitVersionControl { return &GitVersionControl{} }

func (g *GitVersionControl) Status(ctx context.Context, req VersionControlRequest) VersionControlResult {
	return g.run(ctx, req, VCStatus, "status", "--short", "--branch")
}

func (g *GitVersionControl) Diff(ctx context.Context, req VersionControlRequest) VersionControlResult {
	return g.run(ctx, req, VCDiff, "diff", "--")
}

func (g *GitVersionControl) Commit(ctx context.Context, req VersionControlRequest) VersionControlResult {
	if err := req.Validate(); err != nil {
		return failedVCResult(req, err)
	}
	return g.run(ctx, req, VCCommit, "commit", "-am", req.Message)
}

func (g *GitVersionControl) Branch(ctx context.Context, req VersionControlRequest) VersionControlResult {
	if err := req.Validate(); err != nil {
		return failedVCResult(req, err)
	}
	return g.run(ctx, req, VCBranch, "switch", "-c", req.Branch)
}

func (g *GitVersionControl) History(ctx context.Context, req VersionControlRequest) VersionControlResult {
	return g.run(ctx, req, VCHistory, "log", "--oneline", "--decorate", "-20")
}

func (g *GitVersionControl) Revert(ctx context.Context, req VersionControlRequest) VersionControlResult {
	if err := req.Validate(); err != nil {
		return failedVCResult(req, err)
	}
	// Revert is deliberately an explicit operation. B95 must authorize it;
	// this adapter never performs rollback implicitly after another operation.
	return g.run(ctx, req, VCRevert, "revert", "--no-edit", req.Commit)
}

func (g *GitVersionControl) run(ctx context.Context, req VersionControlRequest, op VersionControlOperation, args ...string) VersionControlResult {
	started := time.Now().UTC()
	result := VersionControlResult{ID: req.ID, ProjectID: req.Project.ID, Operation: op, StartedAt: started}
	if err := req.Validate(); err != nil {
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
	if !pathWithin(req.Project.Root, req.Project.Root) {
		result.Errors = []string{"working directory fuera del proyecto"}
		result.FinishedAt = time.Now().UTC()
		return result
	}
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = filepath.Clean(req.Project.Root)
	output, err := cmd.CombinedOutput()
	result.Output = string(output)
	result.ExitCode = vcExitCode(err)
	result.FinishedAt = time.Now().UTC()
	if err != nil {
		result.Errors = []string{err.Error()}
		if ctx.Err() != nil {
			result.Errors = append(result.Errors, ctx.Err().Error())
		}
		return result
	}
	result.Success = true
	return result
}

func vcExitCode(err error) int {
	if err == nil {
		return 0
	}
	if ee, ok := err.(*exec.ExitError); ok {
		return ee.ExitCode()
	}
	return -1
}

func failedVCResult(req VersionControlRequest, err error) VersionControlResult {
	return VersionControlResult{ID: req.ID, ProjectID: req.Project.ID, Operation: req.Operation, Errors: []string{err.Error()}, FinishedAt: time.Now().UTC()}
}
