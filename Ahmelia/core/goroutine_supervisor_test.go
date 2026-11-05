package core

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"ahmelia/autonomy"
)

func TestSupervisedAutonomyDecisionConsumerRecoversAfterPanic(t *testing.T) {
	observer := autonomy.NewContextObserver(autonomy.ObserverConfig{MaxBufferSize: 8})
	initiative := autonomy.NewInitiativeEngine(autonomy.InitiativeConfig{})
	conversation := autonomy.NewConversationManager()
	loop := autonomy.NewAutonomousLoop(
		autonomy.LoopConfig{Interval: 10 * time.Millisecond, BufferSize: 4},
		observer,
		initiative,
		conversation,
		nil,
		nil,
	)
	engine := &Engine{autonomousLoop: loop}

	var calls atomic.Int32
	engine.autonomyDecisionConsumerHook = func(autonomy.ConversationDecision) {
		if calls.Add(1) == 1 {
			panic("forced consumer panic")
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	engine.startSupervisedAutonomyDecisionConsumer(ctx)
	loop.Start(ctx)
	defer loop.Stop()

	observer.Observe(autonomy.TypeSystemSignal, "supervisor-test", map[string]interface{}{"severity": "CRITICAL"})

	deadline := time.NewTimer(2 * time.Second)
	defer deadline.Stop()
	for calls.Load() < 1 {
		select {
		case <-deadline.C:
			t.Fatalf("consumer did not receive the first decision; calls=%d", calls.Load())
		default:
			time.Sleep(time.Millisecond)
		}
	}

	observer.Observe(autonomy.TypeSystemSignal, "supervisor-test-2", map[string]interface{}{"severity": "CRITICAL"})
	for calls.Load() < 2 {
		select {
		case <-deadline.C:
			t.Fatalf("consumer did not restart after panic; calls=%d", calls.Load())
		default:
			time.Sleep(time.Millisecond)
		}
	}

	select {
	case failure := <-engine.autonomySupervisor.Reports():
		if failure.Name != "core.consumeAutonomyDecisions" {
			t.Fatalf("unexpected supervisor report name: %q", failure.Name)
		}
		if failure.Panic != "forced consumer panic" {
			t.Fatalf("unexpected panic report: %q", failure.Panic)
		}
		if !failure.Restarted {
			t.Fatal("expected supervisor to restart the consumer")
		}
	default:
		t.Fatal("expected a supervisor failure report")
	}
}
