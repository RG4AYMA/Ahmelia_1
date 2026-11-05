package contextintelligence

import (
	"ahmelia/interaction"
	"strings"
	"time"
)

type ContextSource string

const (
	SourceCurrent    ContextSource = "current"
	SourceRecent     ContextSource = "recent"
	SourceHistorical ContextSource = "historical"
)

type Candidate struct {
	Source ContextSource
	Value  string
	Score  float64
	Reason string
}
type ContextModel struct{}

func NewContextModel() *ContextModel { return &ContextModel{} }

func (m *ContextModel) Select(req interaction.SemanticRequest, refs []Reference, ctx interaction.ConversationSnapshot, now time.Time, continuity Continuity) []Candidate {
	out := make([]Candidate, 0, 6)
	if continuity.Kind == ContinuityNewTopic {
		// An explicit topic boundary makes the old active/recent lane
		// ineligible for automatic selection. Historical context remains
		// available only when the current request explicitly asks for it.
	} else if ctx.ActiveTopic != "" || ctx.ActiveObjective != "" {
		out = append(out, Candidate{Source: SourceCurrent, Value: firstNonEmpty(ctx.ActiveTopic, ctx.ActiveObjective), Score: 0.95, Reason: "active conversational focus"})
	}
	if continuity.Kind != ContinuityNewTopic && len(ctx.Turns) > 0 {
		t := ctx.Turns[len(ctx.Turns)-1]
		age := now.Sub(t.At)
		score := 0.80
		if age < 2*time.Minute {
			score = 0.92
		} else if age > 30*time.Minute {
			score = 0.65
		}
		out = append(out, Candidate{Source: SourceRecent, Value: firstNonEmpty(t.Topic, t.Objective, t.UserText), Score: score, Reason: "recent semantic turn"})
	}
	for _, r := range refs {
		if r.Resolved && strings.TrimSpace(r.Value) != "" && r.Kind != ReferenceTemporal {
			score := 0.91
			if continuity.Kind == ContinuityContinue {
				score = 0.94
			}
			out = append(out, Candidate{Source: SourceCurrent, Value: r.Value, Score: score, Reason: "resolved conversational reference"})
		}
	}
	for _, r := range refs {
		if r.Kind == ReferenceTemporal || containsAny(strings.ToLower(req.Normalized), "anterior", "antes", "otra conversacion", "historial", "hace tiempo") {
			out = append(out, Candidate{Source: SourceHistorical, Value: firstNonEmpty(r.Value, req.Normalized), Score: 0.88, Reason: "historical/temporal reference"})
			break
		}
	}
	return out
}
