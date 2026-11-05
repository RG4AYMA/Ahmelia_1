package programming

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	memoryintelligence "ahmelia/memory_intelligence"
	"ahmelia/production"
)

// B96.2 — Code Workspace
//
// ProgrammingWorkspace is the programming-specific context over the generic
// production Workspace. It does not own files, processes, builds or runtime
// state. The generic Workspace remains the source of environment truth.
// CodeProject supplies programming metadata for the same workspace.
type ProgrammingWorkspace struct {
	ProductionWorkspace *production.Workspace
	Project             CodeProject
	CreatedAt           time.Time
}

func NewProgrammingWorkspace(workspace *production.Workspace, project CodeProject) (*ProgrammingWorkspace, error) {
	if workspace == nil {
		return nil, fmt.Errorf("programming: production workspace nil")
	}
	if err := project.Validate(); err != nil {
		return nil, err
	}
	if project.Root == "" {
		return nil, fmt.Errorf("programming: project root vacío")
	}
	return &ProgrammingWorkspace{
		ProductionWorkspace: workspace,
		Project:             cloneCodeProject(project),
		CreatedAt:           time.Now(),
	}, nil
}

// WorkspaceID is the stable identity owned by Production.
func (w *ProgrammingWorkspace) WorkspaceID() string {
	if w == nil || w.ProductionWorkspace == nil {
		return ""
	}
	return w.ProductionWorkspace.ID
}

// Snapshot delegates to the existing production workspace. No programming
// layer cache is introduced.
func (w *ProgrammingWorkspace) Snapshot() (memoryintelligence.WorkspaceSnapshot, error) {
	if w == nil || w.ProductionWorkspace == nil {
		return memoryintelligence.WorkspaceSnapshot{}, fmt.Errorf("programming: workspace unavailable")
	}
	return w.ProductionWorkspace.Snapshot()
}

// ProjectMetadata returns a defensive copy so callers cannot mutate the
// ProgrammingWorkspace's project context without going through its contract.
func (w *ProgrammingWorkspace) ProjectMetadata() (CodeProject, error) {
	if w == nil {
		return CodeProject{}, fmt.Errorf("programming: workspace nil")
	}
	return cloneCodeProject(w.Project), nil
}

// Context combines the live generic workspace observation with programming
// metadata. The observation is always obtained on demand.
type WorkspaceContext struct {
	WorkspaceID string
	Project     CodeProject
	Snapshot    memoryintelligence.WorkspaceSnapshot
	ObservedAt  time.Time
}

func (w *ProgrammingWorkspace) Context() (WorkspaceContext, error) {
	if w == nil {
		return WorkspaceContext{}, fmt.Errorf("programming: workspace nil")
	}
	snapshot, err := w.Snapshot()
	if err != nil {
		return WorkspaceContext{}, err
	}
	project, err := w.ProjectMetadata()
	if err != nil {
		return WorkspaceContext{}, err
	}
	return WorkspaceContext{
		WorkspaceID: w.WorkspaceID(),
		Project:     project,
		Snapshot:    snapshot,
		ObservedAt:  time.Now(),
	}, nil
}

// ProgrammingWorkspaceRegistry keeps programming contexts indexed by the
// existing production workspace identity. It stores references/metadata, not
// duplicated environment state.
type ProgrammingWorkspaceRegistry struct {
	mu         sync.RWMutex
	workspaces map[string]*ProgrammingWorkspace
}

func NewProgrammingWorkspaceRegistry() *ProgrammingWorkspaceRegistry {
	return &ProgrammingWorkspaceRegistry{workspaces: make(map[string]*ProgrammingWorkspace)}
}

func (r *ProgrammingWorkspaceRegistry) Register(workspace *ProgrammingWorkspace) error {
	if r == nil {
		return fmt.Errorf("programming: workspace registry nil")
	}
	if workspace == nil || workspace.WorkspaceID() == "" {
		return fmt.Errorf("programming: invalid workspace")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	id := workspace.WorkspaceID()
	if _, exists := r.workspaces[id]; exists {
		return fmt.Errorf("programming: workspace %q ya registrado", id)
	}
	r.workspaces[id] = workspace
	return nil
}

func (r *ProgrammingWorkspaceRegistry) Get(workspaceID string) (*ProgrammingWorkspace, bool) {
	if r == nil || workspaceID == "" {
		return nil, false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	workspace, ok := r.workspaces[workspaceID]
	return workspace, ok
}

func (r *ProgrammingWorkspaceRegistry) Count() int {
	if r == nil {
		return 0
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.workspaces)
}

// SourcePath resolves a project-relative source path without allowing it to
// escape the project root. It only normalizes/validates a path; it does not
// touch the filesystem. Actual file operations belong to B96.3.
func (w *ProgrammingWorkspace) SourcePath(relative string) (string, error) {
	if w == nil {
		return "", fmt.Errorf("programming: workspace nil")
	}
	if strings.TrimSpace(relative) == "" {
		return "", fmt.Errorf("programming: relative path vacío")
	}
	root, err := filepath.Abs(w.Project.Root)
	if err != nil {
		return "", fmt.Errorf("programming: project root inválido: %w", err)
	}
	candidate, err := filepath.Abs(filepath.Join(root, relative))
	if err != nil {
		return "", fmt.Errorf("programming: source path inválido: %w", err)
	}
	rel, err := filepath.Rel(root, candidate)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("programming: source path fuera del proyecto")
	}
	return candidate, nil
}

func cloneCodeProject(project CodeProject) CodeProject {
	project.SourceRoots = append([]string(nil), project.SourceRoots...)
	project.TestRoots = append([]string(nil), project.TestRoots...)
	return project
}
