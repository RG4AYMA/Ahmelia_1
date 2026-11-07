package core

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"ahmelia/autonomy"
	"ahmelia/capability"
	"ahmelia/cognition"
	"ahmelia/contextintelligence"
	"ahmelia/dialogue"
	"ahmelia/initiative"
	"ahmelia/interaction"
	"ahmelia/linguistics"
	"ahmelia/naturaldialogue"
	"ahmelia/personality"
	"ahmelia/production"
	"ahmelia/shared"
	"ahmelia/situation"
)

// processTurn is the mutable, per-turn carrier used by Process orchestration.
// It contains state/evidence/results only; it is not an authority itself.
type processTurn struct {
	input     string
	normInput string
	channel   interaction.InputChannel

	semanticReq interaction.SemanticRequest
	// Captured before context/history enrichment so pending authorization cannot
	// be reinterpreted by a later conversational stage.
	authorizationActs interaction.CommunicativeActs
	contextResolution contextintelligence.Result
	interactionCtx    interaction.Context

	workBridgeDecision     interaction.WorkBridgeDecision
	interactionWorkContext interaction.WorkContext
	interactionHasWork     bool

	specialistEvidence        specialistEvidenceBundle
	responseSpecialistResults SpecialistResponseResults

	previousTurn                 interaction.TurnRecord
	hasPreviousTurn              bool
	previousConversationSnapshot interaction.ConversationSnapshot
	dialogueLink                 interaction.DialogueLink
	historicalRequired           bool

	// Fase 2 (2026-10-18): lecturas diferidas acumuladas por el turno. El
	// envío no es inmediato: el render del propio turno usa el modelo primero
	// y la lectura — evidencia, nunca autoridad — arranca cuando el turno ya
	// habló (submitTurnEndDeferredReadings). La excepción es el turno sin
	// acto tipado que la lectura puede resolver: ese espera acotado en vivo.
	pendingDeferred []pendingDeferredReading

	// cognitionRan marca que esta pasada produjo una decisión conductual. La
	// deliberación en sombra solo corre si hay algo real que comparar; de lo
	// contrario usaría la decisión del turno anterior.
	cognitionRan bool
	// ownerRestrictionHandled marca que la puerta determinista ya consumió la
	// frase de descanso/reapertura. La consulta al modelo no debe reinterpretar
	// una orden que el arnés ya aplicó.
	ownerRestrictionHandled bool
	// ownerStateHandled marca que la puerta de estado efímero consumió la frase.
	ownerStateHandled bool
}

// inputContextPhase crosses the input and context boundaries. It does not execute,
// generate language, or decide terminal behavior.
func (e *Engine) semanticTurnAbandonsObjectiveDraft(req interaction.SemanticRequest) bool {
	if req.Kind == interaction.KindStop || req.Kind == interaction.KindPause {
		return true
	}
	return false
}

func isOperationalWorkStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "pending", "running", "blocked", "recovering":
		return true
	default:
		return false
	}
}

// currentConversationWorkContext is the single read boundary from
// conversation ownership to the operational WorkStore. WorkStore.Last is a
// historical query and must never masquerade as the active conversational
// work owner; doing so lets a failed/unrelated previous Work leak into a new
// turn. Explicit resume commands are handled separately by WorkBridge.
func (e *Engine) currentConversationWorkContext() (interaction.WorkContext, bool) {
	if e == nil || e.workStore == nil || e.conversationState == nil {
		return interaction.WorkContext{}, false
	}
	workID := strings.TrimSpace(e.conversationState.ActiveWork())
	if workID == "" {
		return interaction.WorkContext{}, false
	}
	w, ok := e.workStore.Get(workID)
	if !ok {
		return interaction.WorkContext{}, false
	}
	// A completed owner remains readable while ConversationState still points
	// to it. Cognition needs its verified artifact evidence to decide whether a
	// user correction targets already materialized work and should propose
	// repair. Completed work is not thereby reopened or made executable.
	completedVerified := w.Status == production.WorkCompleted && w.LastActionResult != nil &&
		w.LastActionResult.Success && w.LastActionResult.Verified
	if !isOperationalWorkStatus(string(w.Status)) && !completedVerified {
		return interaction.WorkContext{}, false
	}
	return workContextFromSession(w), true
}

