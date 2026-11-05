package core

import (
	"context"
	"testing"

	"ahmelia/autonomy"
	"ahmelia/planning"
)

type b912TestMapper struct{}

func (b912TestMapper) MapTask(_ context.Context, _ *planning.Goal, task *planning.Task) (autonomy.ActionRequest, error) {
	return autonomy.ActionRequest{ID: task.ID, Action: "noop"}, nil
}

func newB912Runtime(t *testing.T) (*planning.Planner, *autonomy.AutonomousState, *autonomy.GoalPlanningAdapter) {
	t.Helper()
	planner := planning.NewPlanner()
	state := autonomy.New()
	monitor := autonomy.NewGoalMonitor(state, 0)
	mapper := b912TestMapper{}
	adapter, err := autonomy.NewGoalPlanningAdapter(state, monitor, planner, mapper, nil, nil)
	if err != nil {
		t.Fatalf("NewGoalPlanningAdapter: %v", err)
	}
	return planner, state, adapter
}

func TestB912RecoveryAnalyzer_SanoConTrabajoPendiente(t *testing.T) {
	planner, _, adapter := newB912Runtime(t)
	goal := planning.NewGoal("Mejorar Reasoner", "continuar", planning.PriorityHigh, "ahmelia")
	planner.RegisterGoal(goal)

	plan, err := adapter.EnsurePlan(goal)
	if err != nil {
		t.Fatalf("EnsurePlan: %v", err)
	}

	analyzer, err := NewRecoveryAnalyzer(planner, adapter)
	if err != nil {
		t.Fatalf("NewRecoveryAnalyzer: %v", err)
	}
	got := analyzer.Analyze(goal)

	if got == nil || got.Candidate.Goal != goal || got.Candidate.Plan != plan {
		t.Fatal("el análisis no preservó la identidad runtime")
	}
	if !got.PlanExists || !got.PlanFeasible {
		t.Fatalf("plan sano mal diagnosticado: %+v", got)
	}
	if got.PendingTasks == 0 || got.PendingRunnable == 0 {
		t.Fatalf("trabajo pendiente no detectado: %+v", got)
	}
	if got.Reason != RecoveryReasonPendingWork {
		t.Fatalf("Reason = %q, quería pending_work", got.Reason)
	}
	if got.Candidate.Priority != planning.PriorityHigh {
		t.Fatalf("Priority = %v, quería high", got.Candidate.Priority)
	}
	// B91.2 no decide todavía.
	if got.Reason == RecoveryReasonRequiresUser {
		t.Fatal("B91.2 no debe inventar una petición al usuario")
	}
}

func TestB912RecoveryAnalyzer_SinPlanNoGeneraPlan(t *testing.T) {
	planner, _, adapter := newB912Runtime(t)
	goal := planning.NewGoal("Objetivo sin plan", "", planning.PriorityMedium, "user")
	planner.RegisterGoal(goal)

	analyzer, err := NewRecoveryAnalyzer(planner, adapter)
	if err != nil {
		t.Fatalf("NewRecoveryAnalyzer: %v", err)
	}
	got := analyzer.Analyze(goal)

	if got == nil || got.PlanExists {
		t.Fatalf("esperaba diagnóstico sin plan: %+v", got)
	}
	if got.Reason != RecoveryReasonNoPlan {
		t.Fatalf("Reason = %q, quería no_plan", got.Reason)
	}
	if len(adapter.Plans()) != 0 {
		t.Fatal("B91.2 no debe generar un Plan como efecto lateral")
	}
}

