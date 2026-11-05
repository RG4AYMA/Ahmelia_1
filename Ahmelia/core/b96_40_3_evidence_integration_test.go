package core

import (
	"ahmelia/interaction"
	"testing"
)

func TestB96403EngineExposesSharedDecisionEvidence(t *testing.T) {
	e := newB963334AuditEngine(t)
	e.Process("hola ahmelia")
	if e.lastDecisionEvidence.SituationID == "" {
		t.Fatal("DecisionEvidence has no Situation identity")
	}
	if e.lastDecisionEvidence.SituationID != e.lastSituation.ID {
		t.Fatalf("evidence=%q situation=%q", e.lastDecisionEvidence.SituationID, e.lastSituation.ID)
	}
	if e.lastConversationalCognition.DecisionEvidence.SituationID != e.lastDecisionEvidence.SituationID {
		t.Fatal("cognition did not retain the shared DecisionEvidence")
	}
	_ = interaction.ResponseIntentAnswer
}
