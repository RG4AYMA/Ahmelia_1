package core

import (
	"os"
	"strings"
	"testing"

	"ahmelia/interaction"
)

// B96.32.2 — Process consumes the shared SemanticRequest contract instead of
// re-running classifier/comprehension over the same raw turn.
func TestB96322ProcessUsesSemanticRequestAsSingleInteractionContract(t *testing.T) {
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
	required := []string{
		"e.inputContextPhase(t)",
		"e.situationPhase(t)",
		"e.cognitionPhase(t)",
		"e.dialoguePhase(t)",
		"e.decisionPhase(t)",
		"e.preExecutionPhase(t)",
	}
	for _, marker := range required {
		if !strings.Contains(process, marker) {
			t.Fatalf("Process coordinator lost phase marker %q", marker)
		}
	}
	for _, marker := range []string{
		"classifier.ClassifyWithContext(",
		"classifier.ClassifyCommand(",
		"comprehension.DetectIntentWithContext(",
		"interaction.ResolveCapabilityInteraction(",
		"route := e.capabilityResolver.Resolve(semanticReq.Raw)",
		"e.authorityExecute(",
	} {
		if strings.Contains(process, marker) {
			t.Fatalf("Process still contains embedded semantic/execution terminal %q", marker)
		}
	}
}

func TestB96322SemanticActionCarriesExecutionParameters(t *testing.T) {
	// The semantic contract owns extraction of parameters; capability/execute
	// layers merely consume them.
	cases := []struct{ input, action, param, value string }{
		{"volumen al 40", "volumen_set", "percent", "40"},
		{"mueve archivo a documentos", "mover_archivo", "destination", "documentos"},
	}
	for _, tc := range cases {
		r := interaction.Analyze(tc.input, interaction.Context{})
		if r.Action.Name != tc.action {
			t.Fatalf("%q: action=%q want %q", tc.input, r.Action.Name, tc.action)
		}
		if r.Action.Parameters[tc.param] != tc.value {
			t.Fatalf("%q: parameter %q=%q want %q", tc.input, tc.param, r.Action.Parameters[tc.param], tc.value)
		}
	}
}
