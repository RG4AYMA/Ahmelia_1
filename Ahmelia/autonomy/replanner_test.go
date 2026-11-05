package autonomy

import (
	"testing"
	"time"

	"ahmelia/planning"
)

// chainedPlan arma un plan con `n` tareas encadenadas (cada una depende de
// la anterior), igual que planning.Planner.detectDependencies — suficiente
// para ejercitar hasPendingDependent sin depender del Planner real.
func chainedPlan(goalID string, n int) *planning.Plan {
	tasks := make([]*planning.Task, 0, n)
	for i := 0; i < n; i++ {
		task := planning.NewTask(goalID, "tarea", "desc", i+1, time.Hour)
		if i > 0 {
			task.Dependencies = append(task.Dependencies, tasks[i-1].ID)
		}
		tasks = append(tasks, task)
	}
	return &planning.Plan{ID: "plan-test", GoalID: goalID, Tasks: tasks, Feasible: true}
}

func TestReplanner_RetriesWhileAttemptsUnderLimit(t *testing.T) {
	goal := planning.NewGoal("g", "d", planning.PriorityMedium, "user")
	plan := chainedPlan(goal.ID, 3)
	task := plan.Tasks[0]
	task.Start() // Attempts = 1

	r := NewReplanner(ReplanLimits{MaxTaskRetries: 2, MaxPlanRegenerations: 1})
	decision := r.Decide(goal, plan, task)

	if decision.Action != ReplanRetryTask {
		t.Fatalf("Action = %q, esperaba %q (Attempts=%d < MaxTaskRetries=2)", decision.Action, ReplanRetryTask, task.Attempts)
	}
}

func TestReplanner_SkipsWhenNoPendingDependent(t *testing.T) {
	goal := planning.NewGoal("g", "d", planning.PriorityMedium, "user")
	plan := chainedPlan(goal.ID, 3)
	last := plan.Tasks[2] // nadie depende de la última tarea de la cadena
	last.Start()
	last.Start() // Attempts = 2, agota MaxTaskRetries=2

	r := NewReplanner(ReplanLimits{MaxTaskRetries: 2, MaxPlanRegenerations: 1})
	decision := r.Decide(goal, plan, last)

	if decision.Action != ReplanSkipTask {
		t.Fatalf("Action = %q, esperaba %q (última tarea, sin dependientes)", decision.Action, ReplanSkipTask)
	}
}

func TestReplanner_RegeneratesWhenDependentsRemainAndBudgetAvailable(t *testing.T) {
	goal := planning.NewGoal("g", "d", planning.PriorityMedium, "user")
	plan := chainedPlan(goal.ID, 3)
	first := plan.Tasks[0] // task[1] y task[2] dependen, directa o indirectamente
	first.Start()
	first.Start() // Attempts = 2, agota MaxTaskRetries=2

	r := NewReplanner(ReplanLimits{MaxTaskRetries: 2, MaxPlanRegenerations: 1})
	decision := r.Decide(goal, plan, first)

	if decision.Action != ReplanRegeneratePlan {
		t.Fatalf("Action = %q, esperaba %q (hay dependientes pendientes, regeneración disponible)", decision.Action, ReplanRegeneratePlan)
	}
}

func TestReplanner_GivesUpWhenRegenerationBudgetExhausted(t *testing.T) {
	goal := planning.NewGoal("g", "d", planning.PriorityMedium, "user")
	plan := chainedPlan(goal.ID, 3)
	first := plan.Tasks[0]
	first.Start()
	first.Start() // Attempts = 2

	r := NewReplanner(ReplanLimits{MaxTaskRetries: 2, MaxPlanRegenerations: 1})
	r.RecordRegeneration(goal.ID) // ya se gastó la única regeneración permitida

	decision := r.Decide(goal, plan, first)
	if decision.Action != ReplanGiveUp {
		t.Fatalf("Action = %q, esperaba %q (regeneraciones agotadas)", decision.Action, ReplanGiveUp)
	}
}

func TestReplanner_ResetGoalClearsRegenerationCount(t *testing.T) {
	goal := planning.NewGoal("g", "d", planning.PriorityMedium, "user")
	plan := chainedPlan(goal.ID, 3)
	first := plan.Tasks[0]
	first.Start()
	first.Start()

	r := NewReplanner(ReplanLimits{MaxTaskRetries: 2, MaxPlanRegenerations: 1})
	r.RecordRegeneration(goal.ID)
	r.ResetGoal(goal.ID)

	decision := r.Decide(goal, plan, first)
	if decision.Action != ReplanRegeneratePlan {
		t.Fatalf("Action = %q, esperaba %q tras ResetGoal (contador debía volver a 0)", decision.Action, ReplanRegeneratePlan)
	}
}

func TestReplanner_DefaultLimitsUsedOnZeroValue(t *testing.T) {
	r := NewReplanner(ReplanLimits{})
	if r.limits != DefaultReplanLimits {
		t.Fatalf("limits = %+v, esperaba DefaultReplanLimits = %+v", r.limits, DefaultReplanLimits)
	}
}

func TestReplanner_DecideHandlesNilInputsSafely(t *testing.T) {
	r := NewReplanner(ReplanLimits{})
	decision := r.Decide(nil, nil, nil)
	if decision.Action != ReplanGiveUp {
		t.Fatalf("Action = %q, esperaba %q ante entradas nil", decision.Action, ReplanGiveUp)
	}
}
