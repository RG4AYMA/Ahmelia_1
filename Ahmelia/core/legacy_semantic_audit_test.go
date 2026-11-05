package core

import (
	"os"
	"strings"
	"testing"
)

// B96.32.1 — executable inventory of the remaining legacy semantic paths.
// The legacy packages remain for compatibility, but Process must not route
// human turns through them after the shared Interaction + Semantic Evaluation boundary.
func TestB96321LegacySemanticPathsRemainAfterSemanticBoundary(t *testing.T) {
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
	for _, marker := range []string{"result := classifier.ClassifyWithContext(input, hint)", "intent := comprehension.DetectIntentWithContext(input, e.context.GetCurrentTopic())"} {
		if strings.Contains(process, marker) {
			t.Fatalf("legacy path still executes in Process: %q", marker)
		}
	}
}

func TestB96321MainProcessUsesSemanticRequestBeforeProductionFallback(t *testing.T) {
	data, err := os.ReadFile("engine.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(data)

	if !strings.Contains(src, "e.inputContextPhase(t)") || !strings.Contains(src, "e.preExecutionPhase(t)") {
		t.Fatal("Process no longer delegates semantic input/context and execution routing to the dedicated phases")
	}
	if strings.Contains(src, "e.handleProductionRuntimeInput(input)") {
		t.Fatal("Process must not call the legacy raw production router")
	}
}

func TestB96321ConversationStateIsNotRebuiltByCoreLegacyRouter(t *testing.T) {
	data, err := os.ReadFile("engine.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(data)
	if strings.Contains(src, "NewConversationState()") {
		t.Fatal("core Process must not rebuild ConversationState per turn")
	}
}

func TestB963213LegacyRoutingMigrationKeepsProcessAsCoordinator(t *testing.T) {
	data, err := os.ReadFile("engine.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(data)
	start := strings.Index(src, "func (e *Engine) Process(input string) {")
	end := start + strings.Index(src[start:], "\n}\n\n/* =======================\n   EXECUTE")
	if start < 0 || end <= start {
		t.Fatal("Process boundaries not found")
	}
	process := src[start:end]

	for _, forbidden := range []string{
		"classifier.ClassifyWithContext(",
		"classifier.ClassifyCommand(",
		"comprehension.DetectIntentWithContext(",
		"e.productionRuntime.Handle(",
		"e.handleProductionRuntimeInput(",
		"interaction.Analyze(input, interaction.Context{Language:",
		"e.authorityExecute(",
		"e.sayAsync(",
	} {
		if strings.Contains(process, forbidden) {
			t.Fatalf("runtime terminal/legacy routing still executes in Process: %q", forbidden)
		}
	}

	required := []string{
		"e.inputContextPhase(t)",
		"e.situationPhase(t)",
		"e.cognitionPhase(t)",
		"e.dialoguePhase(t)",
		"e.decisionPhase(t)",
		"e.preExecutionPhase(t)",
		"e.finishExecutionPhase(t)",
	}
	for _, marker := range required {
		if !strings.Contains(process, marker) {
			t.Fatalf("Phase 8 coordinator marker missing: %q", marker)
		}
	}
}
