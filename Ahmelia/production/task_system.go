package production

import (
	"fmt"
	"sort"
	"sync"

	"ahmelia/planning"
)

// TaskSystem B95.3 is the production-facing coordinator for planning.Task.
// It does not introduce a second Task model or copy tasks. planning remains
// the owner of task identity and task lifecycle fields.
type TaskSystem struct {
	mu   sync.RWMutex
	plan *planning.Plan
}

func NewTaskSystem(plan *planning.Plan) (*TaskSystem, error) {
	if plan == nil {
		return nil, fmt.Errorf("production: task system plan nil")
	}
	if plan.ID == "" || plan.GoalID == "" {
		return nil, fmt.Errorf("production: invalid plan")
	}
	return &TaskSystem{plan: plan}, nil
}

// Plan returns the same planning.Plan identity, never a copy.
func (s *TaskSystem) Plan() *planning.Plan {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.plan
}

// Tasks returns a new slice containing the original Task pointers.
func (s *TaskSystem) Tasks() []*planning.Task {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := append([]*planning.Task(nil), s.plan.Tasks...)
	sort.SliceStable(out, func(i, j int) bool { return out[i].Order < out[j].Order })
	return out
}

func (s *TaskSystem) Get(id string) (*planning.Task, bool) {
	if s == nil || id == "" {
		return nil, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, t := range s.plan.Tasks {
		if t != nil && t.ID == id {
			return t, true
		}
	}
	return nil, false
}

// NextRunnable returns the first pending task whose dependencies are complete.
// It does not start the task; the caller remains responsible for lifecycle.
func (s *TaskSystem) NextRunnable() *planning.Task {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	all := make(map[string]*planning.Task, len(s.plan.Tasks))
	for _, t := range s.plan.Tasks {
		if t != nil {
			all[t.ID] = t
		}
	}
	tasks := append([]*planning.Task(nil), s.plan.Tasks...)
	sort.SliceStable(tasks, func(i, j int) bool { return tasks[i].Order < tasks[j].Order })
	for _, t := range tasks {
		if t.Status == planning.TaskPending && t.CanStart(all) {
			return t
		}
	}
	return nil
}

func (s *TaskSystem) Start(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.getLocked(id)
	if !ok {
		return fmt.Errorf("production: task %q not found", id)
	}
	if t.Status != planning.TaskPending {
		return fmt.Errorf("production: task %q no está pendiente", id)
	}
	all := make(map[string]*planning.Task, len(s.plan.Tasks))
	for _, x := range s.plan.Tasks {
		if x != nil {
			all[x.ID] = x
		}
	}
	if !t.CanStart(all) {
		return fmt.Errorf("production: task %q tiene dependencias incompletas", id)
	}
	t.Start()
	return nil
}

func (s *TaskSystem) Complete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.getLocked(id)
	if !ok {
		return fmt.Errorf("production: task %q not found", id)
	}
	if t.Status != planning.TaskInProgress {
		return fmt.Errorf("production: task %q no está en progreso", id)
	}
	t.Complete()
	return nil
}

func (s *TaskSystem) Fail(id, reason string) error {
	if reason == "" {
		return fmt.Errorf("production: failure reason vacío")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.getLocked(id)
	if !ok {
		return fmt.Errorf("production: task %q not found", id)
	}
	if t.Status != planning.TaskInProgress {
		return fmt.Errorf("production: task %q no está en progreso", id)
	}
	t.Fail(reason)
	return nil
}

func (s *TaskSystem) Retry(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.getLocked(id)
	if !ok {
		return fmt.Errorf("production: task %q not found", id)
	}
	if t.Status != planning.TaskBlocked {
		return fmt.Errorf("production: task %q no está bloqueada", id)
	}
	t.Retry()
	return nil
}

func (s *TaskSystem) getLocked(id string) (*planning.Task, bool) {
	if id == "" {
		return nil, false
	}
	for _, t := range s.plan.Tasks {
		if t != nil && t.ID == id {
			return t, true
		}
	}
	return nil, false
}