func (e *Engine) inputContextPhase(t *processTurn) func() {
	t.interactionCtx = interaction.Context{}
	if e.interactionManager != nil {
		var lastArtifactName, lastArtifactPath string
		var pendingObjective string
		if e.conversationState != nil {
			if draft, ok := e.conversationState.PendingObjectiveDraft(); ok {
				pendingObjective = draft.Text
			}
		}
		if e.productionContext != nil {
			_, _, project, artifacts := e.productionContext.Snapshot()
			if project.Name != "" {
				lastArtifactName, lastArtifactPath = project.Name, project.Path
			}
			if len(artifacts) > 0 {
				lastArtifactName, lastArtifactPath = artifacts[len(artifacts)-1].Name, artifacts[len(artifacts)-1].Path
			}
		}
		previous := e.interactionManager.Last()
		t.interactionCtx = interaction.Context{
			PreviousInput: previous.Raw, PreviousKind: previous.Kind,
			ActiveObjective:  e.context.GetCurrentTopic(),
			LastArtifactName: lastArtifactName, LastArtifactPath: lastArtifactPath,
			Language:         previous.Language,
			PendingObjective: pendingObjective,
			Channel:          t.channel,
		}
		if wc, ok := e.currentConversationWorkContext(); ok {
			t.interactionCtx.ActiveWorkID = wc.ID
			t.interactionCtx.ActiveWorkStatus = wc.Status
			t.interactionCtx.CurrentStep = wc.CurrentStep
			t.interactionCtx.ActiveWorkProvider = wc.Provider
			if strings.TrimSpace(wc.Objective) != "" {
				t.interactionCtx.ActiveObjective = wc.Objective
			}
			if strings.TrimSpace(wc.LastResult) != "" {
				interationLastResult := wc.LastResult
				t.interactionCtx.LastResult = interationLastResult
			}
		}
		t.semanticReq = e.analyzeSemanticBoundary(t.input, t.interactionCtx, false, &t.pendingDeferred)
		t.authorizationActs = interaction.CanonicalActs(t.semanticReq)
		if pendingObjective != "" && e.conversationState != nil &&
			(e.semanticTurnAbandonsObjectiveDraft(t.semanticReq) || interaction.IsExplicitNewObjective(t.semanticReq.Normalized)) {
			e.conversationState.DiscardObjectiveDraft()
			if e.sess != nil {
				e.sess.RestoreVoiceMode()
			}
			pendingObjective = ""
			if e.flowTrace != nil {
				e.flowTrace.Mark("OBJECTIVE DRAFT", "discarded: explicit control or new objective boundary")
			}
		}
		// A voice or UI adapter may legitimately deliver one human objective in
		// several transport turns. If ConversationState owns an unfinished
		// objective draft, assemble the new fragment before Situation/Cognition
		// sees it. This is the channel-independent recovery boundary; it does
		// not create Work or grant execution authority.
		if pendingObjective != "" && interaction.ShouldAppendObjectiveDraft(t.semanticReq) && !interaction.IsExplicitNewObjective(t.semanticReq.Normalized) {
			composed := pendingObjective
			fragments := 1
			if draft := e.conversationState.AppendObjectiveDraft(t.input); draft != nil {
				composed = draft.Text
				fragments = draft.Fragments
			}
			if composed != pendingObjective {
				t.semanticReq = e.analyzeSemanticBoundary(composed, t.interactionCtx, true, &t.pendingDeferred)
				if e.flowTrace != nil {
					e.flowTrace.Mark("OBJECTIVE DRAFT", fmt.Sprintf("assembled pending objective fragments=%d chars=%d", fragments, len([]rune(composed))))
				}
			} else if e.flowTrace != nil {
				e.flowTrace.Mark("OBJECTIVE ASSEMBLY", fmt.Sprintf("structural fragment ignored fragments=%d", fragments))
			}
		}
		if e.flowTrace != nil {
			e.flowTrace.Mark("SEMANTIC", fmt.Sprintf("subject=%s topic=%q references=%d addresses_current=%t %s", t.semanticReq.Subject, t.semanticReq.Topic, len(t.semanticReq.References), t.semanticReq.AddressesCurrent, formatActs(t.semanticReq.CommunicativeActs)))
		}
	}

	// B96.36 — derived context is a read-only transformation of the current
	// semantic request; concrete state owners remain responsible for mutation.
	resolvedReq, resolved := e.resolveContextBoundary(t.semanticReq)
	t.semanticReq = resolvedReq
	t.contextResolution = resolved
	if e.flowTrace != nil {
		e.flowTrace.Mark("STATE", fmt.Sprintf("context_valid=%t continuity=%s", resolved.Valid, resolved.Continuity.Kind))
		e.flowTrace.Mark("MEMORY", fmt.Sprintf("historical_required=%t references=%d", resolved.Historical.Required, len(t.semanticReq.References)))
	}

	if e.conversationState != nil {
		if t.semanticReq.BreaksContinuity {
			if e.context != nil {
				e.context.ClearCurrentTopic()
			}
			if e.conversationFlow != nil {
				e.conversationFlow.ResetCurrentTopic()
			}
		}
		if e.threadRouter != nil {
			_ = e.threadRouter.Route(e.conversationState, t.semanticReq)
		}
		snap := e.conversationState.Snapshot()
		if e.workBridge != nil {
			wc, hasWork := e.currentConversationWorkContext()
			// WorkStore.Last is historical, not an implicit owner. However, a
			// semantic reference to recent/current work is an explicit recovery
			// request. Resolve that reference against the latest operational work
			// and restore its canonical conversation owner before deciding how the
			// turn relates to it. This covers restart/lost-link recovery without
			// making unrelated new requests inherit historical work.
			if !hasWork && e.workStore != nil && interaction.HasReference(t.semanticReq, interaction.ReferenceRecentWork) {
				if historical, ok := e.workStore.Last(); ok && isOperationalWorkStatus(string(historical.Status)) {
					wc = workContextFromSession(historical)
					hasWork = true
					e.conversationState.SetActiveWork(historical.ID)
					snap.ActiveWorkID = historical.ID
					if e.flowTrace != nil {
						e.flowTrace.Mark("WORK RECOVERY", "explicit recent-work reference resolved to persisted operational owner")
					}
				}
			}
			t.interactionWorkContext, t.interactionHasWork = wc, hasWork
			t.workBridgeDecision = e.workBridge.Resolve(t.semanticReq, snap, wc, hasWork)
			if t.workBridgeDecision.AppendObjective && hasWork {
				composed := interaction.ComposeObjective(wc.Objective, t.semanticReq.Objective.Text)
				if composed != "" && composed != wc.Objective {
					t.semanticReq.Objective.Text = composed
					t.workBridgeDecision.Objective = composed
					if e.workStore != nil {
						_ = e.workStore.UpdateObjective(wc.ID, composed, "objective specification continued from conversation")
					}
				}
			}
		}
	}

	resumeID := uint64(0)
	if e.clarificationQueue != nil {
		if pc := e.clarificationQueue.Oldest(); pc != nil {
			if pc.AnswersRequest(t.semanticReq) {
				resolved := e.clarificationQueue.Resolve(pc.ID())
				if resolved != nil {
					t.input = resolved.Merge(t.input, isBareConfirmation(t.input))
					if e.interactionManager != nil {
						t.semanticReq = e.analyzeSemanticBoundary(t.input, t.interactionCtx, true, &t.pendingDeferred)
						var contextResolved contextintelligence.Result
						t.semanticReq, contextResolved = e.resolveContextBoundary(t.semanticReq)
						t.contextResolution = contextResolved
						if e.flowTrace != nil {
							e.flowTrace.Mark("SEMANTIC", "re-finalized after clarification")
							e.flowTrace.Mark("MEMORY", "context re-resolved after clarification")
						}
					}
				}
			} else {
				resumeID = pc.ID()
			}
		}
	}
	return func() { e.resumeClarificationIfStillOpen(resumeID) }
}

// conversationStatePhase persists the current semantic turn and installs the
// end-of-turn persistence boundary. The returned cleanup deliberately executes
// before the clarification cleanup, matching the legacy LIFO ordering.
func (e *Engine) conversationStatePhase(t *processTurn) func() {
	if e.conversationState == nil {
		return func() {}
	}
	t.previousConversationSnapshot = e.conversationState.Snapshot()
	t.previousTurn, t.hasPreviousTurn = e.conversationState.LastTurn()
	if t.hasPreviousTurn {
		t.dialogueLink = interaction.LinkTurn(t.semanticReq, t.previousTurn, true, e.conversationState.Snapshot())
	}
	if t.semanticReq.Correction.IsCorrection {
		if lastTurn, ok := e.conversationState.LastTurn(); ok {
			t.semanticReq = interaction.ApplyCorrection(t.semanticReq, lastTurn.UserText)
		}
	}
	if t.dialogueLink.Relation == interaction.RelationAnswer {
		t.semanticReq = interaction.ResolveAnswerSemantics(t.semanticReq, t.previousTurn)
	}
	if e.flowTrace != nil {
		e.flowTrace.Mark("RESPONSE GATE", "deferred until after Situation/Cognition")
	}
	e.activeConversationTurnID = e.conversationState.RecordTurnLinked(t.semanticReq, "", t.dialogueLink.Relation, t.dialogueLink.ParentTurnID)
	if t.workBridgeDecision.WorkID != "" {
		e.conversationState.SetActiveWork(t.workBridgeDecision.WorkID)
		e.conversationState.RecordWorkLink(interaction.ConversationWorkLink{ThreadID: t.workBridgeDecision.ThreadID, WorkID: t.workBridgeDecision.WorkID, Relation: t.workBridgeDecision.Relation, SourceTurnID: e.activeConversationTurnID, Confidence: t.workBridgeDecision.Confidence, Reason: t.workBridgeDecision.Reason})
	}
	if e.interactionState != nil {
		e.interactionState.Observe(t.semanticReq, t.interactionWorkContext, t.interactionHasWork)
	}
	e.conversationTurnActive = e.activeConversationTurnID != 0
	return func() {
		e.conversationTurnActive = false
		e.activeConversationTurnID = 0
		if e.conversationHistory != nil && e.conversationEpisodeID != "" {
			snap := e.conversationState.Snapshot()
			var last interaction.TurnRecord
			if len(snap.Turns) > 0 {
				last = snap.Turns[len(snap.Turns)-1]
			}
			if last.ID != 0 {
				e.conversationHistory.RecordTurn(e.conversationEpisodeID, dialogue.HistoricalTurn{ID: last.ID, At: last.At, UserText: last.UserText, Assistant: last.Assistant, Kind: string(last.Kind), SpeechAct: string(last.SpeechAct), Language: last.Language, Topic: last.Topic, Objective: last.Objective, Relation: string(last.Relation), ParentTurnID: last.ParentTurnID})
				_ = e.conversationHistory.Save(dialogue.ConversationHistoryPath)
			}
		}
		_ = e.conversationState.Save(interaction.ConversationStatePath)
	}
}

