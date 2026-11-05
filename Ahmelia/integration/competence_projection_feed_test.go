package integration

import (
	"testing"
	"time"

	"ahmelia/competence"
	"ahmelia/production"
)

// feedWorkSessions construye trabajo REAL de prueba (no fixtures de behavior:
// son las formas serializadas que memory/production_work.json persiste).
// Un intento verificado, uno fallido, uno sin capability (no evaluable) y uno
// sin FinishedAt (aún sin fin).
func feedWorkSessions() []production.WorkSession {
	fin := time.Date(2026, 10, 19, 10, 0, 0, 0, time.UTC)
	return []production.WorkSession{{
		ID: "ws-demo-1",
		Artifacts: []production.ProductionArtifact{{
			ID: "art-1", ProjectID: "proj-demo",
		}},
		Attempts: []production.WorkAttempt{
			{
				SkillID: "media.organize", CapabilityID: "media.organize",
				Operation: "organize", FinishedAt: fin.Add(2 * time.Minute),
				ExecutionAttemptID: "exec-1", Result: &production.ActionResult{Verified: true},
			},
			{
				SkillID: "media.organize", CapabilityID: "media.organize",
				Operation: "organize", FinishedAt: fin.Add(5 * time.Minute),
				ExecutionAttemptID: "exec-2", Result: &production.ActionResult{Verified: false, FailureReason: "timeout"},
			},
			{
				SkillID: "media.organize", CapabilityID: "",
				Operation: "organize", FinishedAt: fin.Add(6 * time.Minute),
			},
			{
				SkillID: "media.organize", CapabilityID: "media.organize",
				Operation: "organize", // sin FinishedAt
			},
		}},
	}
}

// TestAttemptsForLedgerProyectaTrabajoRealSoloEvaluable: el feed proyecta
// intentos terminados CON capability; contexto desde artefactos; skill como
// estrategia; token estable.
func TestAttemptsForLedgerProyectaTrabajoRealSoloEvaluable(t *testing.T) {
	attempts := AttemptsForLedger(feedWorkSessions())
	if len(attempts) != 2 {
		t.Fatalf("expected only the two finished evaluated attempts, got %d: %+v", len(attempts), attempts)
	}
	first := attempts[0]
	if first.Token != "exec-1" {
		t.Fatalf("stable token must prefer ExecutionAttemptID, got %q", first.Token)
	}
	if first.ContextKey != "proj-demo" {
		t.Fatalf("context must come from the first artifact project, got %q", first.ContextKey)
	}
	if first.Strategy != "media.organize" {
		t.Fatalf("strategy must carry the executing skill, got %q", first.Strategy)
	}
	if !first.Verified {
		t.Fatalf("verified attempt must project as verified: %+v", first)
	}
	if attempts[1].Verified || attempts[1].FailureClass != "timeout" {
		t.Fatalf("failed attempt must keep its failure class: %+v", attempts[1])
	}
	// Determinismo: la misma sesión persistida produce los mismos tokens en
	// cualquier boot — de aquí nace la idempotencia del feed.
	again := AttemptsForLedger(feedWorkSessions())
	for i := range attempts {
		if again[i].Token != attempts[i].Token {
			t.Fatalf("token must be deterministic across projections: %q vs %q", again[i].Token, attempts[i].Token)
		}
	}
}

// TestRebuildCompetenceAttemptsDosBootsNoDuplican: criterio de aceptación del
// Paso 1.2 — reconstruir la ledger desde el MISMO work store persistido en
// dos boots seguidos no duplica intentos.
func TestRebuildCompetenceAttemptsDosBootsNoDuplican(t *testing.T) {
	ledger, err := competence.Load("")
	if err != nil {
		t.Fatalf("empty in-memory ledger: %v", err)
	}
	works := feedWorkSessions()

	n1, err := RebuildCompetenceAttempts(ledger, works)
	if err != nil {
		t.Fatalf("first boot rebuild: %v", err)
	}
	if n1 != 2 {
		t.Fatalf("first boot must add exactly the two evaluable attempts, added %d", n1)
	}
	n2, err := RebuildCompetenceAttempts(ledger, works)
	if err != nil {
		t.Fatalf("second boot rebuild: %v", err)
	}
	if n2 != 0 {
		t.Fatalf("second boot must NOT duplicate attempts, added %d", n2)
	}
}
