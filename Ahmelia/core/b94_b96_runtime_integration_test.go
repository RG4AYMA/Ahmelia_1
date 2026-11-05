package core

import (
	"os"
	"testing"

	"ahmelia/memory"
	"ahmelia/selfknowledge"
)

func TestB94B96RuntimeIntegration_IsReachableFromEngine(t *testing.T) {
	isolateTestWorkspace(t)
	t.Setenv("AHMELIA_SKIP_PROGRAM_INDEX_SCAN", "1")
	longMem := memory.LoadLongMemory()
	shortMem := LoadShortMemory()
	sess := memory.NewSession()
	history := selfknowledge.NewActionHistory(4)
	registry := selfknowledge.NewModuleRegistry()
	selfQuery := selfknowledge.NewSelfQuery(history, registry, "test")
	engine := NewEngine(sess, longMem, &shortMem, history, selfQuery)
	if engine.capabilityRegistry == nil || engine.capabilityDiscovery == nil {
		t.Fatal("B94 registry/discovery no quedaron conectados al Engine")
	}
	if engine.programmingIntegration == nil {
		t.Fatal("B96 ProgrammingIntegration no quedó conectado al Engine")
	}
	if got := engine.capabilityRegistry.Count(); got < 6 {
		t.Fatalf("esperaba capacidades de programación registradas, got %d", got)
	}
	best, ok, err := engine.capabilityDiscovery.Best("crear un programa Go")
	if err != nil || !ok {
		t.Fatalf("Capability Discovery no resolvió una tarea de programación: ok=%v err=%v", ok, err)
	}
	if best.Capability.Domain != "local" {
		t.Fatalf("capability inesperada: %q", best.Capability.Domain)
	}
	_ = os.Setenv("AHMELIA_SKIP_PROGRAM_INDEX_SCAN", "1")
}
