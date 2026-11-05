package core

import (
	"context"
	"fmt"
	"time"
)

// ============================================================================
// HEARTBEAT.GO — Latido N0+N1 v0 (continuidad temporal, 2026-10-17)
// ============================================================================
//
// Directiva del propietario: "el sistema debe estar continuamente activo y
// continuamente comprendiendo lo que pasa… sus ciclos deben ser lo más
// rápidos posibles y repetir."
//
// Este archivo pedalea ruedas que ya existían sin driver (auditoría T0):
//
//	N0 (default 1 s, barato): percepción de foco (system.GetForegroundProcess
//	  por syscalls Win32, costo de microsegundos) + driver del gobernador de
//	  recursos (PreemptIfNeeded: envejecimiento y preempt de titulares
//	  vencidos). Cero LLM. No toma locks de conversación ni de autonomía:
//	  el turno del usuario nunca espera al latido.
//
//	N1 (default cada 10 pulsos N0): digestión completa del mundo
//	  (ComputerUseSnapshot: app activa/proyecto/archivo/actividad) con
//	  huella única; los cambios se registran en un anillo ACOTADO
//	  (heartbeatMaxDeltas) con hora y resumen. Cero LLM.
//
// Mediciones reales (2026-10-17, esta máquina): el escaneo completo
// (scanOSWindows → PowerShell) cuesta ~310 ms por invocación — por eso N1
// va a 10 s y N0 NO lo toca; la reconciliación de foco por syscalls cuesta
// microsegundos y sostiene el 1 Hz. La lista de apps abiertas se refresca
// dentro de la misma pasada de N1 y la lee quien la pida (mismo contrato
// bajo demanda que ya existía).
//
// Arranque explícito: solo la raíz viva (main.go) llama a StartHeartbeat.
// Las suites no lo arrancan solas, así que su comportamiento no cambia.
// Las fuentes (pulso, lectura de foco, digestión, gobernador) son campos
// inyectables para tests; los defaults reales se cablean en startHeartbeat.

const (
	heartbeatDefaultN0Pulse = time.Second
	heartbeatDefaultN1Every = 10
	heartbeatMaxDeltas      = 8
)

// HeartbeatConfig permite ajustar cadencias (los tests usan valores cortos).
type HeartbeatConfig struct {
	// N0Pulse es el período del pulso barato. <= 0 usa el default (1 s).
	N0Pulse time.Duration
	// N1Every es cada cuántos pulsos N0 corre la digestión completa.
	// <= 0 usa el default (10).
	N1Every int
}

// HeartbeatDelta es un cambio observado por el latido. Kind distingue la
// capa que lo vio: "foco" (N0: la referencia activa cambió) o "mundo"
// (N1: la huella de contexto de trabajo cambió).
type HeartbeatDelta struct {
	At          time.Time `json:"at"`
	Kind        string    `json:"kind"`
	Fingerprint string    `json:"fingerprint,omitempty"`
	Summary     string    `json:"summary"`
}

// HeartbeatStats es la vista pública (copia) del estado del latido.
type HeartbeatStats struct {
	Running              bool
	N0Ticks              int64
	N1Ticks              int64
	GovernorPulses       int64
	FocusChanges         int64
	WorldChanges         int64
	LastN0At             time.Time
	LastN1At             time.Time
	LastFocusKey         string
	LastWorldFingerprint string
	LastWorldSummary     string
	LastWorldChangeAt    time.Time
	RecentDeltas         []HeartbeatDelta
}

type heartbeatState struct {
	cancel               context.CancelFunc
	running              bool
	n0Ticks              int64
	n1Ticks              int64
	governorPulses       int64
	focusChanges         int64
	worldChanges         int64
	lastN0At             time.Time
	lastN1At             time.Time
	lastFocusKey         string
	focusSeeded          bool
	lastWorldFingerprint string
	lastWorldSummary     string
	lastWorldChangeAt    time.Time
	recentDeltas         []HeartbeatDelta
}

// StartHeartbeat arranca el latido en la raíz viva con las cadencias default.
func (e *Engine) StartHeartbeat(ctx context.Context) {
	e.startHeartbeat(ctx, HeartbeatConfig{})
}

