package core

import (
	"testing"
	"time"

	"ahmelia/personality"
	"ahmelia/situation"
)

// Aceptación P4/directiva Paso 2: un posterior FORMADO (evidencia suficiente)
// aparece como DecisionFinding en el camino real del engine Y la propuesta
// interactiva lo lee (buildInteractiveProposal — ya no es productor sin
// consumidor). Semántica derivada end-to-end.
func TestPreferenceStanceReachesDecisionEvidenceAndProposal(t *testing.T) {
	e := newB963334AuditEngine(t)
	// Seis observaciones de peso 1 superan MinN=5. El peso .8 dejaba la
	// evidencia efectiva en 4.8 y el posterior seguía unformed; el test
	// confundía "seis llamadas" con "seis unidades de evidencia".
	for i := 0; i < 6; i++ {
		if err := e.prefModel.Observe(personality.Experience{Key: "gaseosa", Outcome: 1, Weight: 1, Ref: "turn-seed", At: time.Now()}); err != nil {
			t.Fatalf("sembrar posterior: %v", err)
		}
	}
	var responses []string
	e.SetResponseCallback(func(msg string) { responses = append(responses, msg) })

	e.Process("hola ahmelia")

	found := false
	for _, f := range e.lastDecisionEvidence.Findings {
		if f.RelationType != "reflects_user_preference" {
			continue
		}
		if f.Target == "gaseosa" && f.Epistemic == situation.EpistemicDerived && f.Confidence > .5 {
			found = true
		}
	}
	if !found {
		t.Fatalf("el stance formado debe entrar como DecisionFinding derivado: findings=%d", len(e.lastDecisionEvidence.Findings))
	}
	if e.pendingInteractiveDecision == nil || e.pendingInteractiveDecision.prop == nil {
		t.Fatal("la decisión interactiva no se construyó en el turno real")
	}
	propFound := false
	for _, pe := range e.pendingInteractiveDecision.prop.Evidence {
		if pe.Predicate == "reflects_user_preference" && pe.Object == "gaseosa" && pe.Epistemic == situation.EpistemicDerived {
			propFound = true
		}
	}
	if !propFound {
		t.Fatalf("la propuesta interactiva debe LEER el stance del posterior: evidencias=%d", len(e.pendingInteractiveDecision.prop.Evidence))
	}
}

// Piso honesto en el camino real: sin posterior formado NO hay finding ni
// evidencia de preferencia en la propuesta (la ausencia no se disimula, el
// turno queda exactamente como hoy).
func TestPreferenceStanceAbsenceIsSilent(t *testing.T) {
	e := newB963334AuditEngine(t)
	var responses []string
	e.SetResponseCallback(func(msg string) { responses = append(responses, msg) })

	e.Process("hola ahmelia")

	for _, f := range e.lastDecisionEvidence.Findings {
		if f.RelationType == "reflects_user_preference" {
			t.Fatalf("sin posterior formado no debe existir el finding: %+v", f)
		}
	}
	if e.pendingInteractiveDecision != nil && e.pendingInteractiveDecision.prop != nil {
		for _, pe := range e.pendingInteractiveDecision.prop.Evidence {
			if pe.Predicate == "reflects_user_preference" {
				t.Fatalf("sin posterior formado la propuesta no debe llevar evidencia de preferencia: %+v", pe)
			}
		}
	}
}
