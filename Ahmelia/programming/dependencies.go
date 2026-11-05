package programming

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// B96.14 — Dependency Management.
// Programming exposes dependency knowledge and operations; B95 remains the
// authority for authorization, safety, execution and verification.

type DependencyOperation string

const (
	DependencyResolve DependencyOperation = "resolve"
	DependencyInstall DependencyOperation = "install"
	DependencyUpdate  DependencyOperation = "update"
	DependencyRemove  DependencyOperation = "remove"
)

type DependencyRequest struct {
	ID         string
	Project    CodeProject
	Operation  DependencyOperation
	Dependency Dependency
	Command    string
	Timeout    time.Duration
}

func (r DependencyRequest) Validate() error {
	if strings.TrimSpace(r.ID) == "" {
		return fmt.Errorf("programming dependency request: ID vacío")
	}
	if err := r.Project.Validate(); err != nil {
		return err
	}
	switch r.Operation {
	case DependencyResolve, DependencyInstall, DependencyUpdate, DependencyRemove:
	default:
		return fmt.Errorf("programming dependency request %q: operación inválida", r.ID)
	}
	if r.Operation != DependencyResolve {
		if err := r.Dependency.Validate(); err != nil {
			return err
		}
	}
	return nil
}

type DependencyResult struct {
	ID           string
	ProjectID    string
	Operation    DependencyOperation
	Success      bool
	ExitCode     int
	Output       string
	Errors       []string
	Dependencies []Dependency
}

type DependencyManager interface {
	Resolve(context.Context, DependencyRequest) DependencyResult
	Install(context.Context, DependencyRequest) DependencyResult
	Update(context.Context, DependencyRequest) DependencyResult
	Remove(context.Context, DependencyRequest) DependencyResult
}

// GoDependencyManager is the first backend. It uses go.mod/go list and keeps
// the abstraction independent of Go so npm/NuGet/Cargo/etc. can be added later.
type GoDependencyManager struct{}

func NewGoDependencyManager() *GoDependencyManager { return &GoDependencyManager{} }

// DependencyPreparer establishes the language toolchain's canonical dependency
// metadata before compilation. It is separate from dependency discovery because
// preparation may update lock/sum files as part of a production attempt.
type DependencyPreparer interface {
	Prepare(context.Context, DependencyRequest) DependencyResult
}

// Prepare runs the Go toolchain's dependency reconciliation for the current
// project. `go mod tidy` derives go.sum from the manifest and imports; it does
// not fabricate checksums or rely on model-authored lock metadata.
func (m *GoDependencyManager) Prepare(ctx context.Context, req DependencyRequest) DependencyResult {
	if req.Operation != DependencyResolve {
		return failedDependencyResult(req, fmt.Errorf("go dependency manager: preparation requires resolve operation"))
	}
	return m.runGo(ctx, req, "mod", "tidy")
}

func (m *GoDependencyManager) Resolve(ctx context.Context, req DependencyRequest) DependencyResult {
	if err := req.Validate(); err != nil {
		return failedDependencyResult(req, err)
	}
	if err := validateGoProject(req.Project); err != nil {
		return failedDependencyResult(req, err)
	}
	content, err := os.ReadFile(filepath.Join(req.Project.Root, "go.mod"))
	if err != nil {
		return failedDependencyResult(req, err)
	}
	deps := parseGoModDependencies(req.Project.ID, string(content))
	return DependencyResult{ID: req.ID, ProjectID: req.Project.ID, Operation: DependencyResolve, Success: true, Dependencies: deps}
}

func (m *GoDependencyManager) Install(ctx context.Context, req DependencyRequest) DependencyResult {
	return m.runGo(ctx, req, "get", req.Dependency.Name)
}
func (m *GoDependencyManager) Update(ctx context.Context, req DependencyRequest) DependencyResult {
	return m.runGo(ctx, req, "get", "-u", req.Dependency.Name)
}
func (m *GoDependencyManager) Remove(ctx context.Context, req DependencyRequest) DependencyResult {
	return m.runGo(ctx, req, "mod", "edit", "-droprequire="+req.Dependency.Name)
}

func (m *GoDependencyManager) runGo(ctx context.Context, req DependencyRequest, args ...string) DependencyResult {
	if err := req.Validate(); err != nil {
		return failedDependencyResult(req, err)
	}
	if err := validateGoProject(req.Project); err != nil {
		return failedDependencyResult(req, err)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	command := "go"
	if req.Command != "" {
		command = req.Command
	}
	result := DependencyResult{ID: req.ID, ProjectID: req.Project.ID, Operation: req.Operation}
	cmd := goCommand(ctx, command, args...)
	cmd.Dir = req.Project.Root
	output, err := cmd.CombinedOutput()
	result.Output = string(output)
	if err != nil {
		result.Errors = []string{err.Error()}
		result.ExitCode = processExitCode(err)
		return result
	}
	result.Success = true
	return result
}

func validateGoProject(project CodeProject) error {
	if _, err := os.Stat(filepath.Join(project.Root, "go.mod")); err != nil {
		return fmt.Errorf("go dependency manager: go.mod no encontrado: %w", err)
	}
	return nil
}

func parseGoModDependencies(projectID, content string) []Dependency {
	var out []Dependency
	inBlock := false
	scanner := bufio.NewScanner(strings.NewReader(content))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "require (") {
			inBlock = true
			continue
		}
		if inBlock && line == ")" {
			inBlock = false
			continue
		}
		if strings.HasPrefix(line, "require ") && !strings.HasPrefix(line, "require (") {
			line = strings.TrimSpace(strings.TrimPrefix(line, "require "))
		} else if !inBlock {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) >= 2 && !strings.HasPrefix(fields[0], "//") {
			out = append(out, Dependency{Name: fields[0], Version: fields[1], Manager: "go", ProjectID: projectID})
		}
	}
	return out
}

func goCommand(ctx context.Context, command string, args ...string) *exec.Cmd {
	if runtime.GOOS == "windows" {
		return exec.CommandContext(ctx, command, args...)
	}
	return exec.CommandContext(ctx, command, args...)
}

func processExitCode(err error) int {
	if ee, ok := err.(*exec.ExitError); ok {
		return ee.ExitCode()
	}
	return -1
}

func failedDependencyResult(req DependencyRequest, err error) DependencyResult {
	return DependencyResult{ID: req.ID, ProjectID: req.Project.ID, Operation: req.Operation, Success: false, Errors: []string{err.Error()}}
}
