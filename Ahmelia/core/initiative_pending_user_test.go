package core

import (
	"context"
	"strings"
	"testing"

	"ahmelia/production"
)

func TestInitiativeSuppressedWhileUserTurnPending(t *testing.T) {
	e := buildTestEngine(t)
	e.flowTrace = NewTurnFlowTrace("initiative-pending-user")
	e.userTurnPending.Store(true)
	if e.deliverInitiativeIntervention(&InitiativeDelivery{}) {
		t.Fatal("la iniciativa habló con un turno de usuario pendiente")
	}
	trace := strings.Join(e.flowTrace.Snapshot(), "\n")
	if !strings.Contains(trace, "user turn pending") {
		t.Fatalf("la supresión no quedó observable: %s", trace)
	}
}

func TestInitiativeSuppressedWhileBackgroundJobRuns(t *testing.T) {
	e := buildTestEngine(t)
	started := make(chan struct{})
	release := make(chan struct{})
	id := e.StartBackgroundJob(context.Background(), "slow-shadow", func(ctx context.Context) (production.ActionResult, error) {
		close(started)
		select {
		case <-release:
			return production.ActionResult{}, nil
		case <-ctx.Done():
			return production.ActionResult{}, ctx.Err()
		}
	})
	if id == "" {
		t.Fatal("no se creó el job de fondo")
	}
	<-started
	defer func() {
		_ = e.CancelBackgroundJob(id)
		close(release)
		waitForBackgroundJob(t, e, id, BackgroundJobCanceled)
	}()

	e.flowTrace = NewTurnFlowTrace("initiative-background")
	if e.deliverInitiativeIntervention(&InitiativeDelivery{}) {
		t.Fatal("la iniciativa habló con trabajo de fondo en curso")
	}
	trace := strings.Join(e.flowTrace.Snapshot(), "\n")
	if !strings.Contains(trace, "background work running") {
		t.Fatalf("la supresión no quedó observable: %s", trace)
	}
}
