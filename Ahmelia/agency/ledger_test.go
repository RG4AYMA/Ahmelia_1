package agency

import (
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func testImpulse() Impulse {
	return Impulse{
		Kind:     ImpulseKnowledgeGap,
		Reason:   "el turno dejó una incógnita",
		Strength: 0.70,
		Evidence: []string{"clarification:¿qué versión?"},
	}
}

func TestLedgerProposeAndOpen(t *testing.T) {
	now := time.Date(2026, 11, 1, 12, 0, 0, 0, time.Local)
	l := NewLedger("")
	l.SetClock(func() time.Time { return now })

	rec, err := l.Propose("t1", testImpulse(), validGoal())
	if err != nil {
		t.Fatalf("propose: %v", err)
	}
	if rec.Status != GoalProposed || rec.ID == "" {
		t.Fatalf("registro inesperado: %+v", rec)
	}
	open := l.Open()
	if len(open) != 1 || open[0].ID != rec.ID {
		t.Fatalf("open=%+v", open)
	}
	if counts := l.Counts(); counts[GoalProposed] != 1 {
		t.Fatalf("counts=%v", counts)
	}
}

func TestLedgerProposeRequiresEvidence(t *testing.T) {
	l := NewLedger("")
	g := validGoal()
	g.EvidenceIDs = nil
	if _, err := l.Propose("t1", testImpulse(), g); err == nil {
		t.Fatal("esperaba error sin evidencia")
	}
}

func TestLedgerOpenQuota(t *testing.T) {
	l := NewLedger("")
	if _, err := l.Propose("t1", testImpulse(), validGoal()); err != nil {
		t.Fatalf("primera: %v", err)
	}
	second := validGoal()
	second.Title = "Otra meta distinta"
	_, err := l.Propose("t2", testImpulse(), second)
	if !errors.Is(err, ErrQuota) {
		t.Fatalf("err=%v, esperaba ErrQuota", err)
	}
}

func TestLedgerDailyQuota(t *testing.T) {
	now := time.Date(2026, 11, 1, 12, 0, 0, 0, time.Local)
	l := NewLedger("")
	l.SetClock(func() time.Time { return now })
	l.SetPolicy(5, 1, 24*time.Hour)

	rec, err := l.Propose("t1", testImpulse(), validGoal())
	if err != nil {
		t.Fatalf("primera: %v", err)
	}
	if _, err := l.Authorize(rec.ID, "aceptada"); err != nil {
		t.Fatalf("authorize: %v", err)
	}
	second := validGoal()
	second.Title = "Otra meta distinta"
	if _, err := l.Propose("t2", testImpulse(), second); !errors.Is(err, ErrQuota) {
		t.Fatalf("err=%v, esperaba ErrQuota diaria", err)
	}
}

func TestLedgerRejectsDuplicateTitle(t *testing.T) {
	l := NewLedger("")
	rec, err := l.Propose("t1", testImpulse(), validGoal())
	if err != nil {
		t.Fatalf("primera: %v", err)
	}
	if _, err := l.Authorize(rec.ID, "aceptada"); err != nil {
		t.Fatalf("authorize: %v", err)
	}
	dup := validGoal()
	dup.Title = "  aprender a VERIFICAR el estado de blender "
	if _, err := l.Propose("t2", testImpulse(), dup); !errors.Is(err, ErrDuplicate) {
		t.Fatalf("err=%v, esperaba ErrDuplicate", err)
	}
}

func TestLedgerAuthorizeAndRejectAreDecisionsOnly(t *testing.T) {
	l := NewLedger("")
	rec, err := l.Propose("t1", testImpulse(), validGoal())
	if err != nil {
		t.Fatalf("propose: %v", err)
	}
	got, err := l.Authorize(rec.ID, "el propietario la aceptó como candidata")
	if err != nil {
		t.Fatalf("authorize: %v", err)
	}
	if got.Status != GoalAuthorized || got.DecidedAt.IsZero() {
		t.Fatalf("authorize=%+v", got)
	}
	if _, err := l.Authorize(rec.ID, "otra vez"); !errors.Is(err, ErrNotProposed) {
		t.Fatalf("err=%v, esperaba ErrNotProposed", err)
	}

	rec2, err := l.Propose("t2", testImpulse(), GoalProposal{
		Title:           "Otra idea",
		Motive:          "motivo",
		NextStep:        "paso",
		ExpectedMinutes: 5,
		Risk:            RiskLow,
		Mode:            ModeFocus,
		EvidenceIDs:     []string{"w1"},
		Confidence:      0.5,
	})
	if err != nil {
		t.Fatalf("segunda: %v", err)
	}
	rejected, err := l.Reject(rec2.ID, "no es el momento")
	if err != nil {
		t.Fatalf("reject: %v", err)
	}
	if rejected.Status != GoalRejected || rejected.Reason != "no es el momento" {
		t.Fatalf("reject=%+v", rejected)
	}
}

func TestLedgerExpiresOldProposals(t *testing.T) {
	t0 := time.Date(2026, 11, 1, 12, 0, 0, 0, time.Local)
	l := NewLedger("")
	l.SetClock(func() time.Time { return t0 })
	l.SetPolicy(1, 10, time.Hour)

	rec, err := l.Propose("t1", testImpulse(), validGoal())
	if err != nil {
		t.Fatalf("propose: %v", err)
	}
	l.SetClock(func() time.Time { return t0.Add(2 * time.Hour) })
	if open := l.Open(); len(open) != 0 {
		t.Fatalf("open=%+v, esperaba vacío tras vencer", open)
	}
	got, ok := l.Get(rec.ID)
	if !ok || got.Status != GoalExpired {
		t.Fatalf("get=%+v ok=%v", got, ok)
	}
	if _, err := l.Authorize(rec.ID, "tarde"); !errors.Is(err, ErrNotProposed) {
		t.Fatalf("err=%v, esperaba ErrNotProposed", err)
	}
}

func TestLedgerPersistenceReplaysDecisions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent_goals.jsonl")
	now := time.Date(2026, 11, 1, 12, 0, 0, 0, time.Local)
	l := NewLedger(path)
	l.SetClock(func() time.Time { return now })

	rec, err := l.Propose("t1", testImpulse(), validGoal())
	if err != nil {
		t.Fatalf("propose: %v", err)
	}
	if _, err := l.Authorize(rec.ID, "aceptada"); err != nil {
		t.Fatalf("authorize: %v", err)
	}

	reloaded := NewLedger(path)
	if err := reloaded.Load(); err != nil {
		t.Fatalf("load: %v", err)
	}
	got, ok := reloaded.Get(rec.ID)
	if !ok {
		t.Fatalf("no se reprodujo %s", rec.ID)
	}
	if got.Status != GoalAuthorized || got.Goal.Title != rec.Goal.Title {
		t.Fatalf("replay=%+v", got)
	}
	if counts := reloaded.Counts(); counts[GoalAuthorized] != 1 {
		t.Fatalf("counts=%v", counts)
	}
}

