package initiative

import (
	"fmt"
	"strings"
	"time"
)

// EnrichSignals converts upstream projections into auditable initiative
// evidence. It is deliberately observational: it never returns a Decision.
func EnrichSignals(in SignalInput, now time.Time) []InitiativeSignal {
	if now.IsZero() {
		now = time.Now()
	}
	if in.Now.IsZero() {
		in.Now = now
	}
	e := in.Enrichment
	var out []InitiativeSignal
	add := func(source Source, reason Reason, typ, value, target string, confidence float64, observed time.Time, ttl time.Duration) {
		s := makeSignal(in, source, reason, typ, value, target, true, confidence)
		s.ObservedAt = sourceObservedAt(observed, in.Now)
		s.Provenance.Component = "initiative.B96.37.5"
		s.Provenance.Subsystem = "B96.37.5"
		s.Provenance.OriginID = fmt.Sprintf("%s:%s", source, typ)
		if ttl > 0 {
			s = WithExpiry(s, ttl)
		}
		out = append(out, s)
	}

	c := e.ConversationalContext
	if c.HasContext {
		target := strings.TrimSpace(c.Topic)
		if target == "" {
			target = strings.TrimSpace(c.Objective)
		}
		add(SourceConversationalContext, ReasonConversationContinue, "enriched_active_context", target, target, .90, c.ObservedAt, 10*time.Minute)
	}
	if c.OpenThreads > 0 {
		add(SourceInteractionContinuity, ReasonPausedThreadAvailable, "enriched_open_threads", fmt.Sprintf("%d", c.OpenThreads), c.ThreadID, .88, c.ObservedAt, 10*time.Minute)
	}
	if c.BreaksContinuity {
		add(SourceInteractionContinuity, ReasonTopicRedirection, "context_boundary", "breaks_continuity", c.Topic, .95, c.ObservedAt, 2*time.Minute)
	}

	cg := e.Cognition
	if cg.Valid {
		if cg.NeedsClarification {
			add(SourceInteractionContinuity, ReasonClarificationNeeded, "cognition_clarification", "required", cg.Goal, cg.IntentConfidence, cg.ObservedAt, 2*time.Minute)
		}
		if cg.Relevance > 0 {
			add(SourceConversationalContext, ReasonRelevantComment, "cognition_relevance", fmt.Sprintf("%.3f", clamp(cg.Relevance)), cg.Goal, clamp(cg.IntentConfidence), cg.ObservedAt, 2*time.Minute)
		}
	}

	um := e.UserModel
	if um.Available {
		if um.ReliableGoals > 0 {
			add(SourceUserModel, ReasonNextStepAvailable, "user_model_reliable_goals", fmt.Sprintf("%d", um.ReliableGoals), c.Topic, .80, um.ObservedAt, 15*time.Minute)
		}
		if um.ReliablePreferences > 0 {
			add(SourceUserModel, ReasonRelevantDiscovery, "user_model_reliable_preferences", fmt.Sprintf("%d", um.ReliablePreferences), c.Topic, .78, um.ObservedAt, 15*time.Minute)
		}
		if um.CommunicationProfile {
			add(SourceUserModel, ReasonRelevantComment, "user_model_communication_profile", "available", "", .82, um.ObservedAt, 15*time.Minute)
		}
	}

	hm := e.HistoricalMemory
	if hm.Required || hm.Available {
		value := "available"
		reason := ReasonUsefulInformation
		if hm.Required && !hm.Available {
			value = "required_unavailable"
		}
		add(SourceConversationalContext, reason, "enriched_historical_context", value, hm.Query, .90, hm.ObservedAt, 5*time.Minute)
	}

	w := e.Work
	if w.Exists {
		reason := ReasonNextStepAvailable
		if w.PendingSteps > 0 {
			reason = ReasonPendingTask
		}
		add(SourceOperationalWork, reason, "enriched_work", string(w.Status), w.ID, .90, w.UpdatedAt, 5*time.Minute)
		if w.PendingSteps > 0 {
			add(SourceOperationalWork, ReasonPendingTask, "work_pending_steps", fmt.Sprintf("%d", w.PendingSteps), w.ID, .92, w.UpdatedAt, 5*time.Minute)
		}
	}

	cm := e.Commitment
	if cm.OpenCount > 0 {
		add(SourceOpenCommitment, ReasonUnfinishedCommitment, "enriched_open_commitments", fmt.Sprintf("%d", cm.OpenCount), c.Topic, .92, cm.LatestAt, 10*time.Minute)
	}

	a := e.Autonomy
	if a.ExecutionState != "" {
		reason := ReasonRelevantComment
		if a.WaitingForUser {
			reason = ReasonClarificationNeeded
		}
		add(SourceAutonomousSignal, reason, "autonomy_execution_state", a.ExecutionState, a.ActiveTask, .76, a.ObservedAt, 2*time.Minute)
	}
	if a.StaleGoals > 0 {
		add(SourceAutonomousSignal, ReasonPendingTask, "autonomy_stale_goals", fmt.Sprintf("%d", a.StaleGoals), a.ActiveTask, .84, a.ObservedAt, 5*time.Minute)
	}
	if a.BlockedGoals > 0 {
		add(SourceAutonomousSignal, ReasonPotentialProblem, "autonomy_blocked_goals", fmt.Sprintf("%d", a.BlockedGoals), a.ActiveTask, .88, a.ObservedAt, 5*time.Minute)
	}

	av := e.Availability
	if av.Availability != "" && av.Availability != AvailabilityUnknown {
		add(SourceInteractionContinuity, ReasonConversationContinue, "enriched_user_availability", string(av.Availability), "", .96, av.ObservedAt, time.Minute)
	}
	if av.Interruptibility != "" && av.Interruptibility != InterruptSafe {
		add(SourceInteractionContinuity, ReasonPotentialProblem, "enriched_interruptibility", string(av.Interruptibility), "", .94, av.ObservedAt, time.Minute)
	}
	if av.ConversationLoad != LoadNormal && av.ConversationLoad != "" {
		add(SourceInteractionContinuity, ReasonPotentialProblem, "enriched_conversation_load", string(av.ConversationLoad), "", .90, av.ObservedAt, time.Minute)
	}

	ps := e.PrivacySafety
	if ps.PolicyKnown && ps.Policy != InformationAllowed {
		add(SourceUserModel, ReasonPotentialProblem, "enriched_information_policy", string(ps.Policy), "", .98, ps.ObservedAt, 2*time.Minute)
	}
	if ps.Interruptibility != InterruptSafe && ps.Interruptibility != "" {
		add(SourceInteractionContinuity, ReasonPotentialProblem, "enriched_safety_interruptibility", string(ps.Interruptibility), "", .96, ps.ObservedAt, time.Minute)
	}

	ch := e.Channel
	if strings.TrimSpace(ch.Channel) != "" {
		add(SourceInteractionContinuity, ReasonConversationContinue, "enriched_channel", ch.Channel, "", .92, ch.ObservedAt, 10*time.Minute)
	}

	return out
}

