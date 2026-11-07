package core

// agency_shadow.go — METAS PROPIAS (PASO 3.1).
//
// El arnés posee la ocasión: deriva impulsos de su propio estado (incógnita,
// trabajo sin cerrar, acción no verificada, objetivo bloqueado). El modelo
// posee la articulación: si hay un impulso en el espacio de trabajo, puede
// proponer UNA meta propia con evidencia citada.
//
// El arnés posee la puerta: valida el contrato, aplica cupo, deduplica y
// registra la propuesta en un libro durable. El propietario posee la decisión:
// autorizar o rechazar. Autorizar NO registra la meta en el planificador ni la
// ejecuta; el paso 3.1 termina en esa puerta.

import (
	"fmt"
	"strings"

	"ahmelia/agency"
	"ahmelia/deliberation"
	"ahmelia/planning"
)

const (
	// agentGoalImpulseThreshold: por debajo de esta fuerza el impulso no abre
	// una ventana de meta propia. Es un umbral del arnés, no del modelo.
	agentGoalImpulseThreshold = 0.55
	// agentGoalMaxImpulses acota lo que entra al espacio de trabajo.
	agentGoalMaxImpulses = 2
)

// EnableAgentGoals turns on the self-goal book in the live process. Like
// EnableShadowDeliberation, it is opt-in so test suites do not write runtime
// state.
func (e *Engine) EnableAgentGoals() {
	if e == nil {
		return
	}
	if e.agentGoalLedger == nil {
		e.agentGoalLedger = agency.NewLedger(agency.LedgerPath)
	} else {
		e.agentGoalLedger.SetPath(agency.LedgerPath)
	}
	if err := e.agentGoalLedger.Load(); err != nil {
		e.deliberationMu.Lock()
		e.lastAgentGoalErr = "no se pudo cargar el libro: " + err.Error()
		e.deliberationMu.Unlock()
	}
	e.deliberationMu.Lock()
	e.agentGoalOn = true
	e.deliberationMu.Unlock()
}

// AgentGoalProposals returns proposals still waiting for the owner, newest
// first. It is a read-only copy.
func (e *Engine) AgentGoalProposals() []agency.GoalRecord {
	if e == nil || e.agentGoalLedger == nil {
		return nil
	}
	return e.agentGoalLedger.Open()
}

// AgentGoalRecord returns one proposal by ID.
func (e *Engine) AgentGoalRecord(id string) (agency.GoalRecord, bool) {
	if e == nil || e.agentGoalLedger == nil {
		return agency.GoalRecord{}, false
	}
	return e.agentGoalLedger.Get(id)
}

// AuthorizeAgentGoal marks a proposal as accepted by the owner. It does not
// register the goal in the planner and does not execute anything.
func (e *Engine) AuthorizeAgentGoal(id, reason string) (agency.GoalRecord, error) {
	if e == nil || e.agentGoalLedger == nil {
		return agency.GoalRecord{}, fmt.Errorf("agency: libro no configurado")
	}
	return e.agentGoalLedger.Authorize(id, reason)
}

// RejectAgentGoal marks a proposal as declined by the owner.
func (e *Engine) RejectAgentGoal(id, reason string) (agency.GoalRecord, error) {
	if e == nil || e.agentGoalLedger == nil {
		return agency.GoalRecord{}, fmt.Errorf("agency: libro no configurado")
	}
	e.discardGoalPlan(id)
	return e.agentGoalLedger.Reject(id, reason)
}

// AgentGoalCounts summarizes the book by status.
func (e *Engine) AgentGoalCounts() map[agency.GoalStatus]int {
	if e == nil || e.agentGoalLedger == nil {
		return nil
	}
	return e.agentGoalLedger.Counts()
}

// LastAgentGoalError reports the last ledger error, if any.
func (e *Engine) LastAgentGoalError() string {
	if e == nil {
		return ""
	}
	e.deliberationMu.Lock()
	defer e.deliberationMu.Unlock()
	return e.lastAgentGoalErr
}

