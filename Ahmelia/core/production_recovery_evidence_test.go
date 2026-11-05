package core

import (
	"errors"
	"strings"
	"testing"

	"ahmelia/interaction"
	"ahmelia/production"
	"ahmelia/tools"
)

func TestProductionRecoveryRequestCarriesObservedFailureWithoutChangingOriginalObjective(t *testing.T) {
	req := interaction.Analyze("Crea una función Go llamada IsEven y verifica los tests.", interaction.Context{})
	original := req.Objective.Text
	result := production.ActionResult{FailureReason: "tests failed: TestIsEven", FailedStage: "test", Err: errors.New("exit status 1")}
	recovery := tools.RecoveryDecision{Action: tools.RecoveryRetry, Reason: "retry after observed failure"}

	retry := productionRecoveryRequest(req, result, recovery)
	if req.Objective.Text != original {
		t.Fatalf("recovery mutated original request: %q", req.Objective.Text)
	}
	if retry.Objective.Text == original {
		t.Fatal("retry request did not carry failure evidence")
	}
	if !containsAll(retry.Objective.Text, "[Recovery evidence]", "tests failed: TestIsEven", "Failed stage: test") {
		t.Fatalf("retry objective lost structured evidence: %q", retry.Objective.Text)
	}
}

func containsAll(text string, values ...string) bool {
	for _, value := range values {
		if !strings.Contains(text, value) {
			return false
		}
	}
	return true
}
