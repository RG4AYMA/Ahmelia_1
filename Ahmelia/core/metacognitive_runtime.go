package core

import (
	"fmt"
	"strings"
	"time"

	"ahmelia/cognition"
	"ahmelia/interaction"
	"ahmelia/language"
	"ahmelia/personality"
	"ahmelia/reasoning"
	"ahmelia/shared"
)

// collectMetacognitiveDiagnostics keeps all metacognitive capabilities alive,
// but changes their handoff contract: subsystem state/policy becomes typed
// evidence, not finished response text.
func (e *Engine) collectMetacognitiveDiagnostics(req interaction.SemanticRequest) []interaction.DiagnosticResult {
	if e == nil {
		return nil
	}
	norm := shared.Normalize(req.Normalized)
	if norm == "" {
		norm = shared.Normalize(req.Raw)
	}
	now := time.Now()
	var out []interaction.DiagnosticResult

	add := func(d interaction.DiagnosticResult) {
		if d.Valid() {
			out = append(out, d)
		}
	}

	// "¿Qué estás haciendo?" is intentionally broader than one subsystem:
	// combine current runtime state with current work/context as separate facts.
	if cognition.ParseInternalStateQuery(norm) {
		if e.internalState != nil {
			current, detail, since := e.internalState.Snapshot()
			d := interaction.NewDiagnosticResult(interaction.DiagnosticInternalState, "cognition.internal_state", "runtime", 0.98, now)
			d.AddFact("current", string(current), 0.98)
			if detail != "" {
				d.AddFact("detail", detail, 0.95)
			}
			if !since.IsZero() {
				d.AddFact("age", durationDescriptor(time.Since(since)), 0.90)
			}
			add(d)
		}
		if d := e.contextDiagnostic(now); d.Valid() {
			add(d)
		}
		if d := e.pipelineDiagnostic(now); d.Valid() {
			add(d)
		}
	}

	if cognition.ParsePipelineQuery(norm) {
		add(e.pipelineDiagnostic(now))
	}
	if cognition.ParseAttentionQuery(norm) {
		add(e.attentionDiagnostic(req, now))
	}
	if cognition.ParseWorkingMemoryQuery(norm) {
		add(e.workingMemoryDiagnostic(now))
	}
	if cognition.ParseContextQuery(norm) {
		add(e.contextDiagnostic(now))
	}
	if interaction.HasSemanticQuery(req, interaction.QueryMentalState) && e.mentalAgenda != nil {
		state := e.mentalAgenda.Snapshot()
		d := interaction.NewDiagnosticResult(interaction.DiagnosticInternalState, "reasoning_deep.mental_agenda", "mental_agenda", 0.90, now)
		d.AddFact("focus", strings.TrimSpace(strings.Join([]string{state.CurrentFocus, state.SubFocus}, " / ")), 0.90)
		for i, doubt := range state.Doubts {
			d.AddFact(fmt.Sprintf("doubt_%d", i+1), doubt, 0.85)
		}
		for i, concern := range state.Preoccupations {
			d.AddFact(fmt.Sprintf("concern_%d", i+1), concern.Description, 0.85)
		}
		for i, thought := range state.ActiveThoughts {
			d.AddFact(fmt.Sprintf("thought_%d", i+1), thought, 0.80)
		}
		d.AddFact("confidence", fmt.Sprintf("%.0f%%", state.Confidence*100), 0.85)
		add(d)
	}
	if reasoning.ParseTrustQuery(norm) {
		add(e.trustDiagnostic(now))
	}
	if cognition.ParseModeQuery(norm) {
		add(e.modeDiagnostic(now))
	}
	if e.relationship != nil {
		// Relationship is a personality subsystem; its parser remains the semantic
		// trigger authority. No Narrative() call is used here.
		if personality.ParseRelationshipQuery(norm) {
			add(e.relationshipDiagnostic(now))
		}
	}
	if language.ParseConversationSummaryQuery(norm) {
		add(e.conversationSummaryDiagnostic(now))
	}
	if e.humorStyle != nil && personality.ParseHumorQuery(norm) {
		add(e.humorDiagnostic(now))
	}

	return out
}

