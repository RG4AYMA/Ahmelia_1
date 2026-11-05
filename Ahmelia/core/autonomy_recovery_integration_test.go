package core

import (
	"testing"

	"ahmelia/autonomy"
	"ahmelia/planning"
)

func recoveryAssessmentForTest(t *testing.T, decision RecoveryDecision) *RecoveryAssessment {
	t.Helper()
	goal := &planning.Goal{ID: "goal-91-4", Name: "Proyecto recuperación"}
	return NewRecoveryAssessment(
		NewRecoveryCandidate(goal, nil, nil),
		decision,
		RecoveryReasonPendingWork,
	)
}

func TestB914_ResumeSeEntregaAInitiativeYConversation(t *testing.T) {
	ie := autonomy.NewInitiativeEngine(autonomy.InitiativeConfig{})
	cm := autonomy.NewConversationManager()
	ri, err := NewRecoveryIntegration(ie, cm)
	if err != nil {
		t.Fatal(err)
	}

	result, err := ri.Integrate(recoveryAssessmentForTest(t, RecoveryResume))
	if err != nil {
		t.Fatal(err)
	}

	if result.Event.Decision != RecoveryResume {
		t.Fatalf("decision = %q", result.Event.Decision)
	}
	if result.Initiative.Action != autonomy.ActionResumeGoal {
		t.Fatalf("initiative action = %q", result.Initiative.Action)
	}
	if result.Initiative.TargetGoalID != "goal-91-4" {
		t.Fatalf("goal id = %q", result.Initiative.TargetGoalID)
	}
	if result.Conversation.Strategy != autonomy.StrategyResumeTopic {
		t.Fatalf("conversation strategy = %q", result.Conversation.Strategy)
	}
}

func TestB914_NeedsUserLlegaAConversation(t *testing.T) {
	ie := autonomy.NewInitiativeEngine(autonomy.InitiativeConfig{})
	cm := autonomy.NewConversationManager()
	ri, _ := NewRecoveryIntegration(ie, cm)

	result, err := ri.Integrate(recoveryAssessmentForTest(t, RecoveryNeedsUser))
	if err != nil {
		t.Fatal(err)
	}

	if result.Initiative.Action != autonomy.ActionAskUser {
		t.Fatalf("initiative action = %q", result.Initiative.Action)
	}
	if result.Conversation.Strategy != autonomy.StrategyInterruptLightly {
		t.Fatalf("conversation strategy = %q", result.Conversation.Strategy)
	}
}

func TestB914_BlockedInformaSinEjecutar(t *testing.T) {
	ie := autonomy.NewInitiativeEngine(autonomy.InitiativeConfig{})
	cm := autonomy.NewConversationManager()
	ri, _ := NewRecoveryIntegration(ie, cm)

	result, err := ri.Integrate(recoveryAssessmentForTest(t, RecoveryBlocked))
	if err != nil {
		t.Fatal(err)
	}

	if result.Initiative.Action != autonomy.ActionInformUser {
		t.Fatalf("initiative action = %q", result.Initiative.Action)
	}
	if result.Conversation.Strategy != autonomy.StrategyAppendContext {
		t.Fatalf("conversation strategy = %q", result.Conversation.Strategy)
	}
}

func TestB914_IdleNoInterviene(t *testing.T) {
	ie := autonomy.NewInitiativeEngine(autonomy.InitiativeConfig{})
	cm := autonomy.NewConversationManager()
	ri, _ := NewRecoveryIntegration(ie, cm)

	result, err := ri.Integrate(recoveryAssessmentForTest(t, RecoveryIdle))
	if err != nil {
		t.Fatal(err)
	}

	if result.Initiative.Action != autonomy.ActionNoAction {
		t.Fatalf("initiative action = %q", result.Initiative.Action)
	}
	if result.Conversation.Strategy != autonomy.StrategyNone {
		t.Fatalf("conversation strategy = %q", result.Conversation.Strategy)
	}
}

func TestB914_UsuarioHablandoDifiereResume(t *testing.T) {
	ie := autonomy.NewInitiativeEngine(autonomy.InitiativeConfig{})
	cm := autonomy.NewConversationManager()
	cm.SetTurnState(autonomy.TurnUserSpeaking)
	ri, _ := NewRecoveryIntegration(ie, cm)

	result, err := ri.Integrate(recoveryAssessmentForTest(t, RecoveryResume))
	if err != nil {
		t.Fatal(err)
	}

	if !result.Conversation.Deferred {
		t.Fatal("resume durante turno de usuario debería diferirse")
	}
	if result.Conversation.Strategy != autonomy.StrategyDefer {
		t.Fatalf("strategy = %q", result.Conversation.Strategy)
	}
}

func TestB914_NoModificaGoal(t *testing.T) {
	ie := autonomy.NewInitiativeEngine(autonomy.InitiativeConfig{})
	cm := autonomy.NewConversationManager()
	ri, _ := NewRecoveryIntegration(ie, cm)

	a := recoveryAssessmentForTest(t, RecoveryResume)
	status := a.Candidate.Goal.Status
	_, err := ri.Integrate(a)
	if err != nil {
		t.Fatal(err)
	}
	if a.Candidate.Goal.Status != status {
		t.Fatal("integration modificó el Goal")
	}
}
