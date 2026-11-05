package interaction

import "strings"

// TurnRelation describes how the current utterance relates to the immediately
// preceding conversational turn. It is deliberately separate from RequestKind:
// "azul" may be KindConversation syntactically but semantically be an answer
// to "¿qué color prefieres?".
type TurnRelation string

const (
	RelationIndependent  TurnRelation = "independent"
	RelationAnswer       TurnRelation = "answer"
	RelationContinuation TurnRelation = "continuation"
	RelationCorrection   TurnRelation = "correction"
	RelationNewTopic     TurnRelation = "new_topic"
)

// AnalyzeTurnRelation determines whether a turn is participating in an
// already-open conversation. It never selects a capability.
func AnalyzeTurnRelation(current SemanticRequest, previous TurnRecord, hasPrevious bool) TurnRelation {
	if !hasPrevious {
		return RelationIndependent
	}
	if current.Kind == KindCorrection || current.SpeechAct == SpeechCorrect {
		return RelationCorrection
	}

	previousAsked := previous.Kind == KindQuestion ||
		previous.SpeechAct == SpeechQuestion ||
		strings.HasSuffix(strings.TrimSpace(previous.Assistant), "?")

	if previousAsked && looksLikeAnswer(current.Raw) {
		return RelationAnswer
	}
	if current.Continues || current.FollowUp || current.AddressesCurrent {
		return RelationContinuation
	}
	if current.Topic != "" && previous.Topic != "" &&
		!strings.EqualFold(current.Topic, previous.Topic) {
		return RelationNewTopic
	}
	return RelationIndependent
}

func looksLikeAnswer(raw string) bool {
	n := strings.TrimSpace(raw)
	if n == "" {
		return false
	}
	lower := strings.ToLower(n)
	if lower == "si" || lower == "sí" || lower == "no" ||
		lower == "claro" || lower == "vale" || lower == "ok" ||
		lower == "dale" || lower == "la primera" || lower == "la segunda" ||
		lower == "the first" || lower == "the second" {
		return true
	}
	// Short noun phrases are common answers to personal or clarification
	// questions. A long independent sentence should go through normal parsing.
	return len(strings.Fields(n)) <= 5
}

// BuildAnswerAcknowledgement produces a small human-like acknowledgement
// without pretending that a capability was executed. The actual meaning is
// retained in ConversationState and can be consumed by cognition later.
func BuildAnswerAcknowledgement(answer string, question string) string {
	a := strings.TrimSpace(answer)
	q := strings.ToLower(strings.TrimSpace(question))

	switch {
	case strings.HasPrefix(q, "¿"), strings.Contains(q, "?"):
		if a == "sí" || a == "si" || a == "claro" {
			return "Perfecto, entendido."
		}
	}
	return "Perfecto, lo tengo en cuenta."
}

// PersonalAnswer is the structured result of answering one of Ahmelia's
// conversational questions about the user.
type PersonalAnswer struct {
	OK         bool
	Category   string
	Key        string
	Value      string
	Confidence float64
}

// InferPersonalAnswer connects a prior natural-language question to a short
// user answer. It is intentionally narrow and evidence-based: it never
// guesses a preference from an unrelated sentence.
func InferPersonalAnswer(previousQuestion, answer string) (PersonalAnswer, bool) {
	// Legacy compatibility helper. Live Engine paths should use
	// InferPersonalAnswerFromTurn so ownership is part of the contract.
	return inferPersonalAnswer(previousQuestion, answer, UserObservationSubjectUser)
}

// InferPersonalAnswerFromTurn links a short answer to a prior turn only when
// that prior turn explicitly belongs to the user. This is the general guard
// against learning user facts from answers to Ahmelia-owned questions.
func InferPersonalAnswerFromTurn(previous TurnRecord, answer string) (PersonalAnswer, bool) {
	if previous.Subject != UserObservationSubjectUser {
		return PersonalAnswer{}, false
	}
	return inferPersonalAnswer(previous.UserText, answer, previous.Subject)
}

func inferPersonalAnswer(previousQuestion, answer string, subject UserObservationSubject) (PersonalAnswer, bool) {
	a := strings.TrimSpace(answer)
	if a == "" || subject != UserObservationSubjectUser {
		return PersonalAnswer{}, false
	}

	// Reuse the same semantic boundary that serves direct User Model queries.
	// A short answer never gets classified by a second list of attribute
	// names; the previous question establishes the semantic key/domain and the
	// answer supplies only the value.
	req := Analyze(previousQuestion, Context{})
	if req.Subject == "" {
		req.Subject = UserObservationSubjectUser
		if req.SubjectConfidence < .75 {
			req.SubjectConfidence = .75
		}
	}
	if req.CommunicativeActs.Primary == "" {
		req.CommunicativeActs.Primary = ActQuestion
	}
	req = InferSemanticRelations(req)

	if attribute, ok := ResolveSemanticAttribute(req); ok && !attribute.HasValue && attribute.Subject == UserObservationSubjectUser {
		domain := "attribute"
		if SemanticAttributeDomainForKey(attribute.Key) == SemanticAttributeDomainPreference {
			domain = "preference"
		}
		return PersonalAnswer{true, domain, attribute.Key, a, .80}, true
	}

	// Domain-specific B96.34 queries (goal, knowledge, capability) still have
	// generic topic extraction, but they are selected from the same query
	// contract rather than a list of individual user attributes.
	if query, ok := DetectUserModelQuery(req); ok && len(query.Topics) > 0 {
		key := query.Topics[0]
		switch UserModelDomain(query.Domain) {
		case UserModelDomainGoal:
			return PersonalAnswer{true, "goal_answer", key, a, .80}, true
		case UserModelDomainCapability:
			return PersonalAnswer{true, "capability_answer", key, a, .80}, true
		case UserModelDomainKnowledge:
			return PersonalAnswer{true, "knowledge_answer", key, a, .80}, true
		}
	}

	return PersonalAnswer{}, false
}
