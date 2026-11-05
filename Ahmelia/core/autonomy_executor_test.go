package core

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"ahmelia/autonomy"
)

// TestAutonomyExecutor_CrearCarpetaReal confirma el camino arquitectónico
// completo: ActionRequest -> shared cognition -> CapabilityInteractionDecision
// -> Engine.authorityExecute -> solving provider -> disco real.
func TestAutonomyExecutor_CrearCarpetaReal(t *testing.T) {
	e := newB963334AuditEngine(t)
	exec := NewAutonomyExecutor(e)
	dir := t.TempDir()
	target := filepath.Join(dir, "notas_post_auditoria")

	resp := exec.ExecuteAction(context.Background(), autonomy.ActionRequest{
		ID:     "req-1",
		Action: "crear_carpeta",
		Target: target,
	})

	if !resp.Executed {
		t.Fatalf("esperaba Executed=true, obtuve false; error=%v output=%q", resp.Error, resp.Output)
	}
	if resp.RequestID != "req-1" {
		t.Errorf("RequestID no se propagó: obtuve %q", resp.RequestID)
	}
	if info, err := os.Stat(target); err != nil || !info.IsDir() {
		t.Fatalf("la carpeta no existe realmente en disco tras ExecuteAction: %v", err)
	}
}

// TestAutonomyExecutor_MoverArchivoReal confirma que la misma autoridad cubre
// otra operación de la clase de capacidades filesystem, no solo crear_carpeta.
func TestAutonomyExecutor_MoverArchivoReal(t *testing.T) {
	e := newB963334AuditEngine(t)
	exec := NewAutonomyExecutor(e)
	dir := t.TempDir()
	source := filepath.Join(dir, "origen.txt")
	if err := os.WriteFile(source, []byte("contenido"), 0644); err != nil {
		t.Fatalf("no pude preparar el archivo de origen: %v", err)
	}
	destDir := filepath.Join(dir, "destino")
	if err := os.MkdirAll(destDir, 0755); err != nil {
		t.Fatalf("no pude preparar la carpeta destino: %v", err)
	}

	resp := exec.ExecuteAction(context.Background(), autonomy.ActionRequest{
		ID:     "req-2",
		Action: "mover_archivo",
		Parameters: map[string]string{
			"origen":  source,
			"destino": destDir,
		},
	})

	if !resp.Executed {
		t.Fatalf("esperaba Executed=true, obtuve false; error=%v output=%q", resp.Error, resp.Output)
	}
	if _, err := os.Stat(filepath.Join(destDir, "origen.txt")); err != nil {
		t.Fatalf("el archivo no aparece movido realmente en destino: %v", err)
	}
	if _, err := os.Stat(source); !os.IsNotExist(err) {
		t.Fatalf("el archivo origen todavía existe, no se movió de verdad")
	}
}

// TestAutonomyExecutor_MoverArchivoSinParametros confirma que una propuesta
// autónoma mal formada llega a la autoridad y al proveedor, donde queda
// rechazada por parámetros faltantes en lugar de ejecutarse parcialmente.
func TestAutonomyExecutor_MoverArchivoSinParametros(t *testing.T) {
	e := newB963334AuditEngine(t)
	exec := NewAutonomyExecutor(e)

	resp := exec.ExecuteAction(context.Background(), autonomy.ActionRequest{
		ID:     "req-3",
		Action: "mover_archivo",
		Target: "sin-origen-ni-destino",
	})

	if resp.AuthorityResult == nil {
		t.Fatal("el rechazo debe conservar el resultado estructurado de la autoridad")
	}
	if resp.AuthorityResult.AuthoritySucceeded() || resp.AuthorityResult.AuthorityVerified() {
		t.Fatalf("una acción sin parámetros no puede resultar exitosa ni verificada: %+v", resp.AuthorityResult)
	}
	if resp.AuthorityResult.AuthorityFailureReason() == "" && resp.AuthorityResult.AuthorityOutcomeCode() == "" {
		t.Fatal("el resultado estructurado debe explicar el rechazo")
	}
}

// TestAutonomyExecutor_AccionNoSoportadaEsHonesta confirma que una Action que
// no existe en el proveedor autónomo no puede inventar un camino de ejecución.
func TestAutonomyExecutor_AccionNoRegistradaEsHonesta(t *testing.T) {
	e := newB963334AuditEngine(t)
	exec := NewAutonomyExecutor(e)

	resp := exec.ExecuteAction(context.Background(), autonomy.ActionRequest{
		ID:     "req-4",
		Action: "accion_no_registrada",
		Target: "notepad",
	})

	if resp.AuthorityResult == nil {
		t.Fatal("la acción no registrada debe conservar el resultado de rechazo de la autoridad")
	}
	if resp.AuthorityResult.AuthorityHandled() || resp.AuthorityResult.AuthoritySucceeded() || resp.AuthorityResult.AuthorityVerified() {
		t.Fatalf("una acción no registrada no puede aparecer como gestionada, exitosa o verificada: %+v", resp.AuthorityResult)
	}
	if resp.RequestID != "req-4" {
		t.Errorf("RequestID no se propagó en el camino de error: obtuve %q", resp.RequestID)
	}
}

// TestAutonomyExecutor_ContextoCancelado confirma que un contexto cancelado
// no puede alcanzar la autoridad ni tocar el mundo real.
func TestAutonomyExecutor_ContextoCancelado(t *testing.T) {
	e := newB963334AuditEngine(t)
	exec := NewAutonomyExecutor(e)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	dir := t.TempDir()
	target := filepath.Join(dir, "no_deberia_crearse")
	resp := exec.ExecuteAction(ctx, autonomy.ActionRequest{
		ID:     "req-5",
		Action: "crear_carpeta",
		Target: target,
	})

	if resp.Executed {
		t.Fatal("esperaba Executed=false con contexto cancelado, obtuve true")
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatal("la carpeta se creó a pesar del contexto cancelado")
	}
}

func TestAutonomyExecutor_SinEngineFallaCerrado(t *testing.T) {
	exec := NewAutonomyExecutor(nil)
	resp := exec.ExecuteAction(context.Background(), autonomy.ActionRequest{ID: "req-6", Action: "crear_carpeta", Target: "x"})
	if resp.Executed || resp.Error == nil {
		t.Fatalf("sin Engine la ejecución debe quedar cerrada: %+v", resp)
	}
}

func TestAutonomyExecutor_ImplementaExecutorAdapter(t *testing.T) {
	var _ autonomy.ExecutorAdapter = NewAutonomyExecutor(nil)
}
