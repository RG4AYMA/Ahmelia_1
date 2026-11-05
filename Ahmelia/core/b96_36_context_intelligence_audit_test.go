package core

import (
	"os"
	"strings"
	"testing"
)

func TestB9636EngineHasSingleContextIntelligenceBridge(t *testing.T) {
	text, err := os.ReadFile("engine.go")
	if err != nil {
		t.Fatal(err)
	}
	s := string(text)
	if strings.Count(s, "contextIntelligence.Resolve(") != 1 {
		t.Fatalf("expected exactly one B96.36 resolution implementation point, got %d", strings.Count(s, "contextIntelligence.Resolve("))
	}
	if strings.Count(s, "contextintelligence.New()") != 1 {
		t.Fatalf("expected exactly one B96.36 owner instance, got %d", strings.Count(s, "contextintelligence.New()"))
	}
	if !strings.Contains(s, "contextintelligence.ApplyToSemanticRequest(req, resolution)") {
		t.Fatal("B96.36 result is not projected into the semantic contract")
	}
}

func TestB9636DoesNotCreateHistoricalRetrievalAuthority(t *testing.T) {
	for _, name := range []string{"../contextintelligence/historical_bridge.go", "../contextintelligence/gate.go"} {
		text, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		s := string(text)
		if strings.Contains(s, ".SearchExcluding(") || strings.Contains(s, ".Reconstruct(") || strings.Contains(s, "ResolveHistoricalRecall(") || strings.Contains(s, "ResolveHistoricalRecallExcluding(") {
			t.Fatalf("B96.36 duplicated B96.35 historical retrieval in %s", name)
		}
	}
}
