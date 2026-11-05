package situation

import (
	"testing"
	"time"

	"ahmelia/interaction"
)

func TestBuildPreservesWorkAttemptEpistemicStateForNextDecision(t *testing.T) {
	now := time.Unix(1770000000, 0)
	work := interaction.WorkContext{
		ID:                    "work-uncertain-1",
		Status:                "pending_reconciliation",
		Objective:             "Crear y verificar un proyecto Go",
		LastResult:            "El proveedor no pudo confirmar el efecto",
		OutcomeStatus:         "unknown",
		OutcomeCode:           "outcome_unknown",
		Observed:              false,
		LastActionVerified:    false,
		VerificationSource:    "filesystem.inspector",
		VerificationReason:    "No se pudo determinar si el artefacto existe",
		FailureReason:         "deadline exceeded after execution request",
		AttemptNumber:         3,
		AttemptStatus:         "unverified",
		ReconciliationPending: true,
		UpdatedAt:             now,
	}

	got := Build(Input{
		Request:      interaction.SemanticRequest{Raw: "continúa con el proyecto", Normalized: "continúa con el proyecto", Confidence: .9},
		Conversation: interaction.ConversationSnapshot{ActiveWorkID: work.ID},
		Work:         work,
		HasWork:      true,
		Now:          now,
	})

	byID := make(map[string]Evidence, len(got.Evidence))
	for _, ev := range got.Evidence {
		byID[ev.ID] = ev
	}
	checks := map[string]string{
		"work.outcome_status":         "unknown",
		"work.outcome_code":           "outcome_unknown",
		"work.effect_observed":        "false",
		"work.effect_verified":        "false",
		"work.verification_source":    "filesystem.inspector",
		"work.verification_reason":    "No se pudo determinar si el artefacto existe",
		"work.failure_reason":         "deadline exceeded after execution request",
		"work.attempt_number":         "3",
		"work.attempt_status":         "unverified",
		"work.reconciliation_pending": "true",
	}
	for id, want := range checks {
		ev, ok := byID[id]
		if !ok {
			t.Errorf("Situation lost epistemic work evidence %q", id)
			continue
		}
		if ev.Object != want {
			t.Errorf("evidence %q object = %q, want %q", id, ev.Object, want)
		}
		if ev.Authority != "production.WorkStore" || ev.Subject != work.ID {
			t.Errorf("evidence %q lost canonical work provenance/identity: %+v", id, ev)
		}
	}
	if ev := byID["work.reconciliation_pending"]; ev.Epistemic != EpistemicObserved {
		t.Errorf("reconciliation state must represent the persisted WorkStore fact, got epistemic=%q", ev.Epistemic)
	}
	if !containsString(got.Uncertainty, "el último intento del trabajo requiere reconciliación; no debe inferirse éxito ni fracaso del efecto") {
		t.Errorf("pending reconciliation was not surfaced as situation uncertainty: %#v", got.Uncertainty)
	}
	decision := Evaluate(got, []Contribution{OperationalPerspective(got)})
	if !decision.HasKind("requires_reconciliation") {
		t.Errorf("decision evidence did not receive reconciliation state: %+v", decision.Findings)
	}
	foundOperationalCriterion := false
	for _, criterion := range decision.Criteria {
		if criterion.ID == "operational_state" {
			foundOperationalCriterion = true
			break
		}
	}
	if !foundOperationalCriterion {
		t.Errorf("decision evidence omitted operational-state criterion: %+v", decision.Criteria)
	}
	if len(decision.Evaluation.Risks) == 0 {
		t.Errorf("unresolved attempt did not preserve an epistemic risk: %+v", decision.Evaluation)
	}
}

func TestBuildDoesNotInventAttemptEvidenceWhenWorkHasNoAttemptState(t *testing.T) {
	got := Build(Input{
		Request: interaction.SemanticRequest{Raw: "hola", Normalized: "hola", Confidence: .8},
		Work:    interaction.WorkContext{ID: "work-empty", Status: "pending", Objective: "objetivo"},
		HasWork: true,
		Now:     time.Unix(1770000000, 0),
	})
	for _, ev := range got.Evidence {
		switch ev.ID {
		case "work.outcome_status", "work.effect_observed", "work.effect_verified", "work.attempt_number", "work.attempt_status", "work.reconciliation_pending":
			t.Errorf("invented attempt epistemic state for work without attempt result: %+v", ev)
		}
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
