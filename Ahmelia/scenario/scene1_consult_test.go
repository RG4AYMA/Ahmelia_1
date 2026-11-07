package scenario

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"ahmelia/consult"
	"ahmelia/restriction"
)

type scriptedConsultClient struct {
	raw   string
	err   error
	calls int
}

func (s *scriptedConsultClient) GenerateShadowContext(ctx context.Context, prompt string, schema json.RawMessage, numPredict int) (string, error) {
	s.calls++
	return s.raw, s.err
}

func consultProposalRaw(id string, confidence float64) string {
	b, _ := json.Marshal(map[string]any{
		"reading": "el propietario declaró un día de descanso",
		"judgment": map[string]any{
			"owner_state": "resting", "interruptible": false, "risk": "low",
		},
		"choice": map[string]any{
			"candidate_id": id, "reason": "la restricción de trabajo sigue activa", "confidence": confidence,
		},
		"evidence_ids": []string{"turn"},
		"confidence":   0.8,
		"abstain":      id == "",
	})
	return string(b)
}

func newScene1ConsultFixture(t *testing.T) (*Clock, *OwnerSim, time.Duration) {
	t.Helper()
	loc := time.FixedZone("COT", -5*3600)
	clock := NewClock(time.Date(2026, 11, 5, 8, 0, 0, 0, loc), loc)
	store, err := restriction.Open(filepath.Join(t.TempDir(), "owner_restrictions.json"))
	if err != nil {
		t.Fatalf("open restrictions: %v", err)
	}
	owner := NewOwnerSim(clock, store, filepath.Join(t.TempDir(), "owner_sim.json"))
	if _, err := owner.DeclareNoWorkToday("hoy no trabajamos"); err != nil {
		t.Fatalf("declare rest: %v", err)
	}
	return clock, owner, 2 * time.Hour
}

func runScene1Consult(t *testing.T, owner *OwnerSim, clock *Clock, window time.Duration, client consult.Client) (consult.Decision, string, error) {
	t.Helper()
	now := clock.Now()
	candidates := Scene1Candidates(owner, now, window)
	req := consult.Request{
		Scene:          "scene1_rest_day",
		Input:          "hoy no trabajamos",
		HarnessReading: "restricción work_initiative activa hasta fin del día",
		OwnerState:     "resting",
		Receptivity:    owner.Receptivity(now).Level,
		Restriction:    "work_initiative activa",
		Candidates:     candidates,
		Evidence: []consult.Evidence{
			{ID: "turn", Text: "hoy no trabajamos"},
			{ID: "restriction", Text: "work_initiative activa hasta fin del día"},
		},
	}
	proposal, err := consult.Consult(context.Background(), client, req)
	if err != nil {
		d := consult.Decide(consult.Proposal{}, candidates, Scene1Allow(owner, now, window), Scene1Silence)
		action, applyErr := Scene1Apply(owner, now, window, d)
		return d, action, errors.Join(err, applyErr)
	}
	d := consult.Decide(proposal, candidates, Scene1Allow(owner, now, window), Scene1Silence)
	action, applyErr := Scene1Apply(owner, now, window, d)
	return d, action, applyErr
}

func TestScene1ModelChoosesAmongAuthorizedCandidates(t *testing.T) {
	clock, owner, window := newScene1ConsultFixture(t)

	d, action, err := runScene1Consult(t, owner, clock, window, &scriptedConsultClient{raw: consultProposalRaw(Scene1Silence, 0.9)})
	if err != nil {
		t.Fatalf("silencio: %v", err)
	}
	if !d.Accepted || d.Source != consult.SourceModel || action != "silence" {
		t.Fatalf("silencio: decisión=%+v action=%q", d, action)
	}
	if owner.QuestionsAsked() != 0 {
		t.Fatal("el silencio no debía gastar una pregunta")
	}

	d, action, err = runScene1Consult(t, owner, clock, window, &scriptedConsultClient{raw: consultProposalRaw(Scene1AskReason, 0.9)})
	if err != nil || !d.Accepted || action != "asked_reason" {
		t.Fatalf("pregunta: decisión=%+v action=%q err=%v", d, action, err)
	}
	if owner.QuestionsAsked() != 1 {
		t.Fatalf("preguntas = %d, quería 1", owner.QuestionsAsked())
	}

	// The same choice a second time must be rejected by the harness budget.
	d, action, err = runScene1Consult(t, owner, clock, window, &scriptedConsultClient{raw: consultProposalRaw(Scene1AskReason, 0.9)})
	if err != nil {
		t.Fatalf("segunda pregunta: %v", err)
	}
	if d.Accepted || d.Source != consult.SourceFallback || action != "silence" {
		t.Fatalf("segunda pregunta: decisión=%+v action=%q", d, action)
	}
	if owner.QuestionsAsked() != 1 {
		t.Fatalf("preguntas = %d, quería 1", owner.QuestionsAsked())
	}
}

