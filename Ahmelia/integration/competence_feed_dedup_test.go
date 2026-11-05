package integration

import (
	"testing"

	"ahmelia/competence"
)

// El feed idempotente debe deduplicar también dentro de UNA MISMA proyección,
// no solo contra lo ya persistido: dos WorkSessions que representan el mismo
// intento (mismo ExecutionAttemptID) no pueden entrar dos veces en el ledger.
// Este es el complemento del criterio "dos boots no duplican" para el caso de
// una lista de entrada con tokens repetidos.
func TestRebuildCompetenceAttemptsDedupWithinBatch(t *testing.T) {
	ledger, err := competence.Load("")
	if err != nil {
		t.Fatalf("empty in-memory ledger: %v", err)
	}
	works := append(feedWorkSessions(), feedWorkSessions()...)

	n, err := RebuildCompetenceAttempts(ledger, works)
	if err != nil {
		t.Fatalf("rebuild with duplicated batch: %v", err)
	}
	if n != 2 {
		t.Fatalf("duplicated tokens must collapse to the two real attempts; added %d", n)
	}
	again, err := RebuildCompetenceAttempts(ledger, works)
	if err != nil {
		t.Fatalf("second rebuild: %v", err)
	}
	if again != 0 {
		t.Fatalf("duplicated batch must be idempotent across rebuilds; added %d", again)
	}
}
