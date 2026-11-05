package core

import (
	"os"
	"strings"
	"testing"
)

func TestExitRequestContract(t *testing.T) {
	e := &Engine{}
	if e.ExitRequested() {
		t.Fatal("un Engine nuevo no debe tener cierre pedido")
	}
	e.RequestExit()
	if !e.ExitRequested() {
		t.Fatal("RequestExit debe marcar la petición de cierre")
	}
}

// Guarda de regresión H7 (2026-10-17, corrida viva 17:12): «salir» guardaba
// todo con Shutdown() pero el proceso quedaba vivo en el lazo de voz. La
// biblioteca NO debe matar el proceso (os.Exit rompería cualquier suite que
// ejercite el cierre); pide el cierre y la raíz viva (main.go) lo ejecuta.
func TestSalirNeverKillsProcessFromLibrary(t *testing.T) {
	const osExitLiteral = "os.Exit(0)"
	salirFiles := []string{"turn_pipeline.go", "engine.go", "local_action_executor.go"}
	for _, file := range salirFiles {
		src, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("no se pudo leer %s: %v", file, err)
		}
		if strings.Contains(string(src), osExitLiteral) {
			t.Fatalf("%s llama %s: el cierre de sesión no debe matar el proceso desde la biblioteca (H7); usar RequestExit", file, osExitLiteral)
		}
		if !strings.Contains(string(src), "e.RequestExit()") {
			t.Fatalf("%s no pide el cierre con e.RequestExit() (H7)", file)
		}
	}

	engineSrc, err := os.ReadFile("engine.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"func (e *Engine) RequestExit()", "func (e *Engine) ExitRequested()"} {
		if !strings.Contains(string(engineSrc), want) {
			t.Fatalf("engine.go no contiene %q (contrato H7)", want)
		}
	}

	mainSrc, err := os.ReadFile("../main.go")
	if err != nil {
		t.Fatalf("no se pudo leer ../main.go: %v", err)
	}
	if !strings.Contains(string(mainSrc), "engine.ExitRequested()") {
		t.Fatal("main.go debe observar engine.ExitRequested() para terminar el proceso (H7)")
	}
}
