package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"ahmelia/autonomy"
	"ahmelia/capability"
	"ahmelia/execution"
	"ahmelia/interaction"
	"ahmelia/production"
	"ahmelia/tools"
)

// ProductionRuntime performs production work after Cognition has established
// an executable posture and the Capability layer has resolved a production
// route. It returns structured facts; it does not decide conversational
// wording and never emits say()/sayAsync().
type ProductionRuntime struct {
	resolver     *capability.Resolver
	capabilities *capability.Registry
	providers    *production.ProviderRegistry
	context      *production.ProductionContext
	work         WorkStorePort
	guard        *production.ExecutionGuard
	recovery     *production.TechnicalRecoveryService
	execution    *execution.Dispatcher
}

func cloneRawMessages(in []json.RawMessage) []json.RawMessage {
	if len(in) == 0 {
		return nil
	}
	out := make([]json.RawMessage, len(in))
	for i, raw := range in {
		out[i] = append(json.RawMessage(nil), raw...)
	}
	return out
}

func cloneActionParameters(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in)+1)
	for k, v := range in {
		out[k] = v
	}
	return out
}

func NewProductionRuntime(
	resolver *capability.Resolver,
	capabilities *capability.Registry,
	providers *production.ProviderRegistry,
	ctx *production.ProductionContext,
	work WorkStorePort,
	recovery *production.TechnicalRecoveryService,
) (*ProductionRuntime, error) {
	if resolver == nil || capabilities == nil || providers == nil || ctx == nil || work == nil || recovery == nil {
		return nil, fmt.Errorf("integration: production runtime requires resolver, providers, context, work store and recovery")
	}
	guard, err := production.NewExecutionGuard(capabilities, providers)
	if err != nil {
		return nil, err
	}
	registry := execution.NewRegistry()
	dispatcher, err := execution.NewDispatcher(registry)
	if err != nil {
		return nil, err
	}
	worker, err := execution.NewLocalWorker("local", execution.WorkerCapabilities{CPU: 1, MemoryMB: 1, ConcurrentSlots: 4})
	if err != nil {
		return nil, err
	}
	if err := dispatcher.RegisterWorker(worker); err != nil {
		return nil, err
	}
	return &ProductionRuntime{
		resolver: resolver, capabilities: capabilities, providers: providers,
		context: ctx, work: work, guard: guard, recovery: recovery, execution: dispatcher,
	}, nil
}

