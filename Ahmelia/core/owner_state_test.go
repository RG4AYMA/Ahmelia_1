package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ahmelia/ownerstate"
)

func TestOwnerStateGateStoresPainAndExpires(t *testing.T) {
	e := buildTestEngine(t)
	loc := time.FixedZone("COT", -5*3600)
	now := time.Date(2026, 11, 5, 12, 0, 0, 0, loc)
	e.sceneNow = func() time.Time { return now }
	e.enableOwnerStateAt(filepath.Join(t.TempDir(), "owner_state.json"))

	var responses []string
	e.SetResponseCallback(func(response string) { responses = append(responses, response) })

	e.Process("me duele la cabeza")

	if len(responses) == 0 || !strings.Contains(strings.Join(responses, " "), "no voy a proponerte") {
		t.Fatalf("respuesta = %v", responses)
	}
	state, ok := e.OwnerStateActive(now)
	if !ok {
		t.Fatal("el dolor debía quedar activo")
	}
	if state.Privacy != ownerstate.PrivacySensitive || state.Persist != ownerstate.PersistEphemeral {
		t.Fatalf("estado mal clasificado: %+v", state)
	}
	if got := e.buildImpulses(&processTurn{interactionHasWork: true}); len(got) != 0 {
		t.Fatalf("impulsos con dolor = %+v, quería ninguno", got)
	}
	if e.ownerAllowsInitiative(now) {
		t.Fatal("el dolor debía cerrar la iniciativa propia")
	}
	if got := e.agentGoalPlannerQuota(); got != 0 {
		t.Fatalf("cupo con dolor = %d, quería 0", got)
	}
	if got := e.OwnerQuestionsAsked(); got != 0 {
		t.Fatalf("preguntas = %d, quería 0", got)
	}

	now = now.Add(3 * time.Hour)
	if _, ok := e.OwnerStateActive(now); ok {
		t.Fatal("el dolor no debía estar activo fuera del TTL")
	}
	if err := e.SweepOwnerState(now); err != nil {
		t.Fatalf("sweep: %v", err)
	}

	e.Process("ya se me pasó")
	if _, ok := e.OwnerStateActive(now); ok {
		t.Fatal("la frase de recuperación debía limpiar el estado")
	}
	if !e.ownerAllowsInitiative(now) {
		t.Fatal("sin estado sensible la iniciativa debía reabrirse")
	}
}

func TestOwnerStatePersistsAcrossReload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "owner_state.json")
	loc := time.FixedZone("COT", -5*3600)
	now := time.Date(2026, 11, 5, 12, 0, 0, 0, loc)

	e1 := &Engine{}
	e1.sceneNow = func() time.Time { return now }
	e1.enableOwnerStateAt(path)
	if err := e1.SetOwnerEphemeralState(ownerstate.State{
		Kind: "headache", Intensity: 0.8, Source: "explicit",
		ObservedAt: now, ExpiresAt: now.Add(2 * time.Hour),
		Privacy: ownerstate.PrivacySensitive, Persist: ownerstate.PersistEphemeral,
	}); err != nil {
		t.Fatalf("set: %v", err)
	}

	e2 := &Engine{}
	e2.sceneNow = func() time.Time { return now }
	e2.enableOwnerStateAt(path)
	if _, ok := e2.OwnerStateActive(now); !ok {
		t.Fatal("el estado debía sobrevivir al reinicio")
	}
	if _, ok := e2.OwnerStateActive(now.Add(3 * time.Hour)); ok {
		t.Fatal("el estado no debía sobrevivir al TTL")
	}
}

func TestScene1OwnerContinuityAcrossRestart(t *testing.T) {
	dir := t.TempDir()
	restrictionPath := filepath.Join(dir, "owner_restrictions.json")
	statePath := filepath.Join(dir, "owner_state.json")
	loc := time.FixedZone("COT", -5*3600)
	now := time.Date(2026, 11, 5, 12, 0, 0, 0, loc)

	e1 := buildTestEngine(t)
	e1.sceneNow = func() time.Time { return now }
	e1.enableOwnerRestrictionsAt(restrictionPath)
	e1.enableOwnerStateAt(statePath)
	e1.SetResponseCallback(func(string) {})
	e1.Process("hoy no trabajamos")
	e1.Process("me duele la cabeza")
	if got := e1.OwnerQuestionsAsked(); got != 1 {
		t.Fatalf("preguntas tras la primera restricción = %d, quería 1", got)
	}

	e2 := buildTestEngine(t)
	e2.sceneNow = func() time.Time { return now }
	e2.enableOwnerRestrictionsAt(restrictionPath)
	e2.enableOwnerStateAt(statePath)
	if !e2.OwnerWorkInitiativeRestricted(now) {
		t.Fatal("la restricción debía sobrevivir al reinicio")
	}
	if _, ok := e2.OwnerStateActive(now); !ok {
		t.Fatal("el estado sensible debía sobrevivir al reinicio")
	}
	if got := e2.OwnerQuestionsAsked(); got != 1 {
		t.Fatalf("preguntas tras reinicio = %d, quería 1", got)
	}
	if e2.AskOwnerRestQuestion() {
		t.Fatal("el cupo de preguntas no debía reabrirse tras el reinicio")
	}
	if e2.ownerAllowsInitiative(now) {
		t.Fatal("la iniciativa debía seguir cerrada tras el reinicio")
	}

	next := time.Date(2026, 11, 6, 0, 0, 0, 0, loc)
	if err := e2.SweepOwnerRestrictions(next); err != nil {
		t.Fatalf("sweep restricción: %v", err)
	}
	if err := e2.SweepOwnerState(next); err != nil {
		t.Fatalf("sweep estado: %v", err)
	}
	if !e2.ownerAllowsInitiative(next) {
		t.Fatal("la iniciativa debía reabrirse al día siguiente")
	}
}

func TestOwnerStateDoesNotBlockAuthorizedAction(t *testing.T) {
	e := buildTestEngine(t)
	e.EnableAsyncProduction()
	loc := time.FixedZone("COT", -5*3600)
	now := time.Date(2026, 11, 5, 10, 0, 0, 0, loc)
	e.sceneNow = func() time.Time { return now }
	e.enableOwnerStateAt(filepath.Join(t.TempDir(), "owner_state.json"))
	if err := e.SetOwnerEphemeralState(ownerstate.State{
		Kind: "headache", Intensity: 0.8, Source: "explicit",
		ObservedAt: now, ExpiresAt: now.Add(2 * time.Hour),
		Privacy: ownerstate.PrivacySensitive, Persist: ownerstate.PersistEphemeral,
	}); err != nil {
		t.Fatalf("set: %v", err)
	}
	e.SetResponseCallback(func(string) {})

	e.Process("Crea un programa Go que sume dos números y guárdalo en el proyecto")
	job := waitForAnyBackgroundJobCompletion(t, e)
	if job.Status != BackgroundJobCompleted {
		t.Fatalf("la producción autorizada no terminó correctamente: %+v", job)
	}

	if _, ok := e.OwnerStateActive(now); !ok {
		t.Fatal("el estado sensible debía seguir activo")
	}
	root := os.Getenv("AHMELIA_WORKSPACE_ROOT")
	if root == "" {
		t.Fatal("sin workspace aislado")
	}
	found := false
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() && info.Name() == "main.go" {
			found = true
		}
		return nil
	})
	if !found {
		t.Fatalf("la orden autorizada no se ejecutó bajo dolor: %s", root)
	}
}
