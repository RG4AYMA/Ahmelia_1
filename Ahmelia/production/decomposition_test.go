package production

import (
	"ahmelia/planning"
	"testing"
)

type fakeDecomposer struct {
	plan  *planning.Plan
	calls int
}

func (f *fakeDecomposer) Decompose(g *planning.Goal) (*planning.Plan, error) {
	f.calls++
	return f.plan, nil
}

func TestDecomposeAndBindReusesExistingPlanAndTasks(t *testing.T) {
	g := planning.NewGoal("B95.4", "decompose", planning.PriorityMedium, "user")
	task := planning.NewTask(g.ID, "one", "first", 1, 0)
	plan := &planning.Plan{ID: "p-95-4", GoalID: g.ID, Tasks: []*planning.Task{task}, Feasible: true}
	p, err := NewProduction("prod-95-4", g)
	if err != nil {
		t.Fatal(err)
	}
	d := &fakeDecomposer{plan: plan}
	got, err := DecomposeAndBind(p, d)
	if err != nil {
		t.Fatal(err)
	}
	if got != plan || p.Plan != plan || p.Plan.Tasks[0] != task {
		t.Fatal("decomposition must preserve plan/task identity")
	}
	if d.calls != 1 {
		t.Fatalf("calls=%d", d.calls)
	}
	if task.Status != planning.TaskPending {
		t.Fatal("decomposition must not start tasks")
	}
}

func TestPlannerDecomposerUsesExistingPlanner(t *testing.T) {
	g := planning.NewGoal("B95.4 planner", "test", planning.PriorityMedium, "user")
	planner := planning.NewPlanner()
	d, err := NewPlannerDecomposer(planner)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := d.Decompose(g)
	if err != nil {
		t.Fatal(err)
	}
	if plan == nil || plan.GoalID != g.ID || len(plan.Tasks) == 0 {
		t.Fatal("expected existing planner to produce plan/tasks")
	}
	for _, task := range plan.Tasks {
		if task.GoalID != g.ID {
			t.Fatal("task belongs to another goal")
		}
	}
}

func TestDecomposeAndBindRejectsNil(t *testing.T) {
	if _, err := DecomposeAndBind(nil, nil); err == nil {
		t.Fatal("expected error")
	}
	g := planning.NewGoal("x", "x", planning.PriorityLow, "user")
	p, _ := NewProduction("p", g)
	if _, err := DecomposeAndBind(p, nil); err == nil {
		t.Fatal("expected decomposer error")
	}
}
