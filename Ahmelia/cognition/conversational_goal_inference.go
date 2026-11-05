package cognition

import (
	"strings"

	"ahmelia/interaction"
)

// B96.33.3 — CONVERSATIONAL GOAL INFERENCE
//
// B96.33.2 identifies the user's communicative move. This block infers the
// outcome the user is trying to obtain from that move, using the shared
// semantic contract and existing conversation state.
//
// A conversational goal is not an authorization and is not an execution plan.
// In particular, a goal such as "change the enemy system" may be inferred
// from a proposal or an action request, while the decision to actually do it
// remains owned by later cognition/authorization/execution layers.

type ConversationalGoalKind string

const (
	GoalUnknown         ConversationalGoalKind = "unknown"
	GoalUnderstand      ConversationalGoalKind = "understand"
	GoalGetInformation  ConversationalGoalKind = "get_information"
	GoalGetOpinion      ConversationalGoalKind = "get_opinion"
	GoalDiscuss         ConversationalGoalKind = "discuss"
	GoalExplore         ConversationalGoalKind = "explore"
	GoalDecide          ConversationalGoalKind = "decide"
	GoalChange          ConversationalGoalKind = "change"
	GoalCreate          ConversationalGoalKind = "create"
	GoalPlan            ConversationalGoalKind = "plan"
	GoalCorrect         ConversationalGoalKind = "correct"
	GoalConfirm         ConversationalGoalKind = "confirm"
	GoalReject          ConversationalGoalKind = "reject"
	GoalContinue        ConversationalGoalKind = "continue"
	GoalStop            ConversationalGoalKind = "stop"
	GoalLearn           ConversationalGoalKind = "learn"
	GoalProvideFeedback ConversationalGoalKind = "provide_feedback"
	GoalSocialize       ConversationalGoalKind = "socialize"
)

type GoalCandidate struct {
	Kind      ConversationalGoalKind
	Objective string
	Topic     string
	Score     float64
	Evidence  []string
}

type ConversationalGoalInference struct {
	Version            string
	Primary            ConversationalGoalKind
	Objective          string
	Topic              string
	Confidence         float64
	Ambiguous          bool
	NeedsClarification bool
	GoalDirected       bool
	ActionRelated      bool
	DecisionRelated    bool
	CarriedFromContext bool
	Candidates         []GoalCandidate
	Evidence           []string
	Reason             string
}

// InferConversationalGoal answers: "what outcome is the user trying to get
// from this intervention?" It intentionally does not choose a capability,
// authorize execution, or mutate conversation state.
func InferConversationalGoal(model ConversationalCognitiveModel, intent CommunicativeIntentEvaluation) ConversationalGoalInference {
	r, ctx := model.Request, model.Context
	candidates := make([]GoalCandidate, 0, 8)

	add := func(kind ConversationalGoalKind, objective, topic string, score float64, evidence ...string) {
		if score <= 0 {
			return
		}
		candidates = append(candidates, GoalCandidate{
			Kind: kind, Objective: strings.TrimSpace(objective), Topic: strings.TrimSpace(topic),
			Score: clamp01(score), Evidence: uniqueStrings(evidence),
		})
	}

	objective := strings.TrimSpace(r.Objective.Text)
	topic := strings.TrimSpace(r.Topic)
	if topic == "" {
		topic = strings.TrimSpace(model.Situation.Topic)
	}

	// The semantic objective is the strongest direct evidence of desired
	// outcome. It is not replaced by the intent label.
	switch intent.Primary {
	case IntentAskOpinion:
		add(GoalGetOpinion, objective, topic, 0.94, "intent:ask_opinion")
	case IntentQuestion:
		if objective == "" {
			objective = strings.TrimSpace(r.Raw)
		}
		add(GoalGetInformation, objective, topic, 0.86, "intent:question")
	case IntentRequestAction:
		if objective == "" {
			objective = strings.TrimSpace(r.RequestedAction)
		}
		if objective == "" {
			objective = strings.TrimSpace(r.Action.Name)
		}
		add(goalForAction(r), objective, topic, 0.92, "intent:request_action")
	case IntentShareIdea:
		add(GoalDiscuss, objective, topic, 0.72, "intent:share_idea")
		if objective != "" {
			add(goalForObjective(objective), objective, topic, 0.90, "intent:idea_with_objective")
		}
	case IntentDecide:
		add(GoalDecide, objective, topic, 0.95, "intent:decide")
	case IntentExplore:
		add(GoalExplore, objective, topic, 0.86, "intent:explore")
	case IntentDiscuss:
		add(GoalDiscuss, objective, topic, 0.84, "intent:discuss")
	case IntentCorrect:
		if objective == "" {
			objective = strings.TrimSpace(r.CorrectionTarget)
		}
		add(GoalCorrect, objective, topic, 0.94, "intent:correct")
	case IntentConfirm:
		add(GoalConfirm, objective, topic, 0.90, "intent:confirm")
	case IntentReject:
		add(GoalReject, objective, topic, 0.90, "intent:reject")
	case IntentContinue:
		add(GoalContinue, objective, topic, 0.91, "intent:continue")
	case IntentControl:
		if interaction.CanonicalActs(r).Has(interaction.ActControl) {
			add(GoalStop, objective, topic, 0.98, "intent:control", "semantic:stop")
		} else {
			add(GoalStop, objective, topic, 0.80, "intent:control")
		}
	case IntentLearning:
		add(GoalLearn, objective, topic, 0.91, "intent:learning")
	case IntentFeedback:
		add(GoalProvideFeedback, objective, topic, 0.91, "intent:feedback")
	case IntentSocial:
		add(GoalSocialize, objective, topic, 0.88, "intent:social")
	}

	// Meaning concepts refine the goal without reparsing language.
	if hasMeaningConcept(r, interaction.ConceptOpinion) {
		add(GoalGetOpinion, objective, topic, 0.88, "meaning:opinion")
	}
	if hasMeaningConcept(r, interaction.ConceptDecision) {
		add(GoalDecide, objective, topic, 0.90, "meaning:decision")
	}
	if hasMeaningConcept(r, interaction.ConceptPlanning) {
		add(GoalPlan, objective, topic, 0.79, "meaning:planning")
	}
	if hasMeaningConcept(r, interaction.ConceptCorrection) {
		add(GoalCorrect, objective, topic, 0.84, "meaning:correction")
	}

	// Explicit objective/action fields can reveal the desired outcome even
	// when the communicative move is deliberately conversational.
	if objective != "" {
		add(goalForObjective(objective), objective, topic, 0.55, "semantic:objective")
	}
	if r.RequestedAction != "" || r.Action.Name != "" {
		add(goalForAction(r), firstNonEmpty(r.RequestedAction, r.Action.Name, objective), topic, 0.48, "semantic:action_payload")
	}

	// A short continuation often means "continue the current goal", not a
	// brand-new objective. Carrying it forward is explicit and auditable.
	carried := false
	if intent.Primary == IntentContinue || (r.FollowUp || r.Continues) && objective == "" {
		active := strings.TrimSpace(ctx.ActiveObjective)
		if active != "" {
			add(goalForObjective(active), active, firstNonEmpty(topic, ctx.ActiveTopic), 0.93, "context:active_objective", "context:continuation")
			carried = true
		}
	}

	candidates = aggregateGoalCandidates(candidates)
	sortGoalCandidates(candidates)

	out := ConversationalGoalInference{Version: "B96.33.3", Candidates: candidates, CarriedFromContext: carried}
	if len(candidates) == 0 {
		out.Primary, out.Confidence = GoalUnknown, 0
		out.Ambiguous, out.NeedsClarification = true, true
		out.Reason = "no sufficiently grounded conversational goal was established"
		return out
	}

	winner := candidates[0]
	out.Primary, out.Objective, out.Topic = winner.Kind, winner.Objective, winner.Topic
	out.Confidence, out.Evidence = winner.Score, append([]string(nil), winner.Evidence...)
	out.GoalDirected = out.Objective != "" || out.Primary == GoalContinue || out.Primary == GoalSocialize
	out.ActionRelated = out.Primary == GoalChange || out.Primary == GoalCreate
	out.DecisionRelated = out.Primary == GoalDecide
	if len(candidates) > 1 {
		out.Ambiguous = winner.Score-candidates[1].Score < 0.16 || winner.Score < 0.60
	}
	out.NeedsClarification = out.Ambiguous || model.Signals.NeedsClarification
	if out.Ambiguous && out.ActionRelated {
		out.NeedsClarification = true
	}
	out.Reason = "conversational goal inferred from communicative intent, semantic objective and conversation state"
	if out.CarriedFromContext {
		out.Reason = "conversational goal carried from the active conversation objective"
	}
	if out.Ambiguous {
		out.Reason = "multiple conversational goals remain plausible"
	}
	return out
}

