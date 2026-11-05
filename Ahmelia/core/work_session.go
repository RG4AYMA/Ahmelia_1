package core

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"ahmelia/capability"
	"ahmelia/interaction"
	"ahmelia/planning"
	"ahmelia/production"
)

// workContextFromSession creates the deliberately small, non-authoritative
// interaction projection from the canonical persisted WorkSession. It preserves
// epistemic and attempt state needed for safe conversational decisions without
// copying ownership into the interaction layer.
func workContextFromSession(w production.WorkSession) interaction.WorkContext {
	wc := interaction.WorkContext{
		ID: w.ID, Status: string(w.Status), Objective: w.Objective.Text,
		Provider: w.Provider, CurrentStep: w.CurrentStep, UpdatedAt: w.UpdatedAt,
	}
	if w.LastActionResult != nil {
		r := w.LastActionResult
		wc.HasMaterializedResult = r.Success && r.ProductionID != ""
		wc.LastActionSuccess = r.Success
		wc.LastActionVerified = r.Verified
		wc.LastResult = strings.TrimSpace(r.Summary)
		wc.OutcomeStatus = string(r.OutcomeStatus)
		wc.OutcomeCode = strings.TrimSpace(r.OutcomeCode)
		wc.Observed = r.Observed
		wc.VerificationSource = strings.TrimSpace(r.VerificationSource)
		wc.VerificationReason = strings.TrimSpace(r.VerificationReason)
		wc.FailureReason = strings.TrimSpace(r.FailureReason)
		wc.AttemptNumber = r.AttemptNumber
		wc.ReconciliationPending = r.OutcomeStatus == production.OutcomeUnknown
	}
	if len(w.Attempts) > 0 {
		last := w.Attempts[len(w.Attempts)-1]
		wc.AttemptNumber = last.Number
		wc.AttemptStatus = strings.TrimSpace(last.Status)
		if strings.EqualFold(wc.AttemptStatus, "unverified") {
			wc.ReconciliationPending = true
		}
	}
	return wc
}

// ensureWorkSessionForProduction is the Core-owned bridge for B96.43.
//
// It creates/recovers one persistent WorkSession before physical production
// begins, links it to the canonical Goal/Plan owners, and reuses the same work
// on continuation/resume turns. It does not select a provider, authorize, or
// execute anything.
func (e *Engine) ensureWorkSessionForProduction(req interaction.SemanticRequest, route capability.Route, requestedWorkID string) (string, error) {
	return e.ensureWorkSessionForProductionContext(context.Background(), req, route, requestedWorkID)
}

