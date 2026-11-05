package core

import (
	"testing"
	"time"

	"ahmelia/autonomy"
	"ahmelia/interaction"
	"ahmelia/production"
)

func TestAutonomousSemanticRequestSeparatesObjectiveFromAction(t *testing.T) {
	params := map[string]string{"ruta": "C:/tmp/reporte"}
	first := autonomousSemanticRequest(autonomy.ActionRequest{
		Action: "crear_carpeta", Objective: "Preparar el entorno del proyecto", Target: "C:/tmp/reporte",
	}, "crear_carpeta C:/tmp/reporte", params)
	second := autonomousSemanticRequest(autonomy.ActionRequest{
		Action: "crear_carpeta", Objective: "Organizar los documentos del cliente", Target: "C:/tmp/reporte",
	}, "crear_carpeta C:/tmp/reporte", params)

	if first.Objective.Text == second.Objective.Text {
		t.Fatalf("different higher-level objectives collapsed: first=%q second=%q", first.Objective.Text, second.Objective.Text)
	}
	if first.Action.Name != second.Action.Name {
		t.Fatalf("test setup error: expected the same immediate action, got %q and %q", first.Action.Name, second.Action.Name)
	}

	third := autonomousSemanticRequest(autonomy.ActionRequest{
		Action: "renombrar_archivo", Objective: "Preparar el entorno del proyecto", Target: "C:/tmp/reporte",
	}, "renombrar_archivo C:/tmp/reporte", params)
	if first.Objective.Text != third.Objective.Text {
		t.Fatalf("same objective changed when immediate action changed: %q vs %q", first.Objective.Text, third.Objective.Text)
	}
	if first.Action.Name == third.Action.Name {
		t.Fatal("different immediate actions collapsed into the same action")
	}
}

func TestAutonomousSemanticRequestDoesNotInventObjectiveFromAction(t *testing.T) {
	req := autonomousSemanticRequest(autonomy.ActionRequest{Action: "silenciar_volumen"}, "silenciar_volumen", nil)
	if req.Objective.Text != "" {
		t.Fatalf("missing objective must remain explicitly absent, got %q", req.Objective.Text)
	}
}

func TestWorkContextProjectionPreservesEpistemicAndAttemptState(t *testing.T) {
	w := production.WorkSession{
		ID: "work-unknown", Objective: interaction.Objective{Text: "Completar la exportación"},
		Status: production.WorkRunning, Provider: "blender", CurrentStep: "export",
		UpdatedAt: time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC),
		LastActionResult: &production.ActionResult{
			Executed: true, Success: false, OutcomeStatus: production.OutcomeUnknown,
			Observed: false, Verified: false, Summary: "export returned without confirmation",
			VerificationSource: "environment.inspector", VerificationReason: "no fresh observation",
			FailureReason: "", WorkID: "work-unknown", AttemptNumber: 3,
		},
		Attempts: []production.WorkAttempt{{Number: 3, Status: "unverified"}},
	}

	got := workContextFromSession(w)
	if got.OutcomeStatus != string(production.OutcomeUnknown) || got.Observed || got.LastActionVerified {
		t.Fatalf("epistemic state was lost or promoted: %+v", got)
	}
	if !got.ReconciliationPending || got.AttemptNumber != 3 || got.AttemptStatus != "unverified" {
		t.Fatalf("attempt/reconciliation continuity was lost: %+v", got)
	}
	if got.VerificationSource != "environment.inspector" || got.VerificationReason != "no fresh observation" {
		t.Fatalf("verification provenance was lost: %+v", got)
	}
}
