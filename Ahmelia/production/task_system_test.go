package production

import (
	"ahmelia/planning"
	"testing"
	"time"
)

func testPlan() *planning.Plan {
	g := planning.NewGoal("B95.3 test", "test task system", planning.PriorityMedium, "")
	a := planning.NewTask(g.ID, "A", "first", 1, time.Minute)
	b := planning.NewTask(g.ID, "B", "second", 2, time.Minute)
	b.Dependencies = []string{a.ID}
	return &planning.Plan{ID: "plan-test", GoalID: g.ID, Tasks: []*planning.Task{a, b}, Feasible: true}
}

func TestTaskSystemReusesTaskIdentity(t *testing.T) {
	p := testPlan()
	s, err := NewTaskSystem(p)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := s.Get(p.Tasks[0].ID)
	if !ok || got != p.Tasks[0] {
		t.Fatal("task identity was copied")
	}
	tasks := s.Tasks()
	if len(tasks) != 2 || tasks[0] != p.Tasks[0] || tasks[1] != p.Tasks[1] {
		t.Fatal("Tasks must expose original pointers")
	}
}

func TestTaskSystemDependencyLifecycle(t *testing.T) {
	p := testPlan()
	s, _ := NewTaskSystem(p)
	if got := s.NextRunnable(); got != p.Tasks[0] {
		t.Fatal("expected first task")
	}
	if err := s.Start(p.Tasks[1].ID); err == nil {
		t.Fatal("dependent task started too early")
	}
	if err := s.Start(p.Tasks[0].ID); err != nil {
		t.Fatal(err)
	}
	if err := s.Complete(p.Tasks[0].ID); err != nil {
		t.Fatal(err)
	}
	if got := s.NextRunnable(); got != p.Tasks[1] {
		t.Fatal("expected second task")
	}
	if err := s.Start(p.Tasks[1].ID); err != nil {
		t.Fatal(err)
	}
	if err := s.Fail(p.Tasks[1].ID, "verification failed"); err != nil {
		t.Fatal(err)
	}
	if err := s.Retry(p.Tasks[1].ID); err != nil {
		t.Fatal(err)
	}
	if p.Tasks[1].Status != planning.TaskPending {
		t.Fatal("retry must return task to pending")
	}
}

func TestTaskSystemRejectsInvalidPlan(t *testing.T) {
	if _, err := NewTaskSystem(nil); err == nil {
		t.Fatal("nil plan must fail")
	}
	if _, err := NewTaskSystem(&planning.Plan{}); err == nil {
		t.Fatal("invalid plan must fail")
	}
}
