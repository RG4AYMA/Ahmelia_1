package core

import (
	"testing"

	"ahmelia/interaction"
	"ahmelia/production"
)

func TestB96442_ProductionIntentPendingExecutionIsComplete(t *testing.T) {
	raw := "Crea un pequeño programa en Go que calcule el promedio de tres números, ejecútalo y verifica que funciona."
	req := interaction.Analyze(raw, interaction.Context{})
	if req.Action.Name != "" {
		t.Fatalf("test assumption broken: expected no action-alias match, got %q", req.Action.Name)
	}
	decision := interaction.ResolveCapabilityInteraction(req)
	if !decision.Executable {
		t.Fatalf("expected executable production intent: %s", decision.Reason)
	}
	if decision.Request.Operation != "produce" {
		t.Fatalf("expected produce, got %q", decision.Request.Operation)
	}
	pending := production.PendingExecution{Request: req, CapabilityID: "programming.create_file", Operation: "create_file", RouteKind: "production", ProviderID: "programming-provider", AttemptNumber: 1}
	if !pendingExecutionComplete(pending) {
		t.Fatal("legitimate production-intent pending execution rejected")
	}
	resumeDecision := interaction.ResolveCapabilityInteraction(pending.Request)
	if !pendingExecutionStillCoherent(resumeDecision) {
		t.Fatal("legitimate production-intent pending execution rejected at confirmation")
	}
}

func TestB96442_IncompletePendingExecutionStillRejected(t *testing.T) {
	req := interaction.Analyze("Sí, autorizo la ejecución.", interaction.Context{})
	cases := []struct {
		name    string
		pending production.PendingExecution
	}{
		{"no attempt number", production.PendingExecution{Request: req, CapabilityID: "programming.create_file", Operation: "create_file"}},
		{"no capability id", production.PendingExecution{Request: req, Operation: "create_file", AttemptNumber: 1}},
		{"no operation", production.PendingExecution{Request: req, CapabilityID: "programming.create_file", AttemptNumber: 1}},
		{"no raw request", production.PendingExecution{Request: interaction.SemanticRequest{}, CapabilityID: "programming.create_file", Operation: "create_file", RouteKind: "production", ProviderID: "programming-provider", AttemptNumber: 1}},
		{"no route kind", production.PendingExecution{Request: req, CapabilityID: "programming.create_file", Operation: "create_file", ProviderID: "programming-provider", AttemptNumber: 1}},
		{"no provider identity", production.PendingExecution{Request: req, CapabilityID: "programming.create_file", Operation: "create_file", RouteKind: "production", AttemptNumber: 1}},
		{"wrong route kind", production.PendingExecution{Request: req, CapabilityID: "programming.create_file", Operation: "create_file", RouteKind: "action", ProviderID: "programming-provider", AttemptNumber: 1}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if pendingExecutionComplete(c.pending) {
				t.Fatalf("expected incomplete pending execution rejected: %+v", c.pending)
			}
		})
	}
}

func TestB96442_IncoherentDecisionStillRejected(t *testing.T) {
	decision := interaction.CapabilityInteractionDecision{Executable: false, Reason: "turn is not an executable interaction request"}
	if pendingExecutionStillCoherent(decision) {
		t.Fatal("expected non-executable decision rejected")
	}
}
