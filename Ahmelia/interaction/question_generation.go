package interaction

import (
	"fmt"
	"strings"
	"time"
)

// QuestionPurpose describes why Ahmelia is asking. It is deliberately
// domain-neutral: a question can serve conversation, clarification, learning,
// reflection, or continuation without knowing which capability is involved.
type QuestionPurpose string

const (
	QuestionPurposeClarify  QuestionPurpose = "clarify"
	QuestionPurposeContinue QuestionPurpose = "continue"
	QuestionPurposeLearn    QuestionPurpose = "learn"
	QuestionPurposeReflect  QuestionPurpose = "reflect"
	QuestionPurposeConfirm  QuestionPurpose = "confirm"
	QuestionPurposeExplore  QuestionPurpose = "explore"
)

// QuestionCandidate is a semantic question proposal. It is not yet a spoken
// response: response planning and linguistic expression remain downstream.
type QuestionCandidate struct {
	Purpose    QuestionPurpose
	Category   string
	Key        string
	Question   string
	Reason     string
	Priority   float64
	Confidence float64
	Topic      string
	CreatedAt  time.Time
}

// QuestionGenerationContext contains the evidence available to B96.22.21.
// The generator never calls a capability and never depends on a domain.
type QuestionGenerationContext struct {
	Request  SemanticRequest
	Snapshot ConversationSnapshot
	Profile  *UserProfile
}

// QuestionGenerator turns conversational evidence into one or more useful
// questions. It intentionally does not use a fixed questionnaire as its
// primary mechanism: candidates are derived from current gaps, active work,
// open threads, commitments, and the user's latest turn.
type QuestionGenerator struct {
	MaxCandidates int
}

func DefaultQuestionGenerator() QuestionGenerator {
	return QuestionGenerator{MaxCandidates: 3}
}

// Generate produces ranked conversational questions. Immediate user needs
// are excluded here as a second safety boundary; initiative policy remains
// responsible for deciding whether any candidate should actually be spoken.
func (g QuestionGenerator) Generate(ctx QuestionGenerationContext) []QuestionCandidate {
	max := g.MaxCandidates
	if max <= 0 {
		max = 3
	}

	var out []QuestionCandidate
	req := ctx.Request
	s := ctx.Snapshot

	// A direct question/command/correction should be answered or handled by
	// the normal response path, not competed with by a generated question.
	acts := CanonicalActs(req)
	if acts.Has(ActQuestion) || acts.Has(ActCommand) || acts.Has(ActCorrection) || acts.Has(ActControl) || req.NeedsClarification {
		return nil
	}

	// 1. Continue an active conversational objective when a useful choice or
	// missing detail is visible in the current semantic context.
	if strings.TrimSpace(s.ActiveObjective) != "" {
		topic := strings.TrimSpace(s.ActiveTopic)
		if topic == "" {
			topic = strings.TrimSpace(req.Topic)
		}
		q := fmt.Sprintf("¿Qué parte de %s te gustaría explorar o cambiar ahora?", topic)
		if topic == "" {
			q = "¿Qué parte de lo que estamos haciendo te gustaría explorar o cambiar ahora?"
		}
		out = append(out, QuestionCandidate{
			Purpose: QuestionPurposeContinue, Question: q,
			Reason:   "active objective can be advanced through an explicit conversational choice",
			Priority: 0.58, Confidence: 0.70, Topic: topic, CreatedAt: time.Now(),
		})
	}

	// 2. Ask about an open commitment only when it is not already the focus.
	// This keeps unfinished conversational material alive without pretending
	// every open commitment deserves immediate interruption.
	for _, c := range s.Commitments {
		if len(out) >= max {
			break
		}
		if c.Status != "open" || strings.TrimSpace(c.Text) == "" {
			continue
		}
		q := fmt.Sprintf("Sobre lo que dejamos pendiente de %s, ¿quieres que lo retomemos ahora?", strings.TrimSpace(c.Text))
		out = append(out, QuestionCandidate{
			Purpose: QuestionPurposeContinue, Question: q,
			Reason:   "an open conversational commitment can be resumed",
			Priority: 0.54, Confidence: 0.74, Topic: s.ActiveTopic, CreatedAt: time.Now(),
		})
		break
	}

	// 3. Learn a missing personal fact. The facts are represented as semantic
	// slots, while the wording is composed with current context. This makes
	// the mechanism extensible without forcing a questionnaire into the core.
	if ctx.Profile != nil && len(out) < max {
		if q, ok := generateLearningQuestion(ctx.Profile, s); ok {
			out = append(out, q)
		}
	}

	// 4. If the user just stated something interesting but no profile slot
	// applies, invite elaboration rather than inventing a fact.
	if len(out) < max && strings.TrimSpace(req.Raw) != "" &&
		(CanonicalActs(req).Has(ActSocial) || CanonicalActs(req).Has(ActInform) || CanonicalActs(req).Primary == ActUnknown) {
		topic := strings.TrimSpace(req.Topic)
		if topic == "" {
			topic = strings.TrimSpace(s.ActiveTopic)
		}
		q := "¿Quieres contarme un poco más sobre eso?"
		if topic != "" {
			q = fmt.Sprintf("¿Quieres contarme un poco más sobre %s?", topic)
		}
		out = append(out, QuestionCandidate{
			Purpose: QuestionPurposeExplore, Question: q,
			Reason:   "the latest turn offers conversational material that can be explored",
			Priority: 0.46, Confidence: 0.66, Topic: topic, CreatedAt: time.Now(),
		})
	}

	return rankQuestions(out, max)
}

