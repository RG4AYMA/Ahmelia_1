package taste

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"ahmelia/personality"
)

func fixedClock(at time.Time) func() time.Time {
	return func() time.Time { return at }
}

func TestRecordVerifiedRequiresEvidenceAndDeduplicates(t *testing.T) {
	now := time.Date(2026, 11, 2, 9, 0, 0, 0, time.UTC)
	l := NewLedger("")
	l.SetClock(fixedClock(now))

	if _, _, err := l.RecordVerified("", "work-1"); !errors.Is(err, ErrNoEvidence) {
		t.Fatalf("key vacía: %v", err)
	}
	if _, _, err := l.RecordVerified("blender", "  "); !errors.Is(err, ErrNoEvidence) {
		t.Fatalf("ref vacía: %v", err)
	}

	first, created, err := l.RecordVerified("Blender", "work-1")
	if err != nil || !created {
		t.Fatalf("first=%+v created=%v err=%v", first, created, err)
	}
	if first.Key != "blender" || first.Ref != "work-1" || first.Status != CandidatePending {
		t.Fatalf("candidate=%+v", first)
	}
	again, created, err := l.RecordVerified("blender", "work-1")
	if err != nil || created || again.ID != "" {
		t.Fatalf("duplicado: %+v created=%v err=%v", again, created, err)
	}
	other, created, err := l.RecordVerified("blender", "work-2")
	if err != nil || !created || other.ID == first.ID {
		t.Fatalf("otra ref: %+v created=%v err=%v", other, created, err)
	}
}

func TestRecordVerifiedRespectsPendingCap(t *testing.T) {
	l := NewLedger("")
	for i := 0; i < MaxPending; i++ {
		if _, _, err := l.RecordVerified("explorar", "work-"+string(rune('a'+i))); err != nil {
			t.Fatalf("candidate %d: %v", i, err)
		}
	}
	if _, _, err := l.RecordVerified("explorar", "work-overflow"); !errors.Is(err, ErrPendingFull) {
		t.Fatalf("err=%v, esperaba ErrPendingFull", err)
	}
}

func TestConfirmRequiresPendingAndIsTerminal(t *testing.T) {
	l := NewLedger("")
	rec, _, err := l.RecordVerified("explorar", "work-1")
	if err != nil {
		t.Fatalf("record: %v", err)
	}
	if _, err := l.Confirm("tg-unknown"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err=%v", err)
	}
	got, err := l.Confirm(rec.ID)
	if err != nil || got.Status != CandidateConfirmed || got.DecidedAt.IsZero() {
		t.Fatalf("confirm=%+v err=%v", got, err)
	}
	if _, err := l.Confirm(rec.ID); !errors.Is(err, ErrNotPending) {
		t.Fatalf("doble confirm: %v", err)
	}
	if _, err := l.Reject(rec.ID); !errors.Is(err, ErrNotPending) {
		t.Fatalf("reject tras confirm: %v", err)
	}
}

func TestModelOnlyFormsTasteAtMinN(t *testing.T) {
	l := NewLedger("")
	m := NewModel(l)
	for i := 0; i < 5; i++ {
		rec, _, err := m.RecordVerified("explorar", "work-"+string(rune('a'+i)))
		if err != nil {
			t.Fatalf("record %d: %v", i, err)
		}
		if _, err := m.Confirm(rec.ID); err != nil {
			t.Fatalf("confirm %d: %v", i, err)
		}
	}
	if got := m.View("explorar"); got.Stance != personality.StanceUnformed {
		t.Fatalf("con 5 confirmaciones no debe haber gusto: %+v", got)
	}
	if stances := m.Stances(); len(stances) != 0 {
		t.Fatalf("stances con 5: %+v", stances)
	}

	rec, _, err := m.RecordVerified("explorar", "work-f")
	if err != nil {
		t.Fatalf("record 6: %v", err)
	}
	if _, err := m.Confirm(rec.ID); err != nil {
		t.Fatalf("confirm 6: %v", err)
	}
	stances := m.Stances()
	if len(stances) != 1 || stances[0].Key != "explorar" || stances[0].Stance != personality.StanceLikes {
		t.Fatalf("stances con 6: %+v", stances)
	}
	if len(stances[0].Evidence) != 5 {
		t.Fatalf("evidencia=%v", stances[0].Evidence)
	}
}

