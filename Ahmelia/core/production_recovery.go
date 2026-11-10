package core

import (
	"context"
	"fmt"
	"strings"

	"ahmelia/cognition"
	"ahmelia/interaction"
	"ahmelia/production"
	"ahmelia/tools"
)

// productionRecoveryHandlers are ownership adapters only. The recovery
// decision itself is produced by tools.RecoveryCoordinator; these handlers
// ensure its mechanical application remains in the owner of each state.
type productionRecoveryHandlers struct {
	Retry   func() bool
	Replan  func() bool
	Block   func() bool
	AskUser func() bool
}

func dispatchProductionRecovery(decision tools.RecoveryDecision, handlers productionRecoveryHandlers) bool {
	switch decision.Action {
	case tools.RecoveryRetry:
		if handlers.Retry != nil {
			return handlers.Retry()
		}
	case tools.RecoveryReplan:
		if handlers.Replan != nil {
			return handlers.Replan()
		}
	case tools.RecoveryBlock:
		if handlers.Block != nil {
			return handlers.Block()
		}
	case tools.RecoveryAskUser:
		if handlers.AskUser != nil {
			return handlers.AskUser()
		}
	}
	return false
}

// Legacy callers without a runtime snapshot retain the same finite policy.
func productionRecoveryOptions(options production.ExecutionOptions, result production.ActionResult) production.ExecutionOptions {
	if result.RetryState != nil {
		state := result.RetryState.Clone()
		options.RetryState = &state
	}
	if result.WorkID != "" {
		options.WorkID = result.WorkID
	}
	return options
}

