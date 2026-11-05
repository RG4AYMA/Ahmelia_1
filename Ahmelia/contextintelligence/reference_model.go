package contextintelligence

import (
	"ahmelia/interaction"
	"strings"
)

// B96.36.1 — Reference Model
// Reference is a derived interpretation contract. It never owns conversation state.
type ReferenceKind string

const (
	ReferenceEntity    ReferenceKind = "entity"
	ReferenceTopic     ReferenceKind = "topic"
	ReferenceObjective ReferenceKind = "objective"
	ReferenceDecision  ReferenceKind = "decision"
	ReferenceTurn      ReferenceKind = "previous_turn"
	ReferenceResult    ReferenceKind = "result"
	ReferenceWork      ReferenceKind = "work"
	ReferenceLocation  ReferenceKind = "location"
	ReferenceTemporal  ReferenceKind = "temporal"
)

type Reference struct {
	Expression string
	Kind       ReferenceKind
	Value      string
	Source     string
	Resolved   bool
	Confidence float64
}

type Model struct{}

func NewReferenceModel() *Model { return &Model{} }

func (m *Model) Build(req interaction.SemanticRequest, ctx interaction.ConversationSnapshot) []Reference {
	out := make([]Reference, 0, len(req.References)+2)
	for _, r := range req.References {
		kind, value := mapSemanticReference(r, ctx)
		out = append(out, Reference{Expression: r.Expression, Kind: kind, Value: value, Source: "interaction.SemanticRequest", Resolved: value != "", Confidence: referenceConfidence(value)})
	}
	n := strings.ToLower(req.Normalized)
	if containsAny(n, "antes", "anterior", "anteriores", "lo anterior", "hace un rato", "hace un momento") {
		out = append(out, Reference{Expression: "temporal reference", Kind: ReferenceTemporal, Value: "previous_context", Source: "reference-model", Resolved: true, Confidence: 0.90})
	}
	// Interaction owns detection of semantic references; B96.36 enriches the
	// reference contract with discourse forms that require state-aware
	// interpretation. The resolver below chooses a concrete target rather
	// than treating the phrase itself as the target.
	if containsAny(n, "esa decision", "esa decisión") {
		if len(ctx.Decisions) > 0 {
			d := ctx.Decisions[len(ctx.Decisions)-1]
			out = append(out, Reference{Expression: "esa decisión", Kind: ReferenceDecision, Value: firstNonEmpty(d.Decision, d.Topic), Source: "ConversationSnapshot.Decisions", Resolved: firstNonEmpty(d.Decision, d.Topic) != "", Confidence: 0.94})
		} else {
			out = append(out, Reference{Expression: "esa decisión", Kind: ReferenceDecision, Source: "ConversationSnapshot.Decisions", Confidence: 0.35})
		}
	}
	if containsAny(n, "lo anterior", "aquello", "esto", "that", "this") {
		if len(ctx.Turns) > 0 {
			t := ctx.Turns[len(ctx.Turns)-1]
			v := firstNonEmpty(t.Topic, t.Objective, t.UserText)
			out = append(out, Reference{Expression: "previous discourse", Kind: ReferenceTurn, Value: v, Source: "ConversationSnapshot.Turns", Resolved: v != "", Confidence: 0.88})
		}
	}
	return out
}

func mapSemanticReference(r interaction.Reference, ctx interaction.ConversationSnapshot) (ReferenceKind, string) {
	switch r.Kind {
	case interaction.ReferenceCurrentTopic:
		return ReferenceTopic, firstNonEmpty(r.Value, ctx.ActiveTopic)
	case interaction.ReferenceRecentWork:
		return ReferenceWork, firstNonEmpty(r.Value, ctx.ActiveWorkID, ctx.ActiveObjective)
	case interaction.ReferenceRecentArtifact:
		return ReferenceEntity, r.Value
	case interaction.ReferenceCurrentInput:
		return ReferenceTurn, firstNonEmpty(r.Value, ctx.LastUserInput)
	case interaction.ReferenceLocation:
		return ReferenceLocation, r.Value
	default:
		return ReferenceEntity, r.Value
	}
}
func referenceConfidence(v string) float64 {
	if strings.TrimSpace(v) != "" {
		return 0.90
	}
	return 0.35
}
func firstNonEmpty(vs ...string) string {
	for _, v := range vs {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
func containsAny(s string, vals ...string) bool {
	for _, v := range vals {
		if strings.Contains(s, v) {
			return true
		}
	}
	return false
}