// CorrelateEnrichedSignals adds only derived relationships between observations.
// It never collapses those relationships into WAIT/DEFER/INTERVENE.
func CorrelateEnrichedSignals(signals []InitiativeSignal, in SignalInput, now time.Time) []InitiativeSignal {
	if now.IsZero() {
		now = time.Now()
	}
	has := func(typ string) (InitiativeSignal, bool) {
		for _, s := range signals {
			if s.Type == typ {
				return s, true
			}
		}
		return InitiativeSignal{}, false
	}
	add := func(typ string, reason Reason, value string, target string, confidence float64) {
		s := makeSignal(in, SourceInteractionContinuity, reason, typ, value, target, true, confidence)
		s.Provenance.Component = "initiative.B96.37.5.correlation"
		s.Provenance.Subsystem = "B96.37.5"
		s.Provenance.OriginID = "cross-source"
		signals = append(signals, s)
	}
	if w, ok := has("enriched_work"); ok {
		if c, ok2 := has("enriched_active_context"); ok2 && w.Target != "" && c.Target != "" {
			add("cross_source_work_context", ReasonNextStepAvailable, "work_and_context_aligned", w.Target, (w.Confidence+c.Confidence)/2)
		}
	}
	if _, ok := has("cognition_clarification"); ok {
		if _, ok2 := has("user_model_reliable_preferences"); ok2 {
			add("cross_source_cognition_user_model", ReasonClarificationNeeded, "cognition_and_user_model_available", in.ActiveTopic, .82)
		}
	}
	if _, ok := has("enriched_open_commitments"); ok {
		if _, ok2 := has("enriched_active_context"); ok2 {
			add("cross_source_commitment_context", ReasonUnfinishedCommitment, "commitment_attached_to_context", in.ActiveTopic, .90)
		}
	}
	return signals
}

// EnrichAndCorrelate is the canonical B96.37.5 source-enrichment pass.
func EnrichAndCorrelate(in SignalInput, base []InitiativeSignal, now time.Time) []InitiativeSignal {
	all := append([]InitiativeSignal(nil), base...)
	all = append(all, EnrichSignals(in, now)...)
	all = CorrelateEnrichedSignals(all, in, now)
	return NormalizeSignals(all, now)
}
