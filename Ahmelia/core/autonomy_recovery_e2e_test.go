package core

import (
	"context"
	"path/filepath"
	"testing"

	"ahmelia/autonomy"
	"ahmelia/planning"
)

// TestB915_E2E_ReinicioAnalisisDecisionIntegracionYContinuacion demuestra
// la propiedad central de B91: el estado operativo sobrevive al reinicio,
// Recovery lo comprende, decide Resume, lo integra y el ciclo puede
// continuar sobre la misma Task/Plan restaurados.
func TestB915_E2E_ReinicioAnalisisDecisionIntegracionYContinuacion(t *testing.T) {
	planner, state, adapter, _ := newReconciliationRuntime(t)
	goal := planning.NewGoal("B91.5 continuidad cognitiva", "continuar después de reiniciar", planning.PriorityHigh, "ahmelia")
	planner.RegisterGoal(goal)
	state.SetCurrentGoal(goal)

	plan, err := adapter.EnsurePlan(goal)
	if err != nil {
		t.Fatalf("EnsurePlan: %v", err)
	}
	if len(plan.Tasks) < 2 {
		t.Fatalf("el Plan necesita al menos 2 Tasks para demostrar continuidad: %d", len(plan.Tasks))
	}

	// Simulamos trabajo real ya realizado antes del apagado: la primera Task
	// queda completada y la segunda permanece pendiente.
	first := plan.Tasks[0]
	first.Start()
	first.Complete()

	path := filepath.Join(t.TempDir(), "autonomy_state.json")
	if err := buildAutonomyPersistenceSnapshot(planner, state, adapter).Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := LoadAutonomyPersistenceSnapshot(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	newPlanner, newState, newAdapter, _ := newReconciliationRuntime(t)
	if err := ApplyAutonomyPersistenceSnapshot(loaded, newPlanner, newState, newAdapter); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	restoredGoal := newPlanner.Goals()[0]
	restoredPlan := newAdapter.Plans()[restoredGoal.ID]
	if restoredPlan == nil {
		t.Fatal("el Plan no sobrevivió al reinicio")
	}
	if restoredGoal.ID != goal.ID {
		t.Fatalf("Goal.ID restaurado = %q, quería %q", restoredGoal.ID, goal.ID)
	}
	if restoredPlan.ID != plan.ID {
		t.Fatalf("Plan.ID restaurado = %q, quería %q", restoredPlan.ID, plan.ID)
	}
	if len(restoredPlan.Tasks) != len(plan.Tasks) {
		t.Fatalf("Tasks restauradas = %d, quería %d", len(restoredPlan.Tasks), len(plan.Tasks))
	}

	analyzer, err := NewRecoveryAnalyzer(newPlanner, newAdapter)
	if err != nil {
		t.Fatalf("NewRecoveryAnalyzer: %v", err)
	}
	analysis := analyzer.Analyze(restoredGoal)
	if analysis.Reason != RecoveryReasonPendingWork || !analysis.PlanResolvable || analysis.PendingRunnable == 0 {
		t.Fatalf("RecoveryAnalysis inesperado: %+v", analysis)
	}

	assessment := NewRecoveryDecider().Decide(analysis)
	if assessment.Decision != RecoveryResume {
		t.Fatalf("RecoveryDecision = %q, quería resume", assessment.Decision)
	}
	if !assessment.Valid() {
		t.Fatalf("assessment de recovery inválida: %+v", assessment)
	}

	conversation := autonomy.NewConversationManager()
	initiative := autonomy.NewInitiativeEngine(autonomy.InitiativeConfig{})
	integration, err := NewRecoveryIntegration(initiative, conversation)
	if err != nil {
		t.Fatalf("NewRecoveryIntegration: %v", err)
	}
	result, err := integration.Integrate(assessment)
	if err != nil {
		t.Fatalf("Integrate: %v", err)
	}
	if result.Event.GoalID != restoredGoal.ID {
		t.Fatalf("RecoveryEvent.GoalID = %q, quería %q", result.Event.GoalID, restoredGoal.ID)
	}
	if result.Initiative.Action != autonomy.ActionResumeGoal {
		t.Fatalf("Initiative.Action = %q, quería RESUME_GOAL", result.Initiative.Action)
	}
	if result.Conversation.Strategy != autonomy.StrategyResumeTopic {
		t.Fatalf("Conversation.Strategy = %q, quería RESUME_TOPIC", result.Conversation.Strategy)
	}

	// La integración no ejecuta nada; el ciclo autónomo sigue siendo quien
	// obtiene la siguiente acción.
	next, ok, err := newAdapter.GetNextAction(context.Background())
	if err != nil || !ok {
		t.Fatalf("GetNextAction tras recovery = %+v, ok=%v, err=%v", next, ok, err)
	}
	if next.ID == first.ID {
		t.Fatalf("Recovery volvió a ofrecer la Task completada %q", first.ID)
	}
	if newAdapter.Plans()[restoredGoal.ID] != restoredPlan {
		t.Fatal("la continuación reemplazó el Plan restaurado")
	}
}

// TestB915_E2E_TodosLosResultadosDeRecoverySeIntegran comprueba que B91.3 y
// B91.4 mantienen una traducción completa y determinista de los cuatro
// estados terminales/operativos, sin ejecutar Goals.
func TestB915_E2E_TodosLosResultadosDeRecoverySeIntegran(t *testing.T) {
	initiative := autonomy.NewInitiativeEngine(autonomy.InitiativeConfig{})
	conversation := autonomy.NewConversationManager()
	integration, err := NewRecoveryIntegration(initiative, conversation)
	if err != nil {
		t.Fatalf("NewRecoveryIntegration: %v", err)
	}

	cases := []struct {
		name     string
		status   planning.GoalStatus
		decision RecoveryDecision
		reason   RecoveryReason
		action   autonomy.DecisionAction
		strategy autonomy.ConversationStrategy
	}{
		{"resume", planning.GoalActive, RecoveryResume, RecoveryReasonPendingWork, autonomy.ActionResumeGoal, autonomy.StrategyResumeTopic},
		{"idle", planning.GoalCompleted, RecoveryIdle, RecoveryReasonCompleted, autonomy.ActionNoAction, autonomy.StrategyNone},
		{"blocked", planning.GoalBlocked, RecoveryBlocked, RecoveryReasonGoalBlocked, autonomy.ActionInformUser, autonomy.StrategyAppendContext},
		{"needs-user", planning.GoalActive, RecoveryNeedsUser, RecoveryReasonNoPlan, autonomy.ActionAskUser, autonomy.StrategyInterruptLightly},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			goal := planning.NewGoal("B91.5 "+tc.name, "", planning.PriorityMedium, "user")
			goal.Status = tc.status
			assessment := NewRecoveryAssessment(
				NewRecoveryCandidate(goal, nil, nil),
				tc.decision,
				tc.reason,
			)

			result, err := integration.Integrate(assessment)
			if err != nil {
				t.Fatalf("Integrate: %v", err)
			}
			if result.Event.GoalID != goal.ID || result.Event.GoalName != goal.Name {
				t.Fatalf("evento no conserva identidad: %+v", result.Event)
			}
			if result.Initiative.Action != tc.action {
				t.Fatalf("Initiative.Action = %q, quería %q", result.Initiative.Action, tc.action)
			}
			if result.Conversation.Strategy != tc.strategy {
				t.Fatalf("Conversation.Strategy = %q, quería %q", result.Conversation.Strategy, tc.strategy)
			}
			if goal.Status != tc.status {
				t.Fatalf("Integration modificó Goal: %q → %q", tc.status, goal.Status)
			}
		})
	}
}

