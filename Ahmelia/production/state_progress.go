package production

import (
	"fmt"
	"sync"
	"time"

	"ahmelia/planning"
)

// ProductionState is a read-only projection of the current production context.
// It does not replace Goal/Plan/Task state; it exposes their identity and the
// production lifecycle together without copying the owned entities.
type ProductionState struct {
	ProductionID  string
	GoalID        string
	PlanID        string
	CurrentTaskID string
	WorkspaceID   string
	Status        ProductionStatus
	UpdatedAt     time.Time
}

// State returns a stable snapshot of production state. Entity ownership stays
// in the existing planning and workspace systems.
func (p *Production) State() (ProductionState, error) {
	if p == nil {
		return ProductionState{}, fmt.Errorf("production: nil")
	}
	state := ProductionState{
		ProductionID: p.ID,
		WorkspaceID:  p.WorkspaceID,
		Status:       p.Status,
		UpdatedAt:    p.UpdatedAt,
	}
	if p.Goal != nil {
		state.GoalID = p.Goal.ID
	}
	if p.Plan != nil {
		state.PlanID = p.Plan.ID
	}
	if p.CurrentTask != nil {
		state.CurrentTaskID = p.CurrentTask.ID
	}
	return state, nil
}

// ProgressTracker is the Production boundary around planning.Progress.
// planning remains the owner of progress calculations and history; this type
// provides concurrency-safe access and synchronization with the current Plan.
type ProgressTracker struct {
	mu       sync.RWMutex
	progress *planning.Progress
	goal     *planning.Goal
	plan     *planning.Plan
}

func NewProgressTracker(goal *planning.Goal, plan *planning.Plan, estimated time.Duration) (*ProgressTracker, error) {
	if goal == nil || goal.ID == "" {
		return nil, fmt.Errorf("production: progress goal inválido")
	}
	if plan == nil || plan.ID == "" || plan.GoalID != goal.ID {
		return nil, fmt.Errorf("production: progress plan inválido")
	}
	if estimated < 0 {
		return nil, fmt.Errorf("production: negative estimated duration")
	}
	p := planning.NewProgress(goal.ID, len(plan.Tasks), estimated)
	return &ProgressTracker{progress: p, goal: goal, plan: plan}, nil
}

// Progress returns the same planning.Progress identity. Callers that need
// concurrent mutation should use the tracker methods rather than mutating it
// directly.
func (t *ProgressTracker) Progress() *planning.Progress {
	if t == nil {
		return nil
	}
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.progress
}

// Sync derives completed/failed task counts from the existing Task objects.
// It never creates or changes Task state.
func (t *ProgressTracker) Sync() error {
	if t == nil {
		return fmt.Errorf("production: progress tracker unavailable")
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.progress == nil || t.goal == nil || t.plan == nil {
		return fmt.Errorf("production: progress tracker incomplete")
	}
	completed, failed := 0, 0
	for _, task := range t.plan.Tasks {
		if task == nil {
			continue
		}
		switch task.Status {
		case planning.TaskCompleted:
			completed++
		case planning.TaskBlocked:
			// A blocked task is not necessarily a failed execution; retain
			// planning's explicit failed counter semantics.
		}
	}
	// Progress has no setter for a full reconciliation, so only record newly
	// observed transitions. Keep a monotonic completed count and derive the
	// current failure count from explicit task failure history when available.
	if completed > t.progress.TasksCompleted {
		for i := t.progress.TasksCompleted; i < completed; i++ {
			t.progress.UpdateTaskCompleted()
		}
	}
	if failed > t.progress.TasksFailed {
		for i := t.progress.TasksFailed; i < failed; i++ {
			t.progress.UpdateTaskFailed()
		}
	}
	t.goal.UpdateProgress(completed, len(t.plan.Tasks))
	return nil
}

// RecordFailure records a verified production failure in the existing
// planning.Progress model. Task lifecycle remains owned by planning.Task.
func (t *ProgressTracker) RecordFailure() error {
	if t == nil {
		return fmt.Errorf("production: progress tracker unavailable")
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.progress == nil {
		return fmt.Errorf("production: progress unavailable")
	}
	t.progress.UpdateTaskFailed()
	return nil
}

// AddBlocker and RemoveBlocker delegate to planning.Progress while preserving
// the tracker lock.
func (t *ProgressTracker) AddBlocker(blocker string) error {
	if t == nil || blocker == "" {
		return fmt.Errorf("production: invalid blocker")
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.progress == nil {
		return fmt.Errorf("production: progress unavailable")
	}
	t.progress.AddBlocker(blocker)
	return nil
}

func (t *ProgressTracker) RemoveBlocker(blocker string) error {
	if t == nil || blocker == "" {
		return fmt.Errorf("production: invalid blocker")
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.progress == nil {
		return fmt.Errorf("production: progress unavailable")
	}
	t.progress.RemoveBlocker(blocker)
	return nil
}
