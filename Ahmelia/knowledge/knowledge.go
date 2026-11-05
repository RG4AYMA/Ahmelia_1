package knowledge

import (
	"sort"

	"ahmelia/interaction"
	"ahmelia/situation"
)

type Kind string

const (
	KindFact       Kind = "fact"
	KindOpinion    Kind = "opinion"
	KindStructured Kind = "structured"
	KindUnknown    Kind = "unknown"
)

type Result struct {
	Content    string
	Kind       Kind
	Confidence float64
	Source     string
}

// QueryFunc is the provider adapter at the typed semantic boundary.
type QueryFunc func(req interaction.SemanticRequest, ev situation.DecisionEvidence) (content string, confidence float64, ok bool)

// Source is the unique knowledge provider contract. Routing is semantic; the
// deprecated Query method exists only as a compatibility adapter for old tests.
type Source interface {
	Name() string
	Priority() int
	Handles(req interaction.SemanticRequest) (bool, float64)
	QuerySemantic(req interaction.SemanticRequest, ev situation.DecisionEvidence) (*Result, bool)
	Query(topic string) (*Result, bool)
}

type Engine struct{ sources []Source }

func NewEngine() *Engine { return &Engine{sources: []Source{}} }
func (e *Engine) Register(s Source) {
	e.sources = append(e.sources, s)
	sort.SliceStable(e.sources, func(i, j int) bool { return e.sources[i].Priority() < e.sources[j].Priority() })
}
func (e *Engine) QuerySemantic(req interaction.SemanticRequest, ev situation.DecisionEvidence) *Result {
	for _, s := range e.sources {
		handles, conf := s.Handles(req)
		if !handles {
			continue
		}
		if r, ok := s.QuerySemantic(req, ev); ok && r != nil {
			if r.Source == "" {
				r.Source = s.Name()
			}
			if r.Confidence <= 0 {
				r.Confidence = conf
			}
			return r
		}
	}
	return &Result{Kind: KindUnknown, Source: "none"}
}

// Query is a deprecated compatibility adapter. It creates the same canonical
// semantic request used by live runtime callers; no provider reparses text.
// Deprecated: use QuerySemantic. Removal target: 2026-10-31, after compatibility tests retire.
func (e *Engine) Query(topic string) *Result {
	req := interaction.Analyze(topic, interaction.Context{})
	return e.QuerySemantic(req, situation.DecisionEvidence{})
}
func (e *Engine) RegisteredSources() []string {
	out := make([]string, 0, len(e.sources))
	for _, s := range e.sources {
		out = append(out, s.Name())
	}
	return out
}