func (e *Engine) evidencePhase(t *processTurn) {
	if e.longMem != nil {
		if remembered, ok := e.longMem.GetByID(shared.Normalize(t.input)); ok && remembered != nil && strings.TrimSpace(remembered.Action) != "" && remembered.Action != "desconocido" {
			e.pendingSituationObservations = append(e.pendingSituationObservations, situation.Observation{ID: fmt.Sprintf("memory-action-proposal-%d", time.Now().UnixNano()), Kind: "action_proposal", Source: "memory.LongMemory", Subject: t.input, Predicate: "proposes_action", Object: remembered.Action, Value: fmt.Sprintf("target=%s confidence=%.2f source=%s", remembered.Target, remembered.Confidence, remembered.Source), ObservedAt: time.Now(), Confidence: remembered.Confidence, Epistemic: situation.EpistemicDerived})
			if e.flowTrace != nil {
				e.flowTrace.Mark("MEMORY", fmt.Sprintf("action proposal queued: action=%s target=%s confidence=%.2f", remembered.Action, remembered.Target, remembered.Confidence))
			}
		}
	}
	t.historicalRequired = t.contextResolution.Historical.Required
	t.normInput = shared.Normalize(t.input)
	if shouldCollectSpecialistEvidence(t.semanticReq, t.historicalRequired) {
		t.specialistEvidence = e.collectSpecialistEvidence(t.semanticReq)
		if e.flowTrace != nil && !t.specialistEvidence.Empty() {
			e.flowTrace.Mark("SPECIALIST EVIDENCE", fmt.Sprintf("knowledge=%t reasoning=%t", t.specialistEvidence.Knowledge != nil, t.specialistEvidence.Reasoning != nil))
		}
	}
	t.responseSpecialistResults = e.collectResponseSpecialistResults(t.semanticReq, t.historicalRequired, t.contextResolution, t.previousTurn, t.hasPreviousTurn, t.input, t.normInput)
	if !t.specialistEvidence.Empty() && t.responseSpecialistResults.LongMemory != nil && t.responseSpecialistResults.Historical == nil && t.responseSpecialistResults.ConversationalMemory == nil && t.responseSpecialistResults.Knowledge == nil && t.responseSpecialistResults.Skill == nil && t.responseSpecialistResults.Social == nil && t.responseSpecialistResults.Deterministic == nil {
		t.responseSpecialistResults.LongMemory = nil
	}
}

// situationPhase creates the single current-turn situational representation.
func (e *Engine) situationPhase(t *processTurn) {
	pending := append([]situation.Observation(nil), e.pendingSituationObservations...)
	e.pendingSituationObservations = nil
	e.lastSituation, e.lastSituationContributions = e.buildSharedSituation(t.semanticReq, t.previousConversationSnapshot, t.interactionWorkContext, t.interactionHasWork, pending)
	if !t.specialistEvidence.Empty() {
		e.lastSituationContributions = append(e.lastSituationContributions, attachContributionsToSituation(e.lastSituation, t.specialistEvidence.Contribution(time.Now()))...)
	}
	responseContributions := t.responseSpecialistResults.Contributions(time.Now())
	if len(responseContributions) > 0 {
		e.lastSituationContributions = append(e.lastSituationContributions, attachContributionsToSituation(e.lastSituation, responseContributions...)...)
		if e.flowTrace != nil {
			for _, contribution := range responseContributions {
				e.flowTrace.Mark("SPECIALIST RESULT", fmt.Sprintf("source=%s", contribution.Perspective))
			}
		}
	}
	if e.flowTrace != nil {
		e.flowTrace.Mark("SITUATION", fmt.Sprintf("id=%s version=%d evidence=%d relations=%d uncertainty=%d conflicts=%d", e.lastSituation.ID, e.lastSituation.Version, len(e.lastSituation.Evidence), len(e.lastSituation.Relations), len(e.lastSituation.Uncertainty), len(e.lastSituation.Conflicts)))
		e.flowTrace.Mark("CONTRIBUTIONS", fmt.Sprintf("perspectives=%d", len(e.lastSituationContributions)))
	}
}

// cognitionPhase executes the formal Situation → Cognition → Dialogue stages.
func (e *Engine) cognitionPhase(t *processTurn) bool {
	iteration := situation.NewIterationContext("interactive_turn", e.lastSituation, e.lastSituationContributions)
	cognitiveContext := cognition.SituationalCognitionContext{Iteration: iteration, Situation: e.lastSituation, Contributions: e.lastSituationContributions, Work: t.interactionWorkContext, HasWork: t.interactionHasWork, AuthoritativeEvidence: e.authoritativeUserModelEvidence(t.semanticReq)}
	if e.conversationState != nil {
		e.lastConversationalCognition = cognition.RunConversationalCognitionE2E(t.semanticReq, e.conversationState.Snapshot(), cognitiveContext)
	} else {
		e.lastConversationalCognition = cognition.RunConversationalCognitionE2E(t.semanticReq, interaction.ConversationSnapshot{}, cognitiveContext)
	}
	e.lastDecisionEvidence = e.lastConversationalCognition.DecisionEvidence
	var responseHint interaction.ResponseIntentHint
	if e.lastConversationalCognition.SituationalResolution.Used {
		responseHint = interaction.ResponseIntentHint{Kind: string(e.lastConversationalCognition.SituationalResolution.Intent), Confidence: e.lastConversationalCognition.SituationalResolution.Confidence, Reason: e.lastConversationalCognition.SituationalResolution.Reason}
	}
	if responseHint.Kind != "" {
		e.lastResponseIntent = interaction.DetermineResponseIntent(t.semanticReq, t.previousConversationSnapshot, responseHint)
	} else {
		e.lastResponseIntent = interaction.DetermineResponseIntent(t.semanticReq, t.previousConversationSnapshot)
	}
	e.lastBehavioralDecision = cognition.ResolveConversationalBehavior(e.lastConversationalCognition.Action, e.lastConversationalCognition.Strategy, e.lastResponseIntent)
	t.cognitionRan = true
	if e.flowTrace != nil {
		e.flowTrace.Mark("FINAL BEHAVIOR", fmt.Sprintf("action=%s speak_current=%t additional_initiative=%s strategy=%s uncertainty=%t", e.lastBehavioralDecision.Action, e.lastBehavioralDecision.SpeakToCurrentTurn, e.lastBehavioralDecision.AdditionalInitiative, e.lastBehavioralDecision.Strategy.Mode, e.lastBehavioralDecision.Uncertainty))
	}
	if e.flowTrace != nil {
		if e.lastConversationalCognition.SituationalResolution.Used {
			e.flowTrace.Mark("SITUATION→COGNITION", fmt.Sprintf("resolved=%s confidence=%.2f perspective=%s", e.lastConversationalCognition.SituationalResolution.Intent, e.lastConversationalCognition.SituationalResolution.Confidence, e.lastConversationalCognition.SituationalResolution.Perspective))
		} else {
			e.flowTrace.Mark("SITUATION→COGNITION", "no situational override; structured cognition retained")
		}
		e.flowTrace.Mark("RESPONSE GATE", fmt.Sprintf("intent selected=%s confidence=%.2f", e.lastResponseIntent.Kind, e.lastResponseIntent.Confidence))
	}
	if !e.lastConversationalCognition.Valid {
		e.renderCanonicalSystemResponse(t.semanticReq, "cognition_invalid", "No pude completar de forma segura la interpretación de este turno.", interaction.MoveClarify, interaction.ResponseIntentClarify, true)
		return true
	}
	if e.flowTrace != nil {
		e.flowTrace.Mark("COGNITION", fmt.Sprintf("intent=%s goal=%s action=%s confidence=%.2f clarification=%t", e.lastConversationalCognition.Intent.Primary, e.lastConversationalCognition.Goal.Primary, e.lastConversationalCognition.Action.Primary, e.lastConversationalCognition.Model.Signals.Confidence, e.lastConversationalCognition.Interpretation.NeedsClarification))
	}
	return false
}

