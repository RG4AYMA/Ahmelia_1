package knowledge

import (
	"os"
	"strings"
	"testing"

	"ahmelia/interaction"
	"ahmelia/situation"
)

type semanticProbeSource struct {
	called bool
}

func (s *semanticProbeSource) Name() string  { return "semantic_probe" }
func (s *semanticProbeSource) Priority() int { return 1 }
func (s *semanticProbeSource) Handles(req interaction.SemanticRequest) (bool, float64) {
	// Whitelist: routing is intentionally derived from the typed Meaning and
	// semantic fields; Raw/Normalized are forbidden as routing inputs.
	return semanticSourceHandles(req, interaction.QueryEnvironmentResources)
}
func (s *semanticProbeSource) QuerySemantic(req interaction.SemanticRequest, _ situation.DecisionEvidence) (*Result, bool) {
	s.called = true
	return &Result{Content: req.Meaning.Attribute, Kind: KindFact, Confidence: 0.9, Source: s.Name()}, true
}
func (s *semanticProbeSource) Query(topic string) (*Result, bool) {
	return nil, false
}

func TestNoPhraseGateKnowledgeRouting(t *testing.T) {
	data, err := os.ReadFile("sources.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(data)
	// Whitelist: source routing must flow through semanticSourceHandles and the
	// typed request. No provider Handles implementation may inspect raw user text.
	if !strings.Contains(src, "func semanticSourceHandles(req interaction.SemanticRequest") {
		t.Fatal("canonical semantic source routing helper missing")
	}
	for _, forbidden := range []string{"req.Raw", "req.Normalized"} {
		if strings.Contains(src, forbidden) {
			t.Fatalf("knowledge source routing contains forbidden raw semantic access %q", forbidden)
		}
	}
}

func TestQuerySemanticRoutesWithoutRawText(t *testing.T) {
	probe := &semanticProbeSource{}
	e := NewEngine()
	e.Register(probe)
	req := interaction.Analyze("cuanta ram esta usando", interaction.Context{})
	// Deliberately corrupt the raw text after semantic interpretation. Routing
	// must remain stable because the source sees only the typed meaning.
	req.Raw = "totalmente otra cosa"
	result := e.QuerySemantic(req, situation.DecisionEvidence{})
	if result == nil || result.Content != "ram" {
		t.Fatalf("typed routing failed: %+v", result)
	}
	if !probe.called {
		t.Fatal("typed source was not called")
	}
}
