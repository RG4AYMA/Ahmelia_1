package autonomy

import (
	"context"
	"testing"
	"time"

	"ahmelia/consult"
)

type fakeInitiativeConsultant struct {
	proposal    consult.Proposal
	fresh       bool
	submitted   int
	fingerprint string
}

func (f *fakeInitiativeConsultant) ProposeInitiative(req consult.Request, fingerprint string) bool {
	f.submitted++
	f.fingerprint = fingerprint
	return true
}

func (f *fakeInitiativeConsultant) FreshInitiative(fingerprint string) (consult.Proposal, bool) {
	return f.proposal, f.fresh
}

func initiativeProposal(id string) consult.Proposal {
	return consult.Proposal{
		Reading:     "el propietario sigue ausente",
		Judgment:    consult.Judgment{OwnerState: "absent", Interruptible: false, Risk: "low"},
		Choice:      consult.Choice{CandidateID: id, Reason: "el contexto no cambió", Confidence: 0.9},
		EvidenceIDs: []string{"goal"},
		Grounded:    true,
		Confidence:  0.8,
	}
}

func TestInitiativeConsultationVetoesResume(t *testing.T) {
	observer := NewContextObserver(ObserverConfig{})
	loop := newRestrictionGateLoop(observer, true, "goal_1")
	loop.SetInitiativeConsultant(&fakeInitiativeConsultant{
		proposal: initiativeProposal("wait"),
		fresh:    true,
	})

	loop.runIntegratedTick(context.Background())

	if got := len(loop.OutputChan()); got != 0 {
		t.Fatalf("decisiones emitidas = %d, quería 0 con el veto del modelo", got)
	}
	if got := len(observer.GetUnprocessedObservations()); got != 0 {
		t.Fatalf("observaciones sin procesar = %d", got)
	}
}

func TestInitiativeConsultationProceeds(t *testing.T) {
	observer := NewContextObserver(ObserverConfig{})
	loop := newRestrictionGateLoop(observer, true, "goal_1")
	loop.SetInitiativeConsultant(&fakeInitiativeConsultant{
		proposal: initiativeProposal("proceed"),
		fresh:    true,
	})

	loop.runIntegratedTick(context.Background())

	select {
	case decision := <-loop.OutputChan():
		if decision.Strategy != StrategyResumeTopic {
			t.Fatalf("estrategia = %s, quería %s", decision.Strategy, StrategyResumeTopic)
		}
	default:
		t.Fatal("con proceed la reanudación debía emitirse")
	}
}

func TestInitiativeConsultationUnavailableKeepsDeterministicResume(t *testing.T) {
	observer := NewContextObserver(ObserverConfig{})
	loop := newRestrictionGateLoop(observer, true, "goal_1")
	loop.SetInitiativeConsultant(&fakeInitiativeConsultant{fresh: false})

	loop.runIntegratedTick(context.Background())

	select {
	case decision := <-loop.OutputChan():
		if decision.Strategy != StrategyResumeTopic {
			t.Fatalf("estrategia = %s, quería %s", decision.Strategy, StrategyResumeTopic)
		}
	default:
		t.Fatal("sin propuesta la reanudación determinista debía seguir")
	}
}

func TestInitiativeConsultationSubmitsInLeadWindow(t *testing.T) {
	observer := NewContextObserver(ObserverConfig{})
	loop := NewAutonomousLoop(
		LoopConfig{Interval: time.Second},
		observer,
		NewInitiativeEngine(InitiativeConfig{}),
		NewConversationManager(),
		&mockStateProvider{lastActivity: time.Now().Add(-14 * time.Minute)},
		&mockGoalProvider{hasPending: true, goalID: "goal_1"},
	)
	consultant := &fakeInitiativeConsultant{fresh: false}
	loop.SetInitiativeConsultant(consultant)

	loop.runIntegratedTick(context.Background())

	if consultant.submitted != 1 {
		t.Fatalf("consultas enviadas = %d, quería 1 en la ventana de anticipación", consultant.submitted)
	}
	if got := len(loop.OutputChan()); got != 0 {
		t.Fatalf("decisiones emitidas = %d, quería 0 antes del umbral", got)
	}
}

func TestInitiativeConsultationIgnoresCriticalSignal(t *testing.T) {
	observer := NewContextObserver(ObserverConfig{})
	observer.Observe(TypeSystemSignal, "monitor", map[string]interface{}{"severity": "CRITICAL"})
	loop := newRestrictionGateLoop(observer, true, "goal_1")
	loop.SetInitiativeConsultant(&fakeInitiativeConsultant{
		proposal: initiativeProposal("wait"),
		fresh:    true,
	})

	loop.runIntegratedTick(context.Background())

	select {
	case decision := <-loop.OutputChan():
		if decision.Strategy != StrategyInterruptLightly {
			t.Fatalf("estrategia = %s, quería %s", decision.Strategy, StrategyInterruptLightly)
		}
	default:
		t.Fatal("la señal crítica no puede ser vetada por el modelo")
	}
}
