package core

import (
	"path/filepath"
	"strings"

	"ahmelia/cognition"
	"ahmelia/interaction"
	"ahmelia/memory"
	"ahmelia/personality"
	"ahmelia/system"
)

// workflowEvidenceFromSession is the only adapter from the operational
// Session owner into the general cognitive pipeline. Session remains a state
// machine; it does not interpret the turn or decide what Ahmelia should do.

func workflowFileAction(actionName string) string {
	switch actionName {
	case "abrir_archivo", "abrir_programa":
		return "abrir"
	case "mover_archivo":
		return "mover"
	case "renombrar_archivo":
		return "renombrar"
	default:
		return ""
	}
}

func (e *Engine) workflowEvidenceFromSession(req interaction.SemanticRequest, input string) interaction.WorkflowEvidence {
	snap := e.sess.Snapshot()
	state := interaction.WorkflowState(snap.State)
	if state == "" {
		state = interaction.WorkflowIdle
	}

	selection, selectionOK := extractNumber(input)
	volume, volumeOK := systemExtractVolumePercent(input)
	fileAction := workflowFileAction(req.Action.Name)
	newName := ""
	if state == interaction.WorkflowRenaming {
		newName = cleanNewName(input, filepath.Base(snap.LastSelectedFile))
	}
	target := ""
	if fileAction == "mover" {
		target = e.extractFolderTarget(input)
	}

	var pendingGoal string
	var pendingAssumptions []string
	if snap.PendingClarification != nil {
		pendingGoal = snap.PendingClarification.OriginalGoal
		pendingAssumptions = append([]string(nil), snap.PendingClarification.PendingAssumptions...)
	}

	return interaction.WorkflowEvidence{
		State:              state,
		RawInput:           strings.TrimSpace(input),
		ActiveFile:         snap.LastSelectedFile,
		ActiveFileName:     filepath.Base(snap.LastSelectedFile),
		LearningInput:      snap.LearningInput,
		SelectionIndex:     selection,
		SelectionProvided:  selectionOK,
		FileAction:         fileAction,
		Target:             target,
		NewName:            newName,
		VolumePercent:      volume,
		VolumeProvided:     volumeOK,
		SemanticAction:     req.Action,
		CommunicativeActs:  req.CommunicativeActs,
		OriginalGoal:       pendingGoal,
		PendingAssumptions: pendingAssumptions,
		CancelRequested:    e.sess.IsActive() && (req.CommunicativeActs.Has(interaction.ActControl) || req.Action.Name == "salir"),
	}
}

// evaluateWorkflow is the explicit State → Evidence → Cognition → Decision
// boundary. It returns no executable side effect and no generated sentence.
func (e *Engine) evaluateWorkflow(req interaction.SemanticRequest, input string) (interaction.WorkflowDecision, interaction.WorkflowEvidence) {
	evidence := e.workflowEvidenceFromSession(req, input)
	assessment := cognition.EvaluateWorkflow(evidence)
	return interaction.BuildWorkflowDecision(req, assessment), evidence
}

