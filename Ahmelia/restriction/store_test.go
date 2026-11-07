package restriction

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func testStore(t *testing.T) (*Store, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "owner_restrictions.json")
	s, err := Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	return s, path
}

func TestWorkInitiativeExpiresAtEndOfDay(t *testing.T) {
	loc := time.FixedZone("COT", -5*3600)
	now := time.Date(2026, 11, 5, 8, 0, 0, 0, loc)
	s, _ := testStore(t)

	r, err := s.AdmitWorkInitiative("hoy no trabajamos", SourceExplicitOwner, "turn-1", now, loc, []string{"owner"})
	if err != nil {
		t.Fatalf("admit: %v", err)
	}
	want := time.Date(2026, 11, 6, 0, 0, 0, 0, loc)
	if r.ExpiresAt == nil || !r.ExpiresAt.Equal(want) {
		t.Fatalf("expires_at = %v, quería %v", r.ExpiresAt, want)
	}
	if r.Scope != ScopeWorkInitiative || r.Status != StatusActive {
		t.Fatalf("restricción inesperada: %+v", r)
	}
	if !s.WorkInitiativeActive(now) {
		t.Fatal("la restricción debía gobernar a las 08:00")
	}
	if !s.WorkInitiativeActive(want.Add(-time.Minute)) {
		t.Fatal("la restricción debía gobernar a las 23:59")
	}
	if s.WorkInitiativeActive(want) {
		t.Fatal("la restricción no debía gobernar a medianoche")
	}
}

func TestPersistenceAndLiftHistory(t *testing.T) {
	loc := time.FixedZone("COT", -5*3600)
	now := time.Date(2026, 11, 5, 8, 0, 0, 0, loc)
	s, path := testStore(t)

	r, err := s.AdmitWorkInitiative("hoy no trabajamos", SourceExplicitOwner, "turn-1", now, loc, nil)
	if err != nil {
		t.Fatalf("admit: %v", err)
	}

	reloaded, err := Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if !reloaded.WorkInitiativeActive(now) {
		t.Fatal("la restricción debía sobrevivir al reinicio")
	}

	lifted, err := reloaded.Lift(r.ID, "owner", now.Add(time.Hour))
	if err != nil {
		t.Fatalf("lift: %v", err)
	}
	if lifted.Status != StatusLifted || lifted.LiftedBy != "owner" || lifted.LiftedAt == nil {
		t.Fatalf("levante inesperado: %+v", lifted)
	}

	again, err := Open(path)
	if err != nil {
		t.Fatalf("reopen after lift: %v", err)
	}
	if again.WorkInitiativeActive(now.Add(2 * time.Hour)) {
		t.Fatal("una restricción levantada no debía gobernar")
	}
	history := again.History()
	if len(history) != 1 || history[0].Status != StatusLifted {
		t.Fatalf("historia = %+v, quería una levantada", history)
	}
}

func TestCorruptFileIsQuarantined(t *testing.T) {
	loc := time.FixedZone("COT", -5*3600)
	now := time.Date(2026, 11, 5, 8, 0, 0, 0, loc)
	path := filepath.Join(t.TempDir(), "owner_restrictions.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatalf("write corrupt: %v", err)
	}

	s, err := Open(path)
	if err != nil {
		t.Fatalf("open corrupt: %v", err)
	}
	if len(s.Warnings()) == 0 {
		t.Fatal("un archivo corrupto debía dejar aviso")
	}
	if s.WorkInitiativeActive(now) {
		t.Fatal("un archivo corrupto no puede inventar una restricción")
	}

	if _, err := s.AdmitWorkInitiative("hoy no trabajamos", SourceExplicitOwner, "turn-1", now, loc, nil); err != nil {
		t.Fatalf("admit after corrupt: %v", err)
	}
	backups, err := filepath.Glob(path + ".corrupt-*")
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	if len(backups) != 1 {
		t.Fatalf("copias corruptas = %d, quería 1", len(backups))
	}
}

func TestInvalidScopes(t *testing.T) {
	loc := time.FixedZone("COT", -5*3600)
	now := time.Date(2026, 11, 5, 8, 0, 0, 0, loc)
	s, _ := testStore(t)

	if err := s.Add(OwnerRestriction{
		ID: "bad-1", Scope: ScopeWorkInitiative, Target: "carpeta", Statement: "no toques",
		Source: SourceExplicitOwner, Since: now, Status: StatusActive,
	}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("work_initiative con target: err=%v, quería ErrInvalid", err)
	}
	if err := s.Add(OwnerRestriction{
		ID: "bad-2", Scope: ScopeActionBan, Statement: "no toques",
		Source: SourceExplicitOwner, Since: now, Status: StatusActive,
	}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("action_ban sin target: err=%v, quería ErrInvalid", err)
	}
}

func TestLiftExpiredFails(t *testing.T) {
	loc := time.FixedZone("COT", -5*3600)
	now := time.Date(2026, 11, 5, 8, 0, 0, 0, loc)
	s, _ := testStore(t)

	r, err := s.AdmitWorkInitiative("hoy no trabajamos", SourceExplicitOwner, "turn-1", now, loc, nil)
	if err != nil {
		t.Fatalf("admit: %v", err)
	}
	if _, err := s.Lift(r.ID, "owner", now.Add(24*time.Hour)); !errors.Is(err, ErrNotActive) {
		t.Fatalf("lift expirado: err=%v, quería ErrNotActive", err)
	}
	got, _ := s.ByID(r.ID)
	if got.Status != StatusExpired || got.LiftedBy != "expiry" {
		t.Fatalf("estado tras expirar = %+v", got)
	}
}

func TestEmptyStoreIsInert(t *testing.T) {
	now := time.Now()
	var s *Store
	if s.WorkInitiativeActive(now) || s.ActionBanned("x", now) {
		t.Fatal("un store nil no puede gobernar")
	}
	empty, _ := Open("")
	if empty.WorkInitiativeActive(now) || empty.ActionBanned("x", now) {
		t.Fatal("un store vacío no puede gobernar")
	}
}

func TestActionBanExactMatch(t *testing.T) {
	loc := time.FixedZone("COT", -5*3600)
	now := time.Date(2026, 11, 5, 8, 0, 0, 0, loc)
	s, _ := testStore(t)
	if err := s.Add(OwnerRestriction{
		ID: "ban-1", Scope: ScopeActionBan, Target: "carpeta X", Statement: "no toques la carpeta X",
		Source: SourceExplicitOwner, Since: now, Status: StatusActive,
	}); err != nil {
		t.Fatalf("add ban: %v", err)
	}
	if !s.ActionBanned("Carpeta X", now) {
		t.Fatal("el match normalizado debía bloquear")
	}
	if s.ActionBanned("carpeta Y", now) {
		t.Fatal("otro target no debía bloquear")
	}
}
