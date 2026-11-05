package core

import (
	"fmt"
	"strings"
	"time"

	"ahmelia/interaction"
	"ahmelia/production"
	"ahmelia/situation"
)

// B96.42.3 — Result / Observation / Verification Convergence.
//
// The Core receives one structured action outcome and exposes its three
// epistemic stages independently to Situation. A successful execution is not
// silently treated as a verified world-state change.
func (e *Engine) convergeActionOutcome(req interaction.SemanticRequest, result production.ActionResult) {
	if e == nil {
		return
	}
	persisted := e.hasPersistedActionOutcome(result)
	now := time.Now()
	actionID := strings.TrimSpace(result.ProductionID)
	if actionID == "" {
		actionID = strings.TrimSpace(req.Action.Name)
	}
	if actionID == "" {
		actionID = "unknown-action"
	}

	appendObservation := func(kind, predicate, object, value, source string, confidence float64, epistemic situation.EpistemicStatus) {
		if persisted {
			return
		}
		e.pendingSituationObservations = append(e.pendingSituationObservations, situation.Observation{
			ID:         fmt.Sprintf("execution-%s-%d", kind, now.UnixNano()+int64(len(e.pendingSituationObservations))),
			Kind:       kind,
			Source:     source,
			Subject:    actionID,
			Predicate:  predicate,
			Object:     object,
			Value:      value,
			ObservedAt: now,
			Confidence: confidence,
			Epistemic:  epistemic,
		})
	}

	resultValue := "success=false"
	if result.Success {
		resultValue = "success=true"
	}
	source := strings.TrimSpace(result.Producer)
	if source == "" {
		source = "execution.authority"
	}
	appendObservation("execution_result", "result", req.Action.Name, resultValue, source, result.Confidence(), situation.EpistemicObserved)

	if result.Observed {
		details := strings.TrimSpace(result.ObservationDetails)
		if details == "" {
			details = "el proveedor reportó evidencia observable posterior a la ejecución"
		}
		appendObservation("execution_observation", "observed_state", req.Action.Name, details, source, .92, situation.EpistemicObserved)
	}

	verificationReason := strings.TrimSpace(result.VerificationReason)
	if verificationReason == "" {
		if result.Verified {
			verificationReason = "el proveedor reportó que el resultado quedó verificado"
		} else {
			verificationReason = "el resultado de la ejecución todavía no está verificado"
		}
	}
	verificationValue := "verified=false"
	confidence := .82
	if result.Verified {
		verificationValue = "verified=true"
		confidence = .98
	}
	verificationSource := strings.TrimSpace(result.VerificationSource)
	if verificationSource == "" {
		verificationSource = source
	}
	appendObservation("execution_verification", "verification", verificationValue, verificationReason, verificationSource, confidence, situation.EpistemicDerived)
	if !persisted {
		for _, observation := range productionProvenanceObservations(&result, actionID, persistedAttemptSkillID(e.workStore, result.WorkID, result.AttemptNumber), result.WorkID, result.AttemptNumber, now, situation.TemporalCurrent) {
			e.pendingSituationObservations = append(e.pendingSituationObservations, observation)
		}
	}

	if e.flowTrace != nil {
		e.flowTrace.Mark("RESULT", fmt.Sprintf("action=%s success=%t producer=%s", req.Action.Name, result.Success, source))
		e.flowTrace.Mark("OBSERVATION", fmt.Sprintf("action=%s observed=%t", req.Action.Name, result.Observed))
		e.flowTrace.Mark("VERIFICATION", fmt.Sprintf("action=%s verified=%t source=%s", req.Action.Name, result.Verified, verificationSource))
	}
}

// hasPersistedActionOutcome ensures WorkStore remains the owner of feedback
// whenever the execution result was durably recorded. The next Situation
// reconstructs those observations from the conversation's explicit Work link;
// the in-memory queue is only a fallback for outcomes without that owner.
func (e *Engine) hasPersistedActionOutcome(result production.ActionResult) bool {
	if e == nil || e.workStore == nil || strings.TrimSpace(result.WorkID) == "" {
		return false
	}
	work, ok := e.workStore.Get(result.WorkID)
	if !ok || work.LastActionResult == nil {
		return false
	}
	persisted := work.LastActionResult
	if result.AttemptNumber > 0 && persisted.AttemptNumber != result.AttemptNumber {
		return false
	}
	if result.ProductionID != "" && persisted.ProductionID != result.ProductionID {
		return false
	}
	if persisted.WorkID != "" && persisted.WorkID != result.WorkID {
		return false
	}
	return persisted.Success == result.Success && persisted.Verified == result.Verified
}

