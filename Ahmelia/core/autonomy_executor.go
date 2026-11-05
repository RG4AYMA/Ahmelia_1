package core

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"ahmelia/autonomy"
	"ahmelia/cognition"
	"ahmelia/interaction"
	"ahmelia/production"
	"ahmelia/situation"
)

// AutonomyExecutor is the adapter from the autonomous Cycle into the same
// Engine execution authority used by interactive turns.
//
// Architectural contract:
//
//	autonomous plan
//	    -> provider-neutral CapabilityInteractionDecision
//	    -> Engine.authorityExecute()
//	    -> capability provider (the same registry used by interactive actions)
//
// This adapter never executes a provider directly. The Cycle remains the
// owner of Observe/Evaluate/Plan/Verify, while Engine remains the single
// physical execution authority.
type AutonomyExecutor struct {
	engine             *Engine
	mu                 sync.Mutex
	cycleObservations  []situation.Observation
	cycleGoalID        string
	cycleSituation     *situation.Situation
	cycleContributions []situation.Contribution
	cycleContext       *autonomyCycleContext
}

// autonomyCycleContext keeps the canonical shared-state projection captured
// for situational evaluation available to Cognition during the same cycle.
// It is a detached view, never a second owner of conversation or Work state.
type autonomyCycleContext struct {
	conversation interaction.ConversationSnapshot
	work         interaction.WorkContext
	hasWork      bool
	iteration    situation.IterationContext
}

func NewAutonomyExecutor(e *Engine) *AutonomyExecutor {
	return &AutonomyExecutor{engine: e}
}

func (a *AutonomyExecutor) SetCycleSituationObservations(observations []situation.Observation) {
	if a == nil {
		return
	}
	a.mu.Lock()
	a.cycleObservations = append([]situation.Observation(nil), observations...)
	if len(observations) == 0 {
		a.cycleSituation = nil
		a.cycleContributions = nil
		a.cycleGoalID = ""
		a.cycleContext = nil
	}
	a.mu.Unlock()
}

