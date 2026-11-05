package core

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"ahmelia/autonomy"
)

// Regresión de la familia (a) de T0 (2026-10-16): una ruta real que contiene
// la palabra de un alias de carpeta especial (p. ej. un segmento "Desktop",
// como todo TEMP dentro del Desktop del usuario) no puede ser interceptada
// por el atajo "abrir carpeta especial". Antes de este fix, execute()
// retornaba "abrí la carpeta" con éxito sin crear nada en disco.
func TestAutonomyFolderShortcutDoesNotHijackPathTargets(t *testing.T) {
	e := newB963334AuditEngine(t)
	exec := NewAutonomyExecutor(e)
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "Desktop"), 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(dir, "Desktop", "notas_post_auditoria")

	resp := exec.ExecuteAction(context.Background(), autonomy.ActionRequest{
		ID:     "t0-a-folder-shortcut",
		Action: "crear_carpeta",
		Target: target,
	})
	if !resp.Executed {
		t.Fatalf("esperaba Executed=true, obtuve false; error=%v output=%q", resp.Error, resp.Output)
	}
	if info, err := os.Stat(target); err != nil || !info.IsDir() {
		t.Fatalf("la carpeta no existe realmente en disco tras ExecuteAction: %v", err)
	}
}