func (e *Engine) startHeartbeat(ctx context.Context, cfg HeartbeatConfig) {
	if e == nil {
		return
	}
	if cfg.N0Pulse <= 0 {
		cfg.N0Pulse = heartbeatDefaultN0Pulse
	}
	if cfg.N1Every <= 0 {
		cfg.N1Every = heartbeatDefaultN1Every
	}

	// Fuentes reales por defecto; los tests pueden inyectar fakes antes.
	if e.heartbeatPulse == nil {
		e.heartbeatPulse = func() {
			if e.world != nil {
				e.world.SyncForeground()
			}
		}
	}
	if e.heartbeatFocusKey == nil {
		e.heartbeatFocusKey = func() (string, bool) {
			if e.world == nil {
				return "", false
			}
			return e.world.ActiveAppKey()
		}
	}
	if e.heartbeatGovernor == nil {
		e.heartbeatGovernor = func() {
			if e.resourceGovernor != nil {
				e.resourceGovernor.PreemptIfNeeded()
			}
		}
	}
	if e.heartbeatDigest == nil {
		e.heartbeatDigest = func() (string, string, bool) {
			if e.world == nil {
				return "", "", false
			}
			snap, ok := e.world.ComputerUseSnapshot()
			if !ok {
				return "", "", false
			}
			summary := snap.ActiveApplication
			if activity := snap.UI["activity"]; activity != "" {
				summary += " activity=" + activity
			}
			if project := snap.UI["project"]; project != "" {
				summary += " project=" + project
			}
			if file := snap.UI["file"]; file != "" {
				summary += " file=" + file
			}
			return snap.Fingerprint, summary, true
		}
	}

	e.heartbeatMu.Lock()
	if e.heartbeat.running {
		e.heartbeatMu.Unlock()
		return
	}
	loopCtx, cancel := context.WithCancel(ctx)
	e.heartbeat.cancel = cancel
	e.heartbeat.running = true
	e.heartbeatMu.Unlock()

	fmt.Printf("💓 LATIDO | arrancado N0=%s N1=1/%d → foco + gobernador; digestión del mundo cada %s\n",
		cfg.N0Pulse, cfg.N1Every, time.Duration(cfg.N1Every)*cfg.N0Pulse)

	go func() {
		defer func() {
			e.heartbeatMu.Lock()
			e.heartbeat.cancel = nil
			e.heartbeat.running = false
			e.heartbeatMu.Unlock()
		}()
		e.runHeartbeatLoop(loopCtx, cfg)
	}()
}

func (e *Engine) runHeartbeatLoop(ctx context.Context, cfg HeartbeatConfig) {
	ticker := time.NewTicker(cfg.N0Pulse)
	defer ticker.Stop()
	pulse := 0
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			pulse++
			e.heartbeatN0Tick(time.Now())
			if pulse%cfg.N1Every == 0 {
				e.heartbeatN1Tick(time.Now())
			}
		}
	}
}

// heartbeatN0Tick es el pulso barato: foco real + gobernador + registro del
// cambio de referencia activa. No escanea el sistema completo.
func (e *Engine) heartbeatN0Tick(now time.Time) {
	if e == nil {
		return
	}
	if e.heartbeatPulse != nil {
		e.heartbeatPulse()
	}
	if e.heartbeatGovernor != nil {
		e.heartbeatGovernor()
	}

	focusKey, focusOK := "", false
	if e.heartbeatFocusKey != nil {
		focusKey, focusOK = e.heartbeatFocusKey()
	}

	var delta HeartbeatDelta
	e.heartbeatMu.Lock()
	e.heartbeat.n0Ticks++
	e.heartbeat.lastN0At = now
	e.heartbeat.governorPulses++
	if focusOK && focusKey != "" {
		switch {
		case !e.heartbeat.focusSeeded:
			e.heartbeat.focusSeeded = true
			e.heartbeat.lastFocusKey = focusKey
		case focusKey != e.heartbeat.lastFocusKey:
			e.heartbeat.focusChanges++
			e.heartbeat.lastFocusKey = focusKey
			delta = HeartbeatDelta{At: now, Kind: "foco", Summary: focusKey}
			e.heartbeat.recentDeltas = appendBoundedDelta(e.heartbeat.recentDeltas, delta)
		}
	}
	n0 := e.heartbeat.n0Ticks
	e.heartbeatMu.Unlock()

	if delta.Kind != "" {
		fmt.Printf("💓 LATIDO | foco: %s | n0=%d\n", delta.Summary, n0)
	}
}

