package core

import (
	"os"
	"path/filepath"
	"testing"

	"ahmelia/memory"
	"ahmelia/selfknowledge"
)

// TestLiveSystemProcessCPU exercises the real Engine turn pipeline with the
// local Ollama model. It is opt-in and does not start voice or autonomy.
func TestLiveSystemProcessCPU(t *testing.T) {
	if os.Getenv("AHMELIA_LIVE_SYSTEM") != "1" {
		t.Skip("definir AHMELIA_LIVE_SYSTEM=1 para ejecutar el Engine real")
	}
	if os.Getenv("AHMELIA_OLLAMA_NUM_GPU") != "" {
		t.Fatalf("esta prueba es CPU-only; quitar AHMELIA_OLLAMA_NUM_GPU")
	}
	t.Setenv("AHMELIA_SKIP_PROGRAM_INDEX_SCAN", "1")
	t.Setenv("AHMELIA_DISABLE_TTS", "1")
	t.Setenv("AHMELIA_WORKSPACE_ROOT", filepath.Join(t.TempDir(), "workspace"))
	isolateTestWorkspace(t)

	sess := memory.NewSession()
	longMem := memory.LoadLongMemory()
	shortMem := LoadShortMemory()
	history := selfknowledge.NewActionHistory(20)
	registry := selfknowledge.NewModuleRegistry()
	selfQuery := selfknowledge.NewSelfQuery(history, registry, "v3.1")
	e := NewEngine(sess, longMem, &shortMem, history, selfQuery)
	defer e.Shutdown()

	var responses []string
	e.SetResponseCallback(func(response string) {
		responses = append(responses, response)
		t.Logf("respuesta: %s", response)
	})

	e.Process("hola")
	if len(responses) == 0 {
		t.Fatal("el Engine real no produjo respuesta al saludo")
	}

	before := len(responses)
	e.Process("Crea un programa Go que sume dos números y guárdalo en el proyecto")
	if len(responses) == before {
		t.Fatal("el Engine real no produjo respuesta a la orden de producción")
	}

	before = len(responses)
	e.Process("Sí, autorizo la ejecución pendiente")
	if len(responses) == before {
		t.Fatal("el Engine real no produjo respuesta a la autorización")
	}
	root := os.Getenv("AHMELIA_WORKSPACE_ROOT")
	found := false
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() && filepath.Ext(info.Name()) == ".go" {
			found = true
		}
		return nil
	})
	if !found {
		t.Fatalf("la ejecución autorizada no dejó archivos Go en %s", root)
	}
}
