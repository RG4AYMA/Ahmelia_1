package planning

import (
	"context"
	"testing"
	"time"
)

type countingDecomposer struct {
	calls      int
	blueprints []GoalTaskBlueprint
	err        error
}

func (d *countingDecomposer) DecomposeGoal(context.Context, Goal) ([]GoalTaskBlueprint, error) {
	d.calls++
	return d.blueprints, d.err
}

func precomputedFixture() []GoalTaskBlueprint {
	return []GoalTaskBlueprint{
		{Title: "Inspeccionar el estado actual", Description: "Leer el proyecto y la evidencia disponible", TimeEstimate: time.Minute},
		{Title: "Verificar el resultado", Description: "Comprobar el efecto observable", Dependencies: []int{0}, TimeEstimate: 2 * time.Minute},
	}
}

func TestPrecomputedPlanIsUsedBeforeDecomposer(t *testing.T) {
	planner := NewPlanner()
	goal := NewGoal("Meta de prueba", "Descripción de la meta", PriorityMedium, "test")
	planner.RegisterGoal(goal)

	blueprints := precomputedFixture()
	if err := ValidateGoalTaskBlueprints(goal, blueprints); err != nil {
		t.Fatalf("blueprints válidos rechazados: %v", err)
	}
	planner.SetPrecomputedBlueprints(goal.ID, blueprints)

	decomposer := &countingDecomposer{blueprints: []GoalTaskBlueprint{
		{Title: "Plan del modelo", Description: "No debía usarse"},
	}}
	planner.SetGoalTaskDecomposer(decomposer)

	plan := planner.GeneratePlan(goal)
	if decomposer.calls != 0 {
		t.Fatalf("el decomposer se llamó %d veces; el plan precomputado debía ganar", decomposer.calls)
	}
	if len(plan.Tasks) != 2 {
		t.Fatalf("tareas = %d, quería 2", len(plan.Tasks))
	}
	if plan.Tasks[0].Title != "Inspeccionar el estado actual" || plan.Tasks[1].Title != "Verificar el resultado" {
		t.Fatalf("plan inesperado: %+v", plan.Tasks)
	}
}

func TestInvalidPrecomputedFallsBackToDecomposer(t *testing.T) {
	planner := NewPlanner()
	goal := NewGoal("Meta de prueba", "Descripción", PriorityMedium, "test")
	planner.RegisterGoal(goal)

	invalid := []GoalTaskBlueprint{
		{Title: "Paso inválido", Description: "Depende de sí mismo", Dependencies: []int{0}},
	}
	if err := ValidateGoalTaskBlueprints(goal, invalid); err == nil {
		t.Fatal("el blueprint inválido debía rechazarse")
	}
	planner.SetPrecomputedBlueprints(goal.ID, invalid)

	decomposer := &countingDecomposer{blueprints: []GoalTaskBlueprint{
		{Title: "Plan válido del modelo", Description: "Recupera el plan"},
	}}
	planner.SetGoalTaskDecomposer(decomposer)

	plan := planner.GeneratePlan(goal)
	if decomposer.calls != 1 {
		t.Fatalf("decomposer calls = %d, quería 1", decomposer.calls)
	}
	if len(plan.Tasks) != 1 || plan.Tasks[0].Title != "Plan válido del modelo" {
		t.Fatalf("plan inesperado: %+v", plan.Tasks)
	}
}

func TestLatePrecomputedDoesNotReplaceGeneratedPlan(t *testing.T) {
	planner := NewPlanner()
	goal := NewGoal("Meta de prueba", "Descripción", PriorityMedium, "test")
	planner.RegisterGoal(goal)

	decomposer := &countingDecomposer{blueprints: []GoalTaskBlueprint{
		{Title: "Plan del modelo", Description: "Plan original"},
	}}
	planner.SetGoalTaskDecomposer(decomposer)
	first := planner.GeneratePlan(goal)
	if decomposer.calls != 1 || first.Tasks[0].Title != "Plan del modelo" {
		t.Fatalf("primer plan inesperado: calls=%d tasks=%+v", decomposer.calls, first.Tasks)
	}

	planner.SetPrecomputedBlueprints(goal.ID, precomputedFixture())
	second := planner.GeneratePlan(goal)
	if decomposer.calls != 2 {
		t.Fatalf("decomposer calls = %d, quería 2; el plan tardío no debía reemplazar", decomposer.calls)
	}
	if second.Tasks[0].Title != "Plan del modelo" {
		t.Fatalf("el plan tardío reemplazó el plan existente: %+v", second.Tasks)
	}
}
