package production

import (
	"testing"
	"time"

	"ahmelia/planning"
)

func TestNewProductionReusesGoalIdentity(t *testing.T) {
	goal := planning.NewGoal("Crear archivo", "Crear un archivo de prueba", planning.PriorityMedium, "user")
	p, err := NewProduction("prod-1", goal)
	if err != nil {
		t.Fatal(err)
	}
	if p.Goal != goal {
		t.Fatal("Production debe conservar la misma identidad de planning.Goal")
	}
	if p.Status != ProductionPending {
		t.Fatalf("estado inicial inesperado: %q", p.Status)
	}
}

func TestProductionBindsExistingPlanAndTaskWithoutCopy(t *testing.T) {
	goal := planning.NewGoal("Crear archivo", "Crear un archivo de prueba", planning.PriorityMedium, "user")
	plan := &planning.Plan{ID: "plan-1", GoalID: goal.ID, Tasks: []*planning.Task{}}
	task := planning.NewTask(goal.ID, "Crear", "crear archivo", 1, time.Minute)
	plan.Tasks = append(plan.Tasks, task)

	p, err := NewProduction("prod-2", goal)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.BindPlan(plan); err != nil {
		t.Fatal(err)
	}
	if p.Plan != plan {
		t.Fatal("Production debe conservar la misma identidad de planning.Plan")
	}
	if err := p.SetCurrentTask(task); err != nil {
		t.Fatal(err)
	}
	if p.CurrentTask != task {
		t.Fatal("Production debe conservar la misma identidad de planning.Task")
	}
}

func TestProductionRejectsCrossGoalPlanAndTask(t *testing.T) {
	goalA := planning.NewGoal("A", "A", planning.PriorityMedium, "user")
	goalB := planning.NewGoal("B", "B", planning.PriorityMedium, "user")
	planB := &planning.Plan{ID: "plan-b", GoalID: goalB.ID}
	taskB := planning.NewTask(goalB.ID, "B", "B", 1, time.Minute)

	p, err := NewProduction("prod-3", goalA)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.BindPlan(planB); err == nil {
		t.Fatal("debe rechazar un plan de otro Goal")
	}
	if err := p.SetCurrentTask(taskB); err == nil {
		t.Fatal("debe rechazar una Task de otro Goal")
	}
}

func TestProductionLifecycleDoesNotMutateGoalOrTask(t *testing.T) {
	goal := planning.NewGoal("A", "A", planning.PriorityMedium, "user")
	task := planning.NewTask(goal.ID, "A", "A", 1, time.Minute)
	originalGoalStatus := goal.Status
	originalTaskStatus := task.Status

	p, err := NewProduction("prod-4", goal)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.SetCurrentTask(task); err != nil {
		t.Fatal(err)
	}
	if err := p.Start(); err != nil {
		t.Fatal(err)
	}
	if err := p.Complete(); err != nil {
		t.Fatal(err)
	}

	if goal.Status != originalGoalStatus {
		t.Fatal("B95.1 no debe mutar el estado del Goal al completar Production")
	}
	if task.Status != originalTaskStatus {
		t.Fatal("B95.1 no debe mutar el estado de la Task al completar Production")
	}
	if p.Status != ProductionCompleted {
		t.Fatalf("estado final inesperado: %q", p.Status)
	}
}

func TestProductionWorkspaceIsReferenceOnly(t *testing.T) {
	goal := planning.NewGoal("A", "A", planning.PriorityMedium, "user")
	p, err := NewProduction("prod-5", goal)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.SetWorkspaceID("workspace-1"); err != nil {
		t.Fatal(err)
	}
	if p.WorkspaceID != "workspace-1" {
		t.Fatalf("workspace ID inesperado: %q", p.WorkspaceID)
	}
}

func TestReuseMapHasNoProductionDuplicateEntities(t *testing.T) {
	entries := ReuseMap()
	if len(entries) == 0 {
		t.Fatal("ReuseMap no puede estar vacío")
	}
	for _, entry := range entries {
		if entry.Concept == "" || entry.Owner == "" {
			t.Fatalf("entrada inválida: %+v", entry)
		}
		if entry.Owner == "production" {
			t.Fatalf("B95.1 no debe declararse dueño duplicado: %+v", entry)
		}
	}
}