// buildImpulses derives the harness-owned pressures for one turn. It reads
// only state the harness already owns and never calls the model.
func (e *Engine) buildImpulses(t *processTurn) []agency.Impulse {
	if e == nil || t == nil {
		return nil
	}
	// Paso 3.4: el modo descanso cierra la ventana de metas propias. Escena 1:
	// una restricción temporal de trabajo o un estado sensible activo hacen lo
	// mismo sin cambiar el modo. El propietario sigue pudiendo dar órdenes
	// concretas y cambiar de modo.
	if !e.persistentModeAllowsImpulses() || e.ownerWorkInitiativeRestrictedNow() || e.OwnerQuiet(e.nowTime()) {
		return nil
	}
	in := agency.ImpulseInput{Now: e.nowTime()}
	in.Uncertainty = e.lastBehavioralDecision.Uncertainty

	if e.clarificationQueue != nil {
		if q := e.clarificationQueue.Oldest(); q != nil {
			in.PendingQuestion = strings.TrimSpace(q.Question)
		}
	}

	if t.interactionHasWork {
		w := t.interactionWorkContext
		in.InteractionActive = true
		in.InteractionStatus = strings.TrimSpace(w.Status)
		if in.InteractionStatus == "" {
			in.InteractionStatus = strings.TrimSpace(w.AttemptStatus)
		}
		in.ActionRan = w.AttemptNumber > 0 ||
			w.OutcomeStatus != "" ||
			strings.TrimSpace(w.LastResult) != "" ||
			w.LastActionSuccess || w.LastActionVerified
		in.LastActionSuccess = w.LastActionSuccess
		in.LastActionVerified = w.LastActionVerified
	}

	if e.goalStore != nil {
		for _, goal := range e.goalStore.All() {
			if goal != nil && goal.Status == planning.GoalBlocked {
				in.BlockedGoals = append(in.BlockedGoals, goal.Name)
			}
		}
	}

	impulses := agency.ComputeImpulses(in)
	out := make([]agency.Impulse, 0, agentGoalMaxImpulses)
	for _, imp := range impulses {
		if imp.Strength < agentGoalImpulseThreshold {
			continue
		}
		out = append(out, imp)
		if len(out) >= agentGoalMaxImpulses {
			break
		}
	}
	return out
}

// addImpulsesToWorkspace places the impulses where the model can cite them.
// They go early so the workspace cap cannot silently drop them behind long
// situational evidence.
func addImpulsesToWorkspace(ws *deliberation.Workspace, impulses []agency.Impulse) {
	if ws == nil {
		return
	}
	ws.Impulses = append([]agency.Impulse(nil), impulses...)
	for _, imp := range impulses {
		ws.Add("impulso", imp.Text(), "agency", imp.Strength)
	}
}

// citesImpulse reports whether at least one cited ID belongs to an impulse
// item in the workspace. A self-goal without an impulse citation is not an
// opening: it is the model inventing a motive.
func citesImpulse(ws *deliberation.Workspace, ids []string) bool {
	if ws == nil {
		return false
	}
	for _, id := range ids {
		for _, item := range ws.Items {
			if item.ID == id && item.Kind == "impulso" {
				return true
			}
		}
	}
	return false
}

// matchImpulse finds the harness impulse behind a cited workspace item. The
// item text is the exact Text() the impulse produced, so the match is
// deterministic and does not depend on model prose.
func matchImpulse(ws *deliberation.Workspace, ids []string) (agency.Impulse, bool) {
	if ws == nil {
		return agency.Impulse{}, false
	}
	for _, id := range ids {
		for _, item := range ws.Items {
			if item.ID != id || item.Kind != "impulso" {
				continue
			}
			for _, imp := range ws.Impulses {
				if imp.Text() == item.Text {
					return imp, true
				}
			}
		}
	}
	return agency.Impulse{}, false
}

// recordAgentGoal records a validated self-goal in the owner-gated ledger. It
// never registers the goal in planning.Planner and never executes anything.
func (e *Engine) recordAgentGoal(rec deliberation.Record, ws *deliberation.Workspace, turnID string) {
	if e == nil || rec.Proposal.SelfGoal == nil {
		return
	}
	switch rec.Outcome {
	case deliberation.OutcomeAbstain, deliberation.OutcomeUngrounded, deliberation.OutcomeInvalid, deliberation.OutcomeUnavailable:
		// A proposal the model itself could not ground, or a turn where it
		// abstained, cannot open a self-goal.
		return
	}
	e.deliberationMu.Lock()
	enabled := e.agentGoalOn && e.agentGoalLedger != nil
	e.deliberationMu.Unlock()
	if !enabled {
		return
	}
	impulse, ok := matchImpulse(ws, rec.Proposal.SelfGoal.EvidenceIDs)
	if !ok {
		return
	}
	record, err := e.agentGoalLedger.Propose(turnID, impulse, *rec.Proposal.SelfGoal)
	if err != nil {
		e.deliberationMu.Lock()
		e.lastAgentGoalErr = err.Error()
		e.deliberationMu.Unlock()
		return
	}
	e.scheduleIdeaPlan(record)
	fmt.Printf("🧭 [meta propia propuesta] %s (id=%s)\n", record.Goal.Summary(), record.ID)
}
