package core

import (
	"os"
	"sync"
	"testing"
)

// TestMain — hermeticidad T0: la suite de core nunca lanza TTS real
// (tts.Speak) durante una corrida de tests. Este es el único punto donde se
// fija el interruptor para todo el proceso; los tests que necesiten
// comprobarlo pueden leer el env.
func TestMain(m *testing.M) {
	_ = os.Setenv("AHMELIA_DISABLE_TTS", "1")
	os.Exit(m.Run())
}

// testWorkspaceIsolated registra los tests que ya movieron su directorio de
// trabajo temporal (clave: *testing.T).
var testWorkspaceIsolated sync.Map

// isolateTestWorkspace mueve el proceso a un directorio temporal propio del
// test para que el estado durable relativo ("memory/...") que el Engine lee y
// escribe no se comparta entre tests ni toque el árbol del repo (hermeticidad
// T0). Es idempotente dentro del mismo test: llamadas repetidas (p. ej. tests
// que levantan dos motores para simular un reinicio) comparten el mismo
// directorio temporal. El directorio original se restaura al terminar el
// test.
func isolateTestWorkspace(t *testing.T) {
	t.Helper()
	if _, seen := testWorkspaceIsolated.LoadOrStore(t, struct{}{}); seen {
		return
	}
	oldDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		testWorkspaceIsolated.Delete(t)
		_ = os.Chdir(oldDir)
	})
}
