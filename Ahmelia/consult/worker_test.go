package consult

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

type workerFakeClient struct {
	raw     string
	err     error
	started chan struct{}
	release <-chan struct{}
}

func (c *workerFakeClient) GenerateShadowContext(ctx context.Context, prompt string, schema json.RawMessage, numPredict int) (string, error) {
	if c.started != nil {
		close(c.started)
	}
	if c.release != nil {
		select {
		case <-c.release:
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	return c.raw, c.err
}

func workerProposal(id string) string {
	b, _ := json.Marshal(map[string]any{
		"reading": "contexto estable",
		"judgment": map[string]any{
			"owner_state": "absent", "interruptible": false, "risk": "low",
		},
		"choice": map[string]any{
			"candidate_id": id, "reason": "el contexto no cambió", "confidence": 0.9,
		},
		"evidence_ids": []string{"event"},
		"confidence":   0.8,
		"abstain":      false,
	})
	return string(b)
}

func workerRequest() Request {
	return Request{
		Scene:    "test",
		Input:    "evento",
		Evidence: []Evidence{{ID: "event", Text: "evento"}},
	}
}

func waitForFresh(t *testing.T, w *Worker, fingerprint string) Proposal {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		if p, ok := w.Fresh(fingerprint); ok {
			return p
		}
		if time.Now().After(deadline) {
			t.Fatal("el worker no produjo una propuesta fresca")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestWorkerStoresFreshProposal(t *testing.T) {
	w := NewWorker(&workerFakeClient{raw: workerProposal("wait")}, time.Minute)
	if !w.Submit(context.Background(), workerRequest(), "fp-1") {
		t.Fatal("Submit debía aceptar el primer trabajo")
	}
	p := waitForFresh(t, w, "fp-1")
	if p.Choice.CandidateID != "wait" {
		t.Fatalf("propuesta = %+v", p)
	}
	if w.Running() {
		t.Fatal("el worker no debía seguir corriendo")
	}
}

func TestWorkerRejectsSecondInFlight(t *testing.T) {
	release := make(chan struct{})
	started := make(chan struct{})
	w := NewWorker(&workerFakeClient{raw: workerProposal("wait"), started: started, release: release}, time.Minute)
	if !w.Submit(context.Background(), workerRequest(), "fp-1") {
		t.Fatal("el primer Submit debía aceptarse")
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("el modelo no arrancó")
	}
	if w.Submit(context.Background(), workerRequest(), "fp-2") {
		t.Fatal("un segundo trabajo no debía aceptarse en paralelo")
	}
	close(release)
	waitForFresh(t, w, "fp-1")
}

func TestWorkerTTLAndFingerprint(t *testing.T) {
	now := time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)
	w := NewWorker(&workerFakeClient{raw: workerProposal("wait")}, time.Minute)
	w.SetClock(func() time.Time { return now })
	if !w.Submit(context.Background(), workerRequest(), "fp-1") {
		t.Fatal("Submit debía aceptarse")
	}
	waitForFresh(t, w, "fp-1")

	if _, ok := w.Fresh("fp-2"); ok {
		t.Fatal("otro fingerprint no puede usar la propuesta")
	}
	now = now.Add(2 * time.Minute)
	if _, ok := w.Fresh("fp-1"); ok {
		t.Fatal("la propuesta debía expirar por TTL")
	}
}

func TestWorkerErrorIsNotFresh(t *testing.T) {
	w := NewWorker(&workerFakeClient{err: errors.New("timeout")}, time.Minute)
	if !w.Submit(context.Background(), workerRequest(), "fp-1") {
		t.Fatal("Submit debía aceptarse")
	}
	deadline := time.Now().Add(2 * time.Second)
	for w.Running() {
		if time.Now().After(deadline) {
			t.Fatal("el worker no terminó")
		}
		time.Sleep(5 * time.Millisecond)
	}
	if _, ok := w.Fresh("fp-1"); ok {
		t.Fatal("un error no puede producir propuesta fresca")
	}
	if w.LastError() == nil {
		t.Fatal("el error debía quedar registrado")
	}
}

func TestWorkerInvalidateCancelsInFlight(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	w := NewWorker(&workerFakeClient{raw: workerProposal("wait"), started: started, release: release}, time.Minute)
	if !w.Submit(context.Background(), workerRequest(), "fp-1") {
		t.Fatal("Submit debía aceptarse")
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("el modelo no arrancó")
	}
	w.Invalidate()
	deadline := time.Now().Add(2 * time.Second)
	for w.Running() {
		if time.Now().After(deadline) {
			t.Fatal("Invalidate no detuvo el worker")
		}
		time.Sleep(5 * time.Millisecond)
	}
	if _, ok := w.Fresh("fp-1"); ok {
		t.Fatal("una propuesta invalidada no puede consumirse")
	}
	close(release)
}
