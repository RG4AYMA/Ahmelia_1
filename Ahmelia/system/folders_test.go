package system

import (
	"testing"
)

// T0 familia a (2026-10-16): el alias de carpeta especial solo puede
// resolver cuando el pedido completo ES el alias. Rutas reales, nombres de
// archivo o frases más largas que contengan las palabras NO deben dispararlo.
func TestSpecialFolderAliasOnlyAcceptsWholeAliasRequests(t *testing.T) {
	rejected := []string{
		"",
		`C:\Users\User\Desktop\Ahmelia-Knowledge\.gotmp-validation\TestT\002\b88-created`,
		`C:\Users\User\Desktop`,
		`sub\carpeta\desktop`,
		"desktop.png",
		"videos de la boda",
		"crea una carpeta llamada documentos",
	}
	for _, target := range rejected {
		if key, _, ok := specialFolderAlias(target); ok {
			t.Errorf("target %q resolvió a %q; no debía resolver", target, key)
		}
	}

	accepted := []string{
		"escritorio",
		"mi escritorio",
		"la carpeta de descargas",
		"documentos",
		"música",
		"imagenes",
		"videos",
	}
	for _, target := range accepted {
		if _, path, ok := specialFolderAlias(target); !ok || path == "" {
			t.Errorf("target %q debería resolver a una carpeta especial", target)
		}
	}
}

// Una ruta nunca dispara la apertura real: TryOpenFolder debe responder
// false sin lanzar Explorer (contrato T0 familia a).
func TestTryOpenFolderDoesNotOpenOnPathLikeTargets(t *testing.T) {
	if key, opened := TryOpenFolder(`C:\Users\User\Desktop\proyectos`); opened {
		t.Errorf("TryOpenFolder abrió una carpeta para una ruta (key=%q)", key)
	}
}
