package core

import (
	"testing"

	"ahmelia/resource"
)

// T3 (2026-10-17) — El gobernador de recursos debe quedar cableado al
// bootstrap real: el cliente lingüístico del Engine atiende todas sus
// generaciones a través de la puerta del gobernador (una sola ranura,
// prioridades reales). Antes de este cableado el motor existía completo
// en resource/ sin un solo consumidor (auditoría T0).
func TestT3ResourceGovernorWired(t *testing.T) {
	e := NewEngine(nil, nil, nil, nil, nil)
	if e == nil {
		t.Fatal("NewEngine devolvió nil")
	}
	if e.resourceGovernor == nil {
		t.Fatal("engine.resourceGovernor nil: el gobernador no está cableado al bootstrap")
	}
	if e.linguisticClient == nil {
		t.Fatal("engine.linguisticClient nil")
	}
	gate, ok := e.linguisticClient.Gate.(*resource.LinguisticsGate)
	if !ok || gate == nil {
		t.Fatalf("linguisticClient.Gate no es *resource.LinguisticsGate: %T", e.linguisticClient.Gate)
	}

	// El gobernador arranca libre y sin cola (nada corriendo aún).
	stats := e.resourceGovernor.Stats()
	if stats.Active != nil || stats.QueueLength != 0 {
		t.Fatalf("gobernador debe arrancar libre: %+v", stats)
	}
}
