package core

import (
	"errors"
	"path/filepath"
	"testing"

	"ahmelia/agency"
	"ahmelia/autonomy"
	"ahmelia/planning"
)

func plannerEngineWithProposal(t *testing.T) (*Engine, agency.GoalRecord) {
	t.Helper()
	e := &Engine{
		agentGoalLedger: agency.NewLedger(""),
		agentGoalOn:     true,
		planner:         planning.NewPlanner(),
		autonomousState: autonomy.New(),
	}
	rec, err := e.agentGoalLedger.Propose("t1", agency.Impulse{
		Kind:     agency.ImpulseKnowledgeGap,
		Reason:   "el turno dejó una incógnita",
		Strength: 0.7,
		Evidence: []string{"clarification:¿qué versión?"},
	}, gateGoalProposal("Revisar la evidencia"))
	if err != nil {
		t.Fatalf("propose: %v", err)
	}
	return e, rec
}

func TestRegisterAgentGoalRequiresAuthorization(t *testing.T) {
	e, rec := plannerEngineWithProposal(t)
	if _, _, err := e.RegisterAgentGoal(rec.ID); !errors.Is(err, agency.ErrNotAuthorized) {
		t.Fatalf("err=%v, esperaba ErrNotAuthorized", err)
	}
	if goals := e.planner.Goals(); len(goals) != 0 {
		t.Fatalf("sin autorización no debe registrar: %+v", goals)
	}
	if got, _ := e.AgentGoalRecord(rec.ID); got.Status != agency.GoalProposed {
		t.Fatalf("el estado no debe cambiar: %s", got.Status)
	}
}

func TestRegisterAgentGoalRefusesRestMode(t *testing.T) {
	e, rec := plannerEngineWithProposal(t)
	e.enablePersistentModeAt(filepath.Join(t.TempDir(), "persistent_mode.json"))
	if _, err := e.SetPersistentMode(agency.ModeRest, "owner"); err != nil {
		t.Fatalf("set rest: %v", err)
	}
	if _, err := e.AuthorizeAgentGoal(rec.ID, "aprobada"); err != nil {
		t.Fatalf("authorize: %v", err)
	}
	if _, _, err := e.RegisterAgentGoal(rec.ID); !errors.Is(err, ErrAgentGoalRestMode) {
		t.Fatalf("err=%v, esperaba ErrAgentGoalRestMode", err)
	}
	if goals := e.planner.Goals(); len(goals) != 0 {
		t.Fatalf("descanso no debe registrar metas: %+v", goals)
	}
}

func TestRegisterAgentGoalRefusesDoubleRegistration(t *testing.T) {
	e, rec := plannerEngineWithProposal(t)
	if _, err := e.AuthorizeAgentGoal(rec.ID, "aprobada"); err != nil {
		t.Fatalf("authorize: %v", err)
	}
	updated, goal, err := e.RegisterAgentGoal(rec.ID)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if updated.Status != agency.GoalRegistered || updated.PlannerGoalID != goal.ID {
		t.Fatalf("registro=%+v goal=%+v", updated, goal)
	}
	if _, _, err := e.RegisterAgentGoal(rec.ID); !errors.Is(err, agency.ErrAlreadyRegistered) {
		t.Fatalf("segundo registro: %v, esperaba ErrAlreadyRegistered", err)
	}
	if goals := e.planner.Goals(); len(goals) != 1 {
		t.Fatalf("no debe duplicar el goal: %+v", goals)
	}
}
