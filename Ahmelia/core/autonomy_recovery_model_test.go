package core

import (
	"testing"

	"ahmelia/planning"
)

func TestB911RecoveryDecision_IsTypedAndValid(t *testing.T) {
	valid := []RecoveryDecision{
		RecoveryResume,
		RecoveryIdle,
		RecoveryBlocked,
		RecoveryNeedsUser,
	}
	for _, decision := range valid {
		if !decision.IsValid() {
			t.Fatalf("decision %q debería ser válida", decision)
		}
	}
	if RecoveryDecision("invented").IsValid() {
		t.Fatal("una decisión desconocida no debería ser válida")
	}
}

func TestB911RecoveryReason_IsTypedAndValid(t *testing.T) {
	valid := []RecoveryReason{
		RecoveryReasonPendingWork,
		RecoveryReasonCompleted,
		RecoveryReasonGoalBlocked,
		RecoveryReasonNoPlan,
		RecoveryReasonNoExecutableTask,
		RecoveryReasonRequiresUser,
		RecoveryReasonUnknown,
	}
	for _, reason := range valid {
		if !reason.IsValid() {
			t.Fatalf("reason %q debería ser válida", reason)
		}
	}
	if RecoveryReason("invented").IsValid() {
		t.Fatal("una razón desconocida no debería ser válida")
	}
}

func TestB911RecoveryCandidate_PreservesRuntimeIdentityAndPriority(t *testing.T) {
	goal := planning.NewGoal("Goal B91.1", "probar recuperación", planning.PriorityUrgent, "ahmelia")
	task := planning.NewTask(goal.ID, "continuar", "trabajo pendiente", 1, 0)
	plan := &planning.Plan{
		ID:       "plan-b911",
		GoalID:   goal.ID,
		Tasks:    []*planning.Task{task},
		Priority: goal.Priority,
		Feasible: true,
	}

	candidate := NewRecoveryCandidate(goal, plan, []*planning.Task{task})

	if candidate.Goal != goal {
		t.Fatal("el Candidate debe conservar el mismo puntero Goal")
	}
	if candidate.Plan != plan {
		t.Fatal("el Candidate debe conservar el mismo puntero Plan")
	}
	if len(candidate.PendingTasks) != 1 || candidate.PendingTasks[0] != task {
		t.Fatal("el Candidate debe conservar el mismo puntero Task")
	}
	if candidate.Priority != planning.PriorityUrgent {
		t.Fatalf("Priority = %v, quería %v", candidate.Priority, planning.PriorityUrgent)
	}
}

func TestB911RecoveryCandidate_CopiesTaskSliceWithoutCopyingTasks(t *testing.T) {
	goal := planning.NewGoal("Goal slice", "probar aislamiento del slice", planning.PriorityHigh, "user")
	task := planning.NewTask(goal.ID, "tarea", "", 1, 0)
	tasks := []*planning.Task{task}

	candidate := NewRecoveryCandidate(goal, nil, tasks)
	tasks[0] = nil

	if candidate.PendingTasks[0] != task {
		t.Fatal("el Candidate debe conservar su propio contenedor de slice")
	}
}

func TestB911RecoveryAssessment_ValidatesIdentityAndStructure(t *testing.T) {
	goal := planning.NewGoal("Goal assessment", "", planning.PriorityMedium, "user")
	task := planning.NewTask(goal.ID, "pendiente", "", 1, 0)
	plan := &planning.Plan{ID: "plan-ok", GoalID: goal.ID, Tasks: []*planning.Task{task}}

	assessment := NewRecoveryAssessment(
		NewRecoveryCandidate(goal, plan, []*planning.Task{task}),
		RecoveryResume,
		RecoveryReasonPendingWork,
	)

	if !assessment.Valid() {
		t.Fatal("assessment válida fue rechazada")
	}
}

func TestB911RecoveryAssessment_RejectsPlanFromAnotherGoal(t *testing.T) {
	goal := planning.NewGoal("Goal A", "", planning.PriorityMedium, "user")
	other := planning.NewGoal("Goal B", "", planning.PriorityMedium, "user")
	plan := &planning.Plan{ID: "wrong-plan", GoalID: other.ID}

	assessment := NewRecoveryAssessment(
		NewRecoveryCandidate(goal, plan, nil),
		RecoveryResume,
		RecoveryReasonPendingWork,
	)

	if assessment.Valid() {
		t.Fatal("assessment con Plan de otro Goal no debería ser válida")
	}
}

func TestB911RecoveryAssessment_RejectsTaskFromAnotherGoal(t *testing.T) {
	goal := planning.NewGoal("Goal A", "", planning.PriorityMedium, "user")
	other := planning.NewGoal("Goal B", "", planning.PriorityMedium, "user")
	task := planning.NewTask(other.ID, "wrong task", "", 1, 0)

	assessment := NewRecoveryAssessment(
		NewRecoveryCandidate(goal, nil, []*planning.Task{task}),
		RecoveryResume,
		RecoveryReasonPendingWork,
	)

	if assessment.Valid() {
		t.Fatal("assessment con Task de otro Goal no debería ser válida")
	}
}

func TestB911RecoveryAssessment_RejectsInvalidDecisionOrReason(t *testing.T) {
	goal := planning.NewGoal("Goal invalid", "", planning.PriorityLow, "user")
	candidate := NewRecoveryCandidate(goal, nil, nil)

	if NewRecoveryAssessment(candidate, RecoveryDecision("bad"), RecoveryReasonPendingWork).Valid() {
		t.Fatal("decisión inválida aceptada")
	}
	if NewRecoveryAssessment(candidate, RecoveryResume, RecoveryReason("bad")).Valid() {
		t.Fatal("razón inválida aceptada")
	}
}

func TestB911RecoveryAssessment_DoesNotMutateRuntimeState(t *testing.T) {
	goal := planning.NewGoal("Goal immutable", "", planning.PriorityHigh, "ahmelia")
	task := planning.NewTask(goal.ID, "pendiente", "", 1, 0)
	plan := &planning.Plan{ID: "plan-immutable", GoalID: goal.ID, Tasks: []*planning.Task{task}}

	statusBefore := goal.Status
	taskStatusBefore := task.Status

	assessment := NewRecoveryAssessment(
		NewRecoveryCandidate(goal, plan, []*planning.Task{task}),
		RecoveryResume,
		RecoveryReasonPendingWork,
	)

	if !assessment.Valid() {
		t.Fatal("assessment debería ser válida")
	}
	if goal.Status != statusBefore {
		t.Fatal("crear/validar assessment no debe mutar Goal")
	}
	if task.Status != taskStatusBefore {
		t.Fatal("crear/validar assessment no debe mutar Task")
	}
}