// dialoguePhase is the explicit B96.39 boundary. It consumes Cognition's
// structured interpretation and selects the conversational movement; it does
// not generate the final language.
func (e *Engine) dialoguePhase(t *processTurn) {
	if e.naturalDialogue != nil && e.conversationState != nil {
		e.lastNaturalDialogue = e.naturalDialogue.EvaluateRequest(t.semanticReq, t.previousConversationSnapshot, naturaldialogue.UserTurn)
	} else {
		nd := naturaldialogue.NewIntegration()
		snap := interaction.ConversationSnapshot{}
		if e.conversationState != nil {
			snap = e.conversationState.Snapshot()
		}
		e.lastNaturalDialogue = nd.EvaluateRequest(t.semanticReq, snap, naturaldialogue.UserTurn)
	}
	e.lastNaturalDialogue.Move = naturaldialogue.SelectMove(t.semanticReq, t.previousConversationSnapshot, e.lastResponseIntent, e.lastDecisionEvidence)
	if e.flowTrace != nil {
		e.flowTrace.Mark("DIALOGUE", fmt.Sprintf("move=%s requires_direct=%t allow_follow_up=%t", e.lastNaturalDialogue.Move, e.lastNaturalDialogue.RequiresDirectTurn, e.lastNaturalDialogue.AllowProactiveFollowUp))
	}
}

// decisionPhase is the single decision point for all routes.
// It builds the interactive proposal and runs the unified FocusArbiter.
func (e *Engine) decisionPhase(t *processTurn) bool {
	// Build interactive proposal from all decision sources
	interactiveProp := e.buildInteractiveProposal(t)

	// Run unified FocusArbiter with all proposals
	return e.runFocusArbitration(context.Background(), t, interactiveProp)
}

// buildInteractiveProposal constructs the interactive proposal from all decision sources.
// This encapsulates all the decision logic that was previously inline in decisionPhase.
func (e *Engine) buildInteractiveProposal(t *processTurn) *ProposalContract {
	now := time.Now()
	sit := e.lastSituation

	var evidence []ProposalEvidence
	for _, f := range e.lastDecisionEvidence.Findings {
		evidence = append(evidence, ProposalEvidence{
			Kind:       f.Kind,
			Subject:    f.Subject,
			Predicate:  f.RelationType,
			Object:     f.Target,
			Confidence: f.Confidence,
			Epistemic:  f.Epistemic,
		})
	}

	// Build the proposal with all decision evidence
	prop := &ProposalContract{
		ID:                 "interactive-" + now.UTC().Format("20060102T150405.000000000"),
		Kind:               ProposalKindInteractive,
		CreatedAt:          now,
		Intention:          e.determineIntention(t),
		IntentionDetail:    e.determineIntentionDetail(t),
		Action:             e.determineProposedAction(t),
		Evidence:           evidence,
		Priority:           e.determinePriority(t),
		Urgency:            e.determineUrgency(t),
		InterruptionCost:   e.computeInterruptionCost(t),
		RequiresDirectTurn: e.lastNaturalDialogue.RequiresDirectTurn,
		Privacy:            initiative.InformationAllowed,
		Channel:            t.semanticReq.Channel,
		Provenance: initiative.Provenance{
			Component:  "turn_pipeline.decisionPhase",
			Subsystem:  "interactive",
			SignalType: "user_turn",
			OriginID:   sit.ID,
			CreatedAt:  now,
		},
	}

	// Store the decision context for execution if this proposal wins
	e.pendingInteractiveDecision = &interactiveDecisionContext{
		turn: t,
		prop: prop,
	}

	return prop
}

// interactiveDecisionContext holds the context needed to execute an interactive decision
type interactiveDecisionContext struct {
	turn *processTurn
	prop *ProposalContract
}

// pendingInteractiveDecision holds the interactive decision context for execution
var pendingInteractiveDecision *interactiveDecisionContext

// Helper functions for building interactive proposal

func (e *Engine) determineIntention(t *processTurn) string {
	// Determine the primary intention from cognition
	if e.lastConversationalCognition.Action.Primary != "" {
		return string(e.lastConversationalCognition.Action.Primary)
	}
	if e.lastConversationalCognition.Goal.Primary != "" {
		return string(e.lastConversationalCognition.Goal.Primary)
	}
	return "respond"
}

func (e *Engine) determineIntentionDetail(t *processTurn) string {
	return e.lastConversationalCognition.Goal.Objective
}

func (e *Engine) determineProposedAction(t *processTurn) *ProposedAction {
	action := e.lastConversationalCognition.Action.Primary
	if action == "" {
		return nil
	}
	return &ProposedAction{
		Name:   string(action),
		Target: e.lastConversationalCognition.Goal.Objective,
	}
}