func (e *Engine) evaluateMetacognition(req interaction.SemanticRequest) bool {
	if e == nil || e.conversationState == nil {
		return false
	}
	diagnostics := e.collectMetacognitiveDiagnostics(req)
	decision, ok := cognition.EvaluateMetacognition(req, e.conversationState.Snapshot(), diagnostics)
	if !ok {
		return false
	}
	contract := interaction.NewResponseContract(req, decision.Intent, decision.Move)
	contract.Source = "metacognitive_diagnostic"
	contract.Reason = decision.Reason
	for _, evidence := range decision.Evidence {
		contract.AddEvidence(evidence.Text, evidence.Source, evidence.Relevant, evidence.Confidence)
	}
	if e.flowTrace != nil {
		e.flowTrace.Mark("METACOGNITION", fmt.Sprintf("diagnostics=%d reason=%s", len(decision.Diagnostics), decision.Reason))
	}
	return e.renderResponseContract(req, contract, e.buildConversationalRuntimeFacts(e.conversationState.Snapshot()))
}

func (e *Engine) contextDiagnostic(now time.Time) interaction.DiagnosticResult {
	if e == nil || e.contextBuilder == nil {
		return interaction.DiagnosticResult{}
	}
	uc := e.contextBuilder.Build()
	d := interaction.NewDiagnosticResult(interaction.DiagnosticContext, "cognition.context", "unified_context", 0.95, now)
	if uc == nil || uc.IsEmpty() {
		d.AddFact("status", "empty", 0.75)
		return d
	}
	d.AddFact("world", uc.World, 0.95)
	d.AddFact("conversation", uc.Conversation, 0.90)
	d.AddFact("goals", uc.Goals, 0.85)
	d.AddFact("mental_state", uc.MentalState, 0.90)
	d.AddFact("timeline", uc.Timeline, 0.80)
	d.AddFact("habits", uc.Habits, 0.75)
	d.AddFact("learning", uc.Learning, 0.80)
	d.AddFact("reflection", uc.Reflection, 0.80)
	d.AddFact("insights", uc.Insights, 0.75)
	return d
}

func (e *Engine) pipelineDiagnostic(now time.Time) interaction.DiagnosticResult {
	d := interaction.NewDiagnosticResult(interaction.DiagnosticPipeline, "cognition.pipeline", "latest_pass", 0.98, now)
	if e == nil || e.pipeline == nil {
		return d
	}
	pass := e.pipeline.LastPass()
	if len(pass) == 0 {
		d.AddFact("status", "not_started", 0.95)
		return d
	}
	for i, ev := range pass {
		value := string(ev.Stage)
		if strings.TrimSpace(ev.Detail) != "" {
			value += " | " + strings.TrimSpace(ev.Detail)
		}
		d.AddFact(fmt.Sprintf("step_%d", i+1), value, 0.95)
	}
	return d
}

func (e *Engine) attentionDiagnostic(req interaction.SemanticRequest, now time.Time) interaction.DiagnosticResult {
	d := interaction.NewDiagnosticResult(interaction.DiagnosticAttention, "cognition.attention", "current_focus", 0.90, now)
	if e == nil || e.attention == nil || e.contextBuilder == nil {
		return d
	}
	uc := e.contextBuilder.Build()
	attended, _ := e.attention.Attend(uc, req.Topic)
	if len(attended) == 0 {
		d.AddFact("status", "no_attended_signal", 0.85)
		return d
	}
	for i, s := range attended {
		d.AddFact(fmt.Sprintf("signal_%d", i+1), fmt.Sprintf("label=%s; content=%s; score=%.2f", s.Label, s.Content, s.Score), s.Score)
	}
	return d
}

func (e *Engine) workingMemoryDiagnostic(now time.Time) interaction.DiagnosticResult {
	d := interaction.NewDiagnosticResult(interaction.DiagnosticWorkingMemory, "cognition.working_memory", "active_items", 0.90, now)
	if e == nil || e.workingMemory == nil {
		return d
	}
	items := e.workingMemory.Active()
	if len(items) == 0 {
		d.AddFact("status", "empty", 0.80)
		return d
	}
	for i, item := range items {
		d.AddFact(fmt.Sprintf("item_%d", i+1), fmt.Sprintf("label=%s; content=%s; score=%.2f; age=%s", item.Label, item.Content, item.Score, durationDescriptor(item.Age())), item.Score)
	}
	return d
}

func (e *Engine) trustDiagnostic(now time.Time) interaction.DiagnosticResult {
	d := interaction.NewDiagnosticResult(interaction.DiagnosticTrust, "reasoning.trust", "last_action", 0.85, now)
	if e == nil || e.history == nil {
		d.AddFact("status", "no_action_history", 0.70)
		return d
	}
	last := e.history.GetLastAction()
	if last == nil {
		d.AddFact("status", "no_last_action", 0.70)
		return d
	}
	d.AddFact("action", last.Action, 0.95)
	d.AddFact("target", last.Target, 0.90)
	d.AddFact("success", fmt.Sprintf("%t", last.Success), 0.95)
	if e.learningState != nil && e.learningState.HasConfidence(last.Action) {
		conf := e.learningState.Confidence(last.Action)
		d.AddFact("historical_success_percent", fmt.Sprintf("%.1f", conf), 0.95)
		if e.reasoner != nil {
			tree := e.reasoner.DecideTrust(last.Action, conf)
			d.AddFact("trust_posture", tree.Chosen, tree.Confidence)
			d.AddFact("trust_confidence", fmt.Sprintf("%.2f", tree.Confidence), tree.Confidence)
		}
	}
	return d
}