func productionRecoveryStop(options production.ExecutionOptions) error {
	ctx := options.Context
	if ctx == nil {
		ctx = context.Background()
	}
	if options.RetryState != nil {
		return production.RestoreRetryBudget(*options.RetryState).CanContinue(ctx)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if options.RecoveryAttempt >= options.RetryPolicy.WithDefaults().MaxAttempts {
		return fmt.Errorf("production retry stopped: max_attempts_reached")
	}
	return nil
}

func productionRecoveryRequest(req interaction.SemanticRequest, result production.ActionResult, recovery tools.RecoveryDecision) interaction.SemanticRequest {
	out := req
	base := strings.TrimSpace(out.Objective.Text)
	if base == "" {
		base = strings.TrimSpace(out.Raw)
	}
	if base == "" {
		return out
	}
	evidence := strings.TrimSpace(result.FailureReason)
	if evidence == "" && result.Err != nil {
		evidence = strings.TrimSpace(result.Err.Error())
	}
	if evidence == "" {
		evidence = strings.TrimSpace(recovery.Reason)
	}
	if evidence == "" {
		evidence = "production execution failed without a structured reason"
	}
	if stage := strings.TrimSpace(result.FailedStage); stage != "" {
		evidence += "\nFailed stage: " + stage
	}
	out.Objective.Text = base + "\n\n[Recovery evidence]\n" + evidence
	return out
}

func (e *Engine) applyProductionRecovery(
	decision interaction.CapabilityInteractionDecision,
	req interaction.SemanticRequest,
	result production.ActionResult,
	recovery tools.RecoveryDecision,
	options production.ExecutionOptions,
) bool {
	if e == nil {
		return false
	}
	if options.RecoveryAttempt < 1 {
		options.RecoveryAttempt = 1
	}

	options = productionRecoveryOptions(options, result)
	if recovery.Action == tools.RecoveryRetry || recovery.Action == tools.RecoveryReplan {
		if err := productionRecoveryStop(options); err != nil {
			recovery = tools.RecoveryDecision{Action: tools.RecoveryBlock, Reason: err.Error()}
		}
	}
	handlers := productionRecoveryHandlers{
		Retry: func() bool {
			if e.flowTrace != nil {
				e.flowTrace.Mark("RECOVERY OWNER", "retry → authorityExecute with observed failure evidence")
			}
			options.RecoveryAttempt++
			if options.ResumePendingAuthorization {
				// The authorization was consumed before the first provider call.
				// That attempt is now finalized, so retry must preserve the same
				// authorized capability/provider while beginning a new WorkAttempt;
				// ResumeAttempt is valid only for the still-pending authorization.
				options.AttemptNumber = 0
			}
			retryReq := productionRecoveryRequest(req, result, recovery)
			return e.authorityExecuteWithOptions(decision, options, retryReq).Handled
		},
		Replan: func() bool {
			if e.capabilityResolver == nil {
				e.finalizeProductionRecoveryWork(result, recovery)
				return e.renderProductionRecoveryOutcome(req, result, recovery)
			}
			if e.flowTrace != nil {
				e.flowTrace.Mark("RECOVERY OWNER", "replan → capability resolver / authority")
			}
			// The resolver is the capability-side owner of replanning available to
			// the live provider-neutral path. Re-running authorityExecute performs
			// a fresh semantic resolution; ProductionRuntime never selects the new
			// route itself.
			replanReq := productionRecoveryRequest(req, result, recovery)
			_ = e.capabilityResolver.Resolve(decision, replanReq)
			options.RecoveryAttempt++
			options.AttemptNumber = 0
			return e.authorityExecuteWithOptions(decision, options, replanReq).Handled
		},
		Block: func() bool {
			if e.workStore == nil || strings.TrimSpace(result.WorkID) == "" {
				return e.renderProductionRecoveryOutcome(req, result, recovery)
			}
			if err := e.workStore.Block(result.WorkID, recovery.Reason); err != nil && e.flowTrace != nil {
				e.flowTrace.Mark("RECOVERY OWNER", "block → WorkStore error: "+err.Error())
				return e.renderProductionRecoveryOutcome(req, result, recovery)
			}
			if e.flowTrace != nil {
				e.flowTrace.Mark("RECOVERY OWNER", "block → WorkStore")
			}
			return e.renderProductionRecoveryOutcome(req, result, recovery)
		},
		AskUser: func() bool {
			if e.flowTrace != nil {
				e.flowTrace.Mark("RECOVERY OWNER", "ask_user → Conversation/Initiative")
			}
			return e.renderProductionRecoveryOutcome(req, result, recovery)
		},
	}
	return dispatchProductionRecovery(recovery, handlers)
}

func (e *Engine) finalizeProductionRecoveryWork(result production.ActionResult, recovery tools.RecoveryDecision) {
	if e == nil || e.workStore == nil || strings.TrimSpace(result.WorkID) == "" {
		return
	}
	detail := strings.TrimSpace(recovery.Reason)
	if detail == "" {
		detail = strings.TrimSpace(result.FailureReason)
	}
	if detail == "" {
		detail = "automatic production recovery exhausted"
	}
	if err := e.workStore.Complete(result.WorkID, false, detail); err != nil && e.flowTrace != nil {
		e.flowTrace.Mark("RECOVERY OWNER", "terminal work finalization failed: "+err.Error())
	}
}

// executionAuthorizationResponse and recovery outcome rendering stay in Core
// because Conversation/Language are the sole owners of user-facing output.
func (e *Engine) renderProductionAuthorizationRequired(req interaction.SemanticRequest, result production.ActionResult) bool {
	if e == nil {
		return false
	}
	snap := e.conversationState.Snapshot()
	intent := interaction.ResponseIntent{
		Kind:           interaction.ResponseIntentAsk,
		Confidence:     .98,
		NeedsUserInput: true,
		ShouldContinue: true,
		ShouldSpeak:    true,
		ThreadID:       snap.ActiveThreadID,
		Objective:      strings.TrimSpace(req.Objective.Text),
		Language:       snap.Language,
		Reason:         "production execution is blocked pending explicit capability authorization",
	}
	e.lastResponseIntent = intent
	e.lastNaturalDialogue.Move = interaction.MoveAsk
	contract := interaction.NewResponseContract(req, intent, interaction.MoveAsk)
	contract.Source = "production_authorization_boundary"
	contract.ShouldAct = false
	reason := result.AuthorizationReason
	if strings.TrimSpace(reason) == "" {
		reason = "esta capacidad requiere autorización explícita antes de ejecutar"
	}
	capabilityID := string(result.AuthorizationCapabilityID)
	if capabilityID == "" {
		capabilityID = "la capacidad seleccionada"
	}
	contract.AddEvidence(fmt.Sprintf("Necesito tu autorización explícita para ejecutar %s. %s", capabilityID, reason), "production_authorization_required", true, .98)
	strategy := productionAuthorizationResponseStrategy(intent.Confidence)
	return e.renderResponseContractWithStrategy(req, contract, e.buildConversationalRuntimeFacts(snap), strategy)
}

func productionAuthorizationResponseStrategy(confidence float64) cognition.ConversationalResponseStrategy {
	return cognition.ConversationalResponseStrategy{
		Version: "B96.33.10", Mode: cognition.ResponseModeClarify,
		Directness: cognition.DirectnessDirect, Structure: cognition.StructureExplanation,
		Initiative: cognition.InitiativeHelpful, Confidence: confidence,
		PreserveUserAgency: true, ExecutionBoundary: true, ShouldGenerateLanguage: true,
		Reason: "language realization of an execution authorization request",
	}
}

func (e *Engine) renderProductionRecoveryOutcome(req interaction.SemanticRequest, result production.ActionResult, recovery tools.RecoveryDecision) bool {
	if e == nil {
		return false
	}
	snap := e.conversationState.Snapshot()
	intent := interaction.ResponseIntent{
		Kind:           interaction.ResponseIntentAnswer,
		Confidence:     .90,
		ShouldSpeak:    true,
		ShouldContinue: true,
		ThreadID:       snap.ActiveThreadID,
		Objective:      strings.TrimSpace(req.Objective.Text),
		Language:       snap.Language,
		Reason:         "production recovery decision must be communicated by the conversation owner",
	}
	move := interaction.MoveAnswer
	strategyMode := cognition.ResponseModeAnswer
	switch recovery.Action {
	case tools.RecoveryAskUser:
		intent.Kind = interaction.ResponseIntentAsk
		intent.NeedsUserInput = true
		move = interaction.MoveAsk
		strategyMode = cognition.ResponseModeClarify
	case tools.RecoveryReplan:
		intent.Kind = interaction.ResponseIntentContinue
		move = interaction.MoveContinue
	case tools.RecoveryBlock:
		intent.Kind = interaction.ResponseIntentWait
		move = interaction.MoveWait
	}
	e.lastResponseIntent = intent
	e.lastNaturalDialogue.Move = move
	contract := interaction.NewResponseContract(req, intent, move)
	contract.Source = "production_recovery_boundary"
	contract.ShouldAct = false
	reason := strings.TrimSpace(recovery.Reason)
	if reason == "" {
		reason = strings.TrimSpace(result.FailureReason)
	}
	if reason == "" {
		reason = "la ejecución no pudo completar el resultado esperado"
	}
	contract.AddEvidence(fmt.Sprintf("Recuperación de producción: %s. %s", recovery.Action, reason), "production_recovery", true, .90)
	strategy := cognition.ConversationalResponseStrategy{
		Version: "B96.33.10", Mode: strategyMode,
		Directness: cognition.DirectnessDirect, Structure: cognition.StructureExplanation,
		Initiative: cognition.InitiativeHelpful, Confidence: intent.Confidence,
		PreserveUserAgency: true, ExecutionBoundary: true, ShouldGenerateLanguage: true,
		Reason: "language realization of a structured production recovery outcome",
	}
	return e.renderResponseContractWithStrategy(req, contract, e.buildConversationalRuntimeFacts(snap), strategy)
}
