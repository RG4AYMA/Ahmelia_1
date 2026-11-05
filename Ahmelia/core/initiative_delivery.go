package core

import (
	"fmt"

	"ahmelia/cognition"
	"ahmelia/interaction"
	"ahmelia/naturaldialogue"

	conversationalinitiative "ahmelia/initiative"
)

// InitiativeDelivery is everything needed to turn an already-authorized
// initiative decision into one spoken turn. It is produced where the decision
// is taken (interactive phase or idle evaluation) and consumed by exactly one
// delivery function, so both paths speak the same way.
type InitiativeDelivery struct {
	Request     interaction.SemanticRequest
	Opportunity conversationalinitiative.InitiativeOpportunity
	Decision    conversationalinitiative.InitiativeDecision
	Topic       string
	// Idle marks a turn Ahmelia takes with no user turn in progress. Such a
	// turn must be recorded in ConversationState (nobody else will) and must
	// start the yield boundary: no further unsolicited turn until the user speaks.
	Idle bool
}

// deliverInitiativeIntervention is the single delivery path for initiative:
// InterventionProposal -> Dialogue contract -> Language -> output. It does not
// decide whether to speak (B96.37.2 and the FocusArbiter did); it only speaks,
// and records that it did.
func (e *Engine) deliverInitiativeIntervention(d *InitiativeDelivery) bool {
	if e == nil || d == nil || e.initiativeActionContract == nil || e.conversationState == nil {
		return false
	}
	snap := e.conversationState.Snapshot()
	actionContract, err := e.initiativeActionContract.Build(d.Decision)
	if err != nil {
		return false
	}
	proposal, err := conversationalinitiative.BuildInterventionProposal(actionContract, d.Opportunity, d.Topic)
	if err != nil {
		return false
	}
	contract, ok := naturaldialogue.BuildInterventionResponse(d.Request, snap, proposal)
	if !ok {
		return false
	}
	strategy := e.initiativeResponseStrategy(proposal)
	e.lastBehavioralDecision = cognition.InitiativeBehaviorDecision(strategy, cognition.InitiativeIntervene)
	text, ok := e.generateResponseContractWithStrategy(d.Request, contract, e.buildConversationalRuntimeFacts(snap), e.lastBehavioralDecision.Strategy)
	if !ok {
		// Fase 2 (2026-10-18): sin oración real no hay intervención. La puerta
		// del turno se cierra igual — el turno que disparó la evaluación no se
		// reintenta tick a tick — pero no se registra habla: nada se dijo.
		e.lastInitiativeUserTurnAt = snap.LastTurnAt
		if e.flowTrace != nil {
			e.flowTrace.Mark("INITIATIVE", fmt.Sprintf("decision=%s source=%s suppressed: no generated content", d.Decision.Decision, d.Opportunity.Source))
		}
		return false
	}

	e.deliverRenderedResponse(text)
	if d.Idle {
		e.conversationState.RecordAssistantInitiative(text, d.Topic)
		e.lastAutonomousTurnAt = e.conversationState.Snapshot().LastTurnAt
	}
	// Whatever user turn this intervention answered or followed, it must not
	// be answered or followed again until a new user turn is observed.
	e.lastInitiativeUserTurnAt = snap.LastTurnAt
	return true
}
