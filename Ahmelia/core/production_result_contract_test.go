package core

import (
	"testing"

	"ahmelia/production"
)

func TestExecutionResultUnknownDoesNotBecomeConfirmedLanguageEvidence(t *testing.T) {
	result := production.ActionResult{
		Executed: true, Success: true, OutcomeStatus: production.OutcomeUnknown,
		Summary: "Volumen bajado", VerificationReason: "no se pudo observar el volumen",
	}
	text, source := executionSummaryEvidence(result, "local_action_result")
	if source != "execution_result_unverified" {
		t.Fatalf("unexpected source=%q", source)
	}
	if text != "Ejecución realizada: Volumen bajado; resultado pendiente de verificación." {
		t.Fatalf("unexpected evidence=%q", text)
	}
}

func TestExecutionResultVerifiedKeepsNormalSummaryEvidence(t *testing.T) {
	result := production.ActionResult{
		Executed: true, Success: true, OutcomeStatus: production.OutcomeSucceeded,
		Summary: "Volumen bajado", Verified: true, Observed: true,
	}
	text, source := executionSummaryEvidence(result, "local_action_result")
	if source != "local_action_result" || text != "Volumen bajado" {
		t.Fatalf("unexpected evidence=%q source=%q", text, source)
	}
}
