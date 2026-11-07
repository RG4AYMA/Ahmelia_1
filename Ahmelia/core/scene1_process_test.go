package core

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestScene1ProcessRestDayThroughRealEngine is the closest thing to the
// integrated scene 1 that the current engine allows: a real Engine.Process
// call enters the owner gate, admits the restriction and then the scene clock
// drives the gate for the rest of the day. It does not simulate the whole turn
// pipeline, but it is not a direct call to the policy either.
func TestScene1ProcessRestDayThroughRealEngine(t *testing.T) {
	e := buildTestEngine(t)
	loc := time.FixedZone("COT", -5*3600)
	now := time.Date(2026, 11, 5, 8, 0, 0, 0, loc)
	e.sceneNow = func() time.Time { return now }

	dir := t.TempDir()
	e.enablePersistentModeAt(filepath.Join(dir, "persistent_mode.json"))
	e.enableOwnerRestrictionsAt(filepath.Join(dir, "owner_restrictions.json"))

	var responses []string
	e.SetResponseCallback(func(response string) { responses = append(responses, response) })

	e.Process("hoy no trabajamos")

	if len(responses) == 0 {
		t.Fatal("Process no produjo respuesta")
	}
	spoken := strings.Join(responses, " ")
	if !strings.Contains(spoken, "iniciativa de trabajo") {
		t.Fatalf("la respuesta no cerró la iniciativa: %q", spoken)
	}
	if !e.OwnerWorkInitiativeRestricted(now) {
		t.Fatal("la restricción debía quedar activa tras Process")
	}
	if e.ownerAllowsInitiative(now) {
		t.Fatal("la iniciativa debía quedar cerrada a las 08:00")
	}

	for _, hour := range []int{9, 12, 18, 23} {
		now = time.Date(2026, 11, 5, hour, 0, 0, 0, loc)
		if e.ownerAllowsInitiative(now) {
			t.Fatalf("la iniciativa no debía reabrirse a las %02d:00", hour)
		}
		if got := e.agentGoalPlannerQuota(); got != 0 {
			t.Fatalf("cupo a las %02d:00 = %d, quería 0", hour, got)
		}
	}

	now = time.Date(2026, 11, 6, 0, 0, 0, 0, loc)
	if err := e.SweepOwnerRestrictions(now); err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if !e.ownerAllowsInitiative(now) {
		t.Fatal("la iniciativa debía reabrirse al día siguiente")
	}
	history := e.OwnerRestrictionHistory()
	if len(history) != 1 || history[0].Status != "expired" {
		t.Fatalf("historial = %+v", history)
	}
}
