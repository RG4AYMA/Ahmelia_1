package core

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"ahmelia/consult"
	"ahmelia/restriction"
)

type fakeModelConsultClient struct {
	raw   string
	err   error
	calls int
}

func (f *fakeModelConsultClient) GenerateShadowContext(ctx context.Context, prompt string, schema json.RawMessage, numPredict int) (string, error) {
	f.calls++
	return f.raw, f.err
}

func modelConsultJSON(id string, confidence float64, evidence []string) string {
	b, _ := json.Marshal(map[string]any{
		"reading": "el propietario está declarando un día sin trabajo",
		"judgment": map[string]any{
			"owner_state": "resting", "interruptible": false, "risk": "low",
		},
		"choice": map[string]any{
			"candidate_id": id, "reason": "la frase cierra la iniciativa de trabajo", "confidence": confidence,
		},
		"evidence_ids": evidence,
		"confidence":   0.8,
		"abstain":      id == "",
	})
	return string(b)
}

func newModelConsultEngine(t *testing.T, now *time.Time, client *fakeModelConsultClient) *Engine {
	t.Helper()
	e := &Engine{}
	e.enableOwnerRestrictionsAt("")
	e.consultClient = client
	e.modelConsultationOn = true
	e.sceneNow = func() time.Time { return *now }
	return e
}

func TestLooksLikeRestDeclaration(t *testing.T) {
	cases := []struct {
		input string
		want  bool
	}{
		{"hoy no hay trabajo", true},
		{"hoy no trabajamos", true},
		{"nos tomamos el día libre", true},
		{"hoy descansamos", true},
		{"hoy trabajo en el jardín", false},
		{"hoy sí trabajamos", false},
		{"abre chrome", false},
	}
	for _, tc := range cases {
		if got := looksLikeRestDeclaration(tc.input); got != tc.want {
			t.Errorf("%q: got %t, quería %t", tc.input, got, tc.want)
		}
	}
}

func TestModelConsultationAdmitsProvisionalRestriction(t *testing.T) {
	now := time.Date(2026, 11, 5, 8, 0, 0, 0, time.Local)
	client := &fakeModelConsultClient{raw: modelConsultJSON(consultCandidateAdmitRest, 0.9, []string{"turn"})}
	e := newModelConsultEngine(t, &now, client)

	decision, err := e.consultRestTurn(context.Background(), "hoy no hay trabajo", "turn-1")
	if err != nil {
		t.Fatalf("consultar: %v", err)
	}
	if !decision.Accepted || decision.Source != consult.SourceModel || decision.Candidate.ID != consultCandidateAdmitRest {
		t.Fatalf("decisión = %+v", decision)
	}

	r, applied, err := e.applyModelConsultationDecision("hoy no hay trabajo", "turn-1", decision, now)
	if err != nil || !applied {
		t.Fatalf("aplicar: applied=%t err=%v", applied, err)
	}
	if r.Source != restriction.SourceConfirmedInference {
		t.Fatalf("fuente = %s, quería confirmed_inference", r.Source)
	}
	wantExpiry := time.Date(2026, 11, 6, 0, 0, 0, 0, time.Local)
	if r.ExpiresAt == nil || !r.ExpiresAt.Equal(wantExpiry) {
		t.Fatalf("expiry = %v, quería %v", r.ExpiresAt, wantExpiry)
	}
	if !e.OwnerWorkInitiativeRestricted(now) {
		t.Fatal("la restricción provisional debía gobernar")
	}
	if client.calls != 1 {
		t.Fatalf("llamadas = %d, quería 1", client.calls)
	}
}

func TestModelConsultationRejectsUnauthorizedWork(t *testing.T) {
	now := time.Date(2026, 11, 5, 8, 0, 0, 0, time.Local)
	client := &fakeModelConsultClient{raw: modelConsultJSON(consultCandidateOpenWork, 0.99, []string{"turn"})}
	e := newModelConsultEngine(t, &now, client)

	decision, err := e.consultRestTurn(context.Background(), "hoy no hay trabajo", "turn-1")
	if err != nil {
		t.Fatalf("consultar: %v", err)
	}
	if decision.Accepted || decision.Source != consult.SourceFallback || decision.Candidate.ID != consultCandidateNoChange {
		t.Fatalf("decisión = %+v", decision)
	}
	if decision.Rejected != "not_authorized:"+consultCandidateOpenWork {
		t.Fatalf("rechazo = %q", decision.Rejected)
	}
	if _, applied, _ := e.applyModelConsultationDecision("hoy no hay trabajo", "turn-1", decision, now); applied {
		t.Fatal("open_work no debía aplicarse")
	}
	if e.OwnerWorkInitiativeRestricted(now) {
		t.Fatal("no debía existir restricción")
	}
}