// TestB915_E2E_GoalsIndependientesTrasRecovery demuestra que analizar e
// integrar un Goal bloqueado o sin plan no altera la recuperación de otro
// Goal que sí puede continuar.
func TestB915_E2E_GoalsIndependientesTrasRecovery(t *testing.T) {
	planner, _, adapter, _ := newReconciliationRuntime(t)

	healthy := planning.NewGoal("B91.5 sano", "", planning.PriorityHigh, "user")
	blocked := planning.NewGoal("B91.5 bloqueado", "", planning.PriorityMedium, "user")
	blocked.SetBlocked("capacidad no disponible")
	planner.RegisterGoal(healthy)
	planner.RegisterGoal(blocked)

	healthyPlan, err := adapter.EnsurePlan(healthy)
	if err != nil {
		t.Fatalf("EnsurePlan healthy: %v", err)
	}

	analyzer, err := NewRecoveryAnalyzer(planner, adapter)
	if err != nil {
		t.Fatalf("NewRecoveryAnalyzer: %v", err)
	}
	analyses := analyzer.AnalyzeAll()
	if len(analyses) != 2 {
		t.Fatalf("AnalyzeAll = %d, quería 2", len(analyses))
	}

	decider := NewRecoveryDecider()
	initiative := autonomy.NewInitiativeEngine(autonomy.InitiativeConfig{})
	conversation := autonomy.NewConversationManager()
	integration, err := NewRecoveryIntegration(initiative, conversation)
	if err != nil {
		t.Fatalf("NewRecoveryIntegration: %v", err)
	}

	seenHealthy, seenBlocked := false, false
	for _, analysis := range analyses {
		assessment := decider.Decide(analysis)
		result, err := integration.Integrate(assessment)
		if err != nil {
			t.Fatalf("Integrate: %v", err)
		}

		switch analysis.Candidate.Goal {
		case healthy:
			seenHealthy = true
			if assessment.Decision != RecoveryResume || result.Initiative.TargetGoalID != healthy.ID {
				t.Fatalf("healthy contaminado: assessment=%+v initiative=%+v", assessment, result.Initiative)
			}
			if analysis.Candidate.Plan != healthyPlan {
				t.Fatal("healthy perdió la identidad de su Plan")
			}
		case blocked:
			seenBlocked = true
			if assessment.Decision != RecoveryBlocked || result.Initiative.Action != autonomy.ActionInformUser {
				t.Fatalf("blocked mal integrado: assessment=%+v initiative=%+v", assessment, result.Initiative)
			}
		}
	}
	if !seenHealthy || !seenBlocked {
		t.Fatal("faltó algún Goal en recovery")
	}
}