func persistedWorkOutcomeObservations(work production.WorkSession) []situation.Observation {
	if work.LastActionResult == nil {
		return nil
	}
	result := work.LastActionResult
	if len(work.Attempts) > 0 && work.Attempts[len(work.Attempts)-1].Number != result.AttemptNumber {
		// A newer attempt may be running or unverified after restart. Keep the
		// prior result in Work history, but do not project it as the latest effect.
		return nil
	}
	at := result.ObservationCapturedAt
	if at.IsZero() {
		at = work.UpdatedAt
	}
	source := strings.TrimSpace(result.ObservationSource)
	if source == "" {
		source = strings.TrimSpace(result.Producer)
	}
	if source == "" {
		source = "production.WorkStore"
	}
	actionID := strings.TrimSpace(result.ObservationActionID)
	if actionID == "" {
		actionID = strings.TrimSpace(result.ProductionID)
	}
	if actionID == "" {
		actionID = "unknown-action"
	}
	identity := fmt.Sprintf("work-%s-attempt-%d", work.ID, result.AttemptNumber)
	contextValue := fmt.Sprintf("work=%s; attempt=%d; action=%s; operation=%s; target=%s", work.ID, result.AttemptNumber, actionID, result.ObservationOperation, result.ObservationTarget)

	observations := []situation.Observation{{
		ID: identity + "-result", Kind: "execution_result", Source: source,
		Subject: actionID, Predicate: "result", Object: string(result.OutcomeStatus),
		Value:      fmt.Sprintf("success=%t; %s", result.Success, contextValue),
		ObservedAt: at, Temporal: situation.TemporalHistorical, Confidence: result.Confidence(), Epistemic: situation.EpistemicObserved,
	}}
	if result.Observed {
		details := strings.TrimSpace(result.ObservationDetails)
		if details == "" {
			details = "el resultado contiene evidencia ambiental observada"
		}
		observations = append(observations, situation.Observation{
			ID: identity + "-observation", Kind: "execution_observation", Source: source,
			Subject: actionID, Predicate: "observed_state", Object: result.ObservationTarget,
			Value: details + "; " + contextValue, ObservedAt: at, Temporal: situation.TemporalHistorical, Confidence: .92, Epistemic: situation.EpistemicObserved,
		})
	}
	verifiedValue := "verified=false"
	verificationEpistemic := situation.EpistemicDerived
	verificationConfidence := .82
	if result.Verified {
		verifiedValue = "verified=true"
		verificationEpistemic = situation.EpistemicObserved
		verificationConfidence = .98
	}
	verificationReason := strings.TrimSpace(result.VerificationReason)
	if verificationReason == "" {
		verificationReason = "la verificación está registrada en el resultado persistido"
	}
	verificationSource := strings.TrimSpace(result.VerificationSource)
	if verificationSource == "" {
		verificationSource = source
	}
	observations = append(observations, situation.Observation{
		ID: identity + "-verification", Kind: "execution_verification", Source: verificationSource,
		Subject: actionID, Predicate: "verification", Object: verifiedValue,
		Value: verificationReason + "; " + contextValue, ObservedAt: at, Temporal: situation.TemporalHistorical, Confidence: verificationConfidence, Epistemic: verificationEpistemic,
	})
	observations = append(observations, productionProvenanceObservations(result, actionID, persistedAttemptSkillIDFromSession(work, result.AttemptNumber), work.ID, result.AttemptNumber, at, situation.TemporalHistorical)...)
	for i, fact := range result.ObservationFacts {
		if strings.TrimSpace(fact.Predicate) == "" {
			continue
		}
		subject := strings.TrimSpace(fact.Subject)
		if subject == "" {
			subject = strings.TrimSpace(result.ObservationTarget)
		}
		value := fmt.Sprintf("expected=%s; action=%s; operation=%s; target=%s; attempt=%d", fact.Expected, actionID, result.ObservationOperation, result.ObservationTarget, result.AttemptNumber)
		observations = append(observations, situation.Observation{
			ID: fmt.Sprintf("%s-fact-%d", identity, i), Kind: "execution_postcondition",
			Source: source, Subject: subject, Predicate: fact.Predicate, Object: fact.Observed,
			Value: value, ObservedAt: at, Temporal: situation.TemporalHistorical, Confidence: .99, Epistemic: situation.EpistemicObserved,
		})
	}
	return observations
}

// productionProvenanceObservations places authorship and verification scope in
// the shared Situation so cognition can reason about what Ahmelia produced,
// what an external generator proposed, and what was actually checked.
func productionProvenanceObservations(result *production.ActionResult, actionID, skillID, workID string, attempt int, at time.Time, temporal situation.TemporalStatus) []situation.Observation {
	if result == nil {
		return nil
	}
	contextValue := fmt.Sprintf("work=%s; attempt=%d", strings.TrimSpace(workID), attempt)
	observations := make([]situation.Observation, 0, 6)
	add := func(kind, predicate, object string, epistemic situation.EpistemicStatus) {
		object = strings.TrimSpace(object)
		if object == "" {
			return
		}
		observations = append(observations, situation.Observation{
			ID:   fmt.Sprintf("work-%s-attempt-%d-%s", strings.TrimSpace(workID), attempt, kind),
			Kind: kind, Source: "production.ActionResult", Subject: actionID,
			Predicate: predicate, Object: object, Value: contextValue,
			ObservedAt: at, Temporal: temporal, Confidence: .99, Epistemic: epistemic,
		})
	}
	add("production_skill_identity", "skill_id", skillID, situation.EpistemicDerived)
	add("production_generation_provenance", "generation_source", result.GenerationSource, situation.EpistemicObserved)
	add("production_verification_scope", "verification_profile", result.VerificationProfile, situation.EpistemicObserved)
	add("production_objective_status", "objective_status", result.ObjectiveStatus, situation.EpistemicObserved)
	add("production_objective_contract", "objective_contract_json", result.ObjectiveContractJSON, situation.EpistemicObserved)
	add("production_target_environment", "target_environment", result.Project.Path, situation.EpistemicObserved)
	return observations
}

func persistedAttemptSkillID(store *PersistentWorkStore, workID string, attempt int) string {
	if store == nil || strings.TrimSpace(workID) == "" {
		return ""
	}
	work, ok := store.Get(workID)
	if !ok {
		return ""
	}
	return persistedAttemptSkillIDFromSession(work, attempt)
}

func persistedAttemptSkillIDFromSession(work production.WorkSession, attempt int) string {
	for _, candidate := range work.Attempts {
		if candidate.Number == attempt {
			return strings.TrimSpace(candidate.SkillID)
		}
	}
	return ""
}