func TestModelConsultationRejectsConfidenceBelowRestrictionThreshold(t *testing.T) {
	now := time.Date(2026, 11, 5, 8, 0, 0, 0, time.Local)
	// 0.70 supera el mínimo general (0.55) pero no el de una restricción (0.80).
	client := &fakeModelConsultClient{raw: modelConsultJSON(consultCandidateAdmitRest, 0.70, []string{"turn"})}
	e := newModelConsultEngine(t, &now, client)

	decision, err := e.consultRestTurn(context.Background(), "hoy no hay trabajo", "turn-1")
	if err != nil {
		t.Fatalf("consultar: %v", err)
	}
	if decision.Accepted || decision.Rejected != "confidence_below_restriction_threshold" {
		t.Fatalf("decisión = %+v", decision)
	}
	if _, applied, _ := e.applyModelConsultationDecision("hoy no hay trabajo", "turn-1", decision, now); applied {
		t.Fatal("con confianza 0.70 no debía aplicarse")
	}
}

func TestModelConsultationRejectsUngrounded(t *testing.T) {
	now := time.Date(2026, 11, 5, 8, 0, 0, 0, time.Local)
	client := &fakeModelConsultClient{raw: modelConsultJSON(consultCandidateAdmitRest, 0.95, []string{"made-up"})}
	e := newModelConsultEngine(t, &now, client)

	decision, err := e.consultRestTurn(context.Background(), "hoy no hay trabajo", "turn-1")
	if err != nil {
		t.Fatalf("consultar: %v", err)
	}
	if decision.Accepted || decision.Rejected != "ungrounded" {
		t.Fatalf("decisión = %+v", decision)
	}
}

func TestModelConsultationUnavailable(t *testing.T) {
	now := time.Date(2026, 11, 5, 8, 0, 0, 0, time.Local)
	client := &fakeModelConsultClient{err: errors.New("timeout")}
	e := newModelConsultEngine(t, &now, client)

	decision, err := e.consultRestTurn(context.Background(), "hoy no hay trabajo", "turn-1")
	if !errors.Is(err, consult.ErrUnavailable) {
		t.Fatalf("err = %v, quería ErrUnavailable", err)
	}
	if decision.Accepted || decision.Source != consult.SourceFallback || decision.Candidate.ID != consultCandidateNoChange {
		t.Fatalf("decisión = %+v", decision)
	}
}

func TestModelConsultationRejectsNonRestPhrase(t *testing.T) {
	now := time.Date(2026, 11, 5, 8, 0, 0, 0, time.Local)
	client := &fakeModelConsultClient{raw: modelConsultJSON(consultCandidateAdmitRest, 0.99, []string{"turn"})}
	e := newModelConsultEngine(t, &now, client)

	decision, err := e.consultRestTurn(context.Background(), "hoy trabajo en el jardín", "turn-1")
	if err != nil {
		t.Fatalf("consultar: %v", err)
	}
	if decision.Accepted || decision.Rejected != "not_a_rest_declaration" {
		t.Fatalf("decisión = %+v", decision)
	}
	if _, applied, _ := e.applyModelConsultationDecision("hoy trabajo en el jardín", "turn-1", decision, now); applied {
		t.Fatal("una frase de trabajo no debía admitir restricción")
	}
}

func TestModelConsultationScheduleSkipsHandledTurnAndUnrelatedInput(t *testing.T) {
	now := time.Date(2026, 11, 5, 8, 0, 0, 0, time.Local)
	client := &fakeModelConsultClient{raw: modelConsultJSON(consultCandidateAdmitRest, 0.9, []string{"turn"})}
	e := newModelConsultEngine(t, &now, client)

	e.scheduleModelConsultation(&processTurn{input: "hoy no trabajamos", ownerRestrictionHandled: true})
	e.scheduleModelConsultation(&processTurn{input: "abre chrome"})
	if client.calls != 0 {
		t.Fatalf("llamadas = %d, quería 0", client.calls)
	}
	if e.OwnerWorkInitiativeRestricted(now) {
		t.Fatal("no debía existir restricción")
	}
}

func TestModelConsultationBackgroundApplies(t *testing.T) {
	now := time.Date(2026, 11, 5, 8, 0, 0, 0, time.Local)
	client := &fakeModelConsultClient{raw: modelConsultJSON(consultCandidateAdmitRest, 0.9, []string{"turn"})}
	e := newModelConsultEngine(t, &now, client)

	e.scheduleModelConsultation(&processTurn{input: "hoy no hay trabajo"})
	deadline := time.Now().Add(2 * time.Second)
	for !e.OwnerWorkInitiativeRestricted(now) {
		if time.Now().After(deadline) {
			t.Fatal("la consulta de fondo no admitió la restricción")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestModelConsultationStaleTurnIsDropped(t *testing.T) {
	now := time.Date(2026, 11, 5, 8, 0, 0, 0, time.Local)
	e := newModelConsultEngine(t, &now, &fakeModelConsultClient{})
	e.consultTurnID = "1"
	if !e.modelConsultationCurrent("1") {
		t.Fatal("el turno actual debía aceptarse")
	}
	e.consultTurnID = "2"
	if e.modelConsultationCurrent("1") {
		t.Fatal("un turno posterior debía invalidar la propuesta")
	}
}
