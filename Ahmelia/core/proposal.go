package core

import (
	"strings"
	"time"

	"ahmelia/autonomy"
	"ahmelia/initiative"
	"ahmelia/interaction"
	"ahmelia/situation"
)

// ProposalKind categorizes the origin of a proposal.
type ProposalKind string

const (
	ProposalKindInteractive ProposalKind = "interactive" // from user turn decisionPhase
	ProposalKindInitiative  ProposalKind = "initiative"  // from idle initiative evaluation
	ProposalKindAutonomous  ProposalKind = "autonomous"  // from Cycle.RunStep planner
)

func (k ProposalKind) String() string {
	return string(k)
}

// ProposalContract is the unified proposal that all three routes submit
// to the shared FocusArbiter. It carries the minimum context needed for
// arbitration: intention, evidence, urgency, interruption cost, and constraints.
type ProposalContract struct {
	ID        string
	Kind      ProposalKind
	CreatedAt time.Time

	// Intention: what the proposal wants to do/say
	Intention       string
	IntentionDetail string
	Action          *ProposedAction

	// Evidence backing this proposal (from Situation/DecisionEvidence)
	Evidence []ProposalEvidence

	// Urgency/Priority signals
	Priority ProposalPriority
	Urgency  ProposalUrgency

	// Cost of interruption: how disruptive this would be RIGHT NOW
	InterruptionCost InterruptionCost

	// Constraints and conditions
	RequiresDirectTurn bool
	Privacy            initiative.InformationPolicy
	Channel            interaction.InputChannel

	// Provenance for traceability
	Provenance initiative.Provenance

	// Initiative is the delivery payload of an initiative proposal. The
	// arbiter decides WHETHER it speaks; this payload is what lets the
	// executor speak without re-deriving the decision from strings.
	Initiative *InitiativeDelivery
}

// ProposedAction represents a concrete action the proposal wants to execute.
type ProposedAction struct {
	Name         string
	Target       string
	Parameters   map[string]string
	CapabilityID string
	RequiresAuth bool
}

// ProposalEvidence links to Situation evidence.
type ProposalEvidence struct {
	SituationEvidenceID string
	Kind                string
	Subject             string
	Predicate           string
	Object              string
	Confidence          float64
	Epistemic           situation.EpistemicStatus
}

// ProposalPriority normalized priority.
type ProposalPriority string

const (
	ProposalPriorityNone     ProposalPriority = "none"
	ProposalPriorityLow      ProposalPriority = "low"
	ProposalPriorityMedium   ProposalPriority = "medium"
	ProposalPriorityHigh     ProposalPriority = "high"
	ProposalPriorityCritical ProposalPriority = "critical"
)

// ProposalUrgency normalized urgency.
type ProposalUrgency string

const (
	ProposalUrgencyNone      ProposalUrgency = "none"
	ProposalUrgencyLow       ProposalUrgency = "low"
	ProposalUrgencyMed       ProposalUrgency = "medium"
	ProposalUrgencyHigh      ProposalUrgency = "high"
	ProposalUrgencyImmediate ProposalUrgency = "immediate"
)

// InterruptionCost quantifies the disruption of acting now.
type InterruptionCost struct {
	// Base cost from policy (0-1)
	BaseCost float64
	// User state modifiers
	UserUrgency     float64
	UserFrustration float64
	UserConfusion   float64
	// Context modifiers
	InMeeting        bool
	InFocus          bool
	IsResting        bool
	ConversationLoad ConversationLoadLevel
	// Computed total (0-1, higher = more costly to interrupt)
	Total float64
}

// ConversationLoadLevel normalized load.
type ConversationLoadLevel string

const (
	ConversationLoadNone ConversationLoadLevel = "none"
	ConversationLoadLow  ConversationLoadLevel = "low"
	ConversationLoadMed  ConversationLoadLevel = "med"
	ConversationLoadHigh ConversationLoadLevel = "high"
)

