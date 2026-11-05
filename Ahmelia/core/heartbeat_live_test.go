package core

import (
	"context"
	"os"
	"testing"
	"time"
)

// TestHeartbeatLiveSmoke valida el latido REAL (mundo + gobernador reales,
// sin fakes): para cuando se quiere ver el pulso funcionando de verdad en
// el escritorio, con sus trazas. No corre en suites; se pide a propósito:
//
//	AHMELIA_LIVE_HEARTBEAT=1 go test ./core -run TestHeartbeatLiveSmoke -v -count=1
func TestHeartbeatLiveSmoke(t *testing.T) {
	if os.Getenv("AHMELIA_LIVE_HEARTBEAT") != "1" {
		t.Skip("live: percepción real del escritorio; set AHMELIA_LIVE_HEARTBEAT=1")
	}

	e := NewEngine(nil, nil, nil, nil, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	e.startHeartbeat(ctx, HeartbeatConfig{N0Pulse: time.Second, N1Every: 3})

	time.Sleep(8 * time.Second)

	stats := e.HeartbeatStats()
	t.Logf("latido live: n0=%d n1=%d gobernador=%d foco=%q cambiosFoco=%d mundo=%q cambiosMundo=%d deltas=%d",
		stats.N0Ticks, stats.N1Ticks, stats.GovernorPulses, stats.LastFocusKey, stats.FocusChanges,
		stats.LastWorldSummary, stats.WorldChanges, len(stats.RecentDeltas))

	if stats.N0Ticks < 6 {
		t.Fatalf("N0 no avanzó a 1 Hz: %d pulsos en 8 s", stats.N0Ticks)
	}
	if stats.N1Ticks < 2 {
		t.Fatalf("N1 no corrió: %d digestiones", stats.N1Ticks)
	}
	if stats.GovernorPulses < 6 {
		t.Fatalf("gobernador no fue pulsado: %d", stats.GovernorPulses)
	}
}

// TestHeartbeatLiveScriptedChange es la aceptación "caso (a)" del latido en
// vivo: un cambio scripted del entorno (vía la API pública del Manager — la
// misma que usan los comandos) se percibe dentro de K latidos, con el wiring
// real completo (mundo + gobernador + lazo). Robusto a la actividad real del
// escritorio: solo exige que el cambio se haya contado. Se pide a propósito:
//
//	AHMELIA_LIVE_HEARTBEAT=1 go test ./core -run TestHeartbeatLiveScriptedChange -v -count=1
func TestHeartbeatLiveScriptedChange(t *testing.T) {
	if os.Getenv("AHMELIA_LIVE_HEARTBEAT") != "1" {
		t.Skip("live: percepción real del escritorio; set AHMELIA_LIVE_HEARTBEAT=1")
	}

	e := NewEngine(nil, nil, nil, nil, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	e.startHeartbeat(ctx, HeartbeatConfig{N0Pulse: 300 * time.Millisecond, N1Every: 3})
	time.Sleep(900 * time.Millisecond) // pulso base

	// Cambio scripted #1: la referencia activa pasa a notepad (semilla).
	e.world.RegisterFocused("notepad")
	time.Sleep(900 * time.Millisecond)

	// Cambio scripted #2: pasa a code — este SÍ debe contarse como cambio.
	e.world.RegisterFocused("code")
	time.Sleep(1500 * time.Millisecond)

	stats := e.HeartbeatStats()
	t.Logf("scripted: n0=%d n1=%d foco=%q cambiosFoco=%d mundo=%q cambiosMundo=%d deltas=%d",
		stats.N0Ticks, stats.N1Ticks, stats.LastFocusKey, stats.FocusChanges,
		stats.LastWorldSummary, stats.WorldChanges, len(stats.RecentDeltas))
	for _, d := range stats.RecentDeltas {
		t.Logf("delta: kind=%s summary=%q fp=%s", d.Kind, d.Summary, d.Fingerprint)
	}

	if stats.LastFocusKey == "" {
		t.Fatal("el latido no llegó a tener referencia activa")
	}
	if stats.FocusChanges < 1 {
		t.Fatalf("FocusChanges=%d; el cambio scripted debía contarse", stats.FocusChanges)
	}
	if stats.WorldChanges < 1 {
		t.Fatalf("WorldChanges=%d; la huella del mundo debía cambiar", stats.WorldChanges)
	}
}

// TestHeartbeatLiveSoak mide la cadencia SOSTENIDA real del latido (T3.10):
// corre ~9 minutos con los defaults de producción (N0=1 s, N1=1/10), mide el
// costo real de una digestión completa y reporta cada 90 s. Se pide a
// propósito (una corrida ligera, sin LLM):
//
//	AHMELIA_LIVE_HEARTBEAT_SOAK=1 go test ./core -run TestHeartbeatLiveSoak -v -count=1 -timeout 20m
func TestHeartbeatLiveSoak(t *testing.T) {
	if os.Getenv("AHMELIA_LIVE_HEARTBEAT_SOAK") != "1" {
		t.Skip("live soak: cadencia real del escritorio; set AHMELIA_LIVE_HEARTBEAT_SOAK=1")
	}

	e := NewEngine(nil, nil, nil, nil, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Minute)
	defer cancel()

	start := time.Now()
	e.startHeartbeat(ctx, HeartbeatConfig{}) // defaults reales: N0=1 s, N1=1/10

	// Costo real de UNA digestión completa (lo que N1 paga cada 10 s).
	digestStart := time.Now()
	if _, ok := e.world.ComputerUseSnapshot(); !ok {
		t.Log("digestión: sin contexto activo identificable en este momento")
	}
	t.Logf("digestión real completa (ComputerUseSnapshot): %s", time.Since(digestStart))

	for i := 1; i <= 6; i++ {
		time.Sleep(90 * time.Second)
		stats := e.HeartbeatStats()
		elapsed := time.Since(start)
		t.Logf("soak t=%s n0=%d (%.2f por s) n1=%d gob=%d foco=%q cambiosFoco=%d mundo=%q cambiosMundo=%d deltas=%d",
			elapsed.Round(time.Second), stats.N0Ticks, float64(stats.N0Ticks)/elapsed.Seconds(),
			stats.N1Ticks, stats.GovernorPulses, stats.LastFocusKey, stats.FocusChanges,
			stats.LastWorldSummary, stats.WorldChanges, len(stats.RecentDeltas))
	}

	stats := e.HeartbeatStats()
	elapsed := time.Since(start)
	expected := elapsed.Seconds() * 0.85 // tolera carga: al menos 85% del ideal
	if float64(stats.N0Ticks) < expected {
		t.Fatalf("N0 demasiado lento: %d pulsos en %s", stats.N0Ticks, elapsed)
	}
	if stats.N1Ticks < 6 {
		t.Fatalf("N1 corrió %d veces en %s; se esperaban >= 6 (1/10)", stats.N1Ticks, elapsed)
	}
	if stats.GovernorPulses != stats.N0Ticks {
		t.Fatalf("gobernador y N0 divergen: %d vs %d", stats.GovernorPulses, stats.N0Ticks)
	}
}
