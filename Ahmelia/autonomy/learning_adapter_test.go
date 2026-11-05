package autonomy

import (
	"context"
	"path/filepath"
	"sync"
	"testing"

	"ahmelia/memory_intelligence"
)

// mockEventRecorder implementa eventRecorder en memoria pura — a propósito
// no pasa por memory_intelligence.EventStore, para que el test no dependa
// de escribir memory/events.json (EventStore.Append persiste siempre, igual
// que hacía EventLog.Record antes de B93.3).
type mockEventRecorder struct {
	mu     sync.Mutex
	events []memory_intelligence.RawEvent
}

func (m *mockEventRecorder) Append(action, target string, outcome memory_intelligence.Outcome, sessionID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = append(m.events, memory_intelligence.RawEvent{
		Action:    action,
		Target:    target,
		Outcome:   outcome,
		SessionID: sessionID,
	})
}

func (m *mockEventRecorder) Snapshot() []memory_intelligence.RawEvent {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]memory_intelligence.RawEvent, len(m.events))
	copy(out, m.events)
	return out
}

func TestLearningMemoryAdapter_SuccessUpdatesLearning(t *testing.T) {
	recorder := &mockEventRecorder{}
	learningStore := memory_intelligence.NewLearningStore(memory_intelligence.NewLearningState())
	adapter := newLearningMemoryAdapter(
		recorder, learningStore, "session-test",
		filepath.Join(t.TempDir(), "learning.json"),
	)

	req := ActionRequest{ID: "act-1", Action: "crear_carpeta", Target: "docs"}
	verif := VerificationResult{Success: true, Output: "created", ObservationStatus: ObservationConfirmed}

	if err := adapter.RecordOutcome(context.Background(), req, verif); err != nil {
		t.Fatalf("RecordOutcome: %v", err)
	}
	if err := adapter.UpdateState(context.Background(), req, verif); err != nil {
		t.Fatalf("UpdateState: %v", err)
	}

	if len(recorder.events) != 1 {
		t.Fatalf("se esperaba 1 evento, hubo %d", len(recorder.events))
	}
	if !recorder.events[0].Outcome.Success {
		t.Fatal("el evento debía quedar marcado como exitoso")
	}
	if got := learningStore.Confidence("crear_carpeta"); got != 100 {
		t.Fatalf("se esperaba 100%% de confianza, obtuvo %.0f%%", got)
	}
}

func TestLearningMemoryAdapter_FailureUpdatesFailurePattern(t *testing.T) {
	recorder := &mockEventRecorder{}
	learningStore := memory_intelligence.NewLearningStore(memory_intelligence.NewLearningState())
	adapter := newLearningMemoryAdapter(
		recorder, learningStore, "session-test",
		filepath.Join(t.TempDir(), "learning.json"),
	)

	req := ActionRequest{ID: "act-2", Action: "crear_carpeta", Target: "protected"}
	verif := VerificationResult{
		Success:           false,
		ErrorReason:       "permiso denegado",
		ObservationStatus: ObservationContradicted,
	}

	if err := adapter.UpdateState(context.Background(), req, verif); err != nil {
		t.Fatalf("UpdateState: %v", err)
	}

	if len(recorder.events) != 1 {
		t.Fatalf("se esperaba 1 evento de fallo, hubo %d", len(recorder.events))
	}
	if recorder.events[0].Outcome.Success {
		t.Fatal("el evento debía quedar marcado como fallo")
	}
	if got := learningStore.Confidence("crear_carpeta"); got != 0 {
		t.Fatalf("se esperaba 0%% de confianza tras un único fallo, obtuvo %.0f%%", got)
	}
}

func TestLearningMemoryAdapter_DeduplicatesSameAction(t *testing.T) {
	recorder := &mockEventRecorder{}
	learningStore := memory_intelligence.NewLearningStore(memory_intelligence.NewLearningState())
	adapter := newLearningMemoryAdapter(
		recorder, learningStore, "session-test",
		filepath.Join(t.TempDir(), "learning.json"),
	)

	req := ActionRequest{ID: "same-action", Action: "crear_carpeta", Target: "docs"}
	verif := VerificationResult{Success: true, ObservationStatus: ObservationConfirmed}

	const calls = 20
	var wg sync.WaitGroup
	wg.Add(calls)
	for i := 0; i < calls; i++ {
		go func() {
			defer wg.Done()
			if err := adapter.RecordOutcome(context.Background(), req, verif); err != nil {
				t.Errorf("RecordOutcome: %v", err)
			}
		}()
	}
	wg.Wait()

	if len(recorder.events) != 1 {
		t.Fatalf("se esperaba deduplicación a 1 evento, hubo %d", len(recorder.events))
	}
}

func TestLearningMemoryAdapter_ImplementsCycleContracts(t *testing.T) {
	var _ CycleLearner = (*LearningMemoryAdapter)(nil)
	var _ CycleUpdater = (*LearningMemoryAdapter)(nil)
}
