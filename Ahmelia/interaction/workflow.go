package interaction

import "strings"

// WorkflowState is the narrow operational state exposed to cognition.
// It describes where an already-started interaction is waiting; it does not
// contain a general conversational interpretation or a generated response.
type WorkflowState string

const (
	WorkflowIdle                WorkflowState = "idle"
	WorkflowFileSelect          WorkflowState = "file_select"
	WorkflowFileAction          WorkflowState = "file_action"
	WorkflowRenaming            WorkflowState = "renaming"
	WorkflowLearning            WorkflowState = "learning"
	WorkflowVolumeSet           WorkflowState = "volume_set"
	WorkflowAskingClarification WorkflowState = "asking_clarification"
)

// WorkflowEvidence is the boundary between operational session state and
// Cognition. It contains only facts extracted from the active workflow and
// the current semantic turn. It never contains a final sentence to emit.
type WorkflowEvidence struct {
	State              WorkflowState
	RawInput           string
	ActiveFile         string
	ActiveFileName     string
	LearningInput      string
	SelectionIndex     int
	SelectionProvided  bool
	SelectedFile       string
	FileAction         string
	Target             string
	NewName            string
	VolumePercent      int
	VolumeProvided     bool
	SemanticAction     ActionIntent
	CommunicativeActs  CommunicativeActs
	OriginalGoal       string
	PendingAssumptions []string
	CancelRequested    bool
}

func (e WorkflowEvidence) Active() bool {
	return e.State != WorkflowIdle
}

// WorkflowAssessment is the cognitive result for an active workflow. It says
// what the current turn means relative to that workflow, but does not execute
// anything and does not generate language.
type WorkflowAssessment struct {
	Handled        bool
	Outcome        WorkflowOutcome
	Confidence     float64
	Reason         string
	SelectionIndex int
	SelectedFile   string
	Capability     CapabilityRequest
	ResponseMove   ResponseMove
	NeedsUserInput bool
}

type WorkflowOutcome string

const (
	WorkflowNoDecision     WorkflowOutcome = "none"
	WorkflowContinueAction WorkflowOutcome = "continue_action"
	WorkflowSelectFile     WorkflowOutcome = "select_file"
	WorkflowAskFileAction  WorkflowOutcome = "ask_file_action"
	WorkflowRename         WorkflowOutcome = "rename"
	WorkflowSetVolume      WorkflowOutcome = "set_volume"
	WorkflowTeach          WorkflowOutcome = "teach"
	WorkflowCancel         WorkflowOutcome = "cancel"
	WorkflowClarify        WorkflowOutcome = "clarify"
)

// WorkflowDecision is the decision boundary after Cognition. It may request a
// capability execution or a response, but it never contains provider details
// and never contains a generated conversational sentence.
type WorkflowDecision struct {
	Handled        bool
	Outcome        WorkflowOutcome
	Capability     CapabilityInteractionDecision
	Response       ResponseContract
	SelectionIndex int
	SelectedFile   string
	Reason         string
}

func workflowResponse(req SemanticRequest, move ResponseMove, text string, confidence float64) ResponseContract {
	intent := ResponseIntent{
		Confidence:     confidence,
		Language:       req.Language,
		ShouldSpeak:    true,
		ShouldContinue: true,
	}
	switch move {
	case MoveAsk:
		intent.Kind = ResponseIntentAsk
		intent.NeedsUserInput = true
	case MoveClarify:
		intent.Kind = ResponseIntentClarify
		intent.NeedsUserInput = true
	case MoveContinue:
		intent.Kind = ResponseIntentContinue
	case MoveAcknowledge:
		intent.Kind = ResponseIntentAcknowledge
	default:
		intent.Kind = ResponseIntentAcknowledge
	}
	contract := NewResponseContract(req, intent, move)
	contract.Source = "workflow"
	contract.ShouldAct = false
	contract.AddEvidence(text, "workflow_state", true, confidence)
	return contract
}

// BuildWorkflowDecision converts a cognitive workflow assessment into the
// canonical Action/Response boundary. The supplied text is evidence for
// Language, not final output authored by the workflow.
func BuildWorkflowDecision(req SemanticRequest, assessment WorkflowAssessment) WorkflowDecision {
	if !assessment.Handled {
		return WorkflowDecision{}
	}

	decision := WorkflowDecision{
		Handled:        true,
		Outcome:        assessment.Outcome,
		SelectionIndex: assessment.SelectionIndex,
		SelectedFile:   strings.TrimSpace(assessment.SelectedFile),
		Reason:         assessment.Reason,
	}

	switch assessment.Outcome {
	case WorkflowContinueAction:
		decision.Capability = CapabilityInteractionDecision{
			Executable: assessment.Capability.HasOperation(),
			Request:    assessment.Capability,
			Reason:     assessment.Reason,
		}
	case WorkflowSetVolume:
		if assessment.Capability.HasOperation() {
			decision.Capability = CapabilityInteractionDecision{Executable: true, Request: assessment.Capability, Reason: assessment.Reason}
		} else {
			decision.Response = workflowResponse(req, MoveClarify, "corresponde indicar un porcentaje de volumen entre 0 y 100", assessment.Confidence)
		}
	case WorkflowTeach:
		if assessment.Capability.HasOperation() {
			decision.Capability = CapabilityInteractionDecision{Executable: true, Request: assessment.Capability, Reason: assessment.Reason}
		} else {
			decision.Response = workflowResponse(req, MoveAsk, "corresponde indicar qué acción enseñar", assessment.Confidence)
		}
	case WorkflowSelectFile:
		decision.Response = workflowResponse(req, MoveAsk, "archivo seleccionado; ahora corresponde pedir la acción", assessment.Confidence)
	case WorkflowAskFileAction:
		decision.Response = workflowResponse(req, MoveAsk, "corresponde pedir qué acción realizar sobre el archivo", assessment.Confidence)
	case WorkflowRename:
		decision.Response = workflowResponse(req, MoveAsk, "corresponde pedir el nuevo nombre del archivo", assessment.Confidence)
	case WorkflowClarify:
		decision.Response = workflowResponse(req, MoveClarify, "la continuación del workflow necesita una aclaración", assessment.Confidence)
	case WorkflowCancel:
		decision.Response = workflowResponse(req, MoveAcknowledge, "la operación pendiente fue cancelada", assessment.Confidence)
	}
	return decision
}