func goalForAction(r interaction.SemanticRequest) ConversationalGoalKind {
	text := strings.ToLower(strings.TrimSpace(firstNonEmpty(r.RequestedAction, r.Action.Name, r.Objective.Text)))
	switch {
	case strings.Contains(text, "creat"), strings.Contains(text, "crear"), strings.Contains(text, "new"), strings.Contains(text, "nuevo"):
		return GoalCreate
	default:
		return GoalChange
	}
}

func goalForObjective(text string) ConversationalGoalKind {
	t := strings.ToLower(strings.TrimSpace(text))
	switch {
	case strings.Contains(t, "crear"), strings.Contains(t, "create"), strings.Contains(t, "construir"), strings.Contains(t, "build"):
		return GoalCreate
	case strings.Contains(t, "plan"), strings.Contains(t, "diseñ"), strings.Contains(t, "design"):
		return GoalPlan
	case strings.Contains(t, "aprender"), strings.Contains(t, "learn"):
		return GoalLearn
	case strings.Contains(t, "cambiar"), strings.Contains(t, "modificar"), strings.Contains(t, "mejorar"), strings.Contains(t, "change"), strings.Contains(t, "modify"), strings.Contains(t, "improve"):
		return GoalChange
	default:
		return GoalUnderstand
	}
}

func aggregateGoalCandidates(in []GoalCandidate) []GoalCandidate {
	type agg struct {
		score            float64
		objective, topic string
		evidence         []string
	}
	m := map[ConversationalGoalKind]agg{}
	for _, c := range in {
		a := m[c.Kind]
		a.score += c.Score
		if a.score > 1 {
			a.score = 1
		}
		if a.objective == "" {
			a.objective = c.Objective
		}
		if a.topic == "" {
			a.topic = c.Topic
		}
		a.evidence = append(a.evidence, c.Evidence...)
		m[c.Kind] = a
	}
	out := make([]GoalCandidate, 0, len(m))
	for k, a := range m {
		out = append(out, GoalCandidate{Kind: k, Score: clamp01(a.score), Objective: a.objective, Topic: a.topic, Evidence: uniqueStrings(a.evidence)})
	}
	return out
}

func sortGoalCandidates(c []GoalCandidate) {
	for i := 0; i < len(c); i++ {
		for j := i + 1; j < len(c); j++ {
			if c[j].Score > c[i].Score {
				c[i], c[j] = c[j], c[i]
			}
		}
	}
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
