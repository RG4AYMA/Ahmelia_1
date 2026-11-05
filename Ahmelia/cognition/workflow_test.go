package cognition

import (
	"testing"

	"ahmelia/interaction"
)

func TestEvaluateWorkflowFileSelectionIsCognitiveOnly(t *testing.T) {
	assessment := EvaluateWorkflow(interaction.WorkflowEvidence{
		State:             interaction.WorkflowFileSelect,
		RawInput:          "2",
		SelectionIndex:    2,
		SelectionProvided: true,
	})
	if !assessment.Handled || assessment.Outcome != interaction.WorkflowSelectFile {
		t.Fatalf("unexpected assessment: %+v", assessment)
	}
	if assessment.Capability.HasOperation() {
		t.Fatalf("file selection must not create a capability request: %+v", assessment.Capability)
	}
}

func TestEvaluateWorkflowFileActionProducesCapabilityWithoutExecuting(t *testing.T) {
	assessment := EvaluateWorkflow(interaction.WorkflowEvidence{
		State:      interaction.WorkflowFileAction,
		RawInput:   "abrir",
		ActiveFile: "C:/tmp/a.txt",
		FileAction: "abrir",
	})
	if !assessment.Handled || assessment.Outcome != interaction.WorkflowContinueAction {
		t.Fatalf("unexpected assessment: %+v", assessment)
	}
	if assessment.Capability.Operation != "abrir_archivo" {
		t.Fatalf("unexpected capability: %+v", assessment.Capability)
	}
}

func TestEvaluateWorkflowKeepsCancelAsDecisionInput(t *testing.T) {
	assessment := EvaluateWorkflow(interaction.WorkflowEvidence{
		State:           interaction.WorkflowFileSelect,
		RawInput:        "cancela",
		CancelRequested: true,
	})
	if !assessment.Handled || assessment.Outcome != interaction.WorkflowCancel {
		t.Fatalf("unexpected cancellation assessment: %+v", assessment)
	}
}
