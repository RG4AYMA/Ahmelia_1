package cognition

import (
	"strings"

	"ahmelia/interaction"
)

// EvaluateWorkflow is the cognitive boundary for an already-started
// operational interaction. Workflow state is evidence only: this function
// does not execute, mutate files, teach actions, or generate final language.
func EvaluateWorkflow(e interaction.WorkflowEvidence) interaction.WorkflowAssessment {
	if !e.Active() {
		return interaction.WorkflowAssessment{}
	}

	base := interaction.WorkflowAssessment{Handled: true, Confidence: 0.96}
	if e.CancelRequested {
		base.Outcome = interaction.WorkflowCancel
		base.ResponseMove = interaction.MoveAcknowledge
		base.Reason = "user explicitly cancelled the pending workflow"
		return base
	}
	if strings.TrimSpace(e.RawInput) == "" {
		base.Confidence = 0.70
	}

	switch e.State {
	case interaction.WorkflowFileSelect:
		if !e.SelectionProvided {
			base.Outcome = interaction.WorkflowClarify
			base.ResponseMove = interaction.MoveClarify
			base.Reason = "active file-selection workflow received no valid selection evidence"
			return base
		}
		base.Outcome = interaction.WorkflowSelectFile
		base.SelectionIndex = e.SelectionIndex
		base.SelectedFile = e.SelectedFile
		base.ResponseMove = interaction.MoveAsk
		base.Reason = "current turn supplies the selection required by the active workflow"
		return base

	case interaction.WorkflowFileAction:
		if e.FileAction == "" {
			base.Outcome = interaction.WorkflowAskFileAction
			base.ResponseMove = interaction.MoveAsk
			base.Reason = "active file-action workflow still needs an operation"
			return base
		}
		if e.FileAction == "mover" && strings.TrimSpace(e.Target) == "" {
			base.Outcome = interaction.WorkflowClarify
			base.ResponseMove = interaction.MoveAsk
			base.Reason = "move workflow still needs a destination"
			return base
		}
		if e.FileAction == "renombrar" {
			base.Outcome = interaction.WorkflowRename
			base.ResponseMove = interaction.MoveAsk
			base.Reason = "active file workflow selected rename; the next step is the new name"
			return base
		}
		base.Outcome = interaction.WorkflowContinueAction
		base.Capability = workflowCapabilityForFile(e)
		base.Reason = "current turn supplies the missing operation for the focused file"
		return base

	case interaction.WorkflowRenaming:
		if strings.TrimSpace(e.NewName) == "" {
			base.Outcome = interaction.WorkflowRename
			base.ResponseMove = interaction.MoveAsk
			base.Reason = "active rename workflow still needs a valid new name"
			return base
		}
		base.Outcome = interaction.WorkflowContinueAction
		base.Capability = interaction.CapabilityRequest{
			Operation:  "renombrar_archivo",
			Parameters: map[string]string{"raw": e.RawInput, "new_name": e.NewName},
			Confidence: 1,
			Source:     "workflow.rename_continuation",
		}
		base.Reason = "current turn supplies the missing rename value"
		return base

	case interaction.WorkflowVolumeSet:
		if !e.VolumeProvided {
			base.Outcome = interaction.WorkflowSetVolume
			base.ResponseMove = interaction.MoveClarify
			base.Reason = "active volume workflow received no valid percentage"
			return base
		}
		base.Outcome = interaction.WorkflowContinueAction
		base.Capability = interaction.CapabilityRequest{
			Operation:  "volumen_set",
			Parameters: map[string]string{"raw": e.RawInput, "percent": formatInt(e.VolumePercent)},
			Confidence: 1,
			Source:     "workflow.volume_continuation",
		}
		base.Reason = "current turn supplies the missing volume value"
		return base

	case interaction.WorkflowLearning:
		if e.ActiveFile != "" && e.FileAction != "" {
			if e.FileAction == "renombrar" {
				base.Outcome = interaction.WorkflowRename
				base.ResponseMove = interaction.MoveAsk
				base.Reason = "active learning workflow yielded a concrete file operation; continue the focused file workflow"
				return base
			}
			if e.FileAction == "mover" && strings.TrimSpace(e.Target) == "" {
				base.Outcome = interaction.WorkflowClarify
				base.ResponseMove = interaction.MoveAsk
				base.Reason = "file move continuation still needs a destination"
				return base
			}
			base.Outcome = interaction.WorkflowContinueAction
			base.Capability = workflowCapabilityForFile(e)
			base.Reason = "active learning workflow yielded a concrete file operation"
			return base
		}
		if e.SemanticAction.Name == "" || e.SemanticAction.Confidence < 0.5 {
			base.Outcome = interaction.WorkflowTeach
			base.ResponseMove = interaction.MoveAsk
			base.Confidence = 0.75
			base.Reason = "active learning workflow did not receive a resolved action"
			return base
		}
		base.Outcome = interaction.WorkflowTeach
		base.Capability = interaction.ToCapabilityRequest(e.SemanticAction, e.RawInput)
		base.Reason = "current turn resolves the action being taught by the user"
		return base

	case interaction.WorkflowAskingClarification:
		base.Outcome = interaction.WorkflowClarify
		base.ResponseMove = interaction.MoveClarify
		base.Reason = "workflow carries a pending clarification boundary"
		return base
	}

	return interaction.WorkflowAssessment{}
}

func workflowCapabilityForFile(e interaction.WorkflowEvidence) interaction.CapabilityRequest {
	params := map[string]string{"raw": e.RawInput}
	switch e.FileAction {
	case "abrir":
		return interaction.CapabilityRequest{Operation: "abrir_archivo", Parameters: params, Confidence: 1, Source: "workflow.file_action"}
	case "mover":
		return interaction.CapabilityRequest{Operation: "mover_archivo", Target: strings.TrimSpace(e.Target), Parameters: params, Confidence: 1, Source: "workflow.file_action"}
	case "renombrar":
		return interaction.CapabilityRequest{}
	default:
		return interaction.CapabilityRequest{Operation: "mover_archivo", Target: strings.TrimSpace(e.Target), Parameters: params, Confidence: 1, Source: "workflow.file_action"}
	}
}

func formatInt(v int) string {
	if v == 0 {
		return "0"
	}
	if v < 0 {
		return "-" + formatInt(-v)
	}
	digits := ""
	for v > 0 {
		digits = string(rune('0'+v%10)) + digits
		v /= 10
	}
	return digits
}