// applyWorkflowDecision is the only Engine-side consumer of a workflow
// decision. It performs state transitions, hands capability work to the
// central execution authority, and sends response contracts through Language.
func (e *Engine) applyWorkflowDecision(req interaction.SemanticRequest, input string, decision interaction.WorkflowDecision, evidence interaction.WorkflowEvidence) bool {
	if !decision.Handled {
		return false
	}

	switch decision.Outcome {
	case interaction.WorkflowSelectFile:
		path, valid := e.sess.SelectFile(decision.SelectionIndex)
		if !valid {
			contract := workflowResponseContract(req, interaction.MoveClarify, "la selección indicada no corresponde a una opción disponible", 0.96)
			return e.renderResponseContract(req, contract, e.buildConversationalRuntimeFacts(e.conversationState.Snapshot()))
		}
		if evidence.FileAction != "" {
			// Preserve the legacy compact interaction "2 abrir" / "2 mover ..."
			// without letting the workflow itself execute the action.
			followEvidence := evidence
			followEvidence.State = interaction.WorkflowFileAction
			followAssessment := cognition.EvaluateWorkflow(followEvidence)
			followDecision := interaction.BuildWorkflowDecision(req, followAssessment)
			if followDecision.Handled && followDecision.Outcome == interaction.WorkflowRename {
				e.sess.SetRenaming()
				return e.renderWorkflowAsk(req, "corresponde pedir el nuevo nombre del archivo")
			}
			if followDecision.Handled && followDecision.Capability.Executable {
				_ = e.authorityExecute(followDecision.Capability)
				return true
			}
		}
		contract := workflowResponseContract(req, interaction.MoveAsk, "archivo seleccionado: "+filepath.Base(path)+"; corresponde preguntar qué acción realizar", 0.96)
		return e.renderResponseContract(req, contract, e.buildConversationalRuntimeFacts(e.conversationState.Snapshot()))

	case interaction.WorkflowAskFileAction:
		return e.renderWorkflowAsk(req, "corresponde preguntar qué acción realizar sobre el archivo seleccionado")

	case interaction.WorkflowRename:
		if evidence.NewName == "" {
			e.sess.SetRenaming()
			return e.renderWorkflowAsk(req, "corresponde pedir el nuevo nombre del archivo")
		}
		// This branch is reached only after a rename continuation. The initial
		// file-action "renombrar" branch has no NewName and therefore only moves
		// the session into the renaming state.
		_ = e.authorityExecute(decision.Capability)
		return true

	case interaction.WorkflowContinueAction:
		if evidence.State == interaction.WorkflowLearning && evidence.ActiveFile != "" && evidence.FileAction != "" {
			e.sess.FocusFile(evidence.ActiveFile)
		}
		if evidence.State == interaction.WorkflowVolumeSet {
			e.sess.ClearVolumeWait()
		}
		if decision.Capability.Executable {
			_ = e.authorityExecute(decision.Capability)
			return true
		}
		return false

	case interaction.WorkflowSetVolume:
		if evidence.VolumeProvided && decision.Capability.Executable {
			_ = e.authorityExecute(decision.Capability)
			return true
		}
		return e.renderWorkflowAsk(req, "corresponde pedir un número del 0 al 100")

	case interaction.WorkflowTeach:
		if decision.Capability.Executable {
			// Teaching is a workflow action, not a language decision. The
			// actual mutation remains here, after Cognition/Decision, exactly
			// like any other Engine-side capability transition.
			action := req.Action
			e.learner.LearnAction(e.sess.LearningInput, action.Name, action.Target, action.Parameters)
			memory.SaveLongMemory(e.longMem)
			e.relationship.AddMoment("Aprendió a \""+action.Name+"\" para \""+e.sess.LearningInput+"\"", "aprendizaje", "Acción nueva enseñada por el usuario")
			e.relationship.GrowTrust(0.01)
			_ = e.relationship.Save(personality.RelationshipPath)
			contract := workflowResponseContract(req, interaction.MoveAcknowledge, "la acción fue incorporada al aprendizaje solicitado", 0.99)
			e.sess.ClearLearning()
			return e.renderResponseContract(req, contract, e.buildConversationalRuntimeFacts(e.conversationState.Snapshot()))
		}
		return e.renderWorkflowAsk(req, "la respuesta no resolvió qué acción enseñar")

	case interaction.WorkflowClarify:
		return e.renderWorkflowAsk(req, "la continuación del workflow necesita una aclaración")

	case interaction.WorkflowCancel:
		e.sess.Reset()
		return e.renderResponseContract(req, decision.Response, e.buildConversationalRuntimeFacts(e.conversationState.Snapshot()))
	}

	return false
}

func (e *Engine) renderWorkflowAsk(req interaction.SemanticRequest, evidence string) bool {
	contract := workflowResponseContract(req, interaction.MoveAsk, evidence, 0.96)
	return e.renderResponseContract(req, contract, e.buildConversationalRuntimeFacts(e.conversationState.Snapshot()))
}

func workflowResponseContract(req interaction.SemanticRequest, move interaction.ResponseMove, evidence string, confidence float64) interaction.ResponseContract {
	intent := interaction.ResponseIntent{
		Kind:           interaction.ResponseIntentAsk,
		Confidence:     confidence,
		ShouldSpeak:    true,
		ShouldContinue: true,
		NeedsUserInput: true,
		Reason:         "workflow continuation requires user input",
		Language:       req.Language,
	}
	if move == interaction.MoveClarify {
		intent.Kind = interaction.ResponseIntentClarify
		intent.Reason = "workflow continuation requires clarification"
	}
	contract := interaction.NewResponseContract(req, intent, move)
	contract.Source = "workflow"
	contract.ShouldAct = false
	contract.AddEvidence(evidence, "workflow_state", true, confidence)
	return contract
}

// systemExtractVolumePercent keeps the evidence adapter independent from the
// decision layer while reusing the existing deterministic parser.
func systemExtractVolumePercent(input string) (int, bool) {
	return system.ExtractVolumePercent(input)
}
