package core

import (
	"testing"
	"time"

	"ahmelia/interaction"
	"ahmelia/production"
)

func TestWorkContextProjectionPreservesVerificationMetadata(t *testing.T) {
	updated := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	work := production.WorkSession{
		ID: "work-epistemic", Objective: interaction.Objective{Text: "Construir un prototipo"},
		Status: production.WorkRunning, Provider: "test-provider", CurrentStep: "verify", UpdatedAt: updated,
		Attempts: []production.WorkAttempt{{Number: 3, Status: "unverified"}},
		LastActionResult: &production.ActionResult{
			Success: false, OutcomeStatus: production.OutcomeUnknown, Observed: false, Verified: false,
			Summary: "resultado incierto", VerificationSource: "filesystem-inspector",
			VerificationReason: "no se pudo confirmar el efecto", FailureReason: "", OutcomeCode: "outcome_unknown",
			AttemptNumber: 3,
		},
	}
	got := workContextFromSession(work)
	if got.ID != work.ID || got.Objective != work.Objective.Text || got.Status != string(production.WorkRunning) {
		t.Fatalf("identidad/objetivo/estado perdidos: %+v", got)
	}
	if got.OutcomeStatus != string(production.OutcomeUnknown) || got.Observed || got.LastActionVerified {
		t.Fatalf("estado epistemológico se colapsó: %+v", got)
	}
	if got.AttemptNumber != 3 || got.AttemptStatus != "unverified" {
		t.Fatalf("identidad/estado del intento perdidos: %+v", got)
	}
	if got.VerificationSource != "filesystem-inspector" || got.VerificationReason != "no se pudo confirmar el efecto" || got.OutcomeCode != "outcome_unknown" {
		t.Fatalf("procedencia o razón de verificación perdidas: %+v", got)
	}
	if got.LastResult != "resultado incierto" || !got.UpdatedAt.Equal(updated) {
		t.Fatalf("resultado o tiempo perdido: %+v", got)
	}
}
