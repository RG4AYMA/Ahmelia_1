package solving

import (
	"ahmelia/interaction"
	"strings"
	"testing"
)

// TestDecompose_UnaTaskPorSubgoal confirma la estructura central: una
// Task de nivel 1 por cada Subgoal que trajo Goal Analyzer, en el mismo
// orden.
func TestDecompose_UnaTaskPorSubgoal(t *testing.T) {
	a := NewAnalyzer()
	ga := a.Analyze("quiero crear una aplicacion para gestionar tareas")

	d := NewDecomposer()
	plan := d.Decompose(ga)

	if len(plan.Tasks) != len(ga.Subgoals) {
		t.Fatalf("esperaba %d tareas de nivel 1 (una por subgoal), obtuve %d", len(ga.Subgoals), len(plan.Tasks))
	}
	for i, task := range plan.Tasks {
		if task.Description != ga.Subgoals[i] {
			t.Errorf("Task[%d].Description = %q, esperaba %q (mismo orden que Subgoals)", i, task.Description, ga.Subgoals[i])
		}
	}
}

// TestDecompose_ExpandeSegunPlantillaDeFase confirma que una fase con
// keyword conocido ("implementar") se expande con los pasos de esa
// plantilla, no con los genéricos.
func TestDecompose_ExpandeSegunPlantillaDeFase(t *testing.T) {
	ga := &GoalAnalysis{
		MainGoal: "Probar",
		Subgoals: []string{"Implementar el código"},
	}

	d := NewDecomposer()
	plan := d.Decompose(ga)

	if len(plan.Tasks) != 1 {
		t.Fatalf("esperaba 1 task de nivel 1, obtuve %d", len(plan.Tasks))
	}
	task := plan.Tasks[0]
	if len(task.Subtasks) == 0 {
		t.Fatal("esperaba sub-tareas para 'Implementar el código', obtuve ninguna")
	}
	found := false
	for _, sub := range task.Subtasks {
		if strings.Contains(sub.Description, "esqueleto") {
			found = true
		}
	}
	if !found {
		t.Errorf("esperaba que la plantilla de 'implementar' incluyera 'armar el esqueleto base', subtasks: %+v", task.Subtasks)
	}
}

// TestDecompose_IDsJerarquicos confirma que los IDs de sub-tareas
// siguen el patrón T<n>.<m>, no un contador global plano — así una
// tarea sabe a qué padre pertenece solo mirando el ID.
func TestDecompose_IDsJerarquicos(t *testing.T) {
	ga := &GoalAnalysis{
		MainGoal: "x",
		Subgoals: []string{"Diseñar la arquitectura", "Probar"},
	}
	plan := NewDecomposer().Decompose(ga)

	if plan.Tasks[0].ID != "T1" || plan.Tasks[1].ID != "T2" {
		t.Fatalf("IDs de nivel 1 incorrectos: %q, %q", plan.Tasks[0].ID, plan.Tasks[1].ID)
	}
	for _, sub := range plan.Tasks[0].Subtasks {
		if !strings.HasPrefix(sub.ID, "T1.") {
			t.Errorf("sub-tarea de T1 con ID mal formado: %q", sub.ID)
		}
	}
}

// TestDecompose_SinSubgoalsPideAnalizarPrimero confirma que, sin
// Subgoals (GoalAnalysis vacío o no analizado), Describe() lo dice
// explícito en vez de devolver un plan vacío.
func TestDecompose_SinSubgoalsPideAnalizarPrimero(t *testing.T) {
	plan := NewDecomposer().Decompose(&GoalAnalysis{})
	desc := plan.Describe()
	if !strings.Contains(desc, "Goal Analyzer") {
		t.Fatalf("esperaba que Describe() señale que hace falta analizar primero, obtuve: %q", desc)
	}
}

// TestParseDecomposeQuery_DistintoDeGoalAnalysis confirma que los
// triggers de Problem Decomposer no se pisan con los de Goal Analyzer
// (son preguntas distintas: "qué es" vs "cómo se divide").
func TestParseDecomposeQuery_DistintoDeGoalAnalysis(t *testing.T) {
	if !ParseDecomposeQuery(interaction.Analyze("que tareas necesito para armar una tienda online", interaction.Context{})) {
		t.Error("esperaba detectar el trigger de Problem Decomposer")
	}
	if ParseDecomposeQuery(interaction.Analyze("analiza este objetivo: crear un juego", interaction.Context{})) {
		t.Error("un pedido de Goal Analyzer no debería disparar Problem Decomposer")
	}
}
