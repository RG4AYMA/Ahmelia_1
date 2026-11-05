package cognition

import (
	"strings"
	"unicode"

	"ahmelia/interaction"
)

// B96.33.4 — CONVERSATIONAL RELEVANCE
//
// Relevance answers: "How relevant is this turn to the current conversational
// situation and what should it remain connected to?"
//
// Relevance is not truth, intent, authorization, or execution priority. It is
// a contextual assessment used by later interpretation/decision layers.
// The evaluator consumes already-established semantic cognition and state;
// it never reparses the raw utterance through another linguistic model.

type RelevanceKind string

const (
	RelevanceUnknown      RelevanceKind = "unknown"
	RelevanceCurrentGoal  RelevanceKind = "current_goal"
	RelevanceCurrentTopic RelevanceKind = "current_topic"
	RelevanceCurrentWork  RelevanceKind = "current_work"
	RelevanceRecentTurn   RelevanceKind = "recent_turn"
	RelevanceOpenThread   RelevanceKind = "open_thread"
	RelevanceNewTopic     RelevanceKind = "new_topic"
	RelevanceIndependent  RelevanceKind = "independent"
)

type RelevanceEvidence struct {
	Kind   RelevanceKind
	Score  float64
	Reason string
}

type ConversationalRelevance struct {
	Version          string
	Primary          RelevanceKind
	Score            float64
	Confidence       float64
	Ambiguous        bool
	NeedsContext     bool
	ContinuesGoal    bool
	ContinuesTopic   bool
	RelatedToWork    bool
	RelatedToThread  bool
	IsNewTopic       bool
	IsIndependent    bool
	ReferenceMatches int
	Evidence         []RelevanceEvidence
	Reason           string
}