// execute performs production only after the Engine execution authority has
// accepted the provider-neutral request. ProductionRuntime is an execution
// adapter, not a second execution gateway: callers outside the central
// authority must not invoke this method.
func (r *ProductionRuntime) ExecuteFromAuthority(ctx context.Context, req interaction.SemanticRequest, route capability.Route, options production.ExecutionOptions) production.ActionResult {
	if r == nil || r.guard == nil || r.recovery == nil {
		return production.ActionResult{Err: fmt.Errorf("production runtime: execution dependencies unavailable"), Success: false}
	}
	if route.Kind != capability.IntentProduction && route.Kind != capability.IntentAction {
		return production.ActionResult{Err: fmt.Errorf("production runtime: invalid capability decision %q", route.Kind), Success: false}
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if options.Approval == "" {
		options.Approval = autonomy.ApprovalNone
	}
	if options.RecoveryAttempt < 1 {
		options.RecoveryAttempt = 1
	}
	for _, candidate := range route.Candidates {
		approval := options.Approval
		if options.ApprovalResolver != nil {
			approval = options.ApprovalResolver(candidate.ID)
		}
		if approval == "" {
			approval = autonomy.ApprovalNone
		}
		if options.ResumePendingAuthorization {
			if candidate.ID != options.ExpectedCapabilityID || candidate.Action != options.ExpectedOperation {
				return production.ActionResult{Success: false, WorkID: options.WorkID, FailureReason: "authorization_resume_identity_mismatch", Err: fmt.Errorf("production runtime: persisted capability identity no longer matches")}
			}
			if _, exists := r.providers.ResolveFor(candidate, options.ExpectedProviderID); !exists {
				return production.ActionResult{Success: false, WorkID: options.WorkID, FailureReason: "authorization_resume_identity_mismatch", Err: fmt.Errorf("production runtime: persisted provider identity no longer matches")}
			}
		}
		selectedProviderID := options.SelectedProviderID
		if options.ResumePendingAuthorization {
			selectedProviderID = options.ExpectedProviderID
		}
		preflight, err := r.guard.Check(candidate, candidate.ID, candidate.Action, approval, selectedProviderID)
		if err != nil {
			return production.ActionResult{Success: false, FailureReason: "execution_preflight_failed: " + err.Error(), Err: err}
		}
		if options.ResumePendingAuthorization {
			if candidate.ID != options.ExpectedCapabilityID || candidate.Action != options.ExpectedOperation ||
				preflight.RegisteredProviderID != options.ExpectedProviderID {
				return production.ActionResult{Success: false, WorkID: options.WorkID, FailureReason: "authorization_resume_identity_mismatch", Err: fmt.Errorf("production runtime: persisted capability/provider identity no longer matches current execution registry")}
			}
		}

		if !preflight.Allowed {
			if preflight.NeedsAuthorization && route.Kind == capability.IntentProduction {
				workID := strings.TrimSpace(options.WorkID)
				if workID == "" {
					w, beginErr := r.work.Begin(req)
					if beginErr != nil {
						return production.ActionResult{Err: beginErr, Success: false}
					}
					workID = w.ID
				}
				if _, ok := r.work.Get(workID); !ok {
					return production.ActionResult{Err: fmt.Errorf("production runtime: work %q no existe", workID), Success: false, WorkID: workID}
				}
				if err := r.work.AttachProduction(workID, "", "", preflight.RegisteredProviderID); err != nil {
					return production.ActionResult{Err: err, Success: false, WorkID: workID}
				}
				attemptNumber := options.AttemptNumber
				if attemptNumber < 1 {
					var beginErr error
					attemptNumber, beginErr = r.work.BeginAttempt(workID, string(candidate.ID), candidate.Action, preflight.RegisteredProviderID, string(candidate.SkillID))
					if beginErr != nil {
						return production.ActionResult{Err: beginErr, Success: false, WorkID: workID}
					}
				}
				if err := r.work.SetAttemptTaskID(workID, attemptNumber, options.TaskID); err != nil {
					return production.ActionResult{Err: err, Success: false, WorkID: workID, AttemptNumber: attemptNumber}
				}
				pending := production.PendingExecution{Request: req, TaskID: options.TaskID, CapabilityID: string(candidate.ID), Operation: candidate.Action, RouteKind: string(route.Kind), ProviderID: preflight.RegisteredProviderID, AttemptNumber: attemptNumber, Reason: preflight.AuthorizationReason}
				if err := r.work.SetPendingExecution(workID, pending); err != nil {
					return production.ActionResult{Err: err, Success: false, WorkID: workID}
				}
				return production.ActionResult{Success: false, WorkID: workID, AuthorizationRequired: true, AuthorizationCapabilityID: candidate.ID, AuthorizationReason: preflight.AuthorizationReason, FailureReason: "execution_not_authorized"}
			}
			return production.ActionResult{Success: false, AuthorizationRequired: false, AuthorizationCapabilityID: candidate.ID, AuthorizationReason: preflight.AuthorizationReason, FailureReason: "execution_not_authorized"}
		}

		provider, ok := r.providers.ResolveFor(preflight.Capability, preflight.RegisteredProviderID)
		if !ok || provider == nil {
			return production.ActionResult{Success: false, FailureReason: "execution_provider_missing_after_preflight"}
		}

		trackWork := route.Kind == capability.IntentProduction
		workID := strings.TrimSpace(options.WorkID)
		if trackWork && workID == "" {
			work, err := r.work.Begin(req)
			if err != nil {
				return production.ActionResult{Err: err, Success: false}
			}
			workID = work.ID
		}
		if trackWork && workID != "" {
			if _, ok := r.work.Get(workID); !ok {
				return production.ActionResult{Err: fmt.Errorf("production runtime: work %q no existe", workID), Success: false, WorkID: workID}
			}
			if err := r.work.AttachProduction(workID, "", "", provider.ID()); err != nil {
				return production.ActionResult{Err: err, Success: false, WorkID: workID}
			}
		}

		attemptNumber := 0
		if trackWork {
			attemptNumber = options.AttemptNumber
			if attemptNumber < 1 {
				attemptNumber, err = r.work.BeginAttempt(workID, string(preflight.Capability.ID), preflight.Capability.Action, provider.ID(), string(preflight.Capability.SkillID))
				if err != nil {
					return production.ActionResult{Err: err, Success: false, WorkID: workID}
				}
			} else if err := r.work.ResumeAttempt(workID, attemptNumber); err != nil {
				return production.ActionResult{Err: err, Success: false, WorkID: workID}
			}
			if err := r.work.SetAttemptTaskID(workID, attemptNumber, options.TaskID); err != nil {
				return production.ActionResult{Err: err, Success: false, WorkID: workID, AttemptNumber: attemptNumber}
			}
		}

		params := cloneActionParameters(req.Action.Parameters)
		var project production.ProductionProject
		if trackWork {
			if w, ok := r.work.Get(workID); ok && w.LastActionResult != nil {
				project = w.LastActionResult.Project
			}
		}
		providerInput := strings.TrimSpace(req.Objective.Text)
		if providerInput == "" {
			providerInput = req.Raw
		}
		recoveryEvidence := ""
		var recoveryContext *tools.RecoveryContext
		if trackWork && attemptNumber > 1 {
			if work, ok := r.work.Get(workID); ok {
				for i := len(work.Attempts) - 1; i >= 0; i-- {
					previous := work.Attempts[i]
					if previous.Number >= attemptNumber || previous.Result == nil || previous.Result.Success && previous.Result.Verified {
						continue
					}
					recoveryEvidence = fmt.Sprintf("stage=%s; failure=%s", previous.Result.FailedStage, previous.Result.FailureReason)
					recoveryContext = previous.Result.RecoveryContext.Clone()
					if recoveryContext != nil {
						recoveryContext.AttemptNumber = attemptNumber
						recoveryContext.Objective = req.Frame.PrimaryGoal
						recoveryContext.ExpectedState = append([]string(nil), req.Frame.Verification...)
						recoveryContext.ObservedState = append([]string(nil), previous.Result.ObservationDetails, previous.Result.VerificationReason)
						recoveryContext.FailureReason = previous.Result.FailureReason
					} else {
						recoveryContext = &tools.RecoveryContext{AttemptNumber: attemptNumber, Objective: req.Frame.PrimaryGoal, ExpectedState: append([]string(nil), req.Frame.Verification...), ObservedState: []string{previous.Result.ObservationDetails, previous.Result.VerificationReason}, FailureReason: previous.Result.FailureReason}
					}
					// The next decision is a continuation of the same Work, not a fresh
					// problem. Rehydrate the exact prior production and its domain
					// evaluations so the next synthesizer can reason from what was
					// actually attempted, not only from the final diagnostic.
					recoveryContext.PriorAttempt = &tools.RecoveryAttempt{
						Number:              previous.Number,
						EffectSpecification: append([]byte(nil), previous.EffectSpecification...),
						EffectEvaluations:   cloneRawMessages(previous.EffectEvaluations),
					}
					break
				}
			}
		}
		providerRequest := production.ProviderRequest{Input: providerInput, Target: req.Action.Target, WorkID: workID, AttemptNumber: attemptNumber, RecoveryEvidence: recoveryEvidence, RecoveryContext: recoveryContext, EffectRecorder: func(spec []byte) error { return r.work.SetAttemptEffectSpecification(workID, attemptNumber, spec) }, EffectEvaluationRecorder: func(evaluation []byte) error {
			return r.work.RecordAttemptEffectEvaluation(workID, attemptNumber, evaluation)
		}, Project: project, Parameters: params, Capability: preflight.Capability, CapabilityID: preflight.Capability.ID, Operation: preflight.Capability.Action, ObjectiveFrame: req.Frame}
		job := execution.Job{ID: fmt.Sprintf("%s-attempt-%d", workID, attemptNumber), WorkID: workID, CapabilityID: string(preflight.Capability.ID), Operation: preflight.Capability.Action}
		var providerResult production.ProviderResult
		if r.execution == nil {
			return production.ActionResult{Success: false, WorkID: workID, FailureReason: "execution_fabric_unavailable"}
		}
		executionAttemptID := job.ID + "-execution"
		if trackWork {
			if err := r.work.SetAttemptExecutionIdentity(workID, attemptNumber, job.ID, executionAttemptID); err != nil {
				return production.ActionResult{Success: false, WorkID: workID, FailureReason: "execution_identity_persist_failed", Err: err}
			}
		}
		_, dispatchErr := r.execution.Dispatch(ctx, job, func(execCtx context.Context) error {
			providerResult = provider.Execute(execCtx, providerRequest)
			return providerResult.Err
		})
		executed := dispatchErr == nil || providerResult.Err != nil || providerResult.Success || providerResult.Observed || providerResult.Verified
		if dispatchErr != nil && providerResult.Err == nil {
			providerResult.Err = dispatchErr
			providerResult.Success = false
		}
		if providerResult.ProductionID != "" && providerResult.Project.Valid() {
			r.context.Set(providerResult.ProductionID, providerResult.Producer, providerResult.Project, providerResult.Artifacts)
		}
		var artifactPersistErr error
		if trackWork {
			for _, a := range providerResult.Artifacts {
				if err := r.work.RecordArtifact(workID, a); err != nil && artifactPersistErr == nil {
					artifactPersistErr = err
				}
			}
		}

		verified := providerResult.Verified && providerResult.Observed
		outcomeStatus := providerResult.OutcomeStatus
		if !outcomeStatus.Valid() || (providerResult.Verified && !providerResult.Observed) || (providerResult.Success && !verified && outcomeStatus == production.OutcomeSucceeded) {
			if !providerResult.Success || outcomeStatus == production.OutcomeFailed {
				outcomeStatus = production.OutcomeFailed
			} else if verified {
				outcomeStatus = production.OutcomeSucceeded
			} else {
				outcomeStatus = production.OutcomeUnknown
			}
		}

		result := production.ActionResult{
			Executed:              executed,
			GenerationSource:      providerResult.GenerationSource,
			VerificationProfile:   providerResult.VerificationProfile,
			ObjectiveStatus:       providerResult.ObjectiveStatus,
			ObjectiveContractJSON: providerResult.ObjectiveContractJSON,
			Success:               providerResult.Success, OutcomeStatus: outcomeStatus, Verified: verified, Observed: providerResult.Observed,
			WorkID: workID, AttemptNumber: attemptNumber, ProductionID: providerResult.ProductionID, Producer: providerResult.Producer,
			Project: providerResult.Project, Artifacts: providerResult.Artifacts, Summary: providerResult.Summary,
			ObservationDetails: providerResult.ObservationDetails, ObservationFacts: providerResult.ObservationFacts,
			ExpectationID: providerResult.ExpectationID, ExpectationStatus: providerResult.ExpectationStatus,
			ExpectationSummary: providerResult.ExpectationSummary, ExpectationError: providerResult.ExpectationError,
			VerificationReason: providerResult.VerificationReason,
			VerificationSource: providerResult.VerificationSource, OutcomeCode: providerResult.OutcomeCode, InputRequired: providerResult.InputRequired,
			FailedStage: providerResult.FailedStage, FailureReason: providerResult.FailureReason, RecoveryContext: providerResult.RecoveryContext, Err: providerResult.Err,
		}
		if result.Producer == "" {
			result.Producer = provider.ID()
		}
		if artifactPersistErr != nil {
			// The external effect may exist, but its artifact record did not commit.
			// Preserve uncertainty rather than claiming a fully recorded success.
			result.Success = false
			result.Verified = false
			result.OutcomeStatus = production.OutcomeUnknown
			result.FailureReason = "artifact_persistence_failed: " + artifactPersistErr.Error()
		}

		// A provider error is a failure. A provider that executed successfully
		// but cannot observe the resulting world is UNKNOWN, not FAILED.
		// Recovery must not be triggered merely because verification is absent.
		knownOutcomeFailure := providerResult.Err != nil || !providerResult.Success || providerResult.OutcomeStatus == production.OutcomeFailed
		verificationUnknown := artifactPersistErr != nil || (providerResult.Success && result.OutcomeStatus == production.OutcomeUnknown && (!providerResult.Observed || !result.Verified))
		if knownOutcomeFailure {
			result.Success = false
			result.OutcomeStatus = production.OutcomeFailed
			if verificationUnknown && result.FailureReason == "" {
				if !providerResult.Observed {
					result.FailureReason = "behavioral_verification_missing_observation"
				} else {
					result.FailureReason = "behavioral_verification_failed"
				}
			}
			detail := result.FailureReason
			if detail == "" && result.Err != nil {
				detail = result.Err.Error()
			}
			if detail == "" {
				detail = result.Summary
			}
			if detail == "" {
				detail = "provider_execution_failed"
			}
			result.FailureReason = detail

			recoveryAction := ""
			recoveryReason := ""
			if trackWork {
				if recovery, recoverErr := r.recovery.Recover(ctx, string(preflight.Capability.ID), detail, options.RecoveryAttempt); recoverErr == nil {
					result.Recovery = &recovery.Decision
					recoveryAction = string(recovery.Decision.Action)
					recoveryReason = recovery.Decision.Reason
				} else {
					result.FailureReason = recoverErr.Error()
					recoveryReason = recoverErr.Error()
				}
			}

			if trackWork {
				terminal := production.WorkFailed
				if recoveryAction == "retry" || recoveryAction == "replan" {
					terminal = "" // same Work remains open for the next authority pass
				}
				if err := r.work.FinalizeAttempt(workID, attemptNumber, result, recoveryAction, recoveryReason, terminal, detail); err != nil {
					result.Success = false
					result.Verified = false
					result.OutcomeStatus = production.OutcomeUnknown
					result.FailureReason = "work_state_persistence_failed: " + err.Error()
				}
			}
			return result
		}

		if verificationUnknown {
			result.OutcomeStatus = production.OutcomeUnknown
			if result.FailureReason == "" {
				result.FailureReason = "execution_observed_but_outcome_unverified"
			}
			if trackWork {
				// Attempt outcome, LastActionResult and Work lifecycle are committed
				// together; no partial sequence of ignored writes.
				if err := r.work.FinalizeAttempt(workID, attemptNumber, result, "", "reconciliation_required", "", result.FailureReason); err != nil {
					result.Success = false
					result.Verified = false
					result.OutcomeStatus = production.OutcomeUnknown
					result.FailureReason = "work_state_persistence_failed: " + err.Error()
				}
			}
			return result
		}

		if trackWork {
			if err := r.work.FinalizeAttempt(workID, attemptNumber, result, "", "", production.WorkCompleted, providerResult.Summary); err != nil {
				// The external effect may be real, but its canonical closure did not
				// commit. Do not report verified success to the caller.
				result.Success = false
				result.Verified = false
				result.OutcomeStatus = production.OutcomeUnknown
				result.FailureReason = "work_state_persistence_failed: " + err.Error()
			}
		}
		return result
	}

	return production.ActionResult{Success: false, Verified: false, Summary: "No existe un proveedor ejecutable para la capacidad seleccionada.", FailureReason: "no_production_provider"}
}

// RepairFromAuthority reconciles a previously materialized work result after
// Cognition has established that a correction targets that work. It is not a
// second authority boundary: callers must already have crossed authorityExecute.
func (r *ProductionRuntime) RepairFromAuthority(ctx context.Context, intent production.RepairIntent) production.ActionResult {
	if r == nil {
		return production.ActionResult{Success: false, FailureReason: "production_runtime_unavailable"}
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if strings.TrimSpace(intent.WorkID) == "" || strings.TrimSpace(intent.Replacement) == "" {
		return production.ActionResult{Success: false, FailureReason: "invalid_repair_intent"}
	}
	previous := intent.PreviousResult
	if !previous.Success || !previous.Verified || strings.TrimSpace(previous.Producer) == "" {
		return production.ActionResult{Success: false, FailureReason: "repair_requires_successful_verified_previous_result", WorkID: intent.WorkID}
	}
	work, ok := r.work.Get(intent.WorkID)
	if !ok || work.LastActionResult == nil {
		return production.ActionResult{Success: false, FailureReason: "materialized_work_result_missing", WorkID: intent.WorkID}
	}
	if len(work.Attempts) == 0 {
		return production.ActionResult{Success: false, FailureReason: "repair_requires_prior_attempt_identity", WorkID: intent.WorkID}
	}
	priorAttempt := work.Attempts[len(work.Attempts)-1]
	provider, ok := r.providers.ResolveProvider(previous.Producer)
	if !ok {
		return production.ActionResult{Success: false, FailureReason: "repair_provider_missing", WorkID: intent.WorkID}
	}
	repairProvider, ok := provider.(production.RepairProvider)
	if !ok {
		return production.ActionResult{Success: false, FailureReason: "provider_does_not_support_repair", WorkID: intent.WorkID, Producer: previous.Producer}
	}
	attemptNumber, err := r.work.BeginAttempt(intent.WorkID, priorAttempt.CapabilityID, priorAttempt.Operation, previous.Producer, priorAttempt.SkillID)
	if err != nil {
		return production.ActionResult{Success: false, WorkID: intent.WorkID, FailureReason: "repair_attempt_start_failed", Err: err}
	}
	providerResult := repairProvider.Repair(ctx, intent)
	outcomeStatus := providerResult.OutcomeStatus
	if !outcomeStatus.Valid() {
		if providerResult.Err != nil || !providerResult.Success {
			outcomeStatus = production.OutcomeFailed
		} else if providerResult.Observed && providerResult.Verified {
			outcomeStatus = production.OutcomeSucceeded
		} else {
			outcomeStatus = production.OutcomeUnknown
		}
	}
	result := production.ActionResult{
		Executed:            true,
		GenerationSource:    providerResult.GenerationSource,
		VerificationProfile: providerResult.VerificationProfile,
		ObjectiveStatus:     providerResult.ObjectiveStatus, ObjectiveContractJSON: providerResult.ObjectiveContractJSON,
		Success: providerResult.Success && outcomeStatus != production.OutcomeFailed, OutcomeStatus: outcomeStatus,
		Verified: providerResult.Verified && providerResult.Observed && outcomeStatus == production.OutcomeSucceeded, Observed: providerResult.Observed,
		WorkID: intent.WorkID, AttemptNumber: attemptNumber, ProductionID: providerResult.ProductionID,
		Producer: providerResult.Producer, Project: providerResult.Project,
		Artifacts: providerResult.Artifacts, Summary: providerResult.Summary,
		ObservationDetails: providerResult.ObservationDetails, VerificationReason: providerResult.VerificationReason,
		VerificationSource: providerResult.VerificationSource, OutcomeCode: providerResult.OutcomeCode, InputRequired: providerResult.InputRequired,
		FailedStage: providerResult.FailedStage, FailureReason: providerResult.FailureReason, Err: providerResult.Err,
	}
	if result.Producer == "" {
		result.Producer = previous.Producer
	}
	if result.Err != nil || outcomeStatus == production.OutcomeFailed || !result.Success {
		detail := result.FailureReason
		if detail == "" && result.Err != nil {
			detail = result.Err.Error()
		}
		if detail == "" {
			detail = "repair_failed"
		}
		if err := r.work.FinalizeAttempt(intent.WorkID, attemptNumber, result, "", "", production.WorkFailed, detail); err != nil {
			result.Success = false
			result.Verified = false
			result.OutcomeStatus = production.OutcomeUnknown
			result.FailureReason = "work_state_persistence_failed: " + err.Error()
		}
		return result
	}
	if outcomeStatus == production.OutcomeUnknown || !result.Verified {
		result.OutcomeStatus = production.OutcomeUnknown
		result.Verified = false
		if result.FailureReason == "" {
			result.FailureReason = "repair_outcome_unverified_reconciliation_required"
		}
		if err := r.work.FinalizeAttempt(intent.WorkID, attemptNumber, result, "", "reconciliation_required", "", result.FailureReason); err != nil {
			result.Success = false
			result.Verified = false
			result.OutcomeStatus = production.OutcomeUnknown
			result.FailureReason = "work_state_persistence_failed: " + err.Error()
		}
		return result
	}
	if err := r.work.FinalizeRepairAttempt(intent.WorkID, attemptNumber, result, intent.Replacement, "correction materialized and verified"); err != nil {
		// The effect may be real, but the objective, attempt result and closure
		// did not commit together. Report uncertainty; never report success.
		result.Success = false
		result.Verified = false
		result.OutcomeStatus = production.OutcomeUnknown
		result.FailureReason = "repair_finalization_persist_failed: " + err.Error()
		return result
	}
	if result.ProductionID != "" && result.Project.Valid() {
		r.context.Set(result.ProductionID, result.Producer, result.Project, result.Artifacts)
	}
	return result
}

// QueryWork exposes operational state as structured evidence. It never formats
// a user-facing answer. This preserves follow-up behavior without restoring a
// ProductionRuntime response authority.
func (r *ProductionRuntime) QueryWork(req interaction.SemanticRequest, activeWorkID ...string) (production.WorkQueryResult, bool) {
	if r == nil {
		return production.WorkQueryResult{}, false
	}
	if strings.Contains(req.Normalized, "capacidades") &&
		(strings.Contains(req.Normalized, "tienes") || strings.Contains(req.Normalized, "disponibles") || strings.Contains(req.Normalized, "puedes")) {
		return production.WorkQueryResult{Kind: production.WorkQueryCapabilities, Found: true, Capabilities: r.resolverCapabilities()}, true
	}
	if !(req.FollowUp || req.AddressesCurrent || len(req.References) > 0 || req.Continues) {
		return production.WorkQueryResult{}, false
	}
	var current production.WorkSession
	var ok bool
	// A referenced/current WorkID is conversational evidence and outranks
	// storage recency. WorkStore.Last is allowed only for an explicit recent-work
	// query; it is never an implicit continuity owner.
	for _, ref := range req.References {
		if (ref.Kind == interaction.ReferenceRecentWork || ref.Kind == interaction.ReferenceRecentArtifact) && strings.TrimSpace(ref.Value) != "" {
			current, ok = r.work.Get(strings.TrimSpace(ref.Value))
			if ok {
				break
			}
		}
	}
	if !ok {
		for _, id := range activeWorkID {
			if strings.TrimSpace(id) == "" {
				continue
			}
			current, ok = r.work.Get(strings.TrimSpace(id))
			if ok {
				break
			}
		}
	}
	if !ok && interaction.IsExplicitWorkResume(req.Normalized) {
		current, ok = r.work.Last()
	}
	if !ok {
		return production.WorkQueryResult{}, false
	}
	n := req.Normalized
	result := production.WorkQueryResult{Found: true, Work: current}
	if req.Continues || req.Kind == interaction.KindContinuation {
		result.Kind = production.WorkQueryContinuation
		return result, true
	}
	if strings.Contains(n, "qué estábamos haciendo") || strings.Contains(n, "que estabamos haciendo") || strings.Contains(n, "what were we doing") || strings.Contains(n, "what were we working on") {
		result.Kind = production.WorkQueryActive
		return result, true
	}
	if strings.Contains(n, "resultado") || strings.Contains(n, "qué pasó") || strings.Contains(n, "que paso") || strings.Contains(n, "what happened") || strings.Contains(n, "how did it go") {
		result.Kind = production.WorkQueryRecentResult
		if recent, ok := r.work.RecentResult(); ok {
			result.RecentResult = recent
		}
		return result, true
	}
	if strings.Contains(n, "y el test") || strings.Contains(n, "y la prueba") || strings.Contains(n, "the test") || strings.Contains(n, "the tests") {
		result.Kind = production.WorkQueryTest
		for _, a := range current.Artifacts {
			lower := strings.ToLower(a.Name)
			if strings.Contains(lower, "test") || strings.Contains(lower, "prueba") || strings.HasSuffix(lower, "_test.go") {
				result.Artifact = a
				result.HasArtifact = true
				break
			}
		}
		return result, true
	}
	if hasReference(req, interaction.ReferenceLocation) || strings.Contains(n, "donde") || strings.Contains(n, "dónde") || strings.Contains(n, "where is") || strings.Contains(n, "ruta") {
		result.Kind = production.WorkQueryLocation
		for _, a := range current.Artifacts {
			name := strings.ToLower(a.Name)
			stem := strings.ToLower(strings.TrimSuffix(a.Name, filepath.Ext(a.Name)))
			if strings.Contains(n, name) || (stem != "" && strings.Contains(n, stem)) {
				result.Artifact = a
				result.HasArtifact = true
				break
			}
		}
		if !result.HasArtifact && len(current.Artifacts) > 0 {
			result.Artifact = current.Artifacts[len(current.Artifacts)-1]
			result.HasArtifact = true
		}
		return result, true
	}
	if strings.Contains(n, "como vas") || strings.Contains(n, "qué falta") || strings.Contains(n, "que falta") || strings.Contains(n, "qué queda") || strings.Contains(n, "que queda") {
		result.Kind = production.WorkQueryProgress
		return result, true
	}
	if strings.Contains(n, "que archivos") || strings.Contains(n, "qué archivos") || strings.Contains(n, "que creaste") || strings.Contains(n, "qué creaste") || strings.Contains(n, "what files") || strings.Contains(n, "files you created") {
		result.Kind = production.WorkQueryArtifacts
		return result, true
	}
	return production.WorkQueryResult{}, false
}

func (r *ProductionRuntime) resolverCapabilities() []string {
	if r == nil || r.capabilities == nil {
		return nil
	}
	caps := r.capabilities.List()
	result := make([]string, 0, len(caps))
	for _, c := range caps {
		result = append(result, string(c.ID))
	}
	return result
}

func hasReference(req interaction.SemanticRequest, kind interaction.ReferenceKind) bool {
	for _, ref := range req.References {
		if ref.Kind == kind {
			return true
		}
	}
	return false
}

// LastWork exposes a read-only operational snapshot for integration/E2E
// consumers. It does not expose provider internals or conversational text.
func (r *ProductionRuntime) LastWork() (production.WorkSession, bool) {
	if r == nil || r.work == nil {
		return production.WorkSession{}, false
	}
	return r.work.Last()
}