func TestB912RecoveryAnalyzer_ClasificaEstadosDeTasks(t *testing.T) {
	planner, _, adapter := newB912Runtime(t)
	goal := planning.NewGoal("Clasificar tareas", "", planning.PriorityHigh, "user")
	planner.RegisterGoal(goal)

	t1 := planning.NewTask(goal.ID, "pendiente", "", 1, 0)
	t2 := planning.NewTask(goal.ID, "en curso", "", 2, 0)
	t3 := planning.NewTask(goal.ID, "completada", "", 3, 0)
	t4 := planning.NewTask(goal.ID, "bloqueada", "", 4, 0)
	t5 := planning.NewTask(goal.ID, "cancelada", "", 5, 0)
	t2.Start()
	t3.Complete()
	t4.Fail("fallo")
	t5.Status = planning.TaskCancelled
	t2.Dependencies = []string{t1.ID}
	t3.Dependencies = []string{t1.ID}
	plan := &planning.Plan{
		ID: "plan-clasificacion", GoalID: goal.ID,
		Tasks:    []*planning.Task{t1, t2, t3, t4, t5},
		Feasible: true,
	}
	for _, task := range plan.Tasks {
		planner.RegisterTask(task)
	}
	adapter.LoadState(map[string]*planning.Plan{goal.ID: plan}, nil)

	analyzer, _ := NewRecoveryAnalyzer(planner, adapter)
	got := analyzer.Analyze(goal)

	if got.TaskCount != 5 || got.PendingTasks != 1 || got.InProgressTasks != 1 ||
		got.CompletedTasks != 1 || got.BlockedTasks != 1 || got.CancelledTasks != 1 {
		t.Fatalf("conteos incorrectos: %+v", got)
	}
	if !got.WasInProgress {
		t.Fatal("no detectó Task InProgress")
	}
	if got.PendingRunnable != 1 {
		t.Fatalf("PendingRunnable = %d, quería 1", got.PendingRunnable)
	}
}

func TestB912RecoveryAnalyzer_PendientePeroDependenciaNoResuelta(t *testing.T) {
	planner, _, adapter := newB912Runtime(t)
	goal := planning.NewGoal("Dependencia pendiente", "", planning.PriorityMedium, "user")
	planner.RegisterGoal(goal)

	first := planning.NewTask(goal.ID, "bloqueadora", "", 1, 0)
	first.Status = planning.TaskBlocked
	second := planning.NewTask(goal.ID, "dependiente", "", 2, 0)
	second.Dependencies = []string{first.ID}
	plan := &planning.Plan{
		ID: "plan-dependencia", GoalID: goal.ID,
		Tasks: []*planning.Task{first, second}, Feasible: true,
	}
	planner.RegisterTask(first)
	planner.RegisterTask(second)
	adapter.LoadState(map[string]*planning.Plan{goal.ID: plan}, nil)

	analyzer, _ := NewRecoveryAnalyzer(planner, adapter)
	got := analyzer.Analyze(goal)

	if got.PendingTasks != 1 || got.PendingRunnable != 0 {
		t.Fatalf("dependencia no resuelta mal analizada: %+v", got)
	}
	if got.PlanResolvable {
		t.Fatal("un Plan sin Task ejecutable no debe marcarse resoluble")
	}
	if got.Reason != RecoveryReasonNoExecutableTask {
		t.Fatalf("Reason = %q, quería no_executable_task", got.Reason)
	}
}

func TestB912RecoveryAnalyzer_PlanInfeasibleNoEsResume(t *testing.T) {
	planner, _, adapter := newB912Runtime(t)
	goal := planning.NewGoal("Plan inviable", "", planning.PriorityMedium, "user")
	planner.RegisterGoal(goal)

	task := planning.NewTask(goal.ID, "tarea", "", 1, 0)
	plan := &planning.Plan{
		ID: "plan-inviable", GoalID: goal.ID,
		Tasks: []*planning.Task{task}, Feasible: false,
		Issues: []string{"dependencia circular"},
	}
	planner.RegisterTask(task)
	adapter.LoadState(map[string]*planning.Plan{goal.ID: plan}, nil)

	analyzer, _ := NewRecoveryAnalyzer(planner, adapter)
	got := analyzer.Analyze(goal)

	if got.PlanFeasible {
		t.Fatal("PlanFeasible debería ser false")
	}
	if got.PlanResolvable {
		t.Fatal("Plan inviable no debe ser resoluble")
	}
	if got.Reason != RecoveryReasonNoExecutableTask {
		t.Fatalf("Reason = %q, quería no_executable_task", got.Reason)
	}
	if len(got.PlanIssues) != 1 {
		t.Fatalf("Issues no preservados: %+v", got.PlanIssues)
	}
}