func generateLearningQuestion(profile *UserProfile, ctx ConversationSnapshot) (QuestionCandidate, bool) {
	if profile == nil {
		return QuestionCandidate{}, false
	}

	// B96.34 is an unbounded semantic key/value model. There is no fixed
	// questionnaire of attributes here. A proactive learning prompt is allowed
	// when the profile is empty; an existing active objective still owns the
	// turn because it already has a stronger continuation path.
	if strings.TrimSpace(ctx.ActiveObjective) != "" {
		return QuestionCandidate{}, false
	}
	snap := profile.Snapshot()
	if len(snap.Preferences)+len(snap.Habits)+len(snap.Interests)+len(snap.Routines)+len(snap.Communication)+len(snap.Goals)+len(snap.Notes) > 0 {
		return QuestionCandidate{}, false
	}

	topic := strings.TrimSpace(ctx.ActiveTopic)
	question := "¿Hay algo sobre ti que quieras que recuerde para futuras conversaciones?"
	if topic != "" {
		question = fmt.Sprintf("¿Hay algo importante para ti sobre %s que quieras que recuerde para futuras conversaciones?", topic)
	}
	return QuestionCandidate{
		Purpose:  QuestionPurposeLearn,
		Category: string(UserModelDomainAttribute),
		Key:      "user_attribute",
		Question: question,
		Reason:   "the user model is empty and can be enriched without assuming which attribute matters",
		Priority: 0.52, Confidence: 0.80, Topic: topic, CreatedAt: time.Now(),
	}, true
}

func rankQuestions(in []QuestionCandidate, max int) []QuestionCandidate {
	for i := 1; i < len(in); i++ {
		v := in[i]
		j := i - 1
		for ; j >= 0 && in[j].Priority < v.Priority; j-- {
			in[j+1] = in[j]
		}
		in[j+1] = v
	}
	if len(in) > max {
		in = in[:max]
	}
	return in
}

func lowerFirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToLower(s[:1]) + s[1:]
}

// GenerateQuestion is the compact single-question API used by initiative and
// future response planners.
func GenerateQuestion(ctx QuestionGenerationContext) (QuestionCandidate, bool) {
	candidates := DefaultQuestionGenerator().Generate(ctx)
	if len(candidates) == 0 {
		return QuestionCandidate{}, false
	}
	return candidates[0], true
}