func (e *Engine) ensureWorkSessionForProductionContext(ctx context.Context, req interaction.SemanticRequest, route capability.Route, requestedWorkID string) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if e == nil || route.Kind != capability.IntentProduction || e.workStore == nil || e.goalStore == nil || e.goalPlanningAdapter == nil {
		return "", nil
	}

	// Recovery/re-execution may already carry the canonical Work owner.
	// Reusing it here is what keeps all attempts inside one WorkSession.
	requestedWorkID = strings.TrimSpace(requestedWorkID)
	if requestedWorkID != "" {
		work, ok := e.workStore.Get(requestedWorkID)
		if !ok {
			return "", fmt.Errorf("work session: requested work %q not found", requestedWorkID)
		}
		snap := interaction.ConversationSnapshot{}
		if e.conversationState != nil {
			snap = e.conversationState.Snapshot()
		}
		decision := interaction.WorkBridgeDecision{
			Relation: interaction.WorkRelationResume, WorkID: work.ID,
			ThreadID: snap.ActiveThreadID, Confidence: 1,
			Reason: "canonical work owner supplied by recovery/execution authority",
		}
		if err := e.persistConversationWorkOwner(work.ID, snap, decision); err != nil {
			return "", fmt.Errorf("work session: persist active owner before resumed execution: %w", err)
		}
		return work.ID, nil
	}

	snap := interaction.ConversationSnapshot{}
	if e.conversationState != nil {
		snap = e.conversationState.Snapshot()
	}

	var current interaction.WorkContext
	hasWork := false
	if activeID := strings.TrimSpace(snap.ActiveWorkID); activeID != "" {
		if w, ok := e.workStore.Get(activeID); ok && isOperationalWorkStatus(string(w.Status)) {
			hasWork = true
			current = workContextFromSession(w)
		}
	}
	// Historical WorkStore.Last is consulted only for an explicit user request
	// to resume recent work. It is never the implicit owner of a new objective.
	if !hasWork && (interaction.HasReference(req, interaction.ReferenceRecentWork) || interaction.IsExplicitWorkResume(req.Normalized)) {
		if w, ok := e.workStore.Last(); ok && isOperationalWorkStatus(string(w.Status)) {
			hasWork = true
			current = workContextFromSession(w)
		}
	}
	relation := interaction.WorkRelationAttach
	bridge := interaction.WorkBridgeDecision{}
	if e.workBridge != nil {
		bridge = e.workBridge.Resolve(req, snap, current, hasWork)
		relation = bridge.Relation
	}

	if relation == interaction.WorkRelationContinue || relation == interaction.WorkRelationResume {
		workID := strings.TrimSpace(bridge.WorkID)
		if workID == "" {
			return "", fmt.Errorf("work session: continuation without work owner")
		}
		if _, ok := e.workStore.Get(workID); !ok {
			return "", fmt.Errorf("work session: continuation owner %q no longer exists", workID)
		}
		if err := e.persistConversationWorkOwner(workID, snap, bridge); err != nil {
			return "", fmt.Errorf("work session: persist continuation owner before execution: %w", err)
		}
		return workID, nil
	}

	if relation == interaction.WorkRelationPreserve || relation == interaction.WorkRelationNone {
		return "", nil
	}

	objective := strings.TrimSpace(req.Objective.Text)
	if objective == "" {
		objective = strings.TrimSpace(req.Raw)
	}
	if objective == "" {
		return "", fmt.Errorf("work session: production request without objective")
	}

	goal := planning.NewGoal(objective, "Work Session creada desde la intención del usuario", planning.PriorityMedium, "user")
	goal.Confidence = req.Confidence
	if goal.Confidence <= 0 {
		goal.Confidence = 0.5
	}
	if err := e.goalStore.Register(goal); err != nil {
		return "", fmt.Errorf("work session: register goal: %w", err)
	}
	// Observe a baseline before deriving the first plan. This is a read-only
	// snapshot; failures are represented explicitly rather than guessed away.
	baseline := ""
	if e.world != nil {
		observed, observeErr := (engineEnvironmentSnapshotInspector{manager: e.world}).Snapshot(ctx)
		if observeErr != nil {
			baseline = "OBSERVATION_UNAVAILABLE: " + observeErr.Error()
		} else if encoded, encodeErr := json.Marshal(observed); encodeErr == nil {
			baseline = string(encoded)
		} else {
			baseline = "OBSERVATION_UNAVAILABLE: snapshot serialization failed"
		}
	} else {
		baseline = "OBSERVATION_UNAVAILABLE: environment manager not configured"
	}
	planningCtx := planning.WithEnvironmentBaseline(ctx, baseline)
	plan, err := e.goalPlanningAdapter.EnsurePlanContext(planningCtx, goal)
	if err != nil {
		goal.SetBlocked("no se pudo construir un plan para la Work Session")
		_ = e.goalStore.Persist()
		return "", fmt.Errorf("work session: ensure plan: %w", err)
	}
	// Do not make the Goal runnable until its operational owner and the
	// conversation-to-Work link are durable. Activating first allowed the
	// autonomy loop to observe a Goal that had no recoverable WorkSession if a
	// later persistence step failed.
	work, err := e.workStore.Begin(req)
	if err != nil {
		return "", fmt.Errorf("work session: begin: %w", err)
	}
	if err := e.workStore.AttachPlanning(work.ID, goal.ID, plan.ID); err != nil {
		_ = e.workStore.Block(work.ID, "no se pudo vincular Goal/Plan")
		return "", fmt.Errorf("work session: attach planning: %w", err)
	}

	if e.conversationState != nil {
		e.conversationState.SetActiveWork(work.ID)
		e.conversationState.RecordWorkLink(interaction.ConversationWorkLink{
			ThreadID: snap.ActiveThreadID, WorkID: work.ID, Relation: relation,
			SourceTurnID: e.activeConversationTurnID, At: time.Now(), Confidence: bridge.Confidence,
			Reason: "Core created persistent Work Session for production objective",
		})
		// Ownership must be durable before ProductionRuntime can persist an
		// authorization suspension or dispatch a physical effect. Normal turns
		// also save at turn cleanup, but diagnostic bypass and other authority
		// callers do not necessarily traverse that conversational lifecycle.
		if err := e.conversationState.Save(interaction.ConversationStatePath); err != nil {
			e.conversationState.SetActiveWork("")
			_ = e.workStore.Block(work.ID, "conversation-to-work ownership could not be persisted")
			return "", fmt.Errorf("work session: persist active owner before execution: %w", err)
		}
	}

	// The Goal becomes runnable only after Work→Goal→Plan and conversation
	// ownership have been persisted. If activation itself fails, fail closed:
	// block the Work and remove its active conversational pointer rather than
	// leaving an apparently executable orphan.
	if err := e.goalStore.ActivateOrQueue(goal); err != nil {
		_ = e.workStore.Block(work.ID, "goal activation failed: "+err.Error())
		if e.conversationState != nil {
			e.conversationState.SetActiveWork("")
			_ = e.conversationState.Save(interaction.ConversationStatePath)
		}
		return "", fmt.Errorf("work session: activate/queue goal: %w", err)
	}
	return work.ID, nil
}