func TestB912RecoveryAnalyzer_BlockedAndCompletedSonHechosTerminales(t *testing.T) {
	planner, _, adapter := newB912Runtime(t)

	blocked := planning.NewGoal("Bloqueado", "", planning.PriorityHigh, "user")
	blocked.SetBlocked("capacidad no resoluble")
	completed := planning.NewGoal("Completado", "", planning.PriorityMedium, "user")
	completed.Complete()
	planner.RegisterGoal(blocked)
	planner.RegisterGoal(completed)

	analyzer, _ := NewRecoveryAnalyzer(planner, adapter)
	b := analyzer.Analyze(blocked)
	c := analyzer.Analyze(completed)

	if b.Reason != RecoveryReasonGoalBlocked {
		t.Fatalf("blocked Reason = %q", b.Reason)
	}
	if c.Reason != RecoveryReasonCompleted {
		t.Fatalf("completed Reason = %q", c.Reason)
	}
}

func TestB912RecoveryAnalyzer_AnalyzeAllNoMezclaGoals(t *testing.T) {
	planner, _, adapter := newB912Runtime(t)

	a := planning.NewGoal("A sano", "", planning.PriorityHigh, "user")
	b := planning.NewGoal("B sin plan", "", planning.PriorityLow, "user")
	planner.RegisterGoal(a)
	planner.RegisterGoal(b)

	plan, err := adapter.EnsurePlan(a)
	if err != nil {
		t.Fatalf("EnsurePlan A: %v", err)
	}

	analyzer, _ := NewRecoveryAnalyzer(planner, adapter)
	all := analyzer.AnalyzeAll()

	if len(all) != 2 {
		t.Fatalf("AnalyzeAll devolvió %d análisis, quería 2", len(all))
	}

	seenA, seenB := false, false
	for _, analysis := range all {
		if analysis.Candidate.Goal == a {
			seenA = true
			if analysis.Candidate.Plan != plan || analysis.Reason != RecoveryReasonPendingWork {
				t.Fatalf("A mal analizado: %+v", analysis)
			}
		}
		if analysis.Candidate.Goal == b {
			seenB = true
			if analysis.Candidate.Plan != nil || analysis.Reason != RecoveryReasonNoPlan {
				t.Fatalf("B mal analizado: %+v", analysis)
			}
		}
	}
	if !seenA || !seenB {
		t.Fatal("faltó algún Goal en AnalyzeAll")
	}
}

func TestB912RecoveryAnalyzer_NoMutaRuntime(t *testing.T) {
	planner, state, adapter := newB912Runtime(t)
	goal := planning.NewGoal("No mutar", "", planning.PriorityHigh, "user")
	planner.RegisterGoal(goal)
	plan, err := adapter.EnsurePlan(goal)
	if err != nil {
		t.Fatalf("EnsurePlan: %v", err)
	}
	state.SetCurrentGoal(goal)
	status := goal.Status
	planID := plan.ID
	taskStatuses := make([]planning.TaskStatus, len(plan.Tasks))
	for i, task := range plan.Tasks {
		taskStatuses[i] = task.Status
	}

	analyzer, _ := NewRecoveryAnalyzer(planner, adapter)
	got := analyzer.Analyze(goal)
	if got == nil {
		t.Fatal("análisis nil")
	}

	if goal.Status != status || state.CurrentGoal() != goal {
		t.Fatal("B91.2 modificó estado de Goal/AutonomousState")
	}
	if adapter.Plans()[goal.ID] != plan || plan.ID != planID {
		t.Fatal("B91.2 modificó el Plan")
	}
	for i, task := range plan.Tasks {
		if task.Status != taskStatuses[i] {
			t.Fatalf("B91.2 modificó Task %q", task.ID)
		}
	}
}
