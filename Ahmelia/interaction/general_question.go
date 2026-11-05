package interaction

import "strings"

// GeneralQuestionRoute describes what kind of answer a general question
// requires. It is a semantic policy only: it does not select a provider,
// capability, tool, or external service.
type GeneralQuestionRoute string

const (
	QuestionRouteDirect    GeneralQuestionRoute = "direct"
	QuestionRouteContext   GeneralQuestionRoute = "context"
	QuestionRouteKnowledge GeneralQuestionRoute = "knowledge"
	QuestionRouteReasoning GeneralQuestionRoute = "reasoning"
	QuestionRouteClarify   GeneralQuestionRoute = "clarify"
)

// GeneralQuestionDecision is the B96.22.28 contract. It tells the Core what
// kind of cognitive treatment a question needs while keeping the final
// answer and authority outside this layer.
type GeneralQuestionDecision struct {
	Route        GeneralQuestionRoute
	QuestionType string
	Topic        string
	Confidence   float64
	Reason       string
}

// ClassifyGeneralQuestion performs a conservative, domain-neutral analysis of
// a question. The important distinction is between "I understand the
// question" and "I already know the answer". Knowledge availability belongs
// to the Knowledge Core, not this classifier.
func ClassifyGeneralQuestion(req SemanticRequest, ctx Context) GeneralQuestionDecision {
	acts := CanonicalActs(req)
	if !acts.Has(ActQuestion) && !acts.Has(ActOpinionRequest) {
		return GeneralQuestionDecision{Route: QuestionRouteDirect, Confidence: 1, Reason: "turn is not a general question"}
	}

	t := strings.TrimSpace(req.QuestionType)
	if t == "" {
		t = inferQuestionType(req.Normalized)
	}
	topic := strings.TrimSpace(req.Topic)
	n := strings.ToLower(strings.TrimSpace(req.Normalized))

	// Questions whose answer is already present in the live conversation or
	// operational context should not be sent to an external knowledge source.
	if isContextQuestion(n, ctx) || hasReference(req, ReferenceCurrentTopic) || hasReference(req, ReferenceRecentWork) {
		return GeneralQuestionDecision{Route: QuestionRouteContext, QuestionType: t, Topic: topic, Confidence: 0.94, Reason: "answer may be resolved from current conversational/work context"}
	}

	// Very short or elliptical questions often need clarification rather than
	// an invented interpretation. A question mark alone is not enough to force
	// clarification: common one-line questions remain valid.
	if needsQuestionClarification(req) {
		return GeneralQuestionDecision{Route: QuestionRouteClarify, QuestionType: t, Topic: topic, Confidence: 0.82, Reason: "question lacks enough semantic material for a reliable answer"}
	}

	if isComplexGeneralQuestion(req) {
		return GeneralQuestionDecision{Route: QuestionRouteReasoning, QuestionType: t, Topic: topic, Confidence: 0.84, Reason: "question contains an explicit multi-step/causal/problem-solving demand"}
	}

	return GeneralQuestionDecision{Route: QuestionRouteKnowledge, QuestionType: t, Topic: topic, Confidence: 0.90, Reason: "question is well-formed and may be answered from available knowledge"}
}

func isContextQuestion(n string, ctx Context) bool {
	if n == "" {
		return false
	}
	markers := []string{
		"que estabamos", "qué estábamos", "que estabamos haciendo",
		"qué estábamos haciendo", "que hicimos", "qué hicimos",
		"que haces", "qué haces", "que estas haciendo", "qué estás haciendo",
		"que sigue", "qué sigue", "que falta", "qué falta",
		"what were we doing", "what did we do", "what are you doing", "what's next",
	}
	for _, m := range markers {
		if strings.Contains(n, m) {
			return true
		}
	}
	return strings.TrimSpace(ctx.ActiveObjective) != "" &&
		(strings.Contains(n, "este proyecto") || strings.Contains(n, "este trabajo") || strings.Contains(n, "this project") || strings.Contains(n, "this work"))
}

func needsQuestionClarification(req SemanticRequest) bool {
	n := strings.TrimSpace(req.Normalized)
	if n == "" {
		return true
	}
	words := strings.Fields(n)
	if len(words) == 1 {
		return false // "¿por qué?" is still a legitimate conversational question.
	}
	if len(words) <= 2 && req.Topic == "" && req.QuestionType == "" && !hasReference(req, ReferenceCurrentInput) {
		return true
	}
	return false
}

func isComplexGeneralQuestion(req SemanticRequest) bool {
	n := strings.ToLower(strings.TrimSpace(req.Normalized))
	if len(strings.Fields(n)) >= 12 {
		return true
	}
	return containsAny(n,
		"necesito automatizar", "necesito diseñar", "necesito resolver",
		"por que deberia", "por qué debería", "como puedo", "cómo puedo",
		"como funciona", "cómo funciona", "cual seria la mejor", "cuál sería la mejor",
		"que pasaria si", "qué pasaría si", "what should i", "how can i", "how does", "what would happen",
	)
}
