package core

import (
	"fmt"
	"strings"

	"ahmelia/cognition"
	"ahmelia/expectation"
	"ahmelia/interaction"
	"ahmelia/production"
)

// B96.42.4 — all execution outcomes use one response boundary. Providers
// contribute structured facts; Core determines the response intent and the
// ResponseContract remains the only conversational hand-off to Language.
func (e *Engine) renderExecutionActionResult(req interaction.SemanticRequest, result production.ActionResult, productionResult bool) bool {
	if e == nil {
		return false
	}
	snap := e.conversationState.Snapshot()
	responseKind := interaction.ResponseIntentAnswer
	responseMove := interaction.MoveAnswer
	responseMode := cognition.ResponseModeAnswer
	responseReason := "authorized execution produced a structured result"

	if result.InputRequired {
		responseKind = interaction.ResponseIntentClarify
		responseMove = interaction.MoveClarify
		responseMode = cognition.ResponseModeClarify
		responseReason = "execution provider reported that additional user input is required"
	}
	if e.lastConversationalCognition.Repair.Required &&
		strings.TrimSpace(e.lastConversationalCognition.Repair.WorkID) != "" &&
		strings.TrimSpace(result.WorkID) != "" &&
		e.lastConversationalCognition.Repair.WorkID == result.WorkID {
		responseKind = interaction.ResponseIntentAcknowledge
		responseMove = interaction.MoveAcknowledge
		responseMode = cognition.ResponseModeAnswer
		responseReason = "post-execution repair produced a verified reconciliation result"
	}

	intent := interaction.ResponseIntent{
		Kind:               responseKind,
		Confidence:         result.Confidence(),
		AddressesObjective: true,
		ShouldContinue:     true,
		ShouldSpeak:        true,
		ShouldAct:          false,
		NeedsUserInput:     result.InputRequired,
		ThreadID:           snap.ActiveThreadID,
		Objective:          strings.TrimSpace(req.Objective.Text),
		Language:           snap.Language,
		Reason:             responseReason,
	}
	if productionResult {
		e.lastResponseIntent = intent
		e.lastNaturalDialogue.Move = responseMove
	}
	// Paso 4.1: cuando existe un veredicto de expectativa, la superficie es
	// determinista. El modelo puede leerla, pero no reescribirla ni omitirla:
	// el texto ya contiene esperaba / observé / veredicto.
	if text := expectationVerdictResponse(result); text != "" {
		e.lastResponseIntent = intent
		e.lastNaturalDialogue.Move = responseMove
		return e.renderVerbatimSystemResponse(req, "expectation_verdict", text, responseMove, responseKind, result.InputRequired)
	}
	contract := interaction.NewResponseContract(req, intent, responseMove)
	if productionResult {
		contract.Source = "production_action_result"
	} else {
		contract.Source = "local_action_result"
	}
	contract.ShouldAct = false
	if summary, source := executionSummaryEvidence(result, contract.Source); summary != "" {
		contract.AddEvidence(summary, source, true, result.Confidence())
	}
	if result.Observed && strings.TrimSpace(result.ObservationDetails) != "" {
		contract.AddEvidence("Observación: "+strings.TrimSpace(result.ObservationDetails), "execution_observation", true, .92)
	}
	if strings.TrimSpace(result.VerificationReason) != "" {
		verificationConfidence := .82
		if result.Verified {
			verificationConfidence = .98
		}
		contract.AddEvidence("Verificación: "+strings.TrimSpace(result.VerificationReason), "execution_verification", true, verificationConfidence)
	}
	if productionResult {
		if result.ProductionID != "" {
			contract.AddEvidence("Producción: "+result.ProductionID, "production_metadata", true, 1)
		}
		if result.Project.Valid() {
			contract.AddEvidence("Proyecto: "+result.Project.Name+" en "+result.Project.Path, "production_project", true, 1)
		}
		if len(result.Artifacts) > 0 {
			for _, artifact := range result.Artifacts {
				if artifact.Name == "" && artifact.Path == "" {
					continue
				}
				value := artifact.Name
				if artifact.Path != "" {
					value += " en " + artifact.Path
				}
				contract.AddEvidence("Artefacto: "+value, "production_artifact", true, 1)
			}
		}
	}
	if result.FailureReason != "" && !result.Success {
		contract.AddEvidence("Falló: "+result.FailureReason, "execution_failure", true, .9)
	}

	strategy := cognition.ConversationalResponseStrategy{
		Version: "B96.33.10", Mode: responseMode,
		Directness: cognition.DirectnessDirect, Structure: cognition.StructureExplanation,
		Initiative: cognition.InitiativeHelpful, Confidence: intent.Confidence,
		PreserveUserAgency: true, ExecutionBoundary: true, ShouldGenerateLanguage: true,
		Reason: "language realization of a structured execution result",
	}
	return e.renderResponseContractWithStrategy(req, contract, e.buildConversationalRuntimeFacts(snap), strategy)
}

// expectationVerdictResponse returns the harness-authored sentence when this
// action carries a declared expectation and a terminal verdict. It is the only
// text the response is allowed to publish for that action.
func expectationVerdictResponse(result production.ActionResult) string {
	if strings.TrimSpace(result.ExpectationID) == "" {
		return ""
	}
	if summary := strings.TrimSpace(result.ExpectationSummary); summary != "" {
		return summary
	}
	status := expectation.Status(strings.TrimSpace(result.ExpectationStatus))
	if !status.Terminal() {
		return ""
	}
	return expectation.DescribeVerdict(expectation.Verdict{
		Status: status, LedgerError: result.ExpectationError,
	})
}

