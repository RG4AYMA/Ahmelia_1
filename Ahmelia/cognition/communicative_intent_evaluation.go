package cognition

import (
	"sort"
	"strings"

	"ahmelia/interaction"
)

// ============================================================================
// B96.33.2 — COMMUNICATIVE INTENT EVALUATION
// ============================================================================
//
// B96.33.1 establishes the cognitive situation.
// B96.33.2 evaluates what the user is doing communicatively.
//
// The evaluator consumes the existing SemanticRequest and the B96.32
// ConversationSnapshot. It does not run a second linguistic classifier and
// does not ask the language model to choose Ahmelia's behavior.
//
// Important distinction:
//   communicative intent != operational authorization
//   action-oriented intent != permission to execute
//
// The evaluator may conclude that a turn is a proposal, question, command,
// correction, feedback, continuation, social contribution, etc. It may also
// expose competing interpretations when the semantic evidence is insufficient
// to safely collapse them into one.

type CommunicativeIntentKind string

const (
	IntentUnknown       CommunicativeIntentKind = "unknown"
	IntentShareIdea     CommunicativeIntentKind = "share_idea"
	IntentDiscuss       CommunicativeIntentKind = "discuss"
	IntentAskOpinion    CommunicativeIntentKind = "ask_opinion"
	IntentQuestion      CommunicativeIntentKind = "question"
	IntentRequestAction CommunicativeIntentKind = "request_action"
	IntentDecide        CommunicativeIntentKind = "decide"
	IntentExplore       CommunicativeIntentKind = "explore"
	IntentInform        CommunicativeIntentKind = "inform"
	IntentCorrect       CommunicativeIntentKind = "correct"
	IntentConfirm       CommunicativeIntentKind = "confirm"
	IntentReject        CommunicativeIntentKind = "reject"
	IntentContinue      CommunicativeIntentKind = "continue"
	IntentControl       CommunicativeIntentKind = "control"
	IntentFeedback      CommunicativeIntentKind = "feedback"
	IntentSocial        CommunicativeIntentKind = "social"
	IntentLearning      CommunicativeIntentKind = "teach"
)

// IntentCandidate is an auditable interpretation hypothesis. Candidates are
// ranked, but ranking does not mean that the winner is necessarily certain.
type IntentCandidate struct {
	Kind        CommunicativeIntentKind
	Score       float64
	Evidence    []string
	Contradicts []string
}

// CommunicativeIntentEvaluation is the B96.33.2 output consumed by later
// cognition blocks. It describes the user's communicative move; it does not
// choose a response, action, capability or provider.
type CommunicativeIntentEvaluation struct {
	Version            string
	Primary            CommunicativeIntentKind
	Confidence         float64
	Ambiguous          bool
	NeedsClarification bool
	ActionOriented     bool
	DecisionOriented   bool
	ProposalLike       bool
	QuestionLike       bool
	Candidates         []IntentCandidate
	Evidence           []string
	Reason             string
}