// EvaluateConversationalRelevance evaluates contextual relevance after intent
// and goal inference. It does not mutate state or decide what Ahmelia should
// do with the turn.
func EvaluateConversationalRelevance(model ConversationalCognitiveModel, intent CommunicativeIntentEvaluation, goal ConversationalGoalInference) ConversationalRelevance {
	r, ctx := model.Request, model.Context
	ev := make([]RelevanceEvidence, 0, 8)
	add := func(k RelevanceKind, score float64, reason string) {
		if score > 0 {
			ev = append(ev, RelevanceEvidence{Kind: k, Score: clamp01(score), Reason: reason})
		}
	}

	topic := normalizeTokens(firstNonEmpty(r.Topic, model.Situation.Topic, ctx.ActiveTopic))
	objective := normalizeTokens(firstNonEmpty(goal.Objective, r.Objective.Text))
	// Do not silently replace a missing current-turn objective with the active
	// objective. An explicit new topic must be allowed to break away from the
	// previous goal. The active objective is consulted only by the explicit
	// continuation/current-context signals below.
	activeTopic := normalizeTokens(ctx.ActiveTopic)
	activeGoal := normalizeTokens(ctx.ActiveObjective)

	if topic != "" && activeTopic != "" {
		overlap := tokenOverlap(topic, activeTopic)
		if overlap > 0 {
			add(RelevanceCurrentTopic, 0.45+0.45*overlap, "topic:overlap_with_active_topic")
		}
	}
	goalOverlap := float64(0)
	if objective != "" && activeGoal != "" {
		goalOverlap = tokenOverlap(objective, activeGoal)
		if goalOverlap > 0 {
			add(RelevanceCurrentGoal, 0.50+0.45*goalOverlap, "goal:overlap_with_active_objective")
		}
	}

	if r.AddressesCurrent || r.FollowUp || r.Continues || intent.Primary == IntentContinue || goal.CarriedFromContext {
		add(RelevanceCurrentGoal, 0.86, "semantic:continuation_or_current_context")
	}
	if model.Situation.HasActiveWork && (r.FollowUp || r.Continues || r.AddressesCurrent) {
		add(RelevanceCurrentWork, 0.90, "context:active_work_continuation")
	}
	if len(ctx.OpenThreads) > 0 && (r.FollowUp || r.Continues || hasUnresolvedReference(r)) {
		add(RelevanceOpenThread, 0.70, "context:open_thread_with_follow_up")
	}
	if len(ctx.Turns) > 0 && (intent.Primary == IntentDiscuss || intent.Primary == IntentContinue || r.FollowUp) {
		add(RelevanceRecentTurn, 0.62, "context:recent_conversation_history")
	}

	for _, ref := range r.References {
		if !ref.Resolved {
			continue
		}
		if ref.Value != "" {
			ev = append(ev, RelevanceEvidence{Kind: relevanceForReference(ref.Kind), Score: 0.78, Reason: "reference:resolved"})
		}
	}

	// A clearly explicit topic that has no meaningful connection to the
	// active context is treated as a possible topic switch, not an error.
	explicitTopic := strings.TrimSpace(r.Topic)
	hasActiveContext := activeTopic != "" || activeGoal != "" || model.Situation.HasActiveWork
	if explicitTopic != "" && hasActiveContext {
		topicMatch := tokenOverlap(normalizeTokens(explicitTopic), activeTopic)
		goalMatch := tokenOverlap(normalizeTokens(explicitTopic), activeGoal)
		if topicMatch == 0 && goalMatch == 0 && !r.FollowUp && !r.Continues {
			add(RelevanceNewTopic, 0.84, "topic:no_overlap_with_active_context")
			// An explicit, unrelated topic is a strong boundary signal.
			// Do not let a generic/inferred objective keep the turn attached
			// to the previous goal.
			for i := range ev {
				if ev[i].Kind == RelevanceCurrentGoal || ev[i].Kind == RelevanceCurrentTopic || ev[i].Kind == RelevanceCurrentWork {
					ev[i].Score *= 0.35
				}
			}
		}
	}

	if interaction.CanonicalActs(r).Has(interaction.ActTopicShift) {
		add(RelevanceNewTopic, 0.99, "semantic:explicit_topic_shift")
	}

	// Purely social/independent contributions should not be forced into an
	// active work objective simply because work exists.
	newTopicByEvidence := false
	if explicitTopic != "" && hasActiveContext && !r.FollowUp && !r.Continues {
		tm := tokenOverlap(normalizeTokens(explicitTopic), activeTopic)
		gm := tokenOverlap(normalizeTokens(explicitTopic), activeGoal)
		newTopicByEvidence = tm == 0 && gm == 0
	}
	if (intent.Primary == IntentSocial || interaction.CanonicalActs(r).Has(interaction.ActSocial)) && !r.FollowUp && !r.Continues && !interaction.CanonicalActs(r).Has(interaction.ActTopicShift) && !newTopicByEvidence {
		add(RelevanceIndependent, 0.88, "intent:social_without_continuation")
	}
	if !hasActiveContext && !r.FollowUp && !r.Continues && intent.Primary != IntentContinue {
		add(RelevanceIndependent, 0.68, "context:no_active_context")
	}

	sortRelevanceEvidence(ev)
	out := ConversationalRelevance{Version: "B96.33.4", Evidence: ev}
	if len(ev) == 0 {
		out.Primary = RelevanceUnknown
		out.NeedsContext = true
		out.Ambiguous = true
		out.Reason = "no grounded conversational relevance established"
		return out
	}

	out.Primary = ev[0].Kind
	out.Score = ev[0].Score
	out.Confidence = ev[0].Score
	for _, e := range ev {
		if e.Kind == RelevanceCurrentGoal {
			out.ContinuesGoal = true
		}
		if e.Kind == RelevanceCurrentTopic {
			out.ContinuesTopic = true
		}
		if e.Kind == RelevanceCurrentWork {
			out.RelatedToWork = true
		}
		if e.Kind == RelevanceOpenThread {
			out.RelatedToThread = true
		}
		if e.Kind == RelevanceNewTopic {
			out.IsNewTopic = true
		}
		if e.Kind == RelevanceIndependent {
			out.IsIndependent = true
		}
		if e.Kind == RelevanceCurrentGoal || e.Kind == RelevanceCurrentTopic || e.Kind == RelevanceCurrentWork || e.Kind == RelevanceOpenThread {
			out.ReferenceMatches++
		}
	}
	// The ConversationState snapshot can already contain the current turn's
	// objective/topic. When an explicit action is evaluated against that same
	// objective/topic, the two relevance signals describe one source, not two
	// competing contexts. Counting them as ambiguity incorrectly blocks
	// otherwise grounded production/action requests.
	currentTurnAnchored := intent.ActionOriented &&
		strings.TrimSpace(r.Objective.Text) != "" &&
		normalizeTokens(r.Objective.Text) == activeGoal &&
		strings.TrimSpace(r.Topic) != "" &&
		normalizeTokens(r.Topic) == activeTopic
	if len(ev) > 1 && !currentTurnAnchored {
		out.Ambiguous = ev[0].Score-ev[1].Score < 0.14
	}
	out.NeedsContext = out.Primary == RelevanceUnknown || (out.Ambiguous && out.Score < 0.70)
	if out.IsNewTopic && !out.ContinuesGoal && !out.ContinuesTopic && !out.RelatedToWork {
		out.Reason = "turn appears to introduce a new conversational topic"
	} else if out.ContinuesGoal {
		out.Reason = "turn remains connected to the active conversational goal"
	} else if out.RelatedToWork {
		out.Reason = "turn remains connected to active work through conversational context"
	} else if out.IsIndependent {
		out.Reason = "turn is conversationally independent of the active context"
	} else {
		out.Reason = "turn relevance derived from topic, goal, references and conversation state"
	}
	return out
}

