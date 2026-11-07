package core

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"ahmelia/agency"
	"ahmelia/restriction"
)

func TestOwnerWorkRestrictionClosesInitiative(t *testing.T) {
	dir := t.TempDir()
	e := &Engine{}
	e.enableOwnerRestrictionsAt(filepath.Join(dir, "owner_restrictions.json"))
	if note := e.LastOwnerRestrictionNote(); note != "" {
		t.Fatalf("enable: %s", note)
	}

	now := time.Date(2026, 11, 5, 8, 0, 0, 0, time.Local)
	if !e.ownerAllowsInitiative(now) {
		t.Fatal("sin restricción la iniciativa debía seguir abierta")
	}
	if _, err := e.SetWorkInitiativeRestriction("hoy no trabajamos", "turn-1", now); err != nil {
		t.Fatalf("admitir restricción: %v", err)
	}
	if e.ownerAllowsInitiative(now) {
		t.Fatal("la restricción de trabajo debía cerrar la iniciativa propia")
	}
	active := e.ActiveOwnerRestrictions(now)
	if len(active) != 1 || active[0].Scope != restriction.ScopeWorkInitiative || active[0].Source != restriction.SourceExplicitOwner {
		t.Fatalf("restricciones activas = %+v", active)
	}

	lifted, err := e.LiftOwnerRestriction(active[0].ID, "owner", now)
	if err != nil {
		t.Fatalf("levantar: %v", err)
	}
	if lifted.Status != restriction.StatusLifted || lifted.LiftedBy != "owner" {
		t.Fatalf("levante inesperado: %+v", lifted)
	}
	if !e.ownerAllowsInitiative(now) {
		t.Fatal("tras el levante la iniciativa debía reabrirse")
	}
}

func TestOwnerRestrictionSurvivesRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "owner_restrictions.json")
	e := &Engine{}
	e.enableOwnerRestrictionsAt(path)
	now := time.Date(2026, 11, 5, 8, 0, 0, 0, time.Local)
	if _, err := e.SetWorkInitiativeRestriction("hoy no trabajamos", "turn-1", now); err != nil {
		t.Fatalf("admitir: %v", err)
	}

	reloaded := &Engine{}
	reloaded.enableOwnerRestrictionsAt(path)
	if !reloaded.OwnerWorkInitiativeRestricted(now) {
		t.Fatal("la restricción debía sobrevivir al reinicio")
	}
	if reloaded.ownerAllowsInitiative(now) {
		t.Fatal("la iniciativa debía seguir cerrada tras el reinicio")
	}
}

func TestRestAndOwnerRestrictionAreIndependentClosures(t *testing.T) {
	dir := t.TempDir()
	e := persistentModeTestEngine(t, filepath.Join(dir, "persistent_mode.json"))
	if _, err := e.SetPersistentMode(agency.ModeRest, "owner"); err != nil {
		t.Fatalf("set rest: %v", err)
	}
	now := time.Date(2026, 11, 5, 8, 0, 0, 0, time.Local)
	if e.ownerAllowsInitiative(now) {
		t.Fatal("el modo descanso debía cerrar la iniciativa")
	}

	// Limpiar el modo no reabre nada si la restricción temporal sigue activa.
	if _, err := e.SetPersistentMode(agency.ModeCompanion, "owner"); err != nil {
		t.Fatalf("set companion: %v", err)
	}
	e.enableOwnerRestrictionsAt(filepath.Join(dir, "owner_restrictions.json"))
	if _, err := e.SetWorkInitiativeRestriction("hoy no trabajamos", "turn-1", now); err != nil {
		t.Fatalf("admitir: %v", err)
	}
	if e.ownerAllowsInitiative(now) {
		t.Fatal("la restricción temporal debía mantener cerrada la iniciativa")
	}
}

func TestOwnerWorkRestrictionClosesPlannerQuotaAndImpulses(t *testing.T) {
	dir := t.TempDir()
	e := persistentModeTestEngine(t, filepath.Join(dir, "persistent_mode.json"))
	now := time.Date(2026, 11, 5, 8, 0, 0, 0, time.Local)

	if got := e.agentGoalPlannerQuota(); got != 1 {
		t.Fatalf("cupo inicial = %d, quería 1 en companion", got)
	}
	e.enableOwnerRestrictionsAt(filepath.Join(dir, "owner_restrictions.json"))
	if _, err := e.SetWorkInitiativeRestriction("hoy no trabajamos", "turn-1", now); err != nil {
		t.Fatalf("admitir: %v", err)
	}
	if got := e.agentGoalPlannerQuota(); got != 0 {
		t.Fatalf("cupo con restricción = %d, quería 0", got)
	}
	if got := e.buildImpulses(&processTurn{interactionHasWork: true}); len(got) != 0 {
		t.Fatalf("impulsos con restricción = %+v, quería ninguno", got)
	}

	active := e.ActiveOwnerRestrictions(now)
	if len(active) != 1 {
		t.Fatalf("restricciones activas = %d", len(active))
	}
	if _, err := e.LiftOwnerRestriction(active[0].ID, "owner", now); err != nil {
		t.Fatalf("levantar: %v", err)
	}
	if got := e.agentGoalPlannerQuota(); got != 1 {
		t.Fatalf("cupo tras el levante = %d, quería 1", got)
	}
}

func TestCorruptOwnerRestrictionFileDoesNotInventOne(t *testing.T) {
	path := filepath.Join(t.TempDir(), "owner_restrictions.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatalf("write corrupt: %v", err)
	}
	e := &Engine{}
	e.enableOwnerRestrictionsAt(path)
	if e.LastOwnerRestrictionNote() == "" {
		t.Fatal("un archivo corrupto debía dejar aviso")
	}
	now := time.Date(2026, 11, 5, 8, 0, 0, 0, time.Local)
	if e.OwnerWorkInitiativeRestricted(now) {
		t.Fatal("un archivo corrupto no puede inventar una restricción")
	}

	// El mecanismo sigue operativo con una lista vacía.
	if _, err := e.SetWorkInitiativeRestriction("hoy no trabajamos", "turn-1", now); err != nil {
		t.Fatalf("admitir tras corrupción: %v", err)
	}
}

func TestOwnerRestrictionRequiresEnablement(t *testing.T) {
	e := &Engine{}
	if _, err := e.SetWorkInitiativeRestriction("hoy no trabajamos", "turn-1", time.Now()); err == nil {
		t.Fatal("sin habilitar, admitir una restricción debía fallar")
	}
	if e.OwnerWorkInitiativeRestricted(time.Now()) {
		t.Fatal("sin habilitar no puede haber restricción activa")
	}
	if !e.ownerAllowsInitiative(time.Now()) {
		t.Fatal("sin mecanismo la iniciativa no cambia")
	}
}
