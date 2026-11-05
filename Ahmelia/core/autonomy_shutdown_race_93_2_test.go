package core

import (
	"context"
	"testing"
	"time"

	"ahmelia/autonomy"
)

// TestEngineShutdown_WaitsForAutonomousLoopBeforeReturning es la prueba de
// regresión del hallazgo real de B93.2: antes de este bloque, la lógica
// que hoy vive en stopAutonomyAndWait() (llamada desde Shutdown()) solo
// llamaba a e.autonomyCancel() y seguía de largo hacia
// PersistAutonomyState() sin esperar a que la goroutine de AutonomousLoop
// (que puede seguir mutando planner/goalPlanningAdapter/autonomousState
// varios milisegundos más) efectivamente terminara. Con un Interval de
// ticker muy corto, este test fuerza esa ventana: si stopAutonomyAndWait
// volviera a no esperar, IsRunning() todavía podría dar true justo después
// de la llamada en al menos una corrida (y -count=20 -race lo
// confirmaría). Se prueba stopAutonomyAndWait directamente, no el
// Shutdown() completo: el resto de Shutdown() toca memoria/contexto que
// exigiría un Engine mucho más poblado sin aportar nada a esta prueba
// puntual de sincronización.
func TestEngineShutdown_WaitsForAutonomousLoopBeforeReturning(t *testing.T) {
	loop := autonomy.NewAutonomousExecutionLoop(autonomy.LoopConfig{
		Interval: time.Millisecond,
	}, nil) // cycle nil: runTick no tiene nada que ejecutar, solo nos
	// interesa la vida de la goroutine del ticker en sí.

	ctx, cancel := context.WithCancel(context.Background())
	loop.Start(ctx)

	// Dejamos que el ticker corra un par de vueltas antes de cerrar, para
	// que Shutdown() tenga una goroutine realmente activa por delante.
	time.Sleep(5 * time.Millisecond)

	engine := &Engine{
		autonomousLoop: loop,
		autonomyCancel: cancel,
	}

	engine.stopAutonomyAndWait()

	if loop.IsRunning() {
		t.Fatal("stopAutonomyAndWait() volvió sin esperar a que AutonomousLoop terminara — el contrato de ownership de B93.2 exige que ningún lector corra mientras la única goroutine que escribe ese estado sigue viva")
	}
}
