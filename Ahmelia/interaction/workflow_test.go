package interaction

import "testing"

func TestBuildWorkflowDecisionDoesNotContainGeneratedLanguage(t *testing.T) {
	req := SemanticRequest{Raw: "2"}
	decision := BuildWorkflowDecision(req, WorkflowAssessment{
		Handled:        true,
		Outcome:        WorkflowSelectFile,
		SelectionIndex: 2,
		SelectedFile:   "C:/tmp/a.txt",
		Confidence:     .96,
		Reason:         "selection supplied",
	})
	if !decision.Handled || decision.Response.Intent.Kind != ResponseIntentAsk {
		t.Fatalf("unexpected workflow decision: %+v", decision)
	}
	if len(decision.Response.Evidence) == 0 {
		t.Fatal("workflow response should carry semantic evidence")
	}
	for _, ev := range decision.Response.Evidence {
		if ev.Text == "2" {
			t.Fatalf("raw selection must not become final language evidence: %+v", ev)
		}
	}
}

func TestBuildWorkflowDecisionRejectsUnresolvedCapability(t *testing.T) {
	decision := BuildWorkflowDecision(SemanticRequest{}, WorkflowAssessment{
		Handled:    true,
		Outcome:    WorkflowContinueAction,
		Confidence: .9,
		Reason:     "missing operation",
	})
	if !decision.Handled || decision.Capability.Executable {
		t.Fatalf("unresolved workflow must not become executable: %+v", decision)
	}
}
