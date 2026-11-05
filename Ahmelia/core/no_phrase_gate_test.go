package core

import (
	"os"
	"strings"
	"testing"
)

func TestNoPhraseGateCoreG3Boundaries(t *testing.T) {
	data, err := os.ReadFile("engine.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(data)
	// Whitelist: the core file still contains legitimate C-layer phrase
	// classifiers (file extensions, category names and machine-state text).
	// G3 therefore guards the specific B helpers rather than banning every
	// string comparison in the package.
	for _, removed := range []string{"extractFileAction", "isCancelCommand", "extractMemoryTopic", "extractConversationTopic"} {
		if strings.Contains(src, removed) {
			t.Fatalf("G3 removed helper %q still exists", removed)
		}
	}
	if !strings.Contains(src, "req.Action.Name") {
		t.Fatal("core workflow no longer consumes SemanticRequest.Action.Name")
	}
	if !strings.Contains(src, "req.CommunicativeActs.Has(interaction.ActControl)") {
		t.Fatal("core workflow no longer consumes canonical ActControl")
	}
	if !strings.Contains(src, "req.Meaning.Query") {
		t.Fatal("core semantic providers are not routed by typed Meaning.Query")
	}
}

func TestG35WorkflowNumberRemainsOwnedByWorkflowEvidence(t *testing.T) {
	data, err := os.ReadFile("workflow_runtime.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(data)
	// Whitelist: extractNumber remains a C/workflow adapter because the numeric
	// selection is already represented by WorkflowEvidence.SelectionProvided /
	// SelectionIndex and consumed by Cognition. G3 must not create another number
	// interpreter in SemanticRequest or Cognition.
	if strings.Count(src, "extractNumber(input)") != 1 {
		t.Fatalf("unexpected duplicate numeric workflow adapter")
	}
	for _, marker := range []string{"SelectionIndex:     selection", "SelectionProvided:  selectionOK"} {
		if !strings.Contains(src, marker) {
			t.Fatalf("workflow evidence lost existing numeric ownership marker %q", marker)
		}
	}
}
func TestG4EngineUsesAcceptedCapabilityDecisionForRouting(t *testing.T) {
	data, err := os.ReadFile("engine.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(data)
	// Whitelist: authority still owns the call boundary; only the resolver
	// dependency is typed. No second raw-text classification may occur after
	// the CapabilityInteractionDecision is accepted.
	if strings.Contains(src, "capabilityResolver.Resolve(raw)") || strings.Contains(src, "capabilityResolver.Resolve(decision.Request.Parameters[\"raw\"])") {
		t.Fatal("engine still routes capability work from raw text")
	}
	if !strings.Contains(src, "e.capabilityResolver.Resolve(decision, request)") {
		t.Fatal("engine does not pass the accepted decision and SemanticRequest to G4")
	}
}

func TestG2InitiativeTraceRecordsFindingEvidenceAndConfidence(t *testing.T) {
	data, err := os.ReadFile("initiative_phase_runtime.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(data)
	// Structural guard: the canonical initiative decision path records the
	// disagreement finding IDs and confidence after B96.37.2 evaluates the
	// opportunity. The trace is observational only.
	for _, marker := range []string{"INITIATIVE DISAGREEMENT", "findings=%s", "confidence=%.2f"} {
		if !strings.Contains(src, marker) {
			t.Fatalf("missing disagreement trace marker %q", marker)
		}
	}
}
