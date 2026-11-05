package core

// agency_planner.go — PUENTE AUTORIZADO AL PLANIFICADOR.
//
// Decisión del propietario (2026-11-01): una meta propia autorizada puede
// entrar al planificador con cupo 1. Este puente hace exactamente eso:
//
//   - exige que el libro tenga la meta en estado "authorized";
//   - exige que no haya otra meta de Ahmelia sin completar en el planificador;
//   - registra un planning.Goal con Owner "ahmelia";
//   - vincula el registro del libro con el ID del goal del planificador.
//
// No la activa en el ciclo autónomo y no ejecuta nada: la autonomía lee
// AutonomousState (CurrentGoal/PendingGoals), no el planificador. El registro
// queda visible para el propietario y para las consultas de estado, pero no
// dispara trabajo por sí solo.

import (
	"errors"
	"fmt"
	"math"
	"strings"

	"ahmelia/agency"
	"ahmelia/planning"
)

// AgentGoalPlannerQuota es el cupo decidido por el propietario: una sola meta
// de Ahmelia sin completar en el planificador a la vez.
const AgentGoalPlannerQuota = 1

// ErrAgentGoalPlannerQuota indica que el cupo del planificador está ocupado.
var ErrAgentGoalPlannerQuota = errors.New("agency: cupo 1 del planificador ocupado por una meta propia activa")

// ErrAgentGoalRestMode indica que el propietario eligió descanso: la meta
// puede quedar autorizada en el libro, pero no se registra en el planificador.
var ErrAgentGoalRestMode = errors.New("agency: el modo descanso no abre cupo de metas propias")

// RegisterAgentGoal convierte una meta autorizada en un goal del planificador.
// Es un acto posterior y separado de la autorización: AuthorizeAgentGoal por sí
// solo no registra nada.
func (e *Engine) RegisterAgentGoal(id string) (agency.GoalRecord, planning.Goal, error) {
	if e == nil {
		return agency.GoalRecord{}, planning.Goal{}, errors.New("agency: motor no configurado")
	}
	e.deliberationMu.Lock()
	enabled := e.agentGoalOn && e.agentGoalLedger != nil
	e.deliberationMu.Unlock()
	if !enabled {
		return agency.GoalRecord{}, planning.Goal{}, errors.New("agency: libro de metas propias no configurado")
	}

	rec, ok := e.agentGoalLedger.Get(id)
	if !ok {
		return agency.GoalRecord{}, planning.Goal{}, fmt.Errorf("%w: %s", agency.ErrNotFound, id)
	}
	if rec.PlannerGoalID != "" || rec.Status == agency.GoalRegistered {
		return rec, planning.Goal{}, fmt.Errorf("%w: %s -> %s", agency.ErrAlreadyRegistered, rec.ID, rec.PlannerGoalID)
	}
	if rec.Status != agency.GoalAuthorized {
		return rec, planning.Goal{}, fmt.Errorf("%w: %s está en %s", agency.ErrNotAuthorized, rec.ID, rec.Status)
	}
	quota := e.agentGoalPlannerQuota()
	if quota <= 0 {
		return rec, planning.Goal{}, fmt.Errorf("%w: %s", ErrAgentGoalRestMode, rec.ID)
	}
	if active := e.ahmeliaPlannerGoals(); len(active) >= quota {
		return rec, planning.Goal{}, fmt.Errorf("%w: %s", ErrAgentGoalPlannerQuota, active[0].Name)
	}

	goal := planning.NewGoal(rec.Goal.Title, agentGoalDescription(rec.Goal), agentGoalPriority(rec.Goal.Risk), "ahmelia")
	goal.Confidence = rec.Goal.Confidence
	goal.EstimatedHours = int(math.Ceil(float64(rec.Goal.ExpectedMinutes) / 60.0))
	goal.Notes = append(goal.Notes,
		"meta propia "+rec.ID,
		"modo "+string(rec.Goal.Mode),
		fmt.Sprintf("costo esperado %d min", rec.Goal.ExpectedMinutes),
	)

	switch {
	case e.goalStore != nil:
		if err := e.goalStore.Register(goal); err != nil {
			return rec, *goal, err
		}
	case e.planner != nil:
		e.planner.RegisterGoal(goal)
	default:
		return rec, *goal, errors.New("agency: planificador no configurado")
	}

	updated, err := e.agentGoalLedger.MarkRegistered(rec.ID, goal.ID)
	if err != nil {
		// El goal ya existe en el planificador pero el libro no pudo
		// vincularlo. Se bloquea para que no se use hasta reconciliar; el
		// error queda visible y el cupo sigue ocupado a propósito.
		goal.SetBlocked("no se pudo vincular al libro de metas propias: " + err.Error())
		return rec, *goal, err
	}
	if e.goalStore != nil {
		e.persistAutonomyStateBestEffort("agent_goal_registered")
	}
	return updated, *goal, nil
}

// ahmeliaPlannerGoals lists the Ahmelia-owned planner goals that still count
// against the quota. A blocked goal counts; only completion releases the slot.
func (e *Engine) ahmeliaPlannerGoals() []*planning.Goal {
	var goals []*planning.Goal
	switch {
	case e == nil:
		return nil
	case e.goalStore != nil:
		goals = e.goalStore.All()
	case e.planner != nil:
		goals = e.planner.Goals()
	}
	out := make([]*planning.Goal, 0, len(goals))
	for _, g := range goals {
		if g == nil || g.Owner != "ahmelia" || g.Status == planning.GoalCompleted {
			continue
		}
		out = append(out, g)
	}
	return out
}

func agentGoalDescription(g agency.GoalProposal) string {
	motive := strings.TrimSpace(g.Motive)
	next := strings.TrimSpace(g.NextStep)
	switch {
	case motive == "":
		return "Meta propia: " + next
	case next == "":
		return motive
	default:
		return motive + " Siguiente paso: " + next
	}
}

func agentGoalPriority(risk agency.Risk) planning.GoalPriority {
	switch risk {
	case agency.RiskHigh:
		return planning.PriorityHigh
	case agency.RiskMedium:
		return planning.PriorityMedium
	default:
		return planning.PriorityLow
	}
}
