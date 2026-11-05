package shared

import (
	"testing"

	"ahmelia/actioncatalog"
)

// TestWiredActions_RenombrarArchivoIsWired documents the converged contract:
// the action has real engine and standalone execution bindings, so the
// canonical catalog must expose it as wired.
func TestWiredActions_RenombrarArchivoIsWired(t *testing.T) {
	if !WiredActions["renombrar_archivo"] {
		t.Error("renombrar_archivo tiene ejecución real y debe estar Wired en el catálogo canónico")
	}
}

// TestWiredActions_DerivedFromCanonicalCatalog ensures shared/WiredActions is
// a projection of the canonical action catalog rather than an independent
// hand-maintained list.
func TestWiredActions_DerivedFromCanonicalCatalog(t *testing.T) {
	for action := range WiredActions {
		def, ok := actioncatalog.ActionDefinitionFor(action)
		if !ok || !def.EngineWired {
			t.Errorf("WiredActions contiene %q sin definición EngineWired en el catálogo canónico", action)
		}
	}
}
