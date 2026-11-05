package production

import (
	"fmt"
	"sync"
	"time"

	"ahmelia/environment"
	memoryintelligence "ahmelia/memory_intelligence"
)

// ============================================================================
// WORKSPACE — B95.2
// ============================================================================
//
// B95.2 does not create a second model of the computer's workspace. The
// existing source of truth is environment.Manager, and its existing combined
// view is memory_intelligence.WorkspaceSnapshot.
//
// Production owns only the identity/reference needed to bind a production
// context to that existing workspace. The live contents are obtained through
// a WorkspaceSource whenever they are requested.

// WorkspaceSource is the minimal read contract Production needs from a
// workspace provider. It deliberately exposes the existing snapshot instead
// of inventing a parallel Project/File/App model.
type WorkspaceSource interface {
	Snapshot() (memoryintelligence.WorkspaceSnapshot, error)
}

// EnvironmentWorkspaceSource adapts the existing environment.Manager and
// memory_intelligence.Snapshot() into the production contract.
type EnvironmentWorkspaceSource struct {
	world *environment.Manager
}

func NewEnvironmentWorkspaceSource(world *environment.Manager) (*EnvironmentWorkspaceSource, error) {
	if world == nil {
		return nil, fmt.Errorf("production: workspace source world nil")
	}
	return &EnvironmentWorkspaceSource{world: world}, nil
}

func (s *EnvironmentWorkspaceSource) Snapshot() (memoryintelligence.WorkspaceSnapshot, error) {
	if s == nil || s.world == nil {
		return memoryintelligence.WorkspaceSnapshot{}, fmt.Errorf("production: workspace source unavailable")
	}
	return memoryintelligence.Snapshot(s.world), nil
}

// Workspace is the production-level identity of a workspace.
//
// It is intentionally a reference/context, not a second database containing
// files, tasks, plans, or application state. Those concepts remain owned by
// their existing systems. The source is queried on demand, avoiding stale
// duplicated state.
type Workspace struct {
	ID        string
	Source    WorkspaceSource
	CreatedAt time.Time
}

func NewWorkspace(id string, source WorkspaceSource) (*Workspace, error) {
	if id == "" {
		return nil, fmt.Errorf("production: workspace ID vacío")
	}
	if source == nil {
		return nil, fmt.Errorf("production: workspace source nil")
	}
	return &Workspace{ID: id, Source: source, CreatedAt: time.Now()}, nil
}

// Snapshot returns the live workspace view supplied by the existing source.
func (w *Workspace) Snapshot() (memoryintelligence.WorkspaceSnapshot, error) {
	if w == nil || w.Source == nil {
		return memoryintelligence.WorkspaceSnapshot{}, fmt.Errorf("production: workspace unavailable")
	}
	return w.Source.Snapshot()
}

// WorkspaceBinding connects a Production to a workspace without copying its
// state. It is separate from Production.WorkspaceID so callers can use the
// richer object when they need a live snapshot while retaining the stable ID
// already stored in Production.
type WorkspaceBinding struct {
	Workspace *Workspace
}

// BindWorkspace validates that the workspace identity matches the ID already
// attached to the production context, then returns a binding. It does not
// copy workspace state into Production.
func (p *Production) BindWorkspace(workspace *Workspace) (*WorkspaceBinding, error) {
	if p == nil {
		return nil, fmt.Errorf("production: nil")
	}
	if workspace == nil {
		return nil, fmt.Errorf("production: workspace nil")
	}
	if p.WorkspaceID == "" {
		return nil, fmt.Errorf("production: workspace ID no configurado")
	}
	if workspace.ID != p.WorkspaceID {
		return nil, fmt.Errorf("production: workspace %q no coincide con %q", workspace.ID, p.WorkspaceID)
	}
	return &WorkspaceBinding{Workspace: workspace}, nil
}

// WorkspaceReadiness reports whether a workspace reference can currently be
// read. A workspace may exist while having no active app/project yet; that is
// valid and is represented by the zero-value fields of WorkspaceSnapshot.
type WorkspaceReadiness struct {
	WorkspaceID string
	Readable    bool
	HasContext  bool
	CheckedAt   time.Time
}

func (w *Workspace) Readiness() WorkspaceReadiness {
	result := WorkspaceReadiness{CheckedAt: time.Now()}
	if w == nil {
		return result
	}
	result.WorkspaceID = w.ID
	snap, err := w.Snapshot()
	if err != nil {
		return result
	}
	result.Readable = true
	result.HasContext = snap.HasActiveApp
	return result
}

// WorkspaceRegistry provides stable lookup for production workspaces. It is
// deliberately a registry of references, not a store of duplicated workspace
// state. It is safe for concurrent readers/writers.
type WorkspaceRegistry struct {
	mu         sync.RWMutex
	workspaces map[string]*Workspace
}

func NewWorkspaceRegistry() *WorkspaceRegistry {
	return &WorkspaceRegistry{workspaces: make(map[string]*Workspace)}
}

func (r *WorkspaceRegistry) Register(workspace *Workspace) error {
	if r == nil {
		return fmt.Errorf("production: workspace registry nil")
	}
	if workspace == nil || workspace.ID == "" {
		return fmt.Errorf("production: invalid workspace")
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.workspaces[workspace.ID]; exists {
		return fmt.Errorf("production: workspace %q ya registrado", workspace.ID)
	}
	r.workspaces[workspace.ID] = workspace
	return nil
}

func (r *WorkspaceRegistry) Get(id string) (*Workspace, bool) {
	if r == nil || id == "" {
		return nil, false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	workspace, ok := r.workspaces[id]
	return workspace, ok
}

func (r *WorkspaceRegistry) Count() int {
	if r == nil {
		return 0
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.workspaces)
}