// heartbeatN1Tick es la digestión completa: huella del mundo + anillo
// acotado de cambios. La primera digestión siembra la huella (no cuenta
// como cambio); sin app activa identificable no toca el estado.
func (e *Engine) heartbeatN1Tick(now time.Time) {
	if e == nil || e.heartbeatDigest == nil {
		return
	}
	fingerprint, summary, ok := e.heartbeatDigest()

	var seeded, changed bool
	e.heartbeatMu.Lock()
	e.heartbeat.n1Ticks++
	e.heartbeat.lastN1At = now
	if ok && fingerprint != "" {
		switch {
		case e.heartbeat.lastWorldFingerprint == "":
			e.heartbeat.lastWorldFingerprint = fingerprint
			e.heartbeat.lastWorldSummary = summary
			seeded = true
		case fingerprint != e.heartbeat.lastWorldFingerprint:
			e.heartbeat.worldChanges++
			e.heartbeat.lastWorldChangeAt = now
			e.heartbeat.lastWorldFingerprint = fingerprint
			e.heartbeat.lastWorldSummary = summary
			e.heartbeat.recentDeltas = appendBoundedDelta(e.heartbeat.recentDeltas, HeartbeatDelta{
				At: now, Kind: "mundo", Fingerprint: fingerprint, Summary: summary,
			})
			changed = true
		}
	}
	n1 := e.heartbeat.n1Ticks
	e.heartbeatMu.Unlock()

	switch {
	case seeded:
		fmt.Printf("💓 LATIDO | mundo sembrado: %s | n1=%d\n", summary, n1)
	case changed:
		fmt.Printf("💓 LATIDO | mundo: %s | n1=%d\n", summary, n1)
	}
}

// appendBoundedDelta mantiene el anillo de deltas acotado — memoria fija:
// la digestión N1 no crece sin techo por construcción.
func appendBoundedDelta(deltas []HeartbeatDelta, delta HeartbeatDelta) []HeartbeatDelta {
	deltas = append(deltas, delta)
	if len(deltas) > heartbeatMaxDeltas {
		trimmed := make([]HeartbeatDelta, heartbeatMaxDeltas)
		copy(trimmed, deltas[len(deltas)-heartbeatMaxDeltas:])
		deltas = trimmed
	}
	return deltas
}

// stopHeartbeat cancela el latido si está corriendo (idempotente).
func (e *Engine) stopHeartbeat() {
	if e == nil {
		return
	}
	e.heartbeatMu.Lock()
	cancel := e.heartbeat.cancel
	e.heartbeatMu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// HeartbeatStats devuelve una copia consistente del estado del latido.
func (e *Engine) HeartbeatStats() HeartbeatStats {
	if e == nil {
		return HeartbeatStats{}
	}
	e.heartbeatMu.Lock()
	defer e.heartbeatMu.Unlock()
	stats := HeartbeatStats{
		Running:              e.heartbeat.running,
		N0Ticks:              e.heartbeat.n0Ticks,
		N1Ticks:              e.heartbeat.n1Ticks,
		GovernorPulses:       e.heartbeat.governorPulses,
		FocusChanges:         e.heartbeat.focusChanges,
		WorldChanges:         e.heartbeat.worldChanges,
		LastN0At:             e.heartbeat.lastN0At,
		LastN1At:             e.heartbeat.lastN1At,
		LastFocusKey:         e.heartbeat.lastFocusKey,
		LastWorldFingerprint: e.heartbeat.lastWorldFingerprint,
		LastWorldSummary:     e.heartbeat.lastWorldSummary,
		LastWorldChangeAt:    e.heartbeat.lastWorldChangeAt,
	}
	if len(e.heartbeat.recentDeltas) > 0 {
		stats.RecentDeltas = append([]HeartbeatDelta(nil), e.heartbeat.recentDeltas...)
	}
	return stats
}
