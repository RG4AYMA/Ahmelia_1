package core

import (
	"strings"
	"testing"
)

func TestProductionDelegationPreambleDoesNotReachAuthorization(t *testing.T) {
	t.Setenv("AHMELIA_FLOW_TRACE", "1")
	t.Setenv("AHMELIA_DISABLE_TTS", "1")
	t.Setenv("AHMELIA_SKIP_PROGRAM_INDEX_SCAN", "1")
	e := buildTestEngine(t)

	before, hadBefore := e.workStore.Last()
	e.Process("Quiero que implementes una pequeña utilidad de programación.")

	joined := strings.Join(e.lastFlowTrace.Snapshot(), "\n")
	if !strings.Contains(joined, "OBJECTIVE ASSEMBLY") {
		t.Fatalf("expected objective assembly boundary in trace:\n%s", joined)
	}
	if strings.Contains(joined, "authorization required") {
		t.Fatalf("delegation preamble crossed authorization boundary:\n%s", joined)
	}
	after, hadAfter := e.workStore.Last()
	if !hadBefore && hadAfter {
		t.Fatal("incomplete delegation preamble created a new production WorkSession")
	}
	if hadBefore && hadAfter && after.ID != before.ID {
		t.Fatalf("incomplete delegation preamble replaced operational work: before=%s after=%s", before.ID, after.ID)
	}
}
