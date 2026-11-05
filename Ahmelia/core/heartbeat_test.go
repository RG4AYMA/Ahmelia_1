package core

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"
)

func TestHeartbeatN0TickPulsesGovernorAndTracksFocus(t *testing.T) {
	e := &Engine{}
	var pulses, governor int
	focus := "chrome"
	e.heartbeatPulse = func() { pulses++ }
	e.heartbeatGovernor = func() { governor++ }
	e.heartbeatFocusKey = func() (string, bool) { return focus, true }

	e.heartbeatN0Tick(time.Now()) // siembra chrome
	e.heartbeatN0Tick(time.Now()) // mismo foco
	focus = "code"
	e.heartbeatN0Tick(time.Now()) // cambio real
	e.heartbeatN0Tick(time.Now()) // mismo foco otra vez

	stats := e.HeartbeatStats()
	if stats.N0Ticks != 4 || pulses != 4 || governor != 4 {
		t.Fatalf("n0=%d pulses=%d governor=%d; se esperaban 4/4/4", stats.N0Ticks, pulses, governor)
	}
	if stats.GovernorPulses != 4 {
		t.Fatalf("GovernorPulses=%d; esperaba 4", stats.GovernorPulses)
	}
	if stats.FocusChanges != 1 {
		t.Fatalf("FocusChanges=%d; la primera lectura siembra y solo chrome→code es cambio", stats.FocusChanges)
	}
	if stats.LastFocusKey != "code" {
		t.Fatalf("LastFocusKey=%q; esperaba code", stats.LastFocusKey)
	}
	if len(stats.RecentDeltas) != 1 || stats.RecentDeltas[0].Kind != "foco" || stats.RecentDeltas[0].Summary != "code" {
		t.Fatalf("deltas=%+v; esperaba un delta de foco a code", stats.RecentDeltas)
	}
	if stats.LastN0At.IsZero() {
		t.Fatal("LastN0At debe quedar marcado")
	}
}

func TestHeartbeatN1SeedsThenCountsWorldChanges(t *testing.T) {
	e := &Engine{}
	fingerprint, summary := "fp-uno", "app=Visual Studio Code activity=programando"
	e.heartbeatDigest = func() (string, string, bool) { return fingerprint, summary, true }

	e.heartbeatN1Tick(time.Now()) // siembra
	stats := e.HeartbeatStats()
	if stats.N1Ticks != 1 || stats.WorldChanges != 0 || stats.LastWorldFingerprint != "fp-uno" {
		t.Fatalf("siembra mal registrada: %+v", stats)
	}

	e.heartbeatN1Tick(time.Now()) // misma huella → sin cambio
	stats = e.HeartbeatStats()
	if stats.WorldChanges != 0 || stats.N1Ticks != 2 {
		t.Fatalf("misma huella no debe contar como cambio: %+v", stats)
	}

	fingerprint, summary = "fp-dos", "app=Chrome activity=navegando"
	e.heartbeatN1Tick(time.Now()) // cambio real
	stats = e.HeartbeatStats()
	if stats.WorldChanges != 1 || stats.LastWorldFingerprint != "fp-dos" || stats.LastWorldSummary != summary {
		t.Fatalf("cambio mal registrado: %+v", stats)
	}
	if len(stats.RecentDeltas) != 1 || stats.RecentDeltas[0].Kind != "mundo" || stats.RecentDeltas[0].Fingerprint != "fp-dos" {
		t.Fatalf("delta de mundo mal: %+v", stats.RecentDeltas)
	}
	if stats.LastWorldChangeAt.IsZero() {
		t.Fatal("LastWorldChangeAt debe quedar marcado")
	}

	// Sin app activa identificable (ok=false) no se toca el estado.
	e.heartbeatDigest = func() (string, string, bool) { return "", "", false }
	e.heartbeatN1Tick(time.Now())
	stats = e.HeartbeatStats()
	if stats.WorldChanges != 1 || stats.LastWorldFingerprint != "fp-dos" {
		t.Fatalf("ok=false no debe alterar el estado: %+v", stats)
	}
}

func TestHeartbeatDeltaRingBounded(t *testing.T) {
	e := &Engine{}
	i := 0
	e.heartbeatDigest = func() (string, string, bool) {
		i++
		return fmt.Sprintf("fp-%02d", i), fmt.Sprintf("estado %d", i), true
	}
	for k := 0; k < 25; k++ {
		e.heartbeatN1Tick(time.Now())
	}

	stats := e.HeartbeatStats()
	if stats.WorldChanges != 24 {
		t.Fatalf("WorldChanges=%d; la primera digestión siembra y no cuenta", stats.WorldChanges)
	}
	if len(stats.RecentDeltas) != heartbeatMaxDeltas {
		t.Fatalf("anillo=%d; debe quedar acotado a %d", len(stats.RecentDeltas), heartbeatMaxDeltas)
	}
	if stats.RecentDeltas[len(stats.RecentDeltas)-1].Fingerprint != "fp-25" {
		t.Fatalf("el anillo debe conservar los más recientes: %+v", stats.RecentDeltas)
	}

	// La copia devuelta no debe permitir mutar el estado interno.
	stats.RecentDeltas[0].Fingerprint = "corrompido"
	if again := e.HeartbeatStats(); again.RecentDeltas[0].Fingerprint == "corrompido" {
		t.Fatal("HeartbeatStats debe devolver una copia")
	}
}

func TestHeartbeatLoopRunsCooperativelyAndStops(t *testing.T) {
	e := &Engine{}
	var governor int64
	e.heartbeatPulse = func() {}
	e.heartbeatGovernor = func() { atomic.AddInt64(&governor, 1) }
	e.heartbeatFocusKey = func() (string, bool) { return "terminal", true }
	e.heartbeatDigest = func() (string, string, bool) { return "fp", "estable", true }

	ctx, cancel := context.WithCancel(context.Background())
	e.startHeartbeat(ctx, HeartbeatConfig{N0Pulse: 20 * time.Millisecond, N1Every: 2})

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		stats := e.HeartbeatStats()
		if stats.N0Ticks >= 4 && stats.N1Ticks >= 2 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	stats := e.HeartbeatStats()
	if stats.N0Ticks < 4 || stats.N1Ticks < 2 {
		t.Fatalf("el latido no avanzó: n0=%d n1=%d", stats.N0Ticks, stats.N1Ticks)
	}
	if !stats.Running {
		t.Fatal("debe reportarse corriendo")
	}
	if atomic.LoadInt64(&governor) < 4 {
		t.Fatalf("gobernador pulsado %d veces; esperaba >= 4", atomic.LoadInt64(&governor))
	}

	// Doble arranque: no debe duplicar el lazo (regresa sin hacer nada).
	e.startHeartbeat(ctx, HeartbeatConfig{N0Pulse: 20 * time.Millisecond, N1Every: 2})

	cancel()
	time.Sleep(150 * time.Millisecond)
	before := e.HeartbeatStats()
	time.Sleep(120 * time.Millisecond)
	after := e.HeartbeatStats()
	if after.N0Ticks-before.N0Ticks > 1 {
		t.Fatalf("no se detuvo con el cancel: %d → %d", before.N0Ticks, after.N0Ticks)
	}
	if after.Running {
		t.Fatal("Running debe quedar en false tras cancelar")
	}
}