func (e *Engine) modeDiagnostic(now time.Time) interaction.DiagnosticResult {
	d := interaction.NewDiagnosticResult(interaction.DiagnosticMode, "cognition.conversation_mode", "current_mode", 0.95, now)
	if state, ok := e.persistentModeState(); ok {
		d.AddFact("persistent", persistentModeSummary(state), 1)
	}
	if e == nil || e.conversationMode == nil {
		return d
	}
	d.AddFact("current", e.conversationMode.CurrentName(), 0.95)
	for i, score := range e.conversationMode.LastScores() {
		if score.Reason == "" {
			continue
		}
		d.AddFact(fmt.Sprintf("score_%d", i+1), fmt.Sprintf("mode=%s; score=%.2f; reason=%s", score.Mode, score.Score, score.Reason), score.Score)
	}
	return d
}

func (e *Engine) relationshipDiagnostic(now time.Time) interaction.DiagnosticResult {
	d := interaction.NewDiagnosticResult(interaction.DiagnosticRelationship, "personality.relationship", "user_relation", 0.95, now)
	if e == nil || e.relationship == nil {
		return d
	}
	r := e.relationship
	d.AddFact("type", r.Type, 0.95)
	d.AddFact("intimacy", fmt.Sprintf("%.2f", r.Intimacy), 0.90)
	d.AddFact("trust", fmt.Sprintf("%.2f", r.TrustLevel), 0.95)
	d.AddFact("shared_moments", fmt.Sprintf("%d", len(r.SharedHistory)), 0.95)
	d.AddFact("inside_jokes", fmt.Sprintf("%d", len(r.InsideJokes)), 0.85)
	for i, moment := range r.SharedHistory {
		if i < len(r.SharedHistory)-5 {
			continue
		}
		value := moment.Event
		if moment.LessonLearned != "" {
			value += " | lesson=" + moment.LessonLearned
		}
		d.AddFact(fmt.Sprintf("moment_%d", i+1), value, 0.85)
	}
	return d
}

func (e *Engine) conversationSummaryDiagnostic(now time.Time) interaction.DiagnosticResult {
	d := interaction.NewDiagnosticResult(interaction.DiagnosticConversationSummary, "language.conversation_flow", "session_summary", 0.92, now)
	if e == nil || e.conversationFlow == nil {
		return d
	}
	state := e.conversationFlow.GetCurrentState()
	for _, key := range []string{"turn_number", "current_topic", "decision_count"} {
		if value, ok := state[key]; ok {
			d.AddFact(key, fmt.Sprint(value), 0.90)
		}
	}
	if topics, ok := state["topics_discussed"].([]string); ok && len(topics) > 0 {
		d.AddFact("topics", strings.Join(topics, " | "), 0.90)
	}
	if decisions, ok := state["decisions_made"].([]string); ok && len(decisions) > 0 {
		d.AddFact("decisions", strings.Join(decisions, " | "), 0.85)
	}
	return d
}

func (e *Engine) humorDiagnostic(now time.Time) interaction.DiagnosticResult {
	d := interaction.NewDiagnosticResult(interaction.DiagnosticHumor, "personality.humor", "humor_policy", 0.90, now)
	if e == nil || e.humorStyle == nil {
		return d
	}
	mode := ""
	if e.conversationMode != nil {
		mode = e.conversationMode.CurrentName()
	}
	d.AddFact("uses_humor", fmt.Sprintf("%t", e.humorStyle.UsesHumor), 0.95)
	d.AddFact("mode", mode, 0.90)
	d.AddFact("likelihood", fmt.Sprintf("%.2f", e.humorStyle.LikelihoodForMode(mode)), 0.90)
	if e.relationship != nil {
		tiers := e.humorStyle.AvailableTiers(e.relationship)
		labels := make([]string, 0, len(tiers))
		for _, tier := range tiers {
			labels = append(labels, string(tier))
		}
		d.AddFact("available_tiers", strings.Join(labels, " | "), 0.85)
	}
	return d
}

func durationDescriptor(d time.Duration) string {
	if d < time.Second {
		return "less_than_1s"
	}
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	return fmt.Sprintf("%dm", int(d.Minutes()))
}