func relevanceForReference(k interaction.ReferenceKind) RelevanceKind {
	switch k {
	case interaction.ReferenceRecentWork:
		return RelevanceCurrentWork
	case interaction.ReferenceCurrentTopic:
		return RelevanceCurrentTopic
	case interaction.ReferenceRecentArtifact:
		return RelevanceCurrentWork
	case interaction.ReferenceCurrentInput:
		return RelevanceRecentTurn
	default:
		return RelevanceRecentTurn
	}
}

func hasUnresolvedReference(r interaction.SemanticRequest) bool {
	for _, ref := range r.References {
		if ref.Resolved {
			continue
		}
		// Requirements and recovery targets are subordinate to the primary
		// objective. They are not blockers unless the objective itself lacks a
		// target. This prevents "run the tests" or "fix it if it fails" from
		// displacing the operation being requested.
		if ref.Role == interaction.ReferenceRoleRequirement || ref.Role == interaction.ReferenceRoleRecoveryTarget {
			continue
		}
		return true
	}
	return false
}

func normalizeTokens(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(s)) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsSpace(r) {
			b.WriteRune(r)
		} else {
			b.WriteRune(' ')
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

func tokenOverlap(a, b string) float64 {
	if a == "" || b == "" {
		return 0
	}
	am := map[string]bool{}
	bm := map[string]bool{}
	for _, t := range strings.Fields(a) {
		if len(t) > 2 {
			am[t] = true
		}
	}
	for _, t := range strings.Fields(b) {
		if len(t) > 2 {
			bm[t] = true
		}
	}
	if len(am) == 0 || len(bm) == 0 {
		return 0
	}
	common := 0
	for t := range am {
		if bm[t] {
			common++
		}
	}
	denom := len(am)
	if len(bm) > denom {
		denom = len(bm)
	}
	return float64(common) / float64(denom)
}

func sortRelevanceEvidence(ev []RelevanceEvidence) {
	for i := 0; i < len(ev); i++ {
		for j := i + 1; j < len(ev); j++ {
			if ev[j].Score > ev[i].Score {
				ev[i], ev[j] = ev[j], ev[i]
			}
		}
	}
}