// EvaluateCommunicativeIntent evaluates communicative intent from the
// established semantic contract and conversational state.
func EvaluateCommunicativeIntent(model ConversationalCognitiveModel) CommunicativeIntentEvaluation {
	r := model.Request
	ctx := model.Context
	acts := interaction.CanonicalActs(r)

	candidates := make([]IntentCandidate, 0, 8)
	add := func(kind CommunicativeIntentKind, score float64, evidence ...string) {
		if score <= 0 {
			return
		}
		candidates = append(candidates, IntentCandidate{
			Kind: kind, Score: clamp01(score), Evidence: uniqueStrings(evidence),
		})
	}

	// The interaction layer is the authoritative classifier for communicative
	// acts. B96.33 consumes that result as semantic evidence; it must not
	// reclassify declarations, explorations, ideas, opinions, feedback, etc.
	switch acts.Primary {
	case interaction.ActQuestion:
		add(IntentQuestion, 0.97, "interaction:act=question")
	case interaction.ActOpinionRequest:
		add(IntentAskOpinion, 0.99, "interaction:act=opinion_request")
	case interaction.ActCommand:
		add(IntentRequestAction, 0.97, "interaction:act=command")
	case interaction.ActDeclaration, interaction.ActObservation, interaction.ActInform:
		add(IntentInform, 0.96, "interaction:act=informational_turn")
	case interaction.ActProposal:
		add(IntentShareIdea, 0.99, "interaction:act=proposal")
	case interaction.ActIdea:
		add(IntentShareIdea, 0.96, "interaction:act=idea")
	case interaction.ActExploration:
		add(IntentShareIdea, 0.98, "interaction:act=exploration")
		add(IntentExplore, 0.82, "interaction:act=exploration")
	case interaction.ActFeedback:
		add(IntentFeedback, 0.96, "interaction:act=feedback")
	case interaction.ActCorrection:
		add(IntentCorrect, 0.98, "interaction:act=correction")
	case interaction.ActConfirmation:
		add(IntentConfirm, 0.98, "interaction:act=confirmation")
	case interaction.ActRejection:
		add(IntentReject, 0.98, "interaction:act=rejection")
	case interaction.ActDecision:
		add(IntentDecide, 0.98, "interaction:act=decision")
	case interaction.ActContinuation:
		add(IntentContinue, 0.96, "interaction:act=continuation")
	case interaction.ActSocial:
		add(IntentSocial, 0.96, "interaction:act=social")
	case interaction.ActTopicShift:
		add(IntentDiscuss, 0.92, "interaction:act=topic_shift")
	case interaction.ActControl:
		add(IntentControl, 0.98, "interaction:act=control")
	case interaction.ActLearning:
		add(IntentLearning, 0.96, "interaction:act=learning")
	}

	// Canonical meaning concepts are additional language-independent evidence.
	// They are consumed as already-established semantics; this block never
	// reconstructs them from raw text.
	if hasMeaningConcept(r, interaction.ConceptOpinion) {
		add(IntentAskOpinion, 0.91, "meaning:opinion")
	}
	if hasMeaningConcept(r, interaction.ConceptDecision) {
		add(IntentDecide, 0.91, "meaning:decision")
	}
	if hasMeaningConcept(r, interaction.ConceptProposal) || hasMeaningConcept(r, interaction.ConceptSuggestion) {
		add(IntentShareIdea, 0.82, "meaning:proposal_or_suggestion")
	}
	if hasMeaningConcept(r, interaction.ConceptPlanning) {
		add(IntentExplore, 0.68, "meaning:planning")
	}
	if hasMeaningConcept(r, interaction.ConceptAgreement) {
		add(IntentConfirm, 0.84, "meaning:agreement")
	}
	if hasMeaningConcept(r, interaction.ConceptDisagreement) {
		add(IntentReject, 0.84, "meaning:disagreement")
	}

	// Existing semantic fields refine the communicative interpretation.
	if r.RequestedAction != "" || r.Action.Name != "" {
		add(IntentRequestAction, 0.18, "semantic:action_payload_present")
	}
	if r.QuestionType != "" || acts.Has(interaction.ActQuestion) {
		if r.QuestionType != "" {
			add(IntentQuestion, 0.08, "semantic:question_type_present")
		}
	}
	if r.CorrectionTarget != "" || r.Correction.IsCorrection {
		add(IntentCorrect, 0.10, "semantic:correction_evidence")
	}
	if r.AddressesCurrent && (r.FollowUp || r.Continues) {
		add(IntentContinue, 0.10, "semantic:addresses_current_context")
	}

	// Context determines the likely conversational function of otherwise
	// underspecified turns. This is contextual evidence, not a new parse.
	if hasOpenQuestion(ctx) && isShortFollowUp(r) {
		add(IntentContinue, 0.72, "context:open_question_and_follow_up")
	}
	if model.Situation.HasActiveWork && (r.FollowUp || r.Continues) {
		add(IntentContinue, 0.20, "context:active_work")
	}
	if model.Situation.HasHistory && model.Situation.Topic != "" &&
		acts.Has(interaction.ActInform) || acts.Has(interaction.ActSocial) {
		add(IntentDiscuss, 0.20, "context:existing_topic")
	}

	// Important cognitive distinction for natural language:
	// "I am thinking about changing X" is normally a proposal/idea, not an
	// authorized command. If the semantic boundary already says proposal,
	// preserve that. If it only says conversation/inform, expose the
	// proposal-like hypothesis rather than inventing an action.
	if acts.Has(interaction.ActProposal) {
		add(IntentShareIdea, 0.18, "semantic:proposal_language")
		if r.Action.Name != "" || r.RequestedAction != "" {
			add(IntentRequestAction, 0.22, "semantic:action_signal_inside_proposal")
		}
	}
	if acts.Has(interaction.ActSocial) || acts.Has(interaction.ActInform) || acts.Has(interaction.ActDeclaration) {
		if strings.TrimSpace(r.Objective.Text) != "" && !hasExplicitCommand(r) {
			add(IntentShareIdea, 0.35, "semantic:objective_without_command")
			add(IntentExplore, 0.28, "semantic:goal_without_command")
		}
	}

	// Question subtype is semantic evidence for the conversational job. In
	// particular, an opinion question is not merely a generic information
	// request: it invites Ahmelia's evaluative participation.
	if strings.EqualFold(strings.TrimSpace(r.QuestionType), "opinion") ||
		strings.EqualFold(strings.TrimSpace(r.QuestionType), "opinión") {
		add(IntentAskOpinion, 0.86, "semantic:question_type=opinion")
	}

	// Remove impossible/duplicate candidates by aggregating evidence.
	candidates = aggregateCandidates(candidates)
	// Exploration and idea-sharing are sibling descriptions of the same
	// non-operational family. They must not manufacture ambiguity merely
	// because several evidence sources use different labels for it.
	if hasCandidate(candidates, IntentShareIdea) && hasCandidate(candidates, IntentExplore) && !hasCandidate(candidates, IntentRequestAction) {
		share := candidateScore(candidates, IntentShareIdea)
		for i := range candidates {
			if candidates[i].Kind == IntentExplore && candidates[i].Score > share-0.18 {
				candidates[i].Score = share - 0.18
			}
		}
	}

	// Specific communicative subtypes should outrank their broader family.
	// An opinion question is still a question, but its conversational job is
	// specifically to invite Ahmelia's evaluation. Likewise a decision signal
	// should not lose to a generic conversational/informational hypothesis.
	if hasCandidate(candidates, IntentAskOpinion) {
		for i := range candidates {
			if candidates[i].Kind == IntentQuestion && candidates[i].Score > 0.84 {
				candidates[i].Score = 0.82
			}
		}
	}
	if hasCandidate(candidates, IntentDecide) {
		for i := range candidates {
			if candidates[i].Kind == IntentDiscuss || candidates[i].Kind == IntentInform {
				candidates[i].Score *= 0.75
			}
		}
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		return candidates[i].Score > candidates[j].Score
	})

	out := CommunicativeIntentEvaluation{
		Version:    "B96.33.2",
		Candidates: candidates,
	}
	if len(candidates) == 0 {
		out.Primary = IntentUnknown
		out.Confidence = 0
		out.Ambiguous = true
		out.NeedsClarification = true
		out.Reason = "no sufficiently grounded communicative intent was established"
		return out
	}

	out.Primary = candidates[0].Kind
	out.Confidence = candidates[0].Score
	out.Evidence = append([]string(nil), candidates[0].Evidence...)

	if len(candidates) > 1 {
		margin := candidates[0].Score - candidates[1].Score
		out.Ambiguous = margin < 0.16 || candidates[0].Score < 0.60
	}
	out.NeedsClarification = out.Ambiguous || model.Signals.NeedsClarification
	out.ActionOriented = out.Primary == IntentRequestAction
	out.DecisionOriented = out.Primary == IntentDecide
	out.ProposalLike = out.Primary == IntentShareIdea ||
		(out.Ambiguous && hasCandidate(candidates, IntentShareIdea))
	out.QuestionLike = out.Primary == IntentQuestion || out.Primary == IntentAskOpinion ||
		(out.Ambiguous && (hasCandidate(candidates, IntentQuestion) || hasCandidate(candidates, IntentAskOpinion)))

	// Never turn ambiguity into an action.
	if out.Ambiguous && out.ActionOriented {
		out.NeedsClarification = true
	}

	out.Reason = "communicative intent evaluated from shared semantic evidence and conversation state"
	if out.Ambiguous {
		out.Reason = "multiple communicative interpretations remain plausible"
	}
	return out
}

