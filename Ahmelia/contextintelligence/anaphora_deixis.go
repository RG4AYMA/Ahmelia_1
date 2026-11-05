package contextintelligence

import (
	"ahmelia/interaction"
	"strings"
)

// B96.36.2 — Anaphora / Deixis
// Resolves conversational references against existing state; it does not mutate it.
type AnaphoraResolver struct{}

func NewAnaphoraResolver() *AnaphoraResolver { return &AnaphoraResolver{} }

func (r *AnaphoraResolver) Resolve(refs []Reference, req interaction.SemanticRequest, ctx interaction.ConversationSnapshot) []Reference {
	out := append([]Reference(nil), refs...)
	for i := range out {
		if out[i].Resolved {
			continue
		}
		expr := strings.ToLower(strings.TrimSpace(out[i].Expression))
		switch {
		case containsAny(expr, "current referent", "eso", "that", "aquello"):
			out[i].Kind = ReferenceTurn
			// The active topic is the strongest discourse referent. Raw last
			// input is only a fallback because it is not itself the target.
			out[i].Value = firstNonEmpty(ctx.ActiveTopic, ctx.ActiveObjective, ctx.LastUserInput, ctx.LastAssistant)
			out[i].Source = "ConversationSnapshot"
		case containsAny(expr, "current work", "recent work"):
			out[i].Kind = ReferenceWork
			out[i].Value = firstNonEmpty(ctx.ActiveWorkID, ctx.ActiveObjective, ctx.ActiveTopic)
			out[i].Source = "ConversationSnapshot"
		case containsAny(expr, "test artifact", "recent artifacts"):
			out[i].Kind = ReferenceEntity
			if len(ctx.Turns) > 0 {
				out[i].Value = firstNonEmpty(ctx.Turns[len(ctx.Turns)-1].Topic, ctx.LastUserInput)
			}
			out[i].Source = "ConversationSnapshot"
		}
		if out[i].Value != "" {
			out[i].Resolved = true
			out[i].Confidence = 0.86
		}
	}
	// Add a direct previous-turn anchor for follow-ups that contain no explicit semantic Reference.
	if (req.FollowUp || req.Continues) && len(ctx.Turns) > 0 {
		exists := false
		for _, x := range out {
			if x.Kind == ReferenceTurn && x.Value != "" {
				exists = true
				break
			}
		}
		if !exists {
			t := ctx.Turns[len(ctx.Turns)-1]
			out = append(out, Reference{Expression: "previous turn", Kind: ReferenceTurn, Value: t.UserText, Source: "ConversationSnapshot", Resolved: true, Confidence: 0.82})
		}
	}
	return out
}
