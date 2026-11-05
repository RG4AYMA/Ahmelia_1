package production

import (
	"sync"
	"testing"
	"time"

	"ahmelia/planning"
)

func stateGoalPlan() (*planning.Goal, *planning.Plan, *planning.Task) {
	g := planning.NewGoal("state", "state", planning.PriorityMedium, "user")
	task := planning.NewTask(g.ID, "task", "task", 1, time.Minute)
	plan := &planning.Plan{ID: "plan-state", GoalID: g.ID, Tasks: []*planning.Task{task}}
	return g, plan, task
}

func TestProductionStateReferencesExistingIdentity(t *testing.T) {
	g, plan, task := stateGoalPlan()
	p, err := NewProduction("prod-state", g)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.BindPlan(plan); err != nil {
		t.Fatal(err)
	}
	if err := p.SetCurrentTask(task); err != nil {
		t.Fatal(err)
	}
	if err := p.SetWorkspaceID("ws-1"); err != nil {
		t.Fatal(err)
	}
	state, err := p.State()
	if err != nil {
		t.Fatal(err)
	}
	if state.GoalID != g.ID || state.PlanID != plan.ID || state.CurrentTaskID != task.ID || state.WorkspaceID != "ws-1" {
		t.Fatalf("unexpected state: %+v", state)
	}
}

func TestProgressTrackerReusesPlanningProgress(t *testing.T) {
	g, plan, task := stateGoalPlan()
	tracker, err := NewProgressTracker(g, plan, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if tracker.Progress().GoalID != g.ID {
		t.Fatal("wrong progress goal")
	}
	task.Start()
	task.Complete()
	if err := tracker.Sync(); err != nil {
		t.Fatal(err)
	}
	if tracker.Progress().TasksCompleted != 1 {
		t.Fatalf("completed = %d", tracker.Progress().TasksCompleted)
	}
	if g.TasksCompleted != 1 || g.Progress != 1 {
		t.Fatalf("goal progress not synchronized: %+v", g)
	}
}

func TestProgressTrackerRaceSafeAccess(t *testing.T) {
	g, plan, _ := stateGoalPlan()
	tracker, err := NewProgressTracker(g, plan, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if i%2 == 0 {
				_ = tracker.AddBlocker("b")
			} else {
				_ = tracker.RemoveBlocker("b")
			}
			_, _ = tracker.StateLike()
		}(i)
	}
	wg.Wait()
}

// StateLike is intentionally tiny: it forces a read through the tracker's
// lock without exposing the internal planning.Progress fields as mutable API.
func (t *ProgressTracker) StateLike() (int, int) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if t.progress == nil {
		return 0, 0
	}
	return t.progress.TasksCompleted, t.progress.TasksFailed
}