func aggregateCandidates(in []IntentCandidate) []IntentCandidate {
	type aggregate struct {
		score    float64
		evidence []string
	}
	m := map[CommunicativeIntentKind]aggregate{}
	for _, c := range in {
		a := m[c.Kind]
		a.score += c.Score
		if a.score > 1 {
			a.score = 1
		}
		a.evidence = append(a.evidence, c.Evidence...)
		m[c.Kind] = a
	}
	out := make([]IntentCandidate, 0, len(m))
	for kind, a := range m {
		out = append(out, IntentCandidate{Kind: kind, Score: clamp01(a.score), Evidence: uniqueStrings(a.evidence)})
	}
	return out
}

func hasMeaningConcept(r interaction.SemanticRequest, concept interaction.SemanticConcept) bool {
	for _, c := range r.Meaning.Concepts {
		if c == concept {
			return true
		}
	}
	return false
}

func hasExplicitCommand(r interaction.SemanticRequest) bool {
	return interaction.CanonicalActs(r).Has(interaction.ActCommand) ||
		r.RequestedAction != ""
}

func hasOpenQuestion(ctx interaction.ConversationSnapshot) bool {
	if len(ctx.Turns) == 0 {
		return false
	}
	t := ctx.Turns[len(ctx.Turns)-1]
	return t.Kind == interaction.KindQuestion ||
		t.SpeechAct == interaction.SpeechQuestion ||
		strings.HasSuffix(strings.TrimSpace(t.Assistant), "?")
}

func isShortFollowUp(r interaction.SemanticRequest) bool {
	raw := strings.TrimSpace(r.Raw)
	if raw == "" {
		return false
	}
	words := strings.Fields(raw)
	return len(words) <= 8 && (r.FollowUp || r.Continues || r.AddressesCurrent)
}

func candidateScore(candidates []IntentCandidate, kind CommunicativeIntentKind) float64 {
	for _, c := range candidates {
		if c.Kind == kind {
			return c.Score
		}
	}
	return 0
}

func hasCandidate(candidates []IntentCandidate, kind CommunicativeIntentKind) bool {
	for _, c := range candidates {
		if c.Kind == kind {
			return true
		}
	}
	return false
}

func uniqueStrings(in []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}