func executionSummaryEvidence(result production.ActionResult, normalSource string) (string, string) {
	summary := strings.TrimSpace(result.Summary)
	if summary == "" {
		return "", ""
	}
	if result.OutcomeStatus == production.OutcomeUnknown {
		return "Ejecución realizada: " + summary + "; resultado pendiente de verificación.", "execution_result_unverified"
	}
	return summary, normalSource
}

func (e *Engine) renderProductionActionResult(req interaction.SemanticRequest, result production.ActionResult) bool {
	return e.renderExecutionActionResult(req, result, true)
}

func (e *Engine) renderLocalActionResult(req interaction.SemanticRequest, result production.ActionResult) bool {
	return e.renderExecutionActionResult(req, result, false)
}

func (e *Engine) renderProductionWorkQuery(req interaction.SemanticRequest, query production.WorkQueryResult) bool {
	if e == nil || !query.Found {
		return false
	}
	snap := e.conversationState.Snapshot()
	intent := interaction.ResponseIntent{
		Kind: interaction.ResponseIntentAnswer, Confidence: .95, AddressesQuestion: true,
		ShouldSpeak: true, ShouldContinue: true, ThreadID: snap.ActiveThreadID,
		Objective: strings.TrimSpace(req.Objective.Text), Language: snap.Language,
		Reason: "production state query answered from structured work evidence",
	}
	e.lastResponseIntent = intent
	e.lastNaturalDialogue.Move = interaction.MoveAnswer
	contract := interaction.NewResponseContract(req, intent, interaction.MoveAnswer)
	contract.Source = "production_work_query"
	contract.ShouldAct = false

	add := func(text string, confidence float64) {
		if strings.TrimSpace(text) != "" {
			contract.AddEvidence(text, "production_work_state", true, confidence)
		}
	}
	switch query.Kind {
	case production.WorkQueryCapabilities:
		if len(query.Capabilities) == 0 {
			add("No hay capacidades registradas en el runtime.", .7)
		} else {
			add("Capacidades disponibles: "+strings.Join(query.Capabilities, "; "), 1)
		}
	case production.WorkQueryContinuation:
		add(fmt.Sprintf("El trabajo %s está %s.", query.Work.ID, query.Work.Status), 1)
		if query.Work.CurrentStep != "" {
			add("Paso actual: "+query.Work.CurrentStep+".", 1)
		}
		if len(query.Work.Pending) > 0 {
			add("Pendiente: "+strings.Join(query.Work.Pending, ", ")+".", 1)
		}
	case production.WorkQueryActive:
		add("Estábamos trabajando en: "+query.Work.Objective.Text+".", 1)
		if query.Work.Provider != "" {
			add("Proveedor: "+query.Work.Provider+".", 1)
		}
		add("Estado: "+string(query.Work.Status)+".", 1)
	case production.WorkQueryRecentResult:
		if query.RecentResult != "" {
			add("El último resultado registrado fue: "+query.RecentResult, 1)
		} else {
			add("No hay un resultado reciente registrado.", .6)
		}
	case production.WorkQueryTest:
		if query.HasArtifact {
			add("El artefacto de prueba está en "+query.Artifact.Path+".", 1)
		} else {
			add("El último trabajo no tiene un artefacto de test registrado.", .6)
		}
	case production.WorkQueryLocation:
		if query.HasArtifact {
			add(query.Artifact.Name+" está en "+query.Artifact.Path+".", 1)
		} else {
			add("El último trabajo no tiene artefactos registrados.", .6)
		}
	case production.WorkQueryProgress:
		add("Estado del trabajo: "+string(query.Work.Status)+".", 1)
		if query.Work.CurrentStep != "" {
			add("Paso actual: "+query.Work.CurrentStep+".", 1)
		}
		if len(query.Work.Pending) > 0 {
			add("Pendiente: "+strings.Join(query.Work.Pending, ", ")+".", 1)
		}
	case production.WorkQueryArtifacts:
		if len(query.Work.Artifacts) == 0 {
			add("Todavía no hay artefactos registrados.", .6)
		} else {
			names := make([]string, 0, len(query.Work.Artifacts))
			for _, a := range query.Work.Artifacts {
				names = append(names, a.Name)
			}
			add("Artefactos producidos: "+strings.Join(names, ", ")+".", 1)
		}
	default:
		return false
	}
	return e.renderResponseContract(req, contract, e.buildConversationalRuntimeFacts(snap))
}

func (e *Engine) renderExecutionClarification(req interaction.SemanticRequest) bool {
	if e == nil {
		return false
	}
	snap := e.conversationState.Snapshot()
	intent := interaction.ResponseIntent{
		Kind:           interaction.ResponseIntentClarify,
		Confidence:     .99,
		NeedsUserInput: true,
		ShouldSpeak:    true,
		ShouldContinue: true,
		ThreadID:       snap.ActiveThreadID,
		Objective:      strings.TrimSpace(req.Objective.Text),
		Language:       snap.Language,
		Reason:         "cognitive execution gate requires clarification",
	}
	e.lastResponseIntent = intent
	e.lastNaturalDialogue.Move = interaction.MoveClarify
	contract := interaction.NewResponseContract(req, intent, interaction.MoveClarify)
	contract.Source = "production_execution_gate"
	contract.ShouldAct = false
	contract.AddEvidence("Necesito aclarar el objetivo antes de iniciar el trabajo.", "cognition_execution_gate", true, .99)
	return e.renderResponseContract(req, contract, e.buildConversationalRuntimeFacts(snap))
}
