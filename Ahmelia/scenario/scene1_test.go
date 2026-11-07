package scenario

import (
	"path/filepath"
	"testing"
	"time"

	"ahmelia/restriction"
)

// TestScene1RestDay is the first executable slice of the rest/coexistence
// scene. It does not test prose: it tests that a bounded restriction survives
// the day, that authorized actions still run, that the owner state is ephemeral
// and that recreation only appears with fresh receptivity.
func TestScene1RestDay(t *testing.T) {
	loc := time.FixedZone("COT", -5*3600)
	start := time.Date(2026, 11, 5, 8, 0, 0, 0, loc)
	endOfDay := time.Date(2026, 11, 6, 0, 0, 0, 0, loc)
	dir := t.TempDir()
	restrictionPath := filepath.Join(dir, "owner_restrictions.json")
	statePath := filepath.Join(dir, "owner_sim.json")

	store, err := restriction.Open(restrictionPath)
	if err != nil {
		t.Fatalf("open restrictions: %v", err)
	}
	clock := NewClock(start, loc)
	owner := NewOwnerSim(clock, store, statePath)
	bus := NewFakeDeviceBus(
		FakeDevice{ID: "speaker", Kind: "speaker", Zone: "private", Online: true},
		FakeDevice{ID: "light", Kind: "light", Zone: "shared", Online: true},
	)

	// 08:00 — the owner declares a rest day. This is a restriction, not a mode.
	rest, err := owner.DeclareNoWorkToday("hoy no trabajamos")
	if err != nil {
		t.Fatalf("declare rest: %v", err)
	}
	if rest.Scope != restriction.ScopeWorkInitiative || rest.Source != restriction.SourceExplicitOwner {
		t.Fatalf("restricción inesperada: %+v", rest)
	}
	if rest.ExpiresAt == nil || !rest.ExpiresAt.Equal(endOfDay) {
		t.Fatalf("expiry = %v, quería %v", rest.ExpiresAt, endOfDay)
	}
	if active := store.Active(start); len(active) != 1 {
		t.Fatalf("restricciones activas = %d, quería 1", len(active))
	}

	// At most one question about the reason, and it does not repeat.
	if !owner.AskRestQuestion() {
		t.Fatal("la primera pregunta debía permitirse")
	}
	if owner.AskRestQuestion() {
		t.Fatal("la segunda pregunta no debía permitirse")
	}

	// 09:00 and 09:10 — work triggers must not open initiative.
	clock.At(time.Date(2026, 11, 5, 9, 0, 0, 0, loc))
	if d := DecideWorkInitiative(store, clock.Now()); d.Kind != DecisionSilence || d.Reason != "work_restriction_active" {
		t.Fatalf("deadline: decisión = %+v, quería silencio por restricción", d)
	}
	clock.At(time.Date(2026, 11, 5, 9, 10, 0, 0, loc))
	if d := DecideWorkInitiative(store, clock.Now()); d.Kind != DecisionSilence {
		t.Fatalf("idea nueva: decisión = %+v, quería silencio", d)
	}

	// 10:00 and 10:05 — authorized non-work actions still execute.
	clock.At(time.Date(2026, 11, 5, 10, 0, 0, 0, loc))
	if result, err := bus.Execute("speaker", "play_music", clock.Now()); err != nil || !result.Executed {
		t.Fatalf("música: result=%+v err=%v", result, err)
	}
	clock.At(time.Date(2026, 11, 5, 10, 5, 0, 0, loc))
	if result, err := bus.Execute("light", "dim", clock.Now()); err != nil || !result.Executed {
		t.Fatalf("luz: result=%+v err=%v", result, err)
	}
	if actions := bus.Actions(); len(actions) != 2 {
		t.Fatalf("acciones registradas = %d, quería 2", len(actions))
	}

	// 12:00 — pain is ephemeral and sensitive, with a TTL.
	clock.At(time.Date(2026, 11, 5, 12, 0, 0, 0, loc))
	owner.ReportPain("headache", 0.8, 2*time.Hour, []string{"turn:pain"})
	pain, ok := owner.ActivePain(clock.Now())
	if !ok {
		t.Fatal("el dolor debía estar activo a las 12:00")
	}
	if pain.Persist != "ephemeral" || pain.Privacy != "sensitive" {
		t.Fatalf("estado del propietario mal clasificado: %+v", pain)
	}

	// 15:00 — low receptivity closes recreation proposals.
	clock.At(time.Date(2026, 11, 5, 15, 0, 0, 0, loc))
	owner.SetReceptivity("low", 2*time.Hour, []string{"turn:quiet"})
	if d := owner.ProposeRecreation(clock.Now(), 2*time.Hour); d.Kind != DecisionSilence {
		t.Fatalf("receptividad baja: decisión = %+v, quería silencio", d)
	}

	// 17:00 — the pain TTL expired and the state is no longer used.
	clock.At(time.Date(2026, 11, 5, 17, 0, 0, 0, loc))
	if _, ok := owner.ActivePain(clock.Now()); ok {
		t.Fatal("el dolor expirado no debía usarse")
	}

	// 18:00 — restart. Restriction, counter and readings must survive.
	if err := owner.Save(); err != nil {
		t.Fatalf("save owner: %v", err)
	}
	store2, err := restriction.Open(restrictionPath)
	if err != nil {
		t.Fatalf("reopen restrictions: %v", err)
	}
	clock2 := NewClock(clock.Now(), loc)
	owner2, err := LoadOwnerSim(statePath, clock2, store2)
	if err != nil {
		t.Fatalf("load owner: %v", err)
	}
	if !store2.WorkInitiativeActive(clock2.Now()) {
		t.Fatal("la restricción debía sobrevivir al reinicio")
	}
	if owner2.QuestionsAsked() != 1 {
		t.Fatalf("preguntas tras reinicio = %d, quería 1", owner2.QuestionsAsked())
	}
	if d := DecideWorkInitiative(store2, clock2.Now()); d.Kind != DecisionSilence {
		t.Fatalf("trabajo tras reinicio: decisión = %+v, quería silencio", d)
	}

	// 20:00 — high receptivity allows exactly one proposal in the window.
	clock2.At(time.Date(2026, 11, 5, 20, 0, 0, 0, loc))
	owner2.SetReceptivity("high", 2*time.Hour, []string{"turn:open"})
	if d := owner2.ProposeRecreation(clock2.Now(), 2*time.Hour); d.Kind != DecisionRecreationProposal {
		t.Fatalf("receptividad alta: decisión = %+v, quería propuesta", d)
	}
	if d := owner2.ProposeRecreation(clock2.Now().Add(10*time.Minute), 2*time.Hour); d.Kind != DecisionSilence {
		t.Fatalf("segunda propuesta en la ventana: decisión = %+v, quería silencio", d)
	}

	// 23:59 — still active; 00:00 — expired, not lifted.
	clock2.At(endOfDay.Add(-time.Minute))
	if !store2.WorkInitiativeActive(clock2.Now()) {
		t.Fatal("la restricción debía seguir activa a las 23:59")
	}
	clock2.At(endOfDay)
	if err := store2.Sweep(endOfDay); err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if store2.WorkInitiativeActive(endOfDay) {
		t.Fatal("la restricción no debía gobernar a medianoche")
	}
	history := store2.History()
	if len(history) != 1 || history[0].Status != restriction.StatusExpired || history[0].LiftedBy != "expiry" {
		t.Fatalf("historia tras expirar = %+v", history)
	}

	// Next day — work initiative returns without a catch-up burst.
	clock2.At(endOfDay.Add(8 * time.Hour))
	if d := DecideWorkInitiative(store2, clock2.Now()); d.Kind != DecisionWorkInitiative {
		t.Fatalf("día siguiente: decisión = %+v, quería iniciativa permitida", d)
	}
}
