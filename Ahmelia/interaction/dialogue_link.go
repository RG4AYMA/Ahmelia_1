package interaction

import (
	"strings"
	"unicode"
)

// DialogueLink is the semantic relationship between a new utterance and
// existing dialogue state. It deliberately lives above capabilities and below
// cognition: it answers "what is this turn doing in the conversation?" without
// deciding what Ahmelia should execute.
type DialogueLink struct {
	Relation     TurnRelation
	ParentTurnID uint64
	Confidence   float64
	Reason       string
}

// LinkTurn performs a scored, conservative relation analysis. Unlike the old
// short-answer heuristic, it considers explicit discourse markers, question
// type, topic continuity and the existence of an active objective.
func LinkTurn(current SemanticRequest, previous TurnRecord, hasPrevious bool, ctx ConversationSnapshot) DialogueLink {
	if !hasPrevious {
		return DialogueLink{Relation: RelationIndependent, Confidence: 1}
	}
	if CanonicalActs(current).Has(ActCorrection) {
		return DialogueLink{Relation: RelationCorrection, ParentTurnID: previous.ID, Confidence: 0.98, Reason: "explicit correction"}
	}

	previousActs := turnCommunicativeActs(previous)
	previousQuestion := previousActs.Has(ActQuestion) || strings.HasSuffix(strings.TrimSpace(previous.Assistant), "?")

	if previousQuestion {
		currentActs := CanonicalActs(current)
		if currentActs.Has(ActQuestion) || currentActs.Has(ActOpinionRequest) {
			return DialogueLink{Relation: RelationNewTopic, ParentTurnID: previous.ID, Confidence: 0.96, Reason: "new explicit question overrides answer interpretation"}
		}
		if isExplicitNewCommand(current.Raw) {
			return DialogueLink{Relation: RelationNewTopic, Confidence: 0.96, Reason: "explicit new command overrides answer interpretation"}
		}
		if answerConfidence(current, previous) >= 0.70 {
			return DialogueLink{Relation: RelationAnswer, ParentTurnID: previous.ID, Confidence: answerConfidence(current, previous), Reason: "turn is a plausible answer to the previous question"}
		}
	}

	if current.Continues || current.FollowUp || current.AddressesCurrent {
		return DialogueLink{Relation: RelationContinuation, ParentTurnID: previous.ID, Confidence: 0.90, Reason: "explicit continuation/current-context signal"}
	}

	if sameTopic(current.Topic, previous.Topic) && current.Topic != "" {
		return DialogueLink{Relation: RelationContinuation, ParentTurnID: previous.ID, Confidence: 0.82, Reason: "same conversational topic"}
	}

	if ctx.ActiveObjective != "" && looksLikeShortObjectiveContinuation(current.Raw) {
		return DialogueLink{Relation: RelationContinuation, ParentTurnID: previous.ID, Confidence: 0.78, Reason: "short turn fits active objective"}
	}

	if current.Topic != "" && previous.Topic != "" && !sameTopic(current.Topic, previous.Topic) {
		return DialogueLink{Relation: RelationNewTopic, ParentTurnID: previous.ID, Confidence: 0.86, Reason: "explicit topic shift"}
	}
	return DialogueLink{Relation: RelationIndependent, Confidence: 0.72, Reason: "insufficient evidence for linkage"}
}

