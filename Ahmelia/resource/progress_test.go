package resource

import (
	"sync"
	"testing"
	"time"
)

func TestProgressEmitter_StartStop(t *testing.T) {
	sink := &testSink{}
	e := NewProgressEmitter(sink, DefaultThresholdConfig())

	e.Start(TaskClassInterpretation)
	time.Sleep(10 * time.Millisecond)

	e.Stop(TaskClassInterpretation)

	// No debe haber señales porque no pasó el umbral
	if len(sink.signals) != 0 {
		t.Fatalf("expected 0 signals, got %d", len(sink.signals))
	}
}

func TestProgressEmitter_EmitsAfterWarnAfter(t *testing.T) {
	sink := &testSink{}
	config := ThresholdConfig{
		WarnAfter:   50 * time.Millisecond,
		RepeatEvery: 100 * time.Millisecond,
		MaxSignals:  2,
	}
	e := NewProgressEmitter(sink, config)

	e.Start(TaskClassInterpretation)
	time.Sleep(200 * time.Millisecond)
	e.Stop(TaskClassInterpretation)

	// Debe haber emitido 2 señales (MaxSignals = 2)
	if len(sink.signals) != 2 {
		t.Fatalf("expected 2 signals, got %d: %+v", len(sink.signals), sink.signals)
	}

	// Verificar que las señales tienen la clase correcta
	for _, sig := range sink.signals {
		if sig.Class != TaskClassInterpretation {
			t.Fatalf("signal class = %s, want interpretation", sig.Class)
		}
		if sig.Threshold < 50*time.Millisecond {
			t.Fatalf("signal threshold = %v, want >= 50ms", sig.Threshold)
		}
	}
}

func TestProgressEmitter_NoSignalsBeforeWarnAfter(t *testing.T) {
	sink := &testSink{}
	config := ThresholdConfig{
		WarnAfter:   100 * time.Millisecond,
		RepeatEvery: 50 * time.Millisecond,
		MaxSignals:  10,
	}
	e := NewProgressEmitter(sink, config)

	e.Start(TaskClassGeneration)
	time.Sleep(50 * time.Millisecond)
	e.Stop(TaskClassGeneration)

	if len(sink.signals) != 0 {
		t.Fatalf("expected 0 signals before WarnAfter, got %d", len(sink.signals))
	}
}

func TestProgressEmitter_RespectsMaxSignals(t *testing.T) {
	sink := &testSink{}
	config := ThresholdConfig{
		WarnAfter:   30 * time.Millisecond,
		RepeatEvery: 20 * time.Millisecond,
		MaxSignals:  1,
	}
	e := NewProgressEmitter(sink, config)

	e.Start(TaskClassProposal)
	time.Sleep(200 * time.Millisecond)
	e.Stop(TaskClassProposal)

	if len(sink.signals) != 1 {
		t.Fatalf("expected 1 signal (MaxSignals=1), got %d", len(sink.signals))
	}
}

func TestProgressEmitter_StopBeforeWarnAfter_NoSignal(t *testing.T) {
	sink := &testSink{}
	config := ThresholdConfig{
		WarnAfter:   100 * time.Millisecond,
		RepeatEvery: 50 * time.Millisecond,
		MaxSignals:  10,
	}
	e := NewProgressEmitter(sink, config)

	e.Start(TaskClassInterpretation)
	e.Stop(TaskClassInterpretation)

	if len(sink.signals) != 0 {
		t.Fatalf("expected 0 signals after immediate stop, got %d", len(sink.signals))
	}
}

func TestProgressEmitter_ConcurrentClasses(t *testing.T) {
	sink := &testSink{}
	config := ThresholdConfig{
		WarnAfter:   30 * time.Millisecond,
		RepeatEvery: 20 * time.Millisecond,
		MaxSignals:  3,
	}
	e := NewProgressEmitter(sink, config)

	e.Start(TaskClassInterpretation)
	e.Start(TaskClassGeneration)
	time.Sleep(150 * time.Millisecond)
	e.Stop(TaskClassInterpretation)
	e.Stop(TaskClassGeneration)

	// Ambas clases deben haber emitido señales
	interpretationCount := 0
	generationCount := 0
	for _, sig := range sink.signals {
		if sig.Class == TaskClassInterpretation {
			interpretationCount++
		} else if sig.Class == TaskClassGeneration {
			generationCount++
		}
	}

	if interpretationCount == 0 {
		t.Fatal("interpretation should have signals")
	}
	if generationCount == 0 {
		t.Fatal("generation should have signals")
	}
}

func TestProgressEmitter_GetState(t *testing.T) {
	sink := &testSink{}
	e := NewProgressEmitter(sink, DefaultThresholdConfig())

	startedAt, signals, running := e.GetState(TaskClassInterpretation)
	if running {
		t.Fatal("should not be running before Start")
	}

	e.Start(TaskClassInterpretation)
	time.Sleep(10 * time.Millisecond)

	startedAt, signals, running = e.GetState(TaskClassInterpretation)
	if !running {
		t.Fatal("should be running after Start")
	}
	if signals != 0 {
		t.Fatalf("signals = %d, want 0", signals)
	}
	if startedAt.IsZero() {
		t.Fatal("startedAt should not be zero")
	}

	e.Stop(TaskClassInterpretation)

	startedAt, signals, running = e.GetState(TaskClassInterpretation)
	if running {
		t.Fatal("should not be running after Stop")
	}
}

func TestProgressEmitter_DoubleStartIdempotent(t *testing.T) {
	sink := &testSink{}
	e := NewProgressEmitter(sink, DefaultThresholdConfig())

	e.Start(TaskClassInterpretation)
	e.Start(TaskClassInterpretation) // segunda llamada no debe hacer nada

	_, signals, running := e.GetState(TaskClassInterpretation)
	if !running {
		t.Fatal("should be running")
	}
	if signals != 0 {
		t.Fatalf("signals = %d, want 0", signals)
	}

	e.Stop(TaskClassInterpretation)
}

func TestProgressEmitter_DoubleStopIdempotent(t *testing.T) {
	sink := &testSink{}
	e := NewProgressEmitter(sink, DefaultThresholdConfig())

	e.Start(TaskClassInterpretation)
	e.Stop(TaskClassInterpretation)
	e.Stop(TaskClassInterpretation) // no debe fallar

	_, _, running := e.GetState(TaskClassInterpretation)
	if running {
		t.Fatal("should not be running after Stop")
	}
}

func TestProgressEmitter_ThresholdConfigDefaults(t *testing.T) {
	// Test that DefaultThresholdConfig returns expected defaults
	config := DefaultThresholdConfig()
	if config.WarnAfter != 5*time.Second {
		t.Fatalf("WarnAfter default = %v, want 5s", config.WarnAfter)
	}
	if config.RepeatEvery != 10*time.Second {
		t.Fatalf("RepeatEvery default = %v, want 10s", config.RepeatEvery)
	}
	if config.MaxSignals != 3 {
		t.Fatalf("MaxSignals default = %d, want 3", config.MaxSignals)
	}
}

type testSink struct {
	mu      sync.Mutex
	signals []ProgressSignal
}

func (s *testSink) OnProgress(sig ProgressSignal) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.signals = append(s.signals, sig)
}