func (e *Engine) determinePriority(t *processTurn) ProposalPriority {
	conf := e.lastConversationalCognition.Model.Signals.Confidence
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

func (e *Engine) determineUrgency(t *processTurn) ProposalUrgency {
	snap := e.conversationState.Snapshot()
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

func (e *Engine) computeInterruptionCost(t *processTurn) InterruptionCost {
	snap := e.conversationState.Snapshot()
	cost := InterruptionCost{}

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

	if snap.UserState.Engagement > 0.8 {
		cost.InFocus = true
		cost.BaseCost += 0.3
	}

	if len(snap.OpenThreads) > 3 {
		cost.ConversationLoad = ConversationLoadHigh
		cost.BaseCost += 0.2
	} else if len(snap.OpenThreads) > 1 {
		cost.ConversationLoad = ConversationLoadMed
		cost.BaseCost += 0.1
	} else {
		cost.ConversationLoad = ConversationLoadLow
	}

	if cost.BaseCost > 1.0 {
		cost.BaseCost = 1.0
	}
	cost.Total = cost.BaseCost
	return cost
}

func (e *Engine) runSelfAndUserModelDecisions(t *processTurn) bool {
	if e.lastConversationalCognition.Intent.Primary == cognition.IntentQuestion && e.lastConversationalCognition.Goal.Primary == cognition.GoalGetInformation && e.userModelStore != nil {
		if e.flowTrace != nil {
			e.flowTrace.Mark("USER MODEL", fmt.Sprintf("query gate intent=%s goal=%s subject=%s", e.lastConversationalCognition.Intent.Primary, e.lastConversationalCognition.Goal.Primary, t.semanticReq.Subject))
		}
		if query, ok := interaction.DetectUserModelQuery(t.semanticReq); ok {
			if e.flowTrace != nil {
				e.flowTrace.Mark("USER MODEL", fmt.Sprintf("query match domain=%s topics=%v", query.Domain, query.Topics))
			}
			contract := interaction.NewResponseContract(t.semanticReq, e.lastResponseIntent, interaction.MoveAnswer)
			contract.Source = "user_model"
			ctx := interaction.UserModelQueryContext{Topics: query.Topics}
			found := false
			add := func(text string, confidence float64) {
				if strings.TrimSpace(text) != "" {
					contract.AddEvidence(text, "user_model", true, confidence)
					found = true
				}
			}
			switch query.Domain {
			case string(interaction.UserModelDomainAttribute):
				if attributes, err := interaction.GetRelevantAttributes(e.userModelStore, interaction.UserModelActorCognition, ctx); err == nil {
					if len(attributes) == 0 {
						// Sin dato: respuesta honesta + reconfirmación, que abre la
						// única admisión posible para la respuesta del usuario.
						if len(query.Topics) > 0 {
							e.registerOpenPersonalQuestion("attribute", query.Topics[0])
						}
						add(personalDataAskBack, .4)
						contract.RenderScene = fase2AskBackScene(firstQueryTopic(query.Topics))
					} else if len(attributes) == 1 {
						add("Tu "+attributes[0].Key+" es "+attributes[0].Value+".", attributes[0].Confidence)
						contract.RenderScene = fase2ValueScene(attributes[0].Key, attributes[0].Value)
					} else {
						values := make([]string, 0, len(attributes))
						for _, attribute := range attributes {
							values = append(values, attribute.Key+"="+attribute.Value)
						}
						add("Tengo más de un registro posible para ese dato: "+strings.Join(values, ", ")+". No quiero elegir uno sin certeza.", .5)
					}
				}
			case string(interaction.UserModelDomainPreference):
				if prefs, err := interaction.GetRelevantPreferences(e.userModelStore, interaction.UserModelActorCognition, ctx); err == nil {
					if len(prefs) == 0 {
						// Sin dato: respuesta honesta + reconfirmación, que abre la
						// única admisión posible para la respuesta del usuario.
						if len(query.Topics) > 0 {
							e.registerOpenPersonalQuestion("preference", query.Topics[0])
						}
						add(personalDataAskBack, .4)
						contract.RenderScene = fase2AskBackScene(firstQueryTopic(query.Topics))
					} else if len(prefs) == 1 {
						add("Tu "+prefs[0].Key+" es "+prefs[0].Value+".", prefs[0].Confidence)
						contract.RenderScene = fase2ValueScene(prefs[0].Key, prefs[0].Value)
					} else {
						values := make([]string, 0, len(prefs))
						for _, pref := range prefs {
							values = append(values, pref.Key+"="+pref.Value)
						}
						add("Tengo más de un registro posible para esa preferencia: "+strings.Join(values, ", ")+". No quiero elegir uno sin certeza.", .5)
					}
				}
			case string(interaction.UserModelDomainGoal):
				if goals, err := interaction.GetRelevantGoals(e.userModelStore, interaction.UserModelActorCognition, ctx); err == nil {
					if len(goals) == 0 {
						add("Todavía no tengo ese objetivo modelado.", .4)
					} else {
						values := make([]string, 0, len(goals))
						for _, goal := range goals {
							values = append(values, goal.Key+"="+goal.Value)
						}
						add("Tengo estos objetivos modelados: "+strings.Join(values, ", ")+".", .85)
					}
				}
			case string(interaction.UserModelDomainKnowledge):
				if len(query.Topics) > 0 {
					if items, err := interaction.GetRelevantUserKnowledge(e.userModelStore, interaction.UserModelActorCognition, query.Topics[0]); err == nil {
						if len(items) == 0 {
							add("No tengo conocimiento del usuario modelado sobre "+query.Topics[0]+".", .4)
						} else if len(items) == 1 {
							add("Sobre "+items[0].Key+", tengo modelado que tu nivel de conocimiento es "+items[0].Value+".", items[0].Confidence)
						}
					}
				}
			case string(interaction.UserModelDomainCapability):
				if len(query.Topics) > 0 {
					if level, exists, contested, err := interaction.GetCapabilityLevel(e.userModelStore, interaction.UserModelActorCognition, query.Topics[0]); err == nil {
						if !exists {
							add("No tengo un nivel de capacidad modelado para "+query.Topics[0]+".", .4)
						} else if contested {
							add("Tengo información contradictoria sobre tu nivel en "+level.Key+"; no quiero tratarla como definitiva.", .5)
						} else {
							add("Tu nivel modelado en "+level.Key+" es "+level.Value+".", level.Confidence)
						}
					}
				}
			case string(interaction.UserModelDomainCommunicationProfile):
				if profile, err := interaction.GetCommunicationProfile(e.userModelStore, interaction.UserModelActorCognition, ctx); err == nil {
					if len(profile) == 0 {
						add("Todavía no tengo un perfil de comunicación suficientemente establecido.", .4)
					} else {
						keys := make([]string, 0, len(profile))
						for key, value := range profile {
							keys = append(keys, key+"="+value.Value)
						}
						sort.Strings(keys)
						add("Mi perfil de comunicación contigo tiene estas preferencias: "+strings.Join(keys, ", ")+".", .9)
					}
				}
			}
			if found && e.renderResponseContract(t.semanticReq, contract, e.buildConversationalRuntimeFacts(e.conversationState.Snapshot())) {
				return true
			}
		} else if e.flowTrace != nil {
			e.flowTrace.Mark("USER MODEL", "query gate rejected: no explicit user ownership/domain match")
		}
	}
	return false
}

// resumePendingAuthorizationIfConfirmed is the generic continuation bridge for
// an execution that the canonical Work owner persisted as awaiting explicit
// authorization. It consumes a communicative confirmation, not a phrase list,
// and re-enters the same Execution Authority with the original operation,
// WorkSession and attempt. It never resolves or executes a provider itself.
// pendingExecutionComplete reports whether a persisted PendingExecution
// carries everything the authorization continuation bridge needs to resume
// it. This deliberately does NOT require pending.Request.Action.Name: a
// production-intent request is fully executable via its concrete capability
// contract without populating that optional linguistic field.
func pendingExecutionComplete(pending production.PendingExecution) bool {
	return pending.AttemptNumber >= 1 &&
		strings.TrimSpace(pending.CapabilityID) != "" &&
		strings.TrimSpace(pending.Operation) != "" &&
		pending.RouteKind == string(capability.IntentProduction) &&
		strings.TrimSpace(pending.ProviderID) != "" &&
		strings.TrimSpace(pending.Request.Raw) != ""
}

// pendingExecutionStillCoherent reports whether the pending request still
// resolves as an executable interaction request at all. It intentionally does
// not compare the generic interaction-layer operation against the concrete
// capability operation persisted by Production.
func pendingExecutionStillCoherent(decision interaction.CapabilityInteractionDecision) bool {
	return decision.Executable
}

func (e *Engine) resumePendingAuthorizationIfConfirmed(t *processTurn) bool {
	if e == nil || t == nil || e.workStore == nil || e.productionRuntime == nil {
		return false
	}
	acts := t.authorizationActs
	if acts.Primary == "" {
		acts = interaction.CanonicalActs(t.semanticReq)
	}

	// ConversationState owns the current conversational WorkSession. WorkStore.Last()
	// is historical evidence only and must never consume an authorization reply
	// for whichever work happened to be persisted most recently.
	work, pending, ok := e.currentPendingAuthorization()
	if !ok {
		return false
	}
	// Keep the voice turn boundary open while this exact work awaits the user.
	if e.sess != nil {
		e.sess.SetVoiceContinuation()
	}
	if !pendingExecutionComplete(pending) {
		if e.flowTrace != nil {
			e.flowTrace.Mark("AUTHORIZATION CONTINUATION", "rejected: pending execution contract incomplete; turn consumed fail-closed")
		}
		e.renderCanonicalSystemResponse(t.semanticReq, "authorization_contract_invalid", "Encontré una autorización pendiente, pero su contrato de ejecución no conserva toda la identidad necesaria para reanudarla con seguridad. No ejecutaré otra operación ni inferiré una sustituta. Se requiere reconciliar ese trabajo.", interaction.MoveClarify, interaction.ResponseIntentClarify, true)
		return true
	}
	if acts.Has(interaction.ActAuthorizationInquiry) {
		e.renderCanonicalSystemResponse(
			t.semanticReq,
			"authorization_pending_inquiry",
			fmt.Sprintf("La operación %s sigue pendiente; preguntar o plantear una condición no la autoriza. Si quieres que continúe, dímelo explícitamente.", pending.CapabilityID),
			interaction.MoveClarify,
			interaction.ResponseIntentClarify,
			true,
		)
		return true
	}

	// A pending authorization is itself an operational continuation boundary.
	// A transport-fragmented follow-up must not fall through into historical
	// memory, Situation/Cognition or a new Work owner merely because it is not
	// an explicit "yes". It may extend the same pending objective, but it can
	// never grant authorization implicitly.
	if !acts.Has(interaction.ActConfirmation) && !acts.Has(interaction.ActRejection) {
		if t.workBridgeDecision.Relation == interaction.WorkRelationContinue ||
			t.workBridgeDecision.Relation == interaction.WorkRelationResume ||
			t.semanticReq.Continues || t.semanticReq.FollowUp || t.semanticReq.AddressesCurrent {
			addition := strings.TrimSpace(t.semanticReq.Objective.Text)
			if addition == "" {
				addition = strings.TrimSpace(t.input)
			}
			mergedRaw := interaction.ComposeObjective(pending.Request.Raw, addition)
			mergedObjective := interaction.ComposeObjective(pending.Request.Objective.Text, addition)
			if mergedRaw != "" {
				pending.Request.Raw = mergedRaw
				pending.Request.Normalized = shared.Normalize(mergedRaw)
			}
			if mergedObjective != "" {
				pending.Request.Objective.Text = mergedObjective
			}
			if err := e.workStore.SetPendingExecution(work.ID, pending); err != nil {
				if e.flowTrace != nil {
					e.flowTrace.Mark("AUTHORIZATION CONTINUATION", "failed to persist continued pending objective: "+err.Error())
				}
				return false
			}
			t.semanticReq = pending.Request
			if e.flowTrace != nil {
				e.flowTrace.Mark("AUTHORIZATION CONTINUATION", fmt.Sprintf("continued without authorization: work=%s attempt=%d capability=%s", work.ID, pending.AttemptNumber, pending.CapabilityID))
			}
			e.renderCanonicalSystemResponse(
				t.semanticReq,
				"authorization_pending_continuation",
				fmt.Sprintf("La tarea sigue pendiente de autorización para %s. La mantengo en el mismo trabajo y no la ejecutaré hasta recibir una autorización explícita.", pending.CapabilityID),
				interaction.MoveClarify,
				interaction.ResponseIntentClarify,
				true,
			)
			return true
		}
		return false
	}

	if acts.Has(interaction.ActRejection) {
		_ = e.workStore.RejectPendingExecution(work.ID, "usuario rechazó la autorización pendiente")
		if e.sess != nil {
			e.sess.RestoreVoiceMode()
		}
		if e.flowTrace != nil {
			e.flowTrace.Mark("AUTHORIZATION CONTINUATION", fmt.Sprintf("denied: work=%s attempt=%d capability=%s", work.ID, pending.AttemptNumber, pending.CapabilityID))
		}
		e.renderCanonicalSystemResponse(t.semanticReq, "authorization_denied", "De acuerdo, no ejecutaré esa operación.", interaction.MoveAcknowledge, interaction.ResponseIntentAcknowledge, false)
		return true
	}

	decision := interaction.ResolveCapabilityInteraction(pending.Request)
	if !pendingExecutionStillCoherent(decision) {
		if e.flowTrace != nil {
			e.flowTrace.Mark("AUTHORIZATION CONTINUATION", "rejected: pending request no longer resolves coherently; turn consumed fail-closed")
		}
		e.renderCanonicalSystemResponse(t.semanticReq, "authorization_contract_incoherent", "La operación pendiente ya no se puede reconstruir de forma coherente. No la sustituiré por otra acción. El trabajo queda detenido para reconciliación.", interaction.MoveClarify, interaction.ResponseIntentClarify, true)
		return true
	}
	if e.flowTrace != nil {
		e.flowTrace.Mark("AUTHORIZATION CONTINUATION", fmt.Sprintf("approved: work=%s attempt=%d capability=%s", work.ID, pending.AttemptNumber, pending.CapabilityID))
	}
	if e.sess != nil {
		e.sess.RestoreVoiceMode()
	}
	t.semanticReq = pending.Request
	authority := e.authorityExecuteWithOptions(decision, production.ExecutionOptions{
		Approval:                   autonomy.ApprovalGranted,
		RecoveryAttempt:            1,
		WorkID:                     work.ID,
		TaskID:                     pending.TaskID,
		AttemptNumber:              pending.AttemptNumber,
		ResumePendingAuthorization: true,
		ExpectedCapabilityID:       capability.ID(pending.CapabilityID),
		ExpectedOperation:          pending.Operation,
		ExpectedProviderID:         pending.ProviderID,
		ExpectedRouteKind:          capability.IntentKind(pending.RouteKind),
	}, pending.Request)
	if !authority.Handled {
		if e.flowTrace != nil {
			e.flowTrace.Mark("AUTHORIZATION CONTINUATION", "resume rejected by canonical authority; turn consumed fail-closed")
		}
		e.renderCanonicalSystemResponse(t.semanticReq, "authorization_resume_rejected", "No pude reanudar exactamente la operación autorizada. No ejecuté una alternativa. El trabajo queda detenido para revisión.", interaction.MoveClarify, interaction.ResponseIntentClarify, true)
	}
	return true
}

// isCancelWorkRequest is deliberately narrow: it only treats an explicit
// stop/pause about the current work as a background cancellation. A plain
// "salir" or an unrelated stop keeps its normal meaning.
func isCancelWorkRequest(req interaction.SemanticRequest) bool {
	if req.Kind == interaction.KindQuestion || req.Kind == interaction.KindSearch {
		return false
	}
	normalized := strings.ToLower(strings.TrimSpace(req.Normalized))
	if normalized == "" {
		normalized = strings.ToLower(strings.TrimSpace(req.Raw))
	}
	if normalized == "" {
		return false
	}
	verb := false
	for _, marker := range []string{"cancela", "cancel", "deten", "detén", "aborta", "para el trabajo", "para la tarea"} {
		if strings.Contains(normalized, marker) {
			verb = true
			break
		}
	}
	if !verb {
		return false
	}
	if req.Kind == interaction.KindStop || req.Kind == interaction.KindPause {
		return true
	}
	for _, object := range []string{"trabajo", "tarea", "job", "eso", "proyecto", "segundo plano", "background"} {
		if strings.Contains(normalized, object) {
			return true
		}
	}
	return false
}

func (e *Engine) cancelBackgroundWorkPhase(t *processTurn) bool {
	if e == nil || t == nil || !isCancelWorkRequest(t.semanticReq) {
		return false
	}
	activeWork := ""
	if e.conversationState != nil {
		activeWork = strings.TrimSpace(e.conversationState.ActiveWork())
	}
	canceled := 0
	for _, job := range e.BackgroundJobs() {
		if job.Status != BackgroundJobRunning {
			continue
		}
		if activeWork != "" && job.WorkID != "" && job.WorkID != activeWork {
			continue
		}
		if e.backgroundJobs.cancel(job.ID, true) {
			e.blockCanceledWork(job.WorkID)
			canceled++
		}
	}
	if canceled == 0 {
		return false
	}
	e.renderCanonicalSystemResponse(
		t.semanticReq,
		"background_canceled",
		"Cancelé el trabajo en segundo plano. No aplicaré un resultado tardío; queda bloqueado para reconciliar.",
		interaction.MoveAcknowledge,
		interaction.ResponseIntentAcknowledge,
		false,
	)
	return true
}

func (e *Engine) productionJobInProgress(req interaction.SemanticRequest) bool {
	if req.Kind != interaction.KindProduction && !strings.EqualFold(strings.TrimSpace(req.Action.Name), "produce") {
		return false
	}
	activeWork := ""
	if e.conversationState != nil {
		activeWork = strings.TrimSpace(e.conversationState.ActiveWork())
	}
	for _, job := range e.BackgroundJobs() {
		if job.Status != BackgroundJobRunning {
			continue
		}
		if activeWork != "" && job.WorkID != "" && job.WorkID != activeWork {
			continue
		}
		return true
	}
	return false
}

func (e *Engine) preExecutionPhase(t *processTurn) bool {
	if e.flowTrace != nil {
		e.flowTrace.Mark("RUNTIME", fmt.Sprintf("action=%s", t.semanticReq.Action.Name))
	}
	// A one-sentence delegation preamble is not an execution-ready objective.
	// The channel may still be delivering the rest of the specification. Hold
	// the turn before capability resolution so words such as "implementa" cannot
	// trigger an authorization request or physical action prematurely.
	if e.conversationState != nil {
		if draft, ok := e.conversationState.PendingObjectiveDraft(); ok {
			// A pending voice draft is not execution-ready merely because a
			// fragment looks grammatically complete. The voice adapter is the
			// authority for turn boundaries; if it had to fragment one human
			// intervention, keep assembling until an explicit handoff is given.
			// This prevents "create X" from executing while the user is still
			// dictating conditions, tests, or verification requirements.
			if draft.Fragments < 2 || strings.TrimSpace(t.semanticReq.Raw) != strings.TrimSpace(draft.Text) || interaction.NeedsObjectiveAssembly(t.semanticReq) ||
				(t.semanticReq.Channel == interaction.ChannelVoice && !interaction.ShouldFinalizeObjectiveDraft(t.semanticReq)) {
				if e.sess != nil {
					e.sess.SetVoiceContinuation()
				}
				if e.flowTrace != nil {
					mode := "awaiting substantive specification"
					if t.semanticReq.Channel == interaction.ChannelVoice && !interaction.ShouldFinalizeObjectiveDraft(t.semanticReq) {
						mode = "awaiting voice turn completion"
					}
					e.flowTrace.Mark("OBJECTIVE DRAFT", fmt.Sprintf("%s fragments=%d", mode, draft.Fragments))
				}
				return true
			}
		}
	}
	if interaction.NeedsObjectiveAssembly(t.semanticReq) {
		if e.conversationState != nil {
			draft := e.conversationState.BeginObjectiveDraft(t.semanticReq.Objective.Text, e.activeConversationTurnID)
			if e.sess != nil {
				// The semantic owner has now established that the human
				// intervention remains open. Expose that state to the voice
				// channel so subsequent fragments do not require a repeated
				// wake word. The Session remains a channel-state adapter; the
				// draft itself stays owned by ConversationState.
				e.sess.SetVoiceContinuation()
			}
			if e.flowTrace != nil && draft != nil {
				e.flowTrace.Mark("OBJECTIVE DRAFT", fmt.Sprintf("pending id=%s chars=%d voice=direct", draft.ID, len([]rune(draft.Text))))
			}
		}
		if e.flowTrace != nil {
			e.flowTrace.Mark("OBJECTIVE ASSEMBLY", "deferred: delegation preamble is awaiting specification")
		}
		return true
	}
	capabilityDecision := interaction.ResolveCapabilityInteraction(t.semanticReq)
	if e.flowTrace != nil {
		acts := interaction.CanonicalActs(t.semanticReq)
		e.flowTrace.Mark("CAPABILITY", fmt.Sprintf("kind=%s act=%s action=%s operation=%s executable=%t reason=%s", t.semanticReq.Kind, acts.Primary, t.semanticReq.Action.Name, capabilityDecision.Request.Operation, capabilityDecision.Executable, capabilityDecision.Reason))
	}
	if capabilityDecision.Executable && e.productionJobInProgress(t.semanticReq) {
		e.renderCanonicalSystemResponse(
			t.semanticReq,
			"background_job_in_progress",
			"Hay un trabajo de programación en segundo plano en curso. Si quieres cambiar el objetivo, primero cancélalo; si no, espera a que termine.",
			interaction.MoveClarify,
			interaction.ResponseIntentClarify,
			true,
		)
		return true
	}
	if capabilityDecision.Executable {
		if !capabilityHandoffReady(e.lastConversationalCognition) {
			if e.flowTrace != nil {
				e.flowTrace.Mark("COGNITION→RUNTIME", fmt.Sprintf("capability handoff blocked: action=%s execution_intent=%t blocked=%t needs_clarification=%t uncertainty_blocked=%t conflict_blocked=%t", e.lastConversationalCognition.Action.Primary, e.lastConversationalCognition.Action.ExecutionIntent, e.lastConversationalCognition.Action.Blocked, e.lastConversationalCognition.Action.NeedsClarification, e.lastConversationalCognition.Uncertainty.BlocksAction, e.lastConversationalCognition.Conflict.BlocksAction))
			}
			question := "Necesito aclarar la acción antes de ejecutarla."
			if len(t.semanticReq.References) > 0 {
				question = "¿A qué te refieres exactamente?"
			}
			if e.flowTrace != nil {
				e.flowTrace.Mark("RESPONSE GATE", "execution blocked by cognition/runtime coherence contract")
			}
			e.renderCanonicalSystemResponse(t.semanticReq, "execution_gate", question, interaction.MoveClarify, interaction.ResponseIntentClarify, true)
			return true
		}
		if e.flowTrace != nil {
			e.flowTrace.Mark("COGNITION→RUNTIME", fmt.Sprintf("capability handoff ready: action=%s operation=%s", e.lastConversationalCognition.Action.Primary, capabilityDecision.Request.Operation))
		}
		// Deferred linguistic interpretation is optional evidence. Once this
		// turn has crossed the canonical execution gate, any still-running
		// enrichment must yield the local model to the operational request.
		// The resolver remains alive for future turns; only this obsolete
		// background inference is cancelled.
		if e.deferredLinguistic != nil {
			e.deferredLinguistic.CancelOptionalInference()
			if e.flowTrace != nil {
				e.flowTrace.Mark("LINGUISTIC", "optional deferred inference cancelled for executable work")
			}
		}
		authority := e.authorityExecuteWithOptions(capabilityDecision, production.ExecutionOptions{
			Approval: autonomy.ApprovalNone, RecoveryAttempt: 1, Async: e.asyncProduction,
		}, t.semanticReq)
		if authority.Handled && authority.BackgroundJobID != "" {
			e.renderCanonicalSystemResponse(
				t.semanticReq,
				"production_async_started",
				"Lo estoy trabajando en segundo plano. Te aviso cuando termine.",
				interaction.MoveAcknowledge,
				interaction.ResponseIntentAcknowledge,
				false,
			)
		}
		// Once the completed objective crosses the canonical execution
		// authority, the conversational draft has served its purpose. Work is
		// now the durable operational owner; leaving the draft alive would make
		// later unrelated turns appear to belong to the old assembly.
		if e.conversationState != nil {
			e.conversationState.CommitObjectiveDraft()
		}
		if e.sess != nil {
			e.sess.RestoreVoiceMode()
		}
		return true
	}
	if e.productionRuntime != nil && t.semanticReq.Raw != "" {
		activeWorkID := ""
		if e.conversationState != nil {
			activeWorkID = e.conversationState.ActiveWork()
		}
		if query, handled := e.productionRuntime.QueryWork(t.semanticReq, activeWorkID); handled {
			if e.renderProductionWorkQuery(t.semanticReq, query) {
				return true
			}
		}
		// Production is deliberately not executed here. The single Engine
		// execution authority resolves the production route and crosses the
		// concrete ProductionRuntime adapter. Keeping this phase proposal-only
		// prevents a second physical execution door from appearing beside
		// authorityExecute.
	}
	if e.interactionManager != nil && e.interactionManager.Last().CommunicativeActs.Has(interaction.ActQuestion) {
		if e.renderSpecialistEvidence(t.semanticReq, t.specialistEvidence) {
			return true
		}
	}

	return false
}

func (e *Engine) finishExecutionPhase(t *processTurn) {

	if e.sess.IsExpired() {
		fmt.Println("Estado cancelado por timeout")
		e.sess.Reset()
	}

	if e.flowTrace != nil {
		e.flowTrace.Mark("EXECUTION", "no terminal capability handled; entering semantic action compatibility boundary")
	}
	e.pipeline.Enter(cognition.StageComprehend, t.semanticReq.Normalized)
	e.internalState.Enter(cognition.StateThinking, "usando significado semántico")
	semanticAction := t.semanticReq.Action
	semanticActionResolved := semanticAction.Name != "" && semanticAction.Confidence >= 0.6
	semanticConfidence := t.semanticReq.Confidence
	mode := e.conversationMode.Compute(cognition.ConversationModeContext{Input: t.normInput, ActionResolved: semanticActionResolved, WaitingForLearning: e.sess.WaitingForLearning(), CurrentTopic: t.semanticReq.Topic})
	// Paso 3.4: el modo persistente del propietario sesga una copia del modo
	// del turno. El detector de cognition no se reescribe.
	mode = e.applyPersistentModeToConversationMode(mode)
	e.responseComposer.SetMode(mode)
	if t.semanticReq.Topic != "" {
		e.activateTopic(t.semanticReq.Topic)
	}
	if mode != nil {
		switch {
		case mode.DetailLevel < 0.4:
			e.contextAdapter.PreferredLength = "brief"
		case mode.DetailLevel > 0.65:
			e.contextAdapter.PreferredLength = "comprehensive"
		default:
			e.contextAdapter.PreferredLength = "detailed"
		}
	}
	e.applyConversationalAdaptation()
	if semanticAction.Name == "salir" {
		e.renderCanonicalSystemResponse(t.semanticReq, "goodbye", personality.Goodbye(), interaction.MoveAcknowledge, interaction.ResponseIntentAcknowledge, false)
		e.Shutdown()
		// H7 (2026-10-17): matar el proceso no es competencia de la
		// biblioteca; se pide el cierre y la raíz viva lo ejecuta
		// (main.go observa ExitRequested tras cada turno).
		e.RequestExit()
		return
	}
	if t.semanticReq.CommunicativeActs.Has(interaction.ActQuestion) || t.semanticReq.CommunicativeActs.Has(interaction.ActOpinionRequest) {
		if e.renderSpecialistEvidence(t.semanticReq, t.specialistEvidence) {
			return
		}
	}
	if t.semanticReq.CommunicativeActs.Has(interaction.ActFeedback) {
		e.context.Add(t.input, "feedback", "")
		// Fase 2 (escena feedback_recibido): superficie fija y SIN arrastrar
		// el tema previo — el eco de tema muere en el renderizador.
		e.renderCanonicalSceneResponse(t.semanticReq, "feedback_acknowledgement",
			interaction.RenderScene{Scene: string(linguistics.MoveFeedbackRecibido)},
			"Gracias, lo tendré en cuenta",
			interaction.MoveAcknowledge, interaction.ResponseIntentAcknowledge, false)
		return
	}
	if t.semanticReq.CommunicativeActs.Has(interaction.ActLearning) {
		if e.renderSpecialistEvidence(t.semanticReq, t.specialistEvidence) {
			return
		}
	}
	if semanticConfidence < 0.6 {
		switch {
		case t.semanticReq.CommunicativeActs.Has(interaction.ActCorrection):
			e.sess.Reset()
			e.renderCanonicalSystemResponse(t.semanticReq, "correction_acknowledgement", "Entendido", interaction.MoveAcknowledge, interaction.ResponseIntentAcknowledge, false)
			return
		default:
			prompt := e.learner.Ask(t.input)
			e.sess.SetLearning(t.input)
			if e.renderCanonicalTextResult(t.semanticReq, "learner_prompt", prompt, 0.9, interaction.MoveAsk) {
				return
			}
			return
		}
	}
	if semanticAction.Name == "" {
		// Fase 2 (escena no_entendido): la entrada no dejó acto tipado; la
		// superficie canónica del renderizador reemplaza la frase anterior.
		e.renderCanonicalSceneResponse(t.semanticReq, "unresolved_action",
			interaction.RenderScene{Scene: string(linguistics.MoveNoEntendido)},
			"No supe qué hacer con eso.",
			interaction.MoveClarify, interaction.ResponseIntentClarify, true)
		return
	}
	// There is intentionally no executable fallback here. If an executable
	// request reached this phase, it failed the canonical capability/authority
	// path and must fail closed rather than inventing Executable=true.
	if e.flowTrace != nil {
		e.flowTrace.Mark("EXECUTION", "terminal fallback refused: no canonical executable decision")
	}
	e.renderCanonicalSystemResponse(t.semanticReq, "invalid_executable_decision", "No pude establecer una decisión ejecutable válida para esa solicitud.", interaction.MoveClarify, interaction.ResponseIntentClarify, true)
	return
}

// executionLifecyclePhase installs the late-turn lifecycle bookkeeping that used
// to be embedded inside Process. Callers defer the returned cleanups.
func (e *Engine) executionLifecyclePhase() (func(), func()) {
	autoCleanup := func() {}
	turnCleanup := func() {}
	e.autonomousState.RecordInteraction()
	if e.autonomousState.Execution() == autonomy.ExecIdle {
		_ = e.autonomousState.TransitionTo(autonomy.ExecThinking)
	}
	autoCleanup = func() {
		switch e.autonomousState.Execution() {
		case autonomy.ExecThinking, autonomy.ExecCompleted, autonomy.ExecBlocked:
			_ = e.autonomousState.TransitionTo(autonomy.ExecIdle)
		}
	}
	e.conversationManager.SetTurnState(autonomy.TurnUserSpeaking)
	turnCleanup = func() { e.conversationManager.SetTurnState(autonomy.TurnAhmeliaIdle) }
	return autoCleanup, turnCleanup
}