// ResolveAnswerSemantics attaches an answer to the question that made it
// meaningful. It does not invent the answer; it only preserves it as a
// semantic decision/evidence and, for work questions, promotes the turn into
// a continuation so downstream work receives the combined objective.
func ResolveAnswerSemantics(current SemanticRequest, previous TurnRecord) SemanticRequest {
	answer := strings.TrimSpace(current.Raw)
	if answer == "" {
		return current
	}
	base := strings.TrimSpace(previous.Objective)
	if base == "" {
		base = strings.TrimSpace(previous.UserText)
	}

	current.FollowUp = true
	current.Continues = true
	current.AddressesCurrent = true

	// A short answer to a work-oriented question is not a new standalone
	// objective. Preserve the original objective and add the user's decision
	// as a constraint/decision for cognition.
	if IsWorkQuestion(previous) && base != "" {
		current.Kind = KindContinuation
		current.SpeechAct = SpeechContinue
		current.Objective.Text = base
		current.Objective.Constraints = appendUnique(current.Objective.Constraints, "user_decision: "+answer)
		current.RequestedAction = "continue"
		current.ExpectedResponse = "continuation_or_result"
		current.Confidence = maxFloat(current.Confidence, 0.92)
		current.Meaning = SemanticMeaning{}
		current.CommunicativeActs = ClassifyCommunicativeActs(current)
		current.Meaning = BuildSemanticMeaning(current)
		current.CommunicativeActs = ClassifyCommunicativeActs(current)
		return current
	}

	// A general conversational answer remains conversational. Its raw value
	// is retained in TurnRecord and can be learned by UserProfile or cognition.
	acts := CanonicalActs(current)
	if !acts.Has(ActCommand) && !acts.Has(ActQuestion) && !acts.Has(ActControl) {
		current.Kind = KindConversation
		current.SpeechAct = SpeechInform
		current.ExpectedResponse = "conversation"
		current.CommunicativeActs = acts
	}
	current.Meaning = SemanticMeaning{}
	current.CommunicativeActs = ClassifyCommunicativeActs(current)
	current.Meaning = BuildSemanticMeaning(current)
	current.CommunicativeActs = ClassifyCommunicativeActs(current)
	return current
}

func IsWorkQuestion(t TurnRecord) bool {
	obj := strings.TrimSpace(t.Objective)
	if obj != "" {
		return true
	}
	n := strings.ToLower(strings.TrimSpace(t.UserText))
	return containsAny(n,
		"qué priorizamos", "que priorizamos", "qué hacemos", "que hacemos",
		"qué quieres que", "que quieres que", "qué parte", "que parte",
		"which should", "what should", "which part")
}

func turnCommunicativeActs(previous TurnRecord) CommunicativeActs {
	if previous.CommunicativeActs.Primary != "" {
		return previous.CommunicativeActs
	}
	// Detached compatibility only: persisted/hand-built legacy records may
	// predate CommunicativeActs. Live turns are always recorded with the
	// canonical contract.
	return CanonicalActs(SemanticRequest{Raw: previous.UserText, Normalized: normalize(previous.UserText), Kind: previous.Kind, SpeechAct: previous.SpeechAct})
}

func answerConfidence(current SemanticRequest, previous TurnRecord) float64 {
	n := strings.TrimSpace(strings.ToLower(current.Raw))
	if n == "" {
		return 0
	}
	if n == "sí" || n == "si" || n == "no" || n == "claro" || n == "vale" ||
		n == "ok" || n == "okay" || n == "la primera" || n == "la segunda" ||
		n == "the first" || n == "the second" {
		return 0.99
	}
	// A direct answer to a person/location/quantity question often contains
	// a noun phrase with no verb. Limit this heuristic to compact utterances.
	fields := strings.Fields(n)
	if len(fields) <= 5 && !isExplicitNewCommand(current.Raw) {
		if turnCommunicativeActs(previous).Has(ActQuestion) {
			return 0.78
		}
	}
	return 0.20
}

func isExplicitNewCommand(raw string) bool {
	n := strings.ToLower(strings.TrimSpace(raw))
	if n == "" {
		return false
	}
	// Keep this intentionally broad only for clear imperative openings. It
	// prevents "abre Chrome" from being swallowed as an answer to a question.
	for _, prefix := range []string{
		"abre ", "abrir ", "ejecuta ", "ejecutar ", "crea ", "crear ",
		"genera ", "generar ", "haz ", "hacer ", "mueve ", "mover ",
		"renombra ", "renombrar ", "corrige ", "corregir ",
		"open ", "launch ", "start ", "run ", "create ", "make ",
		"build ", "fix ", "move ", "rename ",
	} {
		if strings.HasPrefix(n, prefix) {
			return true
		}
	}
	return false
}

func looksLikeShortObjectiveContinuation(raw string) bool {
	n := strings.TrimSpace(raw)
	if n == "" || len(strings.Fields(n)) > 8 {
		return false
	}
	// Reject punctuation-only fragments and obvious social turns.
	for _, r := range n {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return true
		}
	}
	return false
}

func sameTopic(a, b string) bool {
	return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b))
}

func appendUnique(values []string, value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return values
	}
	for _, v := range values {
		if strings.EqualFold(strings.TrimSpace(v), value) {
			return values
		}
	}
	return append(values, value)
}

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