// FromInteractiveDecision builds a proposal from interactive turn decision.
func FromInteractiveDecision(
	t *processTurn,
	e *Engine,
	sit situation.Situation,
	contrib []situation.Contribution,
) *ProposalContract {
	// Extract intention from cognition decision
	actionKind := e.lastConversationalCognition.Action.Primary
	goalKind := e.lastConversationalCognition.Goal.Primary

	intention := ""
	if actionKind != "" {
		intention = string(actionKind)
	} else if goalKind != "" {
		intention = string(goalKind)
	}
	if intention == "" {
		intention = "respond"
	}

	// Extract evidence from Situation/DecisionEvidence
	evidence := make([]ProposalEvidence, 0)
	for _, f := range e.lastDecisionEvidence.Findings {
		evidence = append(evidence, ProposalEvidence{
			Kind:       f.Kind,
			Subject:    f.Subject,
			Predicate:  f.RelationType, // DecisionFinding has RelationType not Predicate
			Object:     f.Target,
			Confidence: f.Confidence,
			Epistemic:  f.Epistemic,
		})
	}

	// Compute interruption cost based on current user state
	snap := e.conversationState.Snapshot()
	cost := ComputeInterruptionCost(snap, e.autonomousState, e.lastDecisionEvidence)

	// Determine if this requires a direct turn
	requiresDirect := e.lastNaturalDialogue.RequiresDirectTurn

	return &ProposalContract{
		ID:                 "interactive-" + time.Now().UTC().Format("20060102T150405.000000000"),
		Kind:               ProposalKindInteractive,
		CreatedAt:          time.Now(),
		Intention:          intention,
		IntentionDetail:    e.lastConversationalCognition.Goal.Objective,
		Action:             extractProposedAction(e, t),
		Evidence:           evidence,
		Priority:           priorityFromConfidence(e.lastConversationalCognition.Model.Signals.Confidence),
		Urgency:            urgencyFromSignals(snap),
		InterruptionCost:   cost,
		RequiresDirectTurn: requiresDirect,
		Privacy:            initiative.InformationAllowed, // NaturalDialogue doesn't have Privacy field
		Channel:            t.semanticReq.Channel,
		Provenance: initiative.Provenance{
			Component:  "turn_pipeline.decisionPhase",
			Subsystem:  "interactive",
			SignalType: "user_turn",
			OriginID:   sit.ID,
			CreatedAt:  time.Now(),
		},
	}
}

// FromIdleInitiative builds a proposal from idle initiative evaluation.
func FromIdleInitiative(
	e *Engine,
	sit situation.Situation,
	contrib []situation.Contribution,
	opportunity initiative.InitiativeOpportunity,
	decision initiative.InitiativeDecision,
) *ProposalContract {
	// Use opportunity's signals as evidence
	evidence := make([]ProposalEvidence, 0)
	for _, s := range opportunity.Signals {
		evidence = append(evidence, ProposalEvidence{
			Kind:       string(s.Type),
			Subject:    s.Target,
			Predicate:  string(s.Reason),
			Object:     s.Target,
			Confidence: s.Confidence,
			Epistemic:  situation.EpistemicObserved,
		})
	}

	snap := e.conversationState.Snapshot()
	cost := ComputeInterruptionCost(snap, e.autonomousState, situation.DecisionEvidence{})

	return &ProposalContract{
		ID:                 "initiative-" + time.Now().UTC().Format("20060102T150405.000000000"),
		Kind:               ProposalKindInitiative,
		CreatedAt:          time.Now(),
		Intention:          string(opportunity.Reason),
		IntentionDetail:    opportunity.Target,
		Action:             extractInitiativeAction(opportunity),
		Evidence:           evidence,
		Priority:           priorityFromInitiativePriority(opportunity.Priority),
		Urgency:            urgencyFromInitiativeUrgency(opportunity.Urgency),
		InterruptionCost:   cost,
		RequiresDirectTurn: opportunity.RequiresDirectTurn,
		Privacy:            opportunity.Privacy,
		Channel:            interaction.InputChannel(opportunity.Channel),
		Provenance: initiative.Provenance{
			Component:  "evaluateIdleConversationalInitiative",
			Subsystem:  "initiative",
			SignalType: "idle_initiative",
			OriginID:   sit.ID,
			CreatedAt:  time.Now(),
		},
	}
}

// FromAutonomousPlan builds a proposal from autonomous cycle plan.
func FromAutonomousPlan(
	e *Engine,
	sit situation.Situation,
	contrib []situation.Contribution,
	req autonomy.ActionRequest,
	decisionEvidence situation.DecisionEvidence,
) *ProposalContract {
	evidence := make([]ProposalEvidence, 0)
	for _, f := range decisionEvidence.Findings {
		evidence = append(evidence, ProposalEvidence{
			Kind:       f.Kind,
			Subject:    f.Subject,
			Predicate:  f.RelationType,
			Object:     f.Target,
			Confidence: f.Confidence,
			Epistemic:  f.Epistemic,
		})
	}

	cost := ComputeInterruptionCost(
		interaction.ConversationSnapshot{},
		e.autonomousState,
		decisionEvidence,
	)

	return &ProposalContract{
		ID:              "autonomous-" + time.Now().UTC().Format("20060102T150405.000000000"),
		Kind:            ProposalKindAutonomous,
		CreatedAt:       time.Now(),
		Intention:       req.Objective,
		IntentionDetail: req.Action,
		Action: &ProposedAction{
			Name:         req.Action,
			Target:       req.Target,
			Parameters:   req.Parameters,
			CapabilityID: req.Parameters["capability_id"],
			RequiresAuth: true,
		},
		Evidence:           evidence,
		Priority:           priorityFromAutonomy(req),
		Urgency:            urgencyFromAutonomy(req),
		InterruptionCost:   cost,
		RequiresDirectTurn: false,
		Privacy:            initiative.InformationAllowed,
		Channel:            interaction.ChannelText,
		Provenance: initiative.Provenance{
			Component:  "autonomy.Cycle.RunStep",
			Subsystem:  "autonomous",
			SignalType: "autonomous_plan",
			OriginID:   sit.ID,
			CreatedAt:  time.Now(),
		},
	}
}

