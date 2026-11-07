package ownerstate

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func testState(now time.Time) State {
	return State{
		Kind:       "headache",
		Intensity:  0.8,
		Source:     "explicit",
		ObservedAt: now,
		ExpiresAt:  now.Add(2 * time.Hour),
		Privacy:    PrivacySensitive,
		Persist:    PersistEphemeral,
		Evidence:   []string{"turn:pain"},
	}
}

func openTestStore(t *testing.T, path string, now *time.Time) *Store {
	t.Helper()
	s, err := Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	s.SetClock(func() time.Time { return *now })
	return s
}

func TestStateTTLAndClear(t *testing.T) {
	now := time.Date(2026, 11, 5, 12, 0, 0, 0, time.Local)
	s := openTestStore(t, "", &now)
	if err := s.SetState(testState(now)); err != nil {
		t.Fatalf("set: %v", err)
	}
	if _, ok := s.ActiveState(now.Add(time.Hour)); !ok {
		t.Fatal("el estado debía estar activo dentro del TTL")
	}
	if _, ok := s.ActiveState(now.Add(3 * time.Hour)); ok {
		t.Fatal("el estado no debía estar activo fuera del TTL")
	}
	if err := s.ClearState(); err != nil {
		t.Fatalf("clear: %v", err)
	}
	if _, ok := s.ActiveState(now); ok {
		t.Fatal("el estado limpiado no debía usarse")
	}
}

func TestPersistenceQuestionBudgetAndReceptivity(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "owner_state.json")
	now := time.Date(2026, 11, 5, 8, 0, 0, 0, time.Local)
	s := openTestStore(t, path, &now)
	if err := s.SetState(testState(now)); err != nil {
		t.Fatalf("set: %v", err)
	}
	if err := s.SetReceptivity("high", 2*time.Hour, []string{"turn:open"}); err != nil {
		t.Fatalf("receptivity: %v", err)
	}
	if !s.AskQuestion() {
		t.Fatal("la primera pregunta debía permitirse")
	}
	if s.AskQuestion() {
		t.Fatal("la segunda pregunta no debía permitirse")
	}
	if err := s.MarkProposal(now); err != nil {
		t.Fatalf("mark: %v", err)
	}

	reloaded := openTestStore(t, path, &now)
	if _, ok := reloaded.ActiveState(now); !ok {
		t.Fatal("el estado debía sobrevivir al reinicio")
	}
	if reloaded.QuestionsAsked() != 1 {
		t.Fatalf("preguntas = %d, quería 1", reloaded.QuestionsAsked())
	}
	if got := reloaded.Receptivity(now); got.Level != "high" {
		t.Fatalf("receptividad = %+v", got)
	}
	if reloaded.ProposalWindowOpen(now.Add(10*time.Minute), 2*time.Hour) {
		t.Fatal("la ventana de propuesta no debía estar abierta")
	}
	if !reloaded.ProposalWindowOpen(now.Add(3*time.Hour), 2*time.Hour) {
		t.Fatal("la ventana debía reabrir tras el intervalo")
	}
}

func TestStateRejectsDurableOrPublic(t *testing.T) {
	now := time.Date(2026, 11, 5, 12, 0, 0, 0, time.Local)
	s := openTestStore(t, "", &now)

	durable := testState(now)
	durable.Persist = "durable"
	if err := s.SetState(durable); err == nil {
		t.Fatal("un estado durable debía rechazarse")
	}
	pub := testState(now)
	pub.Privacy = "public"
	if err := s.SetState(pub); err == nil {
		t.Fatal("un estado público debía rechazarse")
	}
	if _, ok := s.ActiveState(now); ok {
		t.Fatal("un estado inválido no debía registrarse")
	}
}

func TestCorruptFileIsQuarantined(t *testing.T) {
	path := filepath.Join(t.TempDir(), "owner_state.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatalf("write corrupt: %v", err)
	}
	now := time.Date(2026, 11, 5, 12, 0, 0, 0, time.Local)
	s := openTestStore(t, path, &now)
	if len(s.Warnings()) == 0 {
		t.Fatal("un archivo corrupto debía dejar aviso")
	}
	if _, ok := s.ActiveState(now); ok {
		t.Fatal("un archivo corrupto no puede inventar estado")
	}
	if err := s.SetState(testState(now)); err != nil {
		t.Fatalf("set tras corrupción: %v", err)
	}
	backups, err := filepath.Glob(path + ".corrupt-*")
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	if len(backups) != 1 {
		t.Fatalf("copias corruptas = %d, quería 1", len(backups))
	}
}

func TestSweepClearsExpiredReadings(t *testing.T) {
	now := time.Date(2026, 11, 5, 12, 0, 0, 0, time.Local)
	s := openTestStore(t, "", &now)
	if err := s.SetState(testState(now)); err != nil {
		t.Fatalf("set: %v", err)
	}
	if err := s.SetReceptivity("high", time.Hour, nil); err != nil {
		t.Fatalf("receptivity: %v", err)
	}
	later := now.Add(3 * time.Hour)
	if err := s.Sweep(later); err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if _, ok := s.ActiveState(later); ok {
		t.Fatal("el estado expirado debía limpiarse")
	}
	if got := s.Receptivity(later); got.Level != "neutral" {
		t.Fatalf("receptividad = %+v", got)
	}
}
