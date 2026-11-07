package core

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestScene1ProcessSurfaceVariation(t *testing.T) {
	phrases := []string{
		"hoy no trabajamos",
		"hoy no trabajo",
		"no trabajamos hoy",
		"no trabajo hoy",
		"hoy descansamos",
		"hoy descanso",
		"descansamos hoy",
		"descanso hoy",
	}
	loc := time.FixedZone("COT", -5*3600)
	for _, phrase := range phrases {
		t.Run(phrase, func(t *testing.T) {
			e := buildTestEngine(t)
			now := time.Date(2026, 11, 5, 8, 0, 0, 0, loc)
			e.sceneNow = func() time.Time { return now }
			dir := t.TempDir()
			e.enableOwnerRestrictionsAt(filepath.Join(dir, "owner_restrictions.json"))
			e.enableOwnerStateAt(filepath.Join(dir, "owner_state.json"))
			var responses []string
			e.SetResponseCallback(func(response string) { responses = append(responses, response) })

			e.Process(phrase)

			if !e.OwnerWorkInitiativeRestricted(now) {
				t.Fatalf("%q no admitió la restricción", phrase)
			}
			if e.ownerAllowsInitiative(now) {
				t.Fatalf("%q no cerró la iniciativa", phrase)
			}
			if !strings.Contains(strings.Join(responses, " "), "hoy no trabajamos") {
				t.Fatalf("%q no produjo la respuesta canónica: %v", phrase, responses)
			}
		})
	}
}

func TestScene1ExpiryDoesNotBurst(t *testing.T) {
	e := buildTestEngine(t)
	loc := time.FixedZone("COT", -5*3600)
	now := time.Date(2026, 11, 5, 8, 0, 0, 0, loc)
	e.sceneNow = func() time.Time { return now }
	dir := t.TempDir()
	e.enableOwnerRestrictionsAt(filepath.Join(dir, "owner_restrictions.json"))
	e.enableOwnerStateAt(filepath.Join(dir, "owner_state.json"))
	e.SetResponseCallback(func(string) {})
	e.Process("hoy no trabajamos")

	next := time.Date(2026, 11, 6, 0, 0, 0, 0, loc)
	if err := e.SweepOwnerRestrictions(next); err != nil {
		t.Fatalf("sweep restricción: %v", err)
	}
	if err := e.SweepOwnerState(next); err != nil {
		t.Fatalf("sweep estado: %v", err)
	}
	if !e.ownerAllowsInitiative(next) {
		t.Fatal("la iniciativa debía reabrirse")
	}
	if got := e.agentGoalPlannerQuota(); got != 1 {
		t.Fatalf("cupo al día siguiente = %d, quería 1", got)
	}
	if got := len(e.AgentGoalProposals()); got != 0 {
		t.Fatalf("propuestas acumuladas = %d, quería 0 (sin ráfaga)", got)
	}
	if got := len(e.planner.Goals()); got != 0 {
		t.Fatalf("metas en el planificador = %d, quería 0 (sin ráfaga)", got)
	}
}
