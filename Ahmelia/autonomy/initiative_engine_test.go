package autonomy

import (
	"testing"
	"time"
)

func TestInitiativeEngine_Rule1_CriticalPriorityOverResume(t *testing.T) {
	cfg := InitiativeConfig{MinInactivityForResume: 5 * time.Minute}
	engine := NewInitiativeEngine(cfg)

	// Usuario inactivo por 10 min Y al mismo tiempo una señal crítica presente
	lastActivity := time.Now().Add(-10 * time.Minute)
	obs := []ContextObservation{
		{
			ID:        generateUUID(),
			Type:      TypeSystemSignal,
			Source:    "OS",
			Payload:   map[string]interface{}{"severity": "CRITICAL"},
			Timestamp: time.Now(),
			Processed: false,
		},
	}

	dec := engine.Evaluate(obs, true, "goal_73_1", lastActivity)

	// Debe ganar la señal crítica sobre RESUME_GOAL
	if dec.Action != ActionAskUser {
		t.Errorf("se esperaba ActionAskUser prioridad crítica, se obtuvo %s", dec.Action)
	}
	if dec.Risk != RiskHigh {
		t.Errorf("se esperaba riesgo HIGH, se obtuvo %s", dec.Risk)
	}
}

func TestInitiativeEngine_Rule2_ResumeGoal(t *testing.T) {
	cfg := InitiativeConfig{MinInactivityForResume: 5 * time.Minute}
	engine := NewInitiativeEngine(cfg)

	lastActivity := time.Now().Add(-10 * time.Minute)
	dec := engine.Evaluate([]ContextObservation{}, true, "goal_73_1", lastActivity)

	if dec.Action != ActionResumeGoal {
		t.Errorf("se esperaba ActionResumeGoal, se obtuvo %s", dec.Action)
	}
	if dec.TargetGoalID != "goal_73_1" {
		t.Errorf("se esperaba TargetGoalID goal_73_1, se obtuvo %s", dec.TargetGoalID)
	}
}

func TestInitiativeEngine_Rule3_GoalStateChangedPayloadExtraction(t *testing.T) {
	engine := NewInitiativeEngine(InitiativeConfig{})

	obs := []ContextObservation{
		{
			ID:     generateUUID(),
			Type:   TypeGoalStateChanged,
			Source: "GoalMonitor",
			Payload: map[string]interface{}{
				"goal_id": "goal_explicit_99",
				"status":  "COMPLETED",
			},
			Timestamp: time.Now(),
			Processed: false,
		},
	}

	dec := engine.Evaluate(obs, false, "", time.Now())

	if dec.Action != ActionInformUser {
		t.Errorf("se esperaba ActionInformUser, se obtuvo %s", dec.Action)
	}
	if dec.TargetGoalID != "goal_explicit_99" {
		t.Errorf("se esperaba extraer goal_explicit_99 del payload, se obtuvo %s", dec.TargetGoalID)
	}
}

func TestInitiativeEngine_Rule4_Wait(t *testing.T) {
	engine := NewInitiativeEngine(InitiativeConfig{})

	obs := []ContextObservation{
		{
			ID:        generateUUID(),
			Type:      TypeFileChanged,
			Source:    "VSCode",
			Payload:   map[string]interface{}{"file": "main.go"},
			Timestamp: time.Now(),
			Processed: false,
		},
	}

	dec := engine.Evaluate(obs, false, "", time.Now())

	if dec.Action != ActionWait {
		t.Errorf("se esperaba ActionWait, se obtuvo %s", dec.Action)
	}
}

func TestInitiativeEngine_Rule5_NoAction(t *testing.T) {
	cfg := InitiativeConfig{MinInactivityForResume: 10 * time.Minute}
	engine := NewInitiativeEngine(cfg)

	dec := engine.Evaluate([]ContextObservation{}, false, "", time.Now())

	if dec.Action != ActionNoAction {
		t.Errorf("se esperaba ActionNoAction, se obtuvo %s", dec.Action)
	}
}

func TestInitiativeEngine_ActiveGoalStateChangeResumesImmediately(t *testing.T) {
	e := NewInitiativeEngine(InitiativeConfig{MinInactivityForResume: time.Hour})
	decision := e.Evaluate([]ContextObservation{{
		ID:      "goal-active",
		Type:    TypeGoalStateChanged,
		Payload: map[string]interface{}{"goal_id": "g1", "status": "active"},
	}}, true, "g1", time.Now())
	if decision.Action != ActionResumeGoal {
		t.Fatalf("action=%q, want %q", decision.Action, ActionResumeGoal)
	}
	if decision.TargetGoalID != "g1" {
		t.Fatalf("target=%q, want g1", decision.TargetGoalID)
	}
}
