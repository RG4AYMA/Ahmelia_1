package core

import (
	"testing"

	"ahmelia/capability"
)

func TestProcedureCatalog_Known(t *testing.T) {
	// Engine mínimo con capabilityRegistry
	capReg := capability.NewRegistry()
	capReg.MustRegister(capability.Capability{
		ID:         capability.ID("programming"),
		Action:     "create_file",
		Domain:     capability.DomainLocal,
		Lane:       capability.LaneAction,
		Permission: capability.PermissionAutomatic,
	})

	e := &Engine{
		capabilityRegistry: capReg,
	}

	// Capability builtin conocida
	if !e.KnownProcedureCapability("programming", "create_file") {
		t.Fatal("capability builtin conocida debería retornar true")
	}

	// ID inexistente
	if e.KnownProcedureCapability("inexistente", "create_file") {
		t.Fatal("ID inexistente debería retornar false")
	}

	// Operación distinta
	if e.KnownProcedureCapability("programming", "operacion_inexistente") {
		t.Fatal("operación distinta debería retornar false")
	}

	// Sin registry
	e2 := &Engine{capabilityRegistry: nil}
	if e2.KnownProcedureCapability("programming", "create_file") {
		t.Fatal("sin registry debería retornar false")
	}
}

func TestRegistryCatalog_Direct(t *testing.T) {
	capReg := capability.NewRegistry()
	capReg.MustRegister(capability.Capability{
		ID:         capability.ID("test_cap"),
		Action:     "test_op",
		Domain:     capability.DomainLocal,
		Lane:       capability.LaneAction,
		Permission: capability.PermissionAutomatic,
	})

	cat := registryCatalog{r: capReg}

	if !cat.Known("test_cap", "test_op") {
		t.Fatal("Known debería retornar true para capability registrada")
	}
	if cat.Known("test_cap", "otra_op") {
		t.Fatal("Known debería retornar false para operación distinta")
	}
	if cat.Known("otra_cap", "test_op") {
		t.Fatal("Known debería retornar false para capability distinta")
	}
}
