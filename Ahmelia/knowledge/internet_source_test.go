package knowledge

import (
	"testing"

	"ahmelia/interaction"
	"ahmelia/situation"
)

type nonHandlingSource struct{}

func (nonHandlingSource) Name() string                                        { return "non_handling" }
func (nonHandlingSource) Priority() int                                       { return 10 }
func (nonHandlingSource) Handles(interaction.SemanticRequest) (bool, float64) { return false, 0 }
func (nonHandlingSource) QuerySemantic(interaction.SemanticRequest, situation.DecisionEvidence) (*Result, bool) {
	return nil, false
}
func (nonHandlingSource) Query(string) (*Result, bool) { return nil, false }

func TestInternetSourceEnabledByControlledConfig(t *testing.T) {
	cases := []struct {
		value string
		want  bool
	}{
		{"", false},
		{"0", false},
		{"true", true},
		{"ON", true},
		{" enabled ", true},
	}
	for _, tc := range cases {
		t.Run(tc.value, func(t *testing.T) {
			got := InternetSourceEnabled(func(string) string { return tc.value })
			if got != tc.want {
				t.Fatalf("InternetSourceEnabled(%q) = %v, want %v", tc.value, got, tc.want)
			}
		})
	}
}

func TestInternetSourceRoutesOnlySearchThroughKnowledgeEngine(t *testing.T) {
	calls := 0
	source := NewInternetSourceWithConfig(func(req interaction.SemanticRequest, _ situation.DecisionEvidence) (string, float64, bool) {
		calls++
		return req.Topic, 0.91, true
	}, InternetSourceConfig{Enabled: true})

	engine := NewEngine()
	engine.Register(nonHandlingSource{})
	engine.Register(source)

	search := interaction.Analyze("busca información sobre Go", interaction.Context{})
	result := engine.QuerySemantic(search, situation.DecisionEvidence{})
	if result == nil || result.Source != "internet" || result.Content != "go" {
		t.Fatalf("search result = %+v, want internet/go result", result)
	}
	if calls != 1 {
		t.Fatalf("internet provider calls = %d, want 1", calls)
	}

	nonSearch := interaction.Analyze("¿qué hicimos?", interaction.Context{})
	before := calls
	result = engine.QuerySemantic(nonSearch, situation.DecisionEvidence{})
	if result == nil || result.Source == "internet" {
		t.Fatalf("non-search result unexpectedly came from internet: %+v", result)
	}
	if calls != before {
		t.Fatalf("internet provider calls changed for non-search request: %d -> %d", before, calls)
	}
}

func TestInternetSourceDisabledFailsClosed(t *testing.T) {
	calls := 0
	source := NewInternetSourceWithConfig(func(interaction.SemanticRequest, situation.DecisionEvidence) (string, float64, bool) {
		calls++
		return "should not run", 1, true
	}, InternetSourceConfig{Enabled: false})
	engine := NewEngine()
	engine.Register(source)

	result := engine.QuerySemantic(interaction.Analyze("busca Go", interaction.Context{}), situation.DecisionEvidence{})
	if result == nil || result.Kind != KindUnknown || result.Source == "internet" {
		t.Fatalf("disabled internet result = %+v, want unknown/non-internet", result)
	}
	if calls != 0 {
		t.Fatalf("disabled internet provider calls = %d, want 0", calls)
	}
}