// TestB915_E2E_NeedsUserDuranteTurnoDelUsuarioNoInterrumpe prueba la frontera
// cognición/conversación: si Recovery necesita al usuario mientras este está
// hablando, B91.4 entrega la decisión pero Conversation la difiere.
func TestB915_E2E_NeedsUserDuranteTurnoDelUsuarioNoInterrumpe(t *testing.T) {
	goal := planning.NewGoal("B91.5 necesita usuario", "", planning.PriorityHigh, "user")
	assessment := NewRecoveryAssessment(
		NewRecoveryCandidate(goal, nil, nil),
		RecoveryNeedsUser,
		RecoveryReasonNoPlan,
	)

	initiative := autonomy.NewInitiativeEngine(autonomy.InitiativeConfig{})
	conversation := autonomy.NewConversationManager()
	conversation.SetTurnState(autonomy.TurnUserSpeaking)
	conversation.SetCurrentTopic("conversación actual")

	integration, err := NewRecoveryIntegration(initiative, conversation)
	if err != nil {
		t.Fatalf("NewRecoveryIntegration: %v", err)
	}
	result, err := integration.Integrate(assessment)
	if err != nil {
		t.Fatalf("Integrate: %v", err)
	}

	if !result.Conversation.Deferred {
		t.Fatal("NeedsUser durante USER_SPEAKING debería quedar diferido")
	}
	if result.Conversation.Strategy != autonomy.StrategyDefer {
		t.Fatalf("Strategy = %q, quería DEFER", result.Conversation.Strategy)
	}
	if conversation.DeferredCount() != 1 {
		t.Fatalf("DeferredCount = %d, quería 1", conversation.DeferredCount())
	}
}

// TestB915_E2E_RecoveryNoDuplicaPlanNiTasks verifica la propiedad de
// continuidad: repetir análisis/decisión/integración no crea Planes ni Tasks
// adicionales.
func TestB915_E2E_RecoveryNoDuplicaPlanNiTasks(t *testing.T) {
	planner, _, adapter, _ := newReconciliationRuntime(t)
	goal := planning.NewGoal("B91.5 idempotencia", "", planning.PriorityHigh, "ahmelia")
	planner.RegisterGoal(goal)
	plan, err := adapter.EnsurePlan(goal)
	if err != nil {
		t.Fatalf("EnsurePlan: %v", err)
	}

	taskCount := len(planner.Tasks())
	analyzer, _ := NewRecoveryAnalyzer(planner, adapter)
	decider := NewRecoveryDecider()
	initiative := autonomy.NewInitiativeEngine(autonomy.InitiativeConfig{})
	conversation := autonomy.NewConversationManager()
	integration, _ := NewRecoveryIntegration(initiative, conversation)

	for i := 0; i < 3; i++ {
		analysis := analyzer.Analyze(goal)
		assessment := decider.Decide(analysis)
		result, err := integration.Integrate(assessment)
		if err != nil {
			t.Fatalf("iteración %d Integrate: %v", i, err)
		}
		if result.Event.GoalID != goal.ID {
			t.Fatalf("iteración %d perdió GoalID", i)
		}
	}

	if len(planner.Tasks()) != taskCount {
		t.Fatalf("Recovery creó Tasks: antes=%d después=%d", taskCount, len(planner.Tasks()))
	}
	if adapter.Plans()[goal.ID] != plan {
		t.Fatal("Recovery reemplazó el Plan cacheado")
	}
	if len(adapter.Plans()) != 1 {
		t.Fatalf("Recovery creó Plans adicionales: %d", len(adapter.Plans()))
	}
}