func TestLedgerMarkRegisteredRequiresAuthorization(t *testing.T) {
	l := NewLedger("")
	rec, err := l.Propose("t1", testImpulse(), validGoal())
	if err != nil {
		t.Fatalf("propose: %v", err)
	}
	if _, err := l.MarkRegistered(rec.ID, "20261101120000-1-goal"); !errors.Is(err, ErrNotAuthorized) {
		t.Fatalf("err=%v, esperaba ErrNotAuthorized", err)
	}
	if _, err := l.Authorize(rec.ID, "aceptada"); err != nil {
		t.Fatalf("authorize: %v", err)
	}
	got, err := l.MarkRegistered(rec.ID, "20261101120000-1-goal")
	if err != nil {
		t.Fatalf("mark registered: %v", err)
	}
	if got.Status != GoalRegistered || got.PlannerGoalID != "20261101120000-1-goal" || got.RegisteredAt.IsZero() {
		t.Fatalf("registro=%+v", got)
	}
	if _, err := l.MarkRegistered(rec.ID, "otro-goal"); !errors.Is(err, ErrAlreadyRegistered) {
		t.Fatalf("err=%v, esperaba ErrAlreadyRegistered", err)
	}
	if _, err := l.Authorize(rec.ID, "otra vez"); !errors.Is(err, ErrNotProposed) {
		t.Fatalf("err=%v, esperaba ErrNotProposed", err)
	}
}

func TestLedgerRegisteredSurvivesReplay(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent_goals.jsonl")
	l := NewLedger(path)
	rec, err := l.Propose("t1", testImpulse(), validGoal())
	if err != nil {
		t.Fatalf("propose: %v", err)
	}
	if _, err := l.Authorize(rec.ID, "aceptada"); err != nil {
		t.Fatalf("authorize: %v", err)
	}
	if _, err := l.MarkRegistered(rec.ID, "planner-1"); err != nil {
		t.Fatalf("mark: %v", err)
	}

	reloaded := NewLedger(path)
	if err := reloaded.Load(); err != nil {
		t.Fatalf("load: %v", err)
	}
	got, ok := reloaded.Get(rec.ID)
	if !ok || got.Status != GoalRegistered || got.PlannerGoalID != "planner-1" {
		t.Fatalf("replay=%+v ok=%v", got, ok)
	}
}

func TestLedgerAuthorizeUnknown(t *testing.T) {
	l := NewLedger("")
	if _, err := l.Authorize("ag-nope", "x"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err=%v, esperaba ErrNotFound", err)
	}
}