func (a *AutonomyExecutor) CyclePlanningCandidateID() string {
	if a == nil {
		return ""
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.cycleGoalID
}

func (a *AutonomyExecutor) takeCycleSituation() ([]situation.Observation, *situation.Situation, []situation.Contribution, *autonomyCycleContext) {
	if a == nil {
		return nil, nil, nil, nil
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	observations := append([]situation.Observation(nil), a.cycleObservations...)
	var sit *situation.Situation
	if a.cycleSituation != nil {
		copySituation := *a.cycleSituation
		sit = &copySituation
	}
	contributions := append([]situation.Contribution(nil), a.cycleContributions...)
	cycleContext := a.cycleContext
	a.cycleObservations = nil
	a.cycleSituation = nil
	a.cycleContributions = nil
	a.cycleContext = nil
	return observations, sit, contributions, cycleContext
}

func (a *AutonomyExecutor) BuildCycleDecisionEvidence(ctx context.Context, observations []situation.Observation) (situation.DecisionEvidence, bool) {
	if err := ctx.Err(); err != nil || a == nil || a.engine == nil || a.engine.goalPlanningAdapter == nil {
		return situation.DecisionEvidence{}, false
	}
	goal := a.engine.goalPlanningAdapter.PreviewNextGoal()
	if goal == nil {
		return situation.DecisionEvidence{}, false
	}
	objective := strings.TrimSpace(goal.Description)
	if objective == "" {
		objective = strings.TrimSpace(goal.Name)
	}
	req := interaction.SemanticRequest{
		Raw: objective, Normalized: strings.ToLower(objective), Kind: interaction.KindAction,
		Objective: interaction.Objective{Text: objective}, Topic: "autonomy", Confidence: .9,
	}
	// Reuse the canonical conversation owner, including current user-state,
	// focus, commitments, and active Work link. Only the SemanticRequest below
	// represents the candidate Goal; the snapshot remains a faithful projection
	// of shared context rather than a synthetic autonomous-only conversation.
	snapshot := interaction.ConversationSnapshot{}
	if a.engine.conversationState != nil {
		snapshot = a.engine.conversationState.Snapshot()
	}
	var workContext interaction.WorkContext
	hasWork := false
	if workID := strings.TrimSpace(snapshot.ActiveWorkID); workID != "" && a.engine.workStore != nil {
		if work, ok := a.engine.workStore.Get(workID); ok {
			workContext = workContextFromSession(work)
			hasWork = true
		}
	}
	sharedObservations := append([]situation.Observation(nil), a.engine.pendingSituationObservations...)
	a.engine.pendingSituationObservations = nil
	sharedObservations = append(sharedObservations, observations...)
	sit, contributions := a.engine.buildSharedSituation(req, snapshot, workContext, hasWork, sharedObservations)
	// Add UserModel contribution so it flows into DecisionEvidence for evaluator/planner
	if umc := a.engine.userModelContribution(time.Now()); umc != nil {
		contributions = append(contributions, *umc)
	}
	contributions = attachContributionsToSituation(sit, contributions...)
	iteration := situation.NewIterationContext("autonomy_cycle", sit, contributions)
	a.mu.Lock()
	copySituation := sit
	a.cycleSituation = &copySituation
	a.cycleContributions = append([]situation.Contribution(nil), contributions...)
	a.cycleGoalID = goal.ID
	a.cycleContext = &autonomyCycleContext{conversation: snapshot, work: workContext, hasWork: hasWork, iteration: iteration}
	a.mu.Unlock()
	a.engine.lastSituation = sit
	a.engine.lastSituationContributions = contributions
	evidence := iteration.Evidence
	a.engine.lastDecisionEvidence = evidence
	return evidence, true
}

// ExecuteAction implements autonomy.ExecutorAdapter. The autonomous request is
// converted into the same provider-neutral interaction contract used by the
// normal Engine path, then crosses the canonical authority gateway.
func (a *AutonomyExecutor) ExecuteAction(ctx context.Context, req autonomy.ActionRequest) autonomy.ActionResponse {
	if err := ctx.Err(); err != nil {
		return autonomy.ActionResponse{
			RequestID: req.ID,
			Executed:  false,
			Error:     fmt.Errorf("contexto cancelado antes de ejecutar %q: %w", req.Action, err),
		}
	}
	if a == nil || a.engine == nil {
		return autonomy.ActionResponse{
			RequestID: req.ID,
			Executed:  false,
			Error:     fmt.Errorf("autoridad de ejecución de Engine no configurada para la acción %q", req.Action),
		}
	}

	observations, sit, contributions, cycleContext := a.takeCycleSituation()
	authority, err := a.engine.executeAutonomousActionWithContext(ctx, req, observations, sit, contributions, cycleContext)
	response := autonomy.ActionResponse{RequestID: req.ID, Error: err}
	if err == nil {
		response.Executed = authority.Executed
		response.Output = authority.Summary
		response.AuthorityResult = authority
	}
	return response
}

// executeAutonomousAction is the autonomous-plan handoff into the existing
// cognition/authority chain. It deliberately reuses B96.33 cognition rather
// than inventing a parallel autonomous "brain": the plan is converted into
// semantic evidence, the canonical cognition model establishes the execution
// posture, and authorityExecute remains the only physical gateway.
func (e *Engine) executeAutonomousAction(ctx context.Context, req autonomy.ActionRequest) (production.AuthorityExecutionResult, error) {
	return e.executeAutonomousActionWithContext(ctx, req, nil, nil, nil, nil)
}

func (e *Engine) executeAutonomousActionWithContext(ctx context.Context, req autonomy.ActionRequest, cycleObservations []situation.Observation, cycleSituation *situation.Situation, cycleContributions []situation.Contribution, cycleContext *autonomyCycleContext) (production.AuthorityExecutionResult, error) {
	if err := ctx.Err(); err != nil {
		return production.AuthorityExecutionResult{}, err
	}

	params := cloneStringMap(req.Parameters)
	canonicalWorkID := strings.TrimSpace(params["work_id"])
	// work_id is execution-continuity metadata, not a provider parameter.
	delete(params, "work_id")
	switch req.Action {
	case "crear_carpeta", "renombrar_archivo":
		if params["ruta"] == "" && strings.TrimSpace(req.Target) != "" {
			params["ruta"] = strings.TrimSpace(req.Target)
		}
	}
	raw := strings.TrimSpace(params["raw"])
	if raw == "" {
		raw = strings.TrimSpace(strings.Join([]string{req.Action, req.Target}, " "))
	}
	semanticReq := autonomousSemanticRequest(req, raw, params)
	semanticReq.Meaning = interaction.BuildSemanticMeaning(semanticReq)

	decision := interaction.ResolveCapabilityInteraction(semanticReq)
	if !decision.Executable {
		return production.AuthorityExecutionResult{}, fmt.Errorf("plan autónomo no produjo una capacidad ejecutable: %s", decision.Reason)
	}
	decision.Request.Source = interaction.CapabilitySourceAutonomyPlan

	// authorityExecute already enforces the canonical Cognition -> Authority
	// handoff. Establish that posture through the same B96.33 pipeline instead
	// of adding an autonomous-specific authorization path.
	snapshot := interaction.ConversationSnapshot{}
	if cycleContext != nil {
		snapshot = cycleContext.conversation
	} else if e.conversationState != nil {
		snapshot = e.conversationState.Snapshot()
	}
	// The autonomous plan is already the current action objective. For direct
	// calls without a captured cycle projection, use it as transient context.
	// During a Cycle, keep the exact shared snapshot already read by Situation.
	if cycleContext == nil {
		snapshot.ActiveObjective = semanticReq.Objective.Text
		snapshot.ActiveTopic = semanticReq.Topic
	}
	var workContext interaction.WorkContext
	hasWork := false
	if cycleContext != nil {
		workContext, hasWork = cycleContext.work, cycleContext.hasWork
	}
	if canonicalWorkID != "" && e.workStore != nil && (!hasWork || workContext.ID != canonicalWorkID) {
		if work, ok := e.workStore.Get(canonicalWorkID); ok {
			workContext = workContextFromSession(work)
			hasWork = true
		}
	}
	observations := append([]situation.Observation(nil), e.pendingSituationObservations...)
	e.pendingSituationObservations = nil
	observations = append(observations, cycleObservations...)
	sharedSituation, contributions := situation.Situation{}, []situation.Contribution(nil)
	iteration := situation.IterationContext{}
	if cycleSituation != nil {
		sharedSituation = *cycleSituation
		contributions = append([]situation.Contribution(nil), cycleContributions...)
		if cycleContext != nil {
			iteration = cycleContext.iteration
		}
	} else {
		sharedSituation, contributions = e.buildSharedSituation(semanticReq, snapshot, workContext, hasWork, observations)
		// Add UserModel contribution for consistency with planning phase
		if umc := e.userModelContribution(time.Now()); umc != nil {
			contributions = append(contributions, *umc)
		}
		contributions = attachContributionsToSituation(sharedSituation, contributions...)
		iteration = situation.NewIterationContext("autonomy_action", sharedSituation, contributions)
	}
	e.lastSituation = sharedSituation
	e.lastSituationContributions = contributions
	cognitiveContext := cognition.SituationalCognitionContext{
		Iteration: iteration,
		Situation: sharedSituation, Contributions: contributions,
		Work: workContext, HasWork: hasWork,
		AuthoritativeEvidence: e.authoritativeUserModelEvidence(semanticReq),
	}
	e.lastConversationalCognition = cognition.RunConversationalCognitionE2E(semanticReq, snapshot, cognitiveContext)
	if !e.lastConversationalCognition.Valid || !capabilityHandoffReady(e.lastConversationalCognition) {
		return production.AuthorityExecutionResult{}, fmt.Errorf("cognición no habilitó la ejecución autónoma: %s", strings.Join(e.lastConversationalCognition.Errors, "; "))
	}

	// Work ownership is propagated by the canonical programming-plan mapper.
	// The Engine remains the sole execution authority; it simply reuses the
	// durable owner established by RunProgrammingGoal instead of inferring or
	// creating a second WorkSession for each planned task.
	options := production.ExecutionOptions{Context: ctx, Approval: autonomy.ApprovalNone, RecoveryAttempt: 1, TaskID: req.ID}
	if canonicalWorkID != "" {
		options.WorkID = canonicalWorkID
	}
	authority := e.authorityExecuteWithOptions(decision, options, semanticReq)
	return authority, nil
}

func cloneStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

// autonomousSemanticRequest keeps the goal, immediate operation, target and
// parameters as separate semantic entities. An absent goal stays absent; an
// operation name is not a truthful substitute for the higher-level objective.
func autonomousSemanticRequest(req autonomy.ActionRequest, raw string, params map[string]string) interaction.SemanticRequest {
	return interaction.SemanticRequest{
		Raw: raw, Normalized: strings.ToLower(raw), Kind: interaction.KindAction,
		Objective: interaction.Objective{Text: strings.TrimSpace(req.Objective)},
		Topic:     "autonomy", Confidence: 1,
		Action:            interaction.ActionIntent{Name: strings.TrimSpace(req.Action), Target: strings.TrimSpace(req.Target), Parameters: params, Confidence: 1},
		CommunicativeActs: interaction.CommunicativeActs{Primary: interaction.ActCommand, Confidence: 1, Evidence: []interaction.CommunicativeActEvidence{{Act: interaction.ActCommand, Confidence: 1, Source: interaction.CapabilitySourceAutonomyPlan}}},
	}
}

// Keep the implementation contract visible to the compiler.
var _ autonomy.ExecutorAdapter = (*AutonomyExecutor)(nil)
