package core

import (
	"testing"
	"time"

	"ahmelia/competence"
	"ahmelia/interaction"
	"ahmelia/situation"
)

// TestCompetenceOverlayReachesDecisionEvidence verifica el cable completo
// T4-M5: trabajo real en la ledger -> Overlay -> buildSharedSituation ->
// CompetencePerspective (relación tipada) -> situation.Evaluate ->
// DecisionFinding. Sin oráculo declarado el estado máximo es unverifiable;
// el finding lo conserva en vez de afirmar una capacidad.
func TestCompetenceOverlayReachesDecisionEvidence(t *testing.T) {
	ledger, err := competence.Load("")
	if err != nil {
		t.Fatalf("in-memory ledger: %v", err)
	}
	if err := ledger.Record(competence.Attempt{
		Capability: "demo.produce",
		Operation:  "create",
		ContextKey: "test-context",
		Strategy:   "procedure:create@1",
		Verified:   true,
		At:         time.Unix(1700000000, 0),
	}); err != nil {
		t.Fatalf("record attempt: %v", err)
	}

	e := &Engine{competenceOverlay: competence.NewOverlay(ledger, nil, nil, competence.DefaultPolicy())}
	sit, contributions := e.buildSharedSituation(
		interaction.SemanticRequest{Raw: "¿Qué sabes hacer?"},
		interaction.ConversationSnapshot{}, interaction.WorkContext{}, false, nil,
	)
	evidence := situation.Evaluate(sit, contributions)

	for _, f := range evidence.Findings {
		if f.Perspective != "competence" || f.Target != "demo.produce" {
			continue
		}
		if f.RelationType != "capability_standing_unverifiable" {
			t.Fatalf("without an oracle the finding must be unverifiable, got %q", f.RelationType)
		}
		if f.Epistemic != situation.EpistemicDerived {
			t.Fatalf("competence finding must be derived, got %q", f.Epistemic)
		}
		return
	}
	t.Fatalf("competence contribution never reached DecisionEvidence: %+v", evidence.Findings)
}