// persistConversationWorkOwner commits the conversational side of a resume or
// continuation before execution proceeds. A WorkID passed to one execution is
// not enough: later turns and pending-authorization recovery need the same
// durable conversation-to-work owner.
func (e *Engine) persistConversationWorkOwner(workID string, snap interaction.ConversationSnapshot, decision interaction.WorkBridgeDecision) error {
	if e == nil || e.conversationState == nil {
		return nil
	}
	workID = strings.TrimSpace(workID)
	if workID == "" {
		return fmt.Errorf("work session: cannot persist an empty conversation owner")
	}
	e.conversationState.SetActiveWork(workID)
	e.conversationState.RecordWorkLink(interaction.ConversationWorkLink{
		ThreadID: snap.ActiveThreadID, WorkID: workID, Relation: decision.Relation,
		SourceTurnID: e.activeConversationTurnID, At: time.Now(), Confidence: decision.Confidence,
		Reason: decision.Reason,
	})
	if err := e.conversationState.Save(interaction.ConversationStatePath); err != nil {
		return err
	}
	return nil
}

// workSessionExecutionOptions adds the persistent work owner to the already
// existing execution options. ProductionRuntime may reuse it, but never owns
// the decision to create or switch WorkSessions.
func workSessionExecutionOptions(options production.ExecutionOptions, workID string) production.ExecutionOptions {
	options.WorkID = strings.TrimSpace(workID)
	return options
}

// currentPendingAuthorization resolves a pending execution exclusively from
// ConversationState ownership. WorkStore.Last is intentionally excluded: a
// late confirmation belongs to the active conversational work, not the most
// recently persisted work record.
func (e *Engine) currentPendingAuthorization() (production.WorkSession, production.PendingExecution, bool) {
	if e == nil || e.workStore == nil || e.conversationState == nil {
		return production.WorkSession{}, production.PendingExecution{}, false
	}
	workID := strings.TrimSpace(e.conversationState.ActiveWork())
	if workID == "" {
		return production.WorkSession{}, production.PendingExecution{}, false
	}
	work, ok := e.workStore.Get(workID)
	if !ok || work.PendingExecution == nil {
		return production.WorkSession{}, production.PendingExecution{}, false
	}
	return work, *work.PendingExecution, true
}