func TestCorrectionClearsPriorEvidenceAndIsTraceable(t *testing.T) {
	l := NewLedger("")
	m := NewModel(l)
	for i := 0; i < 6; i++ {
		rec, _, err := m.RecordVerified("explorar", "work-"+string(rune('a'+i)))
		if err != nil {
			t.Fatalf("record %d: %v", i, err)
		}
		if _, err := m.Confirm(rec.ID); err != nil {
			t.Fatalf("confirm %d: %v", i, err)
		}
	}
	if got := m.View("explorar").Stance; got != personality.StanceLikes {
		t.Fatalf("antes de corregir: %s", got)
	}

	cor, err := m.Correct("explorar", "owner-turn-9", "no era un gusto")
	if err != nil {
		t.Fatalf("correct: %v", err)
	}
	if cor.Ref != "owner-turn-9" || cor.Key != "explorar" {
		t.Fatalf("correction=%+v", cor)
	}
	view := m.View("explorar")
	if view.Stance != personality.StanceUnformed {
		t.Fatalf("una corrección debe borrar el gusto formado: %+v", view)
	}
	if len(view.Evidence) != 1 || view.Evidence[0] != "owner-turn-9" {
		t.Fatalf("la evidencia debe ser la corrección: %v", view.Evidence)
	}
	if got := m.Stances(); len(got) != 0 {
		t.Fatalf("no debe quedar gusto formado: %+v", got)
	}
}

func TestRejectedCandidateNeverBecomesEvidence(t *testing.T) {
	l := NewLedger("")
	m := NewModel(l)
	rec, _, err := m.RecordVerified("blender", "work-1")
	if err != nil {
		t.Fatalf("record: %v", err)
	}
	if _, err := m.Reject(rec.ID); err != nil {
		t.Fatalf("reject: %v", err)
	}
	if got := m.View("blender").EffN; got != 0 {
		t.Fatalf("un rechazo no alimenta el posterior: %v", got)
	}
	if _, err := m.Confirm(rec.ID); !errors.Is(err, ErrNotPending) {
		t.Fatalf("confirm tras rechazo: %v", err)
	}
}

func TestLedgerReplayRebuildsEvidenceAndStances(t *testing.T) {
	path := filepath.Join(t.TempDir(), "self_taste.jsonl")
	l := NewLedger(path)
	m := NewModel(l)
	for i := 0; i < 6; i++ {
		rec, _, err := m.RecordVerified("explorar", "work-"+string(rune('a'+i)))
		if err != nil {
			t.Fatalf("record %d: %v", i, err)
		}
		if _, err := m.Confirm(rec.ID); err != nil {
			t.Fatalf("confirm %d: %v", i, err)
		}
	}
	pending, _, err := m.RecordVerified("blender", "work-pending")
	if err != nil {
		t.Fatalf("pending: %v", err)
	}
	if _, err := m.Correct("explorar", "owner-turn-9", ""); err != nil {
		t.Fatalf("correct: %v", err)
	}

	reloaded := NewLedger(path)
	if err := reloaded.Load(); err != nil {
		t.Fatalf("load: %v", err)
	}
	rm := NewModel(reloaded)
	if got := rm.View("explorar").Stance; got != personality.StanceUnformed {
		t.Fatalf("replay debe conservar la corrección: %s", got)
	}
	if got := rm.Pending(); len(got) != 1 || got[0].ID != pending.ID {
		t.Fatalf("pending tras replay: %+v", got)
	}
	if got, ok := reloaded.Get(pending.ID); !ok || got.Status != CandidatePending {
		t.Fatalf("candidate tras replay: %+v ok=%v", got, ok)
	}
}

func TestNormalizeKey(t *testing.T) {
	cases := map[string]string{
		"  Blender  ":       "blender",
		"Explorar,":         "explorar",
		"concentrarme":      "concentrarme",
		"  dos   espacios ": "dos espacios",
	}
	for in, want := range cases {
		if got := NormalizeKey(in); got != want {
			t.Fatalf("%q -> %q, esperaba %q", in, got, want)
		}
	}
}