// --- Helper functions ---

func ComputeInterruptionCost(
	snap interaction.ConversationSnapshot,
	autoState *autonomy.AutonomousState,
	evidence situation.DecisionEvidence,
) InterruptionCost {
	cost := InterruptionCost{}

	// Base cost from conversation state
	if snap.UserState.Urgency > 0 {
		cost.UserUrgency = snap.UserState.Urgency
		cost.BaseCost += snap.UserState.Urgency * 0.3
	}
	if snap.UserState.Frustration > 0 {
		cost.UserFrustration = snap.UserState.Frustration
		cost.BaseCost += snap.UserState.Frustration * 0.4
	}
	if snap.UserState.Confusion > 0 {
		cost.UserConfusion = snap.UserState.Confusion
		cost.BaseCost += snap.UserState.Confusion * 0.2
	}

	// Context modifiers (from signals if available)
	// InMeeting, InFocus, IsResting would come from UserModel/Context
	// For now, derive from UserState
	if snap.UserState.Engagement > 0.8 {
		cost.InFocus = true
		cost.BaseCost += 0.3
	}

	// Conversation load
	if len(snap.OpenThreads) > 3 {
		cost.ConversationLoad = ConversationLoadHigh
		cost.BaseCost += 0.2
	} else if len(snap.OpenThreads) > 1 {
		cost.ConversationLoad = ConversationLoadMed
		cost.BaseCost += 0.1
	} else {
		cost.ConversationLoad = ConversationLoadLow
	}

	// Cap at 1.0
	if cost.BaseCost > 1.0 {
		cost.BaseCost = 1.0
	}
	cost.Total = cost.BaseCost

	return cost
}

func extractProposedAction(e *Engine, t *processTurn) *ProposedAction {
	action := e.lastConversationalCognition.Action.Primary
	if action == "" {
		return nil
	}
	return &ProposedAction{
		Name:   string(action),
		Target: e.lastConversationalCognition.Goal.Objective,
	}
}

func extractInitiativeAction(op initiative.InitiativeOpportunity) *ProposedAction {
	return &ProposedAction{
		Name:   string(op.Reason),
		Target: op.Target,
	}
}

func priorityFromConfidence(conf float64) ProposalPriority {
	switch {
	case conf >= 0.9:
		return ProposalPriorityHigh
	case conf >= 0.7:
		return ProposalPriorityMedium
	case conf >= 0.5:
		return ProposalPriorityLow
	default:
		return ProposalPriorityNone
	}
}

func urgencyFromSignals(snap interaction.ConversationSnapshot) ProposalUrgency {
	switch {
	case snap.UserState.Urgency >= 0.8:
		return ProposalUrgencyImmediate
	case snap.UserState.Urgency >= 0.6:
		return ProposalUrgencyHigh
	case snap.UserState.Urgency >= 0.4:
		return ProposalUrgencyMed
	case snap.UserState.Urgency > 0:
		return ProposalUrgencyLow
	default:
		return ProposalUrgencyNone
	}
}

func priorityFromInitiativePriority(p initiative.Priority) ProposalPriority {
	switch p {
	case initiative.PriorityCritical:
		return ProposalPriorityCritical
	case initiative.PriorityHigh:
		return ProposalPriorityHigh
	case initiative.PriorityMedium:
		return ProposalPriorityMedium
	case initiative.PriorityLow:
		return ProposalPriorityLow
	default:
		return ProposalPriorityNone
	}
}

func urgencyFromInitiativeUrgency(u initiative.Urgency) ProposalUrgency {
	switch u {
	case initiative.UrgencyCritical:
		return ProposalUrgencyImmediate
	case initiative.UrgencyHigh:
		return ProposalUrgencyHigh
	case initiative.UrgencyMedium:
		return ProposalUrgencyMed
	case initiative.UrgencyLow:
		return ProposalUrgencyLow
	default:
		return ProposalUrgencyNone
	}
}

func priorityFromAutonomy(req autonomy.ActionRequest) ProposalPriority {
	// Autonomous actions from planner are typically medium-high
	return ProposalPriorityMedium
}

func urgencyFromAutonomy(req autonomy.ActionRequest) ProposalUrgency {
	// Check if autonomous action has urgency markers
	if strings.Contains(strings.ToLower(req.Objective), "urgent") ||
		strings.Contains(strings.ToLower(req.Objective), "critical") {
		return ProposalUrgencyHigh
	}
	return ProposalUrgencyMed
}
