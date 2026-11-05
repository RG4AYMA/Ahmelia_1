package contextintelligence

import (
	"ahmelia/dialogue"
	"ahmelia/interaction"
	"strings"
)

type HistoricalBridge struct{}

func NewHistoricalBridge() *HistoricalBridge { return &HistoricalBridge{} }

type HistoricalContext struct {
	Required          bool
	Query             string
	ReferentTopic     string
	ReferentObjective string
}

func (b *HistoricalBridge) Prepare(req interaction.SemanticRequest, refs []Reference, ctx interaction.ConversationSnapshot) HistoricalContext {
	return b.PrepareWithContinuity(req, refs, ctx, Continuity{})
}

func (b *HistoricalBridge) PrepareWithContinuity(req interaction.SemanticRequest, refs []Reference, ctx interaction.ConversationSnapshot, continuity Continuity) HistoricalContext {
	q := strings.TrimSpace(req.Raw)
	// Some phrases contain temporal words such as "antes" but ask about the
	// current conversational lane, not cross-session history (for example,
	// "¿qué era lo que estábamos haciendo antes?"). Current-context recall must
	// stay in the live state/working-memory path; otherwise B96.35 is asked to
	// answer a question it should never own.
	currentContextRecall := isCurrentContextRecall(q)
	required := !currentContextRecall && (dialogue.IsHistoricalRecallIntent(q) || hasHistoricalReference(q, refs))
	if continuity.Kind == ContinuityNewTopic {
		// A topic-break turn must not inherit the previous lane merely because
		// it contains a deictic word such as "eso".
		return HistoricalContext{Required: dialogue.IsHistoricalRecallIntent(q), Query: q}
	}
	topic, obj := ctx.ActiveTopic, ctx.ActiveObjective
	if len(ctx.Turns) > 0 {
		t := ctx.Turns[len(ctx.Turns)-1]
		if t.Topic != "" {
			topic = t.Topic
		}
		if t.Objective != "" {
			obj = t.Objective
		}
	}
	for _, r := range refs {
		if !r.Resolved || strings.TrimSpace(r.Value) == "" {
			continue
		}
		switch r.Kind {
		case ReferenceTopic, ReferenceEntity, ReferenceDecision, ReferenceTurn:
			if topic == "" {
				topic = r.Value
			}
		}
	}
	if required {
		q = dialogue.ResolveHistoricalReferenceQuery(q, topic, obj)
	}
	return HistoricalContext{Required: required, Query: q, ReferentTopic: topic, ReferentObjective: obj}
}
func isCurrentContextRecall(q string) bool {
	n := strings.ToLower(strings.TrimSpace(q))
	for _, phrase := range []string{
		"qué estábamos haciendo", "que estabamos haciendo",
		"qué hacíamos", "que haciamos",
		"qué estábamos trabajando", "que estabamos trabajando",
		"qué era lo que estábamos haciendo", "que era lo que estabamos haciendo",
		"qué era lo que hacíamos", "que era lo que haciamos",
		"what were we doing", "what were we working on",
	} {
		if strings.Contains(n, phrase) {
			return true
		}
	}
	return false
}

func hasHistoricalReference(q string, refs []Reference) bool {
	n := strings.ToLower(q)
	if containsAny(n, "lo anterior", "esa decision", "ese problema", "ese proyecto", "aquello", "esa solucion", "esa solución") {
		return true
	}
	for _, r := range refs {
		if r.Kind == ReferenceTemporal {
			return true
		}
	}
	return false
}
