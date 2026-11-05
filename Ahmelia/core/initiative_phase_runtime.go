package core

import (
	"fmt"
	"strings"
	"time"

	"ahmelia/cognition"
	"ahmelia/interaction"
	"ahmelia/production"

	conversationalinitiative "ahmelia/initiative"
)

func (e *Engine) runInitiativeDecisionPhase(t *processTurn) bool {
	// Paso 3.4: el modo descanso no abre iniciativa proactiva. La respuesta al
	// turno actual sigue su curso normal; lo que se cierra es la intervención.
	if !e.persistentModeAllowsInitiative() {
		return false
	}
	// B96.22.19 — Conversational Initiative. Legacy provider retained only as a candidate/signal source.
	// B96.37.3 — Initiative Signal Architecture. Existing candidate producers
	// and conversation state are observed through adapters. This layer only
	// normalizes/aggregates evidence and constructs an opportunity; B96.37.2
	// remains the sole decision authority.
	if e.conversationState != nil && e.conversationalInitiative != nil && e.initiativeSignals != nil {
		snap := e.conversationState.Snapshot()
		var initiativeProfile *interaction.UserProfile
		if e.userModelStore != nil {
			authority := interaction.NewUserProfile().PrimaryAuthority
			if e.userProfile != nil {
				authority = e.userProfile.PrimaryAuthority
			}
			initiativeProfile, _ = interaction.UserProfileCompatibilityView(e.userModelStore, authority)
		}

		// B96.39 — Natural Dialogue is the structural gate between the current
		// conversational turn and proactive initiative. It does not decide whether
		// to intervene; it only tells initiative whether the current turn has an
		// unmet/direct conversational need and whether optional continuation is
		// structurally admissible.
		dialogueDecision := e.lastNaturalDialogue
		candidate, candidateOK := interaction.GenerateQuestion(interaction.QuestionGenerationContext{
			Request: t.semanticReq, Snapshot: snap, Profile: initiativeProfile,
		})
		currentTurnOwnsResponse := strings.TrimSpace(t.semanticReq.Raw) != "" && e.lastResponseIntent.ShouldSpeak && e.lastResponseIntent.Kind != interaction.ResponseIntentNone
		requiresDirect := dialogueDecision.RequiresDirectTurn || currentTurnOwnsResponse
		// A direct current-turn response is a hard negative signal for proactive
		// candidate generation. Initiative may still observe the turn, but it is
		// never allowed to speak before the canonical response has been emitted.
		if requiresDirect {
			candidate = interaction.QuestionCandidate{}
			candidateOK = false
		}
		cooldown := false // B96.37.13 supersedes this legacy heuristic after interruption assessment.

		// B96.37.5 — collect detached source facts at the Engine boundary.
		// The Engine reads domain owners; initiative only receives observations.
		now := time.Now()
		userModelEnrichment := conversationalinitiative.UserModelEnrichment{}
		if initiativeProfile != nil {
			profileSnap := initiativeProfile.Snapshot()
			reliable := func(m map[string]interaction.ProfileFact) int {
				n := 0
				for _, fact := range m {
					if fact.Status != "superseded" && fact.Confidence >= 0.70 {
						n++
					}
				}
				return n
			}
			userModelEnrichment = conversationalinitiative.UserModelEnrichment{
				Available: true, ReliableFacts: reliable(profileSnap.Preferences) + reliable(profileSnap.Habits) + reliable(profileSnap.Interests) + reliable(profileSnap.Routines) + reliable(profileSnap.Notes) + reliable(profileSnap.Communication) + reliable(profileSnap.Goals),
				ReliableGoals: reliable(profileSnap.Goals), ReliablePreferences: reliable(profileSnap.Preferences),
				CommunicationProfile: reliable(profileSnap.Communication) > 0, ObservedAt: now,
			}
		}
		commitmentEnrichment := conversationalinitiative.CommitmentEnrichment{OpenCount: len(snap.Commitments), LatestAt: now}
		for i := len(snap.Commitments) - 1; i >= 0; i-- {
			if snap.Commitments[i].Status == "open" {
				commitmentEnrichment.LatestText = snap.Commitments[i].Text
				commitmentEnrichment.LatestOwner = snap.Commitments[i].Owner
				commitmentEnrichment.LatestAt = snap.Commitments[i].At
				break
			}
		}
		commitmentEnrichment.OpenCount = 0
		for _, c := range snap.Commitments {
			if c.Status == "open" {
				commitmentEnrichment.OpenCount++
			}
		}
		workEnrichment := conversationalinitiative.WorkEnrichment{}
		if e.workStore != nil {
			if w, ok := e.workStore.Last(); ok {
				activeOperational := w.ID == snap.ActiveWorkID || w.Status == production.WorkPending || w.Status == production.WorkRunning || w.Status == production.WorkBlocked || w.Status == production.WorkFailed
				if activeOperational {
					workEnrichment.Exists, workEnrichment.ID = true, w.ID
					workEnrichment.Status, workEnrichment.CurrentStep = string(w.Status), w.CurrentStep
					workEnrichment.PendingSteps, workEnrichment.UpdatedAt = len(w.Pending), w.UpdatedAt
				}
			}
		}
		autonomyEnrichment := conversationalinitiative.AutonomyEnrichment{}
		if e.autonomousState != nil {
			autonomyEnrichment.ExecutionState = string(e.autonomousState.Execution())
			autonomyEnrichment.ActiveTask = e.autonomousState.ActiveTask()
			autonomyEnrichment.WaitingForUser = e.autonomousState.WaitingForUser()
			autonomyEnrichment.LastActivity = e.autonomousState.LastActivity()
			autonomyEnrichment.LastInteraction = e.autonomousState.LastInteraction()
		}
		if e.goalMonitor != nil {
			for _, g := range e.goalMonitor.Snapshot() {
				if g.Stale {
					autonomyEnrichment.StaleGoals++
				}
				if g.Blocked {
					autonomyEnrichment.BlockedGoals++
				}
			}
		}
		autonomyEnrichment.ObservedAt = now
		signalInput := conversationalinitiative.SignalInput{
			Now: now, RequestKind: string(t.semanticReq.Kind), ActiveTopic: snap.ActiveTopic, ActiveObjective: snap.ActiveObjective,
			ActiveWorkID: snap.ActiveWorkID, LastUserInput: snap.LastUserInput, LastAssistant: snap.LastAssistant, TurnCount: len(snap.Turns),
			OpenThreadCount: len(snap.OpenThreads), CommitmentCount: commitmentEnrichment.OpenCount, CandidateExists: candidateOK, CandidateTopic: candidate.Topic,
			CandidateText: initiativeCandidateSemanticLabel(candidate), CandidateConfidence: candidate.Confidence, Channel: string(t.semanticReq.Channel),
			UserUrgency: snap.UserState.Urgency, UserFrustration: snap.UserState.Frustration, UserConfusion: snap.UserState.Confusion, UserAvailability: conversationalinitiative.AvailabilityUnknown,
			Privacy: conversationalinitiative.InformationAllowed, Interruptibility: conversationalinitiative.InterruptNotRecommended, ConversationLoad: conversationalinitiative.LoadNormal,
			RequiresDirectTurn: requiresDirect, CooldownActive: cooldown, HistoricalAvailable: e.conversationHistory != nil, HistoricalRequired: t.historicalRequired,
			Enrichment: conversationalinitiative.BuildEnrichment(now,
				conversationalinitiative.ContextEnrichment{HasContext: strings.TrimSpace(snap.ActiveTopic) != "" || strings.TrimSpace(snap.ActiveObjective) != "", Topic: snap.ActiveTopic, Objective: snap.ActiveObjective, ThreadID: snap.ActiveThreadID, OpenThreads: len(snap.OpenThreads), ObservedAt: now},
				conversationalinitiative.CognitionEnrichment{Valid: e.lastConversationalCognition.Valid, Version: e.lastConversationalCognition.Version, Intent: string(e.lastConversationalCognition.Intent.Primary), IntentConfidence: e.lastConversationalCognition.Intent.Confidence, Goal: e.lastConversationalCognition.Goal.Objective, Relevance: e.lastConversationalCognition.Relevance.Score, NeedsClarification: e.lastConversationalCognition.Interpretation.NeedsClarification, ActionOriented: e.lastConversationalCognition.Interpretation.ActionOriented, Ambiguous: e.lastConversationalCognition.Intent.Ambiguous, ObservedAt: now},
				userModelEnrichment,
				conversationalinitiative.HistoricalMemoryEnrichment{Available: e.conversationHistory != nil, Required: t.historicalRequired, ObservedAt: now},
				workEnrichment, commitmentEnrichment, autonomyEnrichment,
				conversationalinitiative.AvailabilityEnrichment{Availability: conversationalinitiative.AvailabilityUnknown, Interruptibility: conversationalinitiative.InterruptNotRecommended, ConversationLoad: conversationalinitiative.LoadNormal, ObservedAt: now},
				conversationalinitiative.PrivacySafetyEnrichment{Policy: conversationalinitiative.InformationAllowed, PolicyKnown: true, Interruptibility: conversationalinitiative.InterruptNotRecommended, ObservedAt: now},
				conversationalinitiative.ChannelEnrichment{Channel: string(t.semanticReq.Channel), Language: snap.Language, Supported: t.semanticReq.Channel != interaction.ChannelUnknown, ObservedAt: now},
			),
		}
		// B96.37.9 — interpret availability from explicit evidence/current turn.
		// The policy owns availability classification; Engine only supplies observed facts.
		if e.initiativeAvailabilityPolicy != nil {
			assessment := e.initiativeAvailabilityPolicy.Evaluate(conversationalinitiative.AvailabilityEvidence{
				Explicit: signalInput.UserAvailability, LastUserActivity: autonomyLastInteraction(e.autonomousState),
				CurrentTurnActive: true, WaitingForUser: autonomyWaitingForUser(e.autonomousState), Now: now,
			})
			if err := conversationalinitiative.ValidateAvailabilityAssessment(assessment); err == nil {
				signalInput.UserAvailability = assessment.Availability
				signalInput.Enrichment.Availability.Availability = assessment.Availability
				signalInput.Enrichment.Availability.ObservedAt = assessment.ObservedAt
			}
		}
		collectedSignals := e.initiativeSignals.Collect(signalInput)
		opportunities := conversationalinitiative.BuildCandidateOpportunities(collectedSignals, signalInput)
		// B96.37.10..13 enrich EACH candidate before arbitration. Arbitration
		// compares already-enriched opportunities; it does not recreate policy.
		prepared := make([]conversationalinitiative.InitiativeOpportunity, 0, len(opportunities))
		for _, candidate := range opportunities {
			prepared = append(prepared, e.prepareInitiativeOpportunity(candidate, signalInput, snap, signalInput.Now))
		}
		// B96.37.16 contributes bounded evidence-only adaptation signals. They
		// influence ordering only; they cannot produce INTERVENE/WAIT/DEFER.
		adaptations := e.initiativeSignals.InitiativeAdaptationSignals(signalInput.Now)
		var opportunity conversationalinitiative.InitiativeOpportunity
		var arbitrationResult conversationalinitiative.ArbitrationResult
		if e.initiativeArbiter != nil {
			result, err := e.initiativeArbiter.Arbitrate(conversationalinitiative.NewArbitrationInput(prepared, adaptations, e.initiativeTimingEvaluator, signalInput.Now))
			if err == nil && result.Selected.Valid {
				arbitrationResult = result
				opportunity = result.Selected.Opportunity
			} else {
				// Empty/invalid arbitration fails closed: there is no candidate to
				// authorize. B96.37.2 receives an empty opportunity.
				opportunity = conversationalinitiative.BuildOpportunity(nil, signalInput)
			}
		} else {
			opportunity = conversationalinitiative.BuildOpportunity(nil, signalInput)
		}
		// B96.37.20 — open the technical trace before the canonical decision.
		// The identity is semantic evidence; lifecycle/decision IDs are correlated
		// later. Observability never supplies the decision.
		var initiativeTrace conversationalinitiative.InitiativeTrace
		if e.initiativeObservability != nil {
			traceID := conversationalinitiative.NormalizeInitiativeIdentity(opportunity)
			if !opportunity.Exists {
				traceID = "evaluation-" + signalInput.Now.UTC().Format("20060102T150405.000000000Z07:00")
			}
			if tr, err := e.initiativeObservability.Create(traceID, opportunity.Source, opportunity.Signal.Provenance, signalInput.Now); err == nil {
				initiativeTrace = tr
				for _, sig := range opportunity.Signals {
					_, _ = e.initiativeObservability.RecordSignal(tr.ID, sig, signalInput.Now)
				}
				if opportunity.Exists {
					_, _ = e.initiativeObservability.RecordOpportunity(tr.ID, opportunity, signalInput.Now)
				}
				_, _ = e.initiativeObservability.RecordEvaluation(tr.ID, "B96.37.9..13", map[string]string{
					"availability": string(opportunity.UserAvailability), "privacy": string(opportunity.Privacy), "interruptibility": string(opportunity.Interruptibility), "conversation_load": string(opportunity.ConversationLoad), "priority": string(opportunity.Priority), "urgency": string(opportunity.Urgency),
					"opportunity_exists": fmt.Sprintf("%t", opportunity.Exists), "signal_count": fmt.Sprintf("%d", len(opportunity.Signals)),
				}, signalInput.Now)
				if !opportunity.Exists {
					_, _ = e.initiativeObservability.RecordReason(tr.ID, "B96.37.3..17", "no valid initiative opportunity", "candidate/opportunity construction produced no actionable opportunity", signalInput.Now)
				}
				if !arbitrationResult.ComparedAt.IsZero() {
					_, _ = e.initiativeObservability.RecordArbitration(tr.ID, arbitrationResult, signalInput.Now)
				}
			}
		}
		// B96.37.17 stops here. The selected opportunity is NOT a decision.
		// B96.37.2 remains the sole authority for INTERVENE / WAIT / DEFER.
		decision := e.conversationalInitiative.Evaluate(opportunity, signalInput.Now)
		if decision.Reason == conversationalinitiative.ReasonContradictsAuthoritativeEvidence && e.flowTrace != nil {
			e.flowTrace.Mark("INITIATIVE DISAGREEMENT", fmt.Sprintf("findings=%s confidence=%.2f decision=%s", strings.Join(opportunity.Signal.EvidenceIDs, ","), opportunity.Signal.Confidence, decision.Decision))
		}
		if e.flowTrace != nil {
			e.flowTrace.Mark("INITIATIVE", fmt.Sprintf("decision=%s timing=%s candidate=%t requires_direct=%t", decision.Decision, decision.Timing, candidateOK, requiresDirect))
		}
		if initiativeTrace.ID != "" && e.initiativeObservability != nil {
			_, _ = e.initiativeObservability.RecordDecision(initiativeTrace.ID, decision, signalInput.Now)
			if len(decision.Constraints) > 0 {
				_, _ = e.initiativeObservability.RecordConstraints(initiativeTrace.ID, "B96.37.2", decision.Constraints, signalInput.Now)
			}
			_, _ = e.initiativeObservability.RecordReason(initiativeTrace.ID, "B96.37.2", string(decision.Reason), constraintCondition(decision.Constraints), signalInput.Now)
		}

		// B96.37.19 — create/update lifecycle state only after the canonical
		// B96.37.2 decision exists. Lifecycle records the decision; it never creates it.
		var lifecycle conversationalinitiative.InitiativeLifecycle
		if e.initiativeLifecycleManager != nil && opportunity.Exists {
			if existing, found := e.initiativeLifecycleManager.FindDuplicate(opportunity); found {
				lifecycle = existing
			} else if created, err := e.initiativeLifecycleManager.Create(opportunity, signalInput.Now); err == nil {
				lifecycle = created
			}
			if lifecycle.ID != "" {
				if initiativeTrace.ID != "" && e.initiativeObservability != nil {
					_, _ = e.initiativeObservability.RebindInitiative(initiativeTrace.ID, lifecycle.ID, signalInput.Now)
				}
				_, _ = e.initiativeLifecycleManager.Transition(lifecycle.ID, conversationalinitiative.LifecycleEvaluating, signalInput.Now, "core.Engine", "initiative evaluation cycle")
				if _, err := e.initiativeLifecycleManager.Transition(lifecycle.ID, conversationalinitiative.LifecycleSelected, signalInput.Now, "B96.37.17", "arbitrated candidate selected"); err != nil {
					// An already-progressed lifecycle is acceptable; state ownership remains lifecycle-only.
				}
				_, _ = e.initiativeLifecycleManager.RecordDecision(lifecycle.ID, decision, signalInput.Now)
				if x, err := e.initiativeLifecycleManager.Transition(lifecycle.ID, conversationalinitiative.LifecycleDecided, signalInput.Now, "B96.37.2", "canonical initiative decision recorded"); err == nil {
					lifecycle = x
				}
				switch decision.Decision {
				case conversationalinitiative.Defer:
					if x, err := e.initiativeLifecycleManager.Transition(lifecycle.ID, conversationalinitiative.LifecycleDeferred, signalInput.Now, "B96.37.2", "canonical decision DEFER recorded"); err == nil {
						lifecycle = x
					}
				case conversationalinitiative.Intervene:
					if x, err := e.initiativeLifecycleManager.Transition(lifecycle.ID, conversationalinitiative.LifecycleIntervening, signalInput.Now, "B96.37.2", "canonical decision INTERVENE recorded"); err == nil {
						lifecycle = x
					}
				}
			}
		}
		if initiativeTrace.ID != "" && lifecycle.ID != "" && e.initiativeObservability != nil {
			_, _ = e.initiativeObservability.Correlate(initiativeTrace.ID, map[string]string{
				"initiative_id": lifecycle.ID, "opportunity_id": initiativeTrace.OpportunityID, "decision_id": decision.ID, "lifecycle_id": lifecycle.ID,
			}, signalInput.Now)
			_, _ = e.initiativeObservability.RecordLifecycle(initiativeTrace.ID, lifecycle, signalInput.Now)
		}
		// B96.37.14 — persist DEFER as a revocable pending intent. Persistence does
		// not imply a promise and does not alter the canonical decision authority.
		if e.initiativeDeferredIntentManager != nil && decision.Decision == conversationalinitiative.Defer {
			_, _, _ = e.initiativeDeferredIntentManager.PersistDecision(decision, signalInput.Now)
		}
		// B96.37.18 — persist temporal reevaluation only after B96.37.2
		// explicitly returned DEFER. A schedule is not an intervention and
		// never bypasses policy when its trigger fires.
		if e.initiativeScheduler != nil && decision.Decision == conversationalinitiative.Defer {
			if scheduled, _, err := e.initiativeScheduler.ScheduleDecision(decision, signalInput.Now); err == nil {
				if initiativeTrace.ID != "" && e.initiativeObservability != nil {
					_, _ = e.initiativeObservability.RecordScheduling(initiativeTrace.ID, scheduled, signalInput.Now)
				}
				if lifecycle.ID != "" && e.initiativeLifecycleManager != nil {
					_, _ = e.initiativeLifecycleManager.Transition(lifecycle.ID, conversationalinitiative.LifecycleScheduled, signalInput.Now, "B96.37.18", "temporal reevaluation scheduled")
				}
			}
		}
		if e.initiativeActionSelector != nil {
			finalized, err := e.initiativeActionSelector.FinalizeDecision(decision, signalInput.Now)
			if err != nil {
				if initiativeTrace.ID != "" && e.initiativeObservability != nil {
					_, _ = e.initiativeObservability.RecordReason(initiativeTrace.ID, "B96.37.6", "action finalization failed closed", err.Error(), signalInput.Now)
				}
				// Invalid initiative contracts fail closed: do not emit a conversational turn.
				decision = conversationalinitiative.InitiativeDecision{Decision: conversationalinitiative.Wait, Timing: conversationalinitiative.TimingWait, CreatedAt: signalInput.Now, PolicyVersion: decision.PolicyVersion, Provenance: conversationalinitiative.Provenance{Component: "core.Engine", Subsystem: "B96.37.6", CreatedAt: signalInput.Now}}
			} else {
				decision = finalized
			}
		}
		// B96.37.7 — validate the semantic action contract after B96.37.6.
		// This is a fail-closed contract boundary; it does not authorize or generate.
		if e.initiativeActionContract != nil {
			if contract, err := e.initiativeActionContract.Build(decision); err != nil {
				if initiativeTrace.ID != "" && e.initiativeObservability != nil {
					_, _ = e.initiativeObservability.RecordReason(initiativeTrace.ID, "B96.37.7", "action contract rejected; fail closed", err.Error(), signalInput.Now)
				}
				decision = conversationalinitiative.InitiativeDecision{Decision: conversationalinitiative.Wait, Timing: conversationalinitiative.TimingWait, CreatedAt: signalInput.Now, PolicyVersion: decision.PolicyVersion, Provenance: conversationalinitiative.Provenance{Component: "core.Engine", Subsystem: "B96.37.7", CreatedAt: signalInput.Now}}
			} else if initiativeTrace.ID != "" && e.initiativeObservability != nil {
				_, _ = e.initiativeObservability.RecordAction(initiativeTrace.ID, contract, signalInput.Now)
			}
		}
		if initiativeTrace.ID != "" && e.initiativeObservability != nil {
			if decision.Decision == conversationalinitiative.Intervene {
				_, _ = e.initiativeObservability.RecordTiming(initiativeTrace.ID, "B96.37.8", decision.Timing, "canonical timing", opportunity.CooldownActive, opportunity.UserAvailability, opportunity.Interruptibility, time.Time{}, time.Time{}, signalInput.Now)
			}
		}
		if e.lastBehavioralDecision.Version == "B96.40.1" {
			switch decision.Decision {
			case conversationalinitiative.Intervene:
				e.lastBehavioralDecision = e.lastBehavioralDecision.WithInitiative(cognition.InitiativeIntervene)
			case conversationalinitiative.Defer:
				e.lastBehavioralDecision = e.lastBehavioralDecision.WithInitiative(cognition.InitiativeDefer)
			default:
				e.lastBehavioralDecision = e.lastBehavioralDecision.WithInitiative(cognition.InitiativeWait)
			}
		}
		if decision.Decision == conversationalinitiative.Intervene {
			// B96.37.7 authorizes the semantic action; B96.37.20 records it.
			// Delivery (InterventionProposal -> Dialogue -> Language -> output)
			// is shared with the idle path via deliverInitiativeIntervention.
			// No candidate.Question is allowed to cross this boundary.
			delivery := &InitiativeDelivery{Request: t.semanticReq, Opportunity: opportunity, Decision: decision, Topic: candidate.Topic}
			if e.deliverInitiativeIntervention(delivery) {
				if initiativeTrace.ID != "" && e.initiativeObservability != nil {
					_, _ = e.initiativeObservability.Complete(initiativeTrace.ID, signalInput.Now)
				}
				return true
			}
		}
		if initiativeTrace.ID != "" && e.initiativeObservability != nil {
			_, _ = e.initiativeObservability.Complete(initiativeTrace.ID, signalInput.Now)
		}
	}
	return false
}
