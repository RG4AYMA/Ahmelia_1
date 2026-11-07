package core

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ahmelia/interaction"
	"ahmelia/restriction"
)

func ownerRestrictionTestEngine(t *testing.T, path string) *Engine {
	t.Helper()
	e := &Engine{}
	e.enableOwnerRestrictionsAt(path)
	if note := e.LastOwnerRestrictionNote(); note != "" {
		t.Fatalf("enable: %s", note)
	}
	return e
}

func TestParseOwnerRestrictionCommand(t *testing.T) {
	cases := []struct {
		raw  string
		want ownerRestrictionCommandKind
	}{
		{"hoy no trabajamos", ownerRestrictionRestToday},
		{"Hoy no trabajamos.", ownerRestrictionRestToday},
		{"hoy descansamos", ownerRestrictionRestToday},
		{"descanso hoy", ownerRestrictionRestToday},
		{"hoy sí trabajamos", ownerRestrictionWorkToday},
		{"Hoy trabajamos", ownerRestrictionWorkToday},
		{"volvemos a trabajar", ownerRestrictionWorkToday},
		{"hoy trabajo en el jardín", ownerRestrictionNone},
		{"no trabajamos hoy, pero mañana sí", ownerRestrictionNone},
		{"hola, ¿cómo estás?", ownerRestrictionNone},
	}
	for _, tc := range cases {
		if got := parseOwnerRestrictionCommand(tc.raw).Kind; got != tc.want {
			t.Errorf("%q: kind = %d, quería %d", tc.raw, got, tc.want)
		}
	}
}

func TestOwnerRestrictionApplyIsBoundedAndReversible(t *testing.T) {
	e := ownerRestrictionTestEngine(t, filepath.Join(t.TempDir(), "owner_restrictions.json"))
	loc := time.Local
	now := time.Date(2026, 11, 5, 8, 0, 0, 0, loc)
	endOfDay := time.Date(2026, 11, 6, 0, 0, 0, 0, loc)

	text, handled := e.applyOwnerRestrictionCommand(
		ownerRestrictionCommand{Kind: ownerRestrictionRestToday}, now,
	)
	if !handled || !strings.Contains(text, "hoy no trabajamos") {
		t.Fatalf("respuesta = %q handled=%t", text, handled)
	}
	if !e.OwnerWorkInitiativeRestricted(now) {
		t.Fatal("la restricción debía gobernar a las 08:00")
	}
	if !e.OwnerWorkInitiativeRestricted(endOfDay.Add(-time.Minute)) {
		t.Fatal("la restricción debía gobernar a las 23:59")
	}
	if e.OwnerWorkInitiativeRestricted(endOfDay) {
		t.Fatal("la restricción no debía gobernar a medianoche")
	}

	// Repetir la orden no duplica la restricción.
	if _, handled := e.applyOwnerRestrictionCommand(
		ownerRestrictionCommand{Kind: ownerRestrictionRestToday}, now,
	); !handled {
		t.Fatal("la repetición debía responderse")
	}
	if got := len(e.ActiveOwnerRestrictions(now)); got != 1 {
		t.Fatalf("restricciones activas = %d, quería 1", got)
	}

	// El levante explícito reabre la iniciativa y queda en el historial.
	if text, handled := e.applyOwnerRestrictionCommand(
		ownerRestrictionCommand{Kind: ownerRestrictionWorkToday}, now,
	); !handled || !strings.Contains(text, "levanto") {
		t.Fatalf("levante = %q handled=%t", text, handled)
	}
	if e.OwnerWorkInitiativeRestricted(now) {
		t.Fatal("tras el levante no debía gobernar")
	}
	history := e.OwnerRestrictionHistory()
	if len(history) != 1 || history[0].Status != restriction.StatusLifted || history[0].LiftedBy != "owner" {
		t.Fatalf("historial = %+v", history)
	}
}

