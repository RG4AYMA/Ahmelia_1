package core

import (
	"os"
	"strings"
	"testing"
)

// B96.22.31 — Architecture Audit. The audit does not pretend legacy domains
// disappeared; it verifies that the shared interaction boundary is entered
// and evaluated before legacy/domain-specific routing is allowed to continue.
func TestB9631CoreInteractionBoundaryPrecedesLegacyRouting(t *testing.T) {
	data, err := os.ReadFile("engine.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(data)
	start := strings.Index(src, "func (e *Engine) Process(input string) {")
	end := start + strings.Index(src[start:], "\n/* =======================\n   EXECUTE")
	if start < 0 || end < 0 {
		t.Fatal("Process boundaries not found")
	}
	process := src[start:end]
	phases, err := os.ReadFile("turn_pipeline.go")
	if err != nil {
		t.Fatal(err)
	}
	phaseText := string(phases)
	if !strings.Contains(phaseText, "e.analyzeSemanticBoundary(t.input, t.interactionCtx, false, &t.pendingDeferred)") {
		// The coordinator deliberately delegates semantic evaluation to the Input/Context phase.
		t.Fatal("semantic interpretation/evaluation boundary missing")
	}
	for _, marker := range []string{"classifier.ClassifyWithContext(input", "comprehension.DetectIntentWithContext(input"} {
		if strings.Contains(process, marker) {
			t.Fatalf("legacy semantic path still runs inside Process: %q", marker)
		}
	}
}

func TestB9631CoreDoesNotLetLinguisticAdapterExecute(t *testing.T) {
	data, err := os.ReadFile("engine.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(data)
	start := strings.Index(src, "func interactionNeedsLinguisticDepth")
	end := strings.Index(src, "func mergeLinguisticInterpretation")
	if start < 0 || end < 0 || end <= start {
		t.Fatal("linguistic adapter boundary helpers not found")
	}
	helper := src[start:end]
	for _, forbidden := range []string{"execute(", "e.execute(", "knowledgeEngine", "capability", "authorize"} {
		if strings.Contains(strings.ToLower(helper), strings.ToLower(forbidden)) {
			t.Fatalf("linguistic boundary helper contains forbidden decision/execution reference %q", forbidden)
		}
	}
}