func TestScene1ModelCannotProposeRecreationWithoutReceptivity(t *testing.T) {
	clock, owner, window := newScene1ConsultFixture(t)

	d, action, err := runScene1Consult(t, owner, clock, window, &scriptedConsultClient{raw: consultProposalRaw(Scene1Recreation, 0.9)})
	if err != nil {
		t.Fatalf("recreación con receptividad neutra: %v", err)
	}
	if d.Accepted || d.Source != consult.SourceFallback || action != "silence" {
		t.Fatalf("decisión=%+v action=%q", d, action)
	}
	if d.Rejected == "" {
		t.Fatal("el rechazo debía quedar registrado")
	}

	owner.SetReceptivity("high", 2*time.Hour, []string{"turn:open"})
	d, action, err = runScene1Consult(t, owner, clock, window, &scriptedConsultClient{raw: consultProposalRaw(Scene1Recreation, 0.9)})
	if err != nil || !d.Accepted || action != "proposed_recreation" {
		t.Fatalf("decisión=%+v action=%q err=%v", d, action, err)
	}

	// The proposal window is now closed; a second one falls back to silence.
	d, action, err = runScene1Consult(t, owner, clock, window, &scriptedConsultClient{raw: consultProposalRaw(Scene1Recreation, 0.9)})
	if err != nil {
		t.Fatalf("segunda recreación: %v", err)
	}
	if d.Accepted || d.Source != consult.SourceFallback || action != "silence" {
		t.Fatalf("segunda recreación: decisión=%+v action=%q", d, action)
	}
}

func TestScene1ModelCannotReopenWork(t *testing.T) {
	clock, owner, window := newScene1ConsultFixture(t)

	d, action, err := runScene1Consult(t, owner, clock, window, &scriptedConsultClient{raw: consultProposalRaw(Scene1Work, 0.99)})
	if err != nil {
		t.Fatalf("trabajo: %v", err)
	}
	if d.Accepted || d.Source != consult.SourceFallback || action != "silence" {
		t.Fatalf("decisión=%+v action=%q", d, action)
	}
	if d.Rejected != "not_authorized:"+Scene1Work {
		t.Fatalf("rechazo = %q", d.Rejected)
	}
	if !owner.Restrictions().WorkInitiativeActive(clock.Now()) {
		t.Fatal("la restricción debía seguir activa")
	}
}

func TestScene1ModelUnavailableFallsBackToDeterministic(t *testing.T) {
	clock, owner, window := newScene1ConsultFixture(t)
	client := &scriptedConsultClient{err: errors.New("timeout de 40 s")}

	d, action, err := runScene1Consult(t, owner, clock, window, client)
	if !errors.Is(err, consult.ErrUnavailable) {
		t.Fatalf("err = %v, quería ErrUnavailable", err)
	}
	if d.Accepted || d.Source != consult.SourceFallback || action != "silence" {
		t.Fatalf("decisión=%+v action=%q", d, action)
	}
	if owner.QuestionsAsked() != 0 {
		t.Fatal("sin modelo no debía gastarse una pregunta")
	}
}

func TestScene1ModelInventsCandidate(t *testing.T) {
	clock, owner, window := newScene1ConsultFixture(t)

	d, action, err := runScene1Consult(t, owner, clock, window, &scriptedConsultClient{raw: consultProposalRaw("work_now", 0.99)})
	if err != nil {
		t.Fatalf("candidato inventado: %v", err)
	}
	if d.Accepted || d.Rejected != "unknown_candidate:work_now" || action != "silence" {
		t.Fatalf("decisión=%+v action=%q", d, action)
	}
}
