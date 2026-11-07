package core

// idea_plan.go — PLANIFICACIÓN DE FONDO PARA UNA IDEA.
//
// Cuando el modelo propone una meta propia, el arnés ya tiene una ventana
// mientras el propietario decide. En esa ventana se puede pedir al modelo un
// grafo de tareas con presupuesto de fondo (2–4 min). El planner lo valida y lo
// usa solo si llega antes de generar el primer plan del objetivo. Un plan
// tardío no reemplaza un plan existente y nunca se ejecuta por sí solo.

import (
	"context"
	"math"
	"strings"
	"time"

	"ahmelia/agency"
	"ahmelia/integration"
	"ahmelia/planning"
)

const (
	ideaPlanBudget      = 4 * time.Minute
	ideaPlanProposalTTL = 10 * time.Minute
)

type goalPlanProposal struct {
	blueprints []planning.GoalTaskBlueprint
	at         time.Time
	err        string
}

type pendingPlanGoal struct {
	goalID string
	at     time.Time
}

// scheduleIdeaPlan starts one background task-graph proposal for a newly
// proposed self-goal. At most one plan is in flight to avoid competing with the
// live model gate.
func (e *Engine) scheduleIdeaPlan(rec agency.GoalRecord) {
	if e == nil {
		return
	}
	e.planMu.Lock()
	client := e.planClient
	if client == nil || e.planRunning || e.planner == nil {
		e.planMu.Unlock()
		return
	}
	e.planRunning = true
	now := e.nowTime()
	e.cleanupPlansLocked(now)
	e.planMu.Unlock()

	goal := planning.NewGoal(
		rec.Goal.Title,
		strings.TrimSpace(rec.Goal.Motive+"\n"+rec.Goal.NextStep),
		agentGoalPriority(rec.Goal.Risk),
		"ahmelia",
	)
	goal.ID = rec.ID // provisional: the blueprints do not depend on the final ID
	goal.EstimatedHours = int(math.Ceil(float64(rec.Goal.ExpectedMinutes) / 60.0))

	go func() {
		defer func() {
			e.planMu.Lock()
			e.planRunning = false
			e.planMu.Unlock()
		}()
		ctx, cancel := context.WithTimeout(context.Background(), ideaPlanBudget)
		defer cancel()

		decomposer := integration.NewOllamaGoalTaskDecomposerWithTimeout(client, ideaPlanBudget)
		blueprints, err := decomposer.DecomposeGoal(ctx, *goal)
		if err == nil {
			err = planning.ValidateGoalTaskBlueprints(goal, blueprints)
		}

		e.planMu.Lock()
		proposal := goalPlanProposal{blueprints: blueprints, at: e.nowTime()}
		if err != nil {
			proposal.err = err.Error()
		}
		if e.planProposals == nil {
			e.planProposals = make(map[string]goalPlanProposal)
		}
		e.planProposals[rec.ID] = proposal
		pending, hasGoal := e.planGoalIDs[rec.ID]
		if hasGoal && err != nil {
			delete(e.planGoalIDs, rec.ID)
		}
		e.planMu.Unlock()

		if !hasGoal || err != nil || proposal.at.Sub(pending.at) > ideaPlanProposalTTL {
			return
		}
		e.planner.SetPrecomputedBlueprints(pending.goalID, blueprints)
		e.planMu.Lock()
		delete(e.planProposals, rec.ID)
		delete(e.planGoalIDs, rec.ID)
		e.planMu.Unlock()
	}()
}

// registerGoalPlan connects an authorized agency goal to its planning goal. If
// the background plan is already valid, it is attached before the planner
// generates anything; otherwise the mapping waits for a still-running worker.
func (e *Engine) registerGoalPlan(agencyID, goalID string) {
	if e == nil || e.planner == nil {
		return
	}
	e.planMu.Lock()
	defer e.planMu.Unlock()

	if proposal, ok := e.planProposals[agencyID]; ok {
		defer delete(e.planProposals, agencyID)
		if proposal.err == "" && e.nowTime().Sub(proposal.at) <= ideaPlanProposalTTL {
			e.planner.SetPrecomputedBlueprints(goalID, proposal.blueprints)
			delete(e.planGoalIDs, agencyID)
			return
		}
	}
	if e.planGoalIDs == nil {
		e.planGoalIDs = make(map[string]pendingPlanGoal)
	}
	e.planGoalIDs[agencyID] = pendingPlanGoal{goalID: goalID, at: e.nowTime()}
}

// discardGoalPlan removes any pending proposal for a rejected goal.
func (e *Engine) discardGoalPlan(agencyID string) {
	if e == nil {
		return
	}
	e.planMu.Lock()
	delete(e.planProposals, agencyID)
	delete(e.planGoalIDs, agencyID)
	e.planMu.Unlock()
}

func (e *Engine) cleanupPlansLocked(now time.Time) {
	for id, proposal := range e.planProposals {
		if now.Sub(proposal.at) > ideaPlanProposalTTL {
			delete(e.planProposals, id)
		}
	}
	for id, pending := range e.planGoalIDs {
		if now.Sub(pending.at) > ideaPlanProposalTTL {
			delete(e.planGoalIDs, id)
		}
	}
}