func TestScene1SimulatedDayThroughEngineGates(t *testing.T) {
	loc := time.FixedZone("COT", -5*3600)
	now := time.Date(2026, 11, 5, 8, 0, 0, 0, loc)
	dir := t.TempDir()
	e := persistentModeTestEngine(t, filepath.Join(dir, "persistent_mode.json"))
	e.enableOwnerRestrictionsAt(filepath.Join(dir, "owner_restrictions.json"))
	e.sceneNow = func() time.Time { return now }

	// 08:00 — the owner's phrase passes through the real deterministic gate.
	raw := "hoy no trabajamos"
	turn := &processTurn{
		input: raw,
		semanticReq: interaction.SemanticRequest{
			Raw:       raw,
			Objective: interaction.Objective{Text: raw},
		},
	}
	if !e.ownerRestrictionPhase(turn) {
		t.Fatal("la puerta del motor debía consumir la frase")
	}
	if e.ownerAllowsInitiative(now) {
		t.Fatal("la iniciativa debía quedar cerrada a las 08:00")
	}
	if got := e.agentGoalPlannerQuota(); got != 0 {
		t.Fatalf("cupo a las 08:00 = %d, quería 0", got)
	}

	// The same day, hours later: no reopening, no self-goal window.
	for _, hour := range []int{9, 12, 18, 23} {
		now = time.Date(2026, 11, 5, hour, 0, 0, 0, loc)
		if e.ownerAllowsInitiative(now) {
			t.Fatalf("la iniciativa no debía reabrirse a las %02d:00", hour)
		}
		if got := e.agentGoalPlannerQuota(); got != 0 {
			t.Fatalf("cupo a las %02d:00 = %d, quería 0", hour, got)
		}
		if got := e.buildImpulses(&processTurn{}); len(got) != 0 {
			t.Fatalf("impulsos a las %02d:00 = %+v, quería ninguno", hour, got)
		}
	}

	// 23:59 — still governed. 00:00 — expired by clock, then swept.
	now = time.Date(2026, 11, 5, 23, 59, 0, 0, loc)
	if e.ownerAllowsInitiative(now) {
		t.Fatal("la restricción debía seguir activa a las 23:59")
	}
	now = time.Date(2026, 11, 6, 0, 0, 0, 0, loc)
	if err := e.SweepOwnerRestrictions(now); err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if !e.ownerAllowsInitiative(now) {
		t.Fatal("la iniciativa debía reabrirse al día siguiente")
	}
	if got := e.agentGoalPlannerQuota(); got != 1 {
		t.Fatalf("cupo al día siguiente = %d, quería 1", got)
	}
	history := e.OwnerRestrictionHistory()
	if len(history) != 1 || history[0].Status != restriction.StatusExpired || history[0].LiftedBy != "expiry" {
		t.Fatalf("historial = %+v", history)
	}
}

func TestOwnerRestrictionPhaseConsumesOwnerCommand(t *testing.T) {
	e := ownerRestrictionTestEngine(t, filepath.Join(t.TempDir(), "owner_restrictions.json"))
	var spoken []string
	e.onResponse = func(msg string) { spoken = append(spoken, msg) }

	raw := "hoy no trabajamos"
	turn := &processTurn{
		input: raw,
		semanticReq: interaction.SemanticRequest{
			Raw:       raw,
			Objective: interaction.Objective{Text: raw},
		},
	}
	if !e.ownerRestrictionPhase(turn) {
		t.Fatal("la fase debía consumir la orden de descanso")
	}
	if len(spoken) == 0 || !strings.Contains(strings.Join(spoken, " "), "iniciativa de trabajo") {
		t.Fatalf("no se habló la restricción: %v", spoken)
	}

	plain := &processTurn{input: "hola, ¿cómo estás?"}
	if e.ownerRestrictionPhase(plain) {
		t.Fatal("una frase corriente no puede ser interceptada")
	}
	if e.ownerAllowsInitiative(time.Now()) {
		t.Fatal("la restricción debía cerrar la iniciativa")
	}
}
