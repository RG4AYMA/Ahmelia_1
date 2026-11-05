package initiative

import (
	"testing"
	"time"
)

type testSnapshotSource struct {
	name    string
	signals []InitiativeSignal
}

func (s testSnapshotSource) Name() string { return s.name }
func (s testSnapshotSource) CollectSnapshot(RuntimeSnapshot) []InitiativeSignal {
	return append([]InitiativeSignal(nil), s.signals...)
}

func TestRuntimeCollectsFreshSourceEvidence(t *testing.T) {
	now := time.Date(2026, 9, 7, 14, 0, 0, 0, time.UTC)
	sig := makeSignal(SignalInput{Now: now}, SourceOperationalWork, ReasonNextStepAvailable, "work", "w1", "topic", true, .8)
	sig = WithExpiry(sig, time.Minute)
	r := NewRuntime()
	r.Now = func() time.Time { return now }
	// Runtime's default adapters must still operate from the source-neutral input.
	got := r.Collect(SignalInput{Now: now, ActiveWorkID: "w1"})
	if len(got) == 0 {
		t.Fatal("expected runtime signal")
	}
	if !FreshSignal(sig, now) || FreshSignal(sig, now.Add(2*time.Minute)) {
		t.Fatal("freshness contract failed")
	}
}

func TestRuntimePipelineDoesNotDecide(t *testing.T) {
	now := time.Now()
	sig := makeSignal(SignalInput{Now: now}, SourceOperationalWork, ReasonNextStepAvailable, "work", "w1", "", true, .9)
	p := NewRuntimePipeline(testSnapshotSource{name: "work", signals: []InitiativeSignal{sig}})
	op := p.Opportunity(SignalInput{Now: now}, now)
	if !op.Exists {
		t.Fatal("expected opportunity")
	}
	if op.Signal.Source != SourceOperationalWork {
		t.Fatalf("unexpected source: %s", op.Signal.Source)
	}
}

func TestRuntimeHasAllDeclaredSourceAdapters(t *testing.T) {
	sources := RuntimeSourceAdapters()
	want := []string{"conversational_context", "cognition", "cognition_authoritative_disagreement", "user_model", "historical_memory", "work", "commitment", "autonomy", "availability", "privacy_safety", "channel", "deferred_linguistic"}
	if len(sources) != len(want) {
		t.Fatalf("got %d adapters, want %d", len(sources), len(want))
	}
	for i, name := range want {
		if sources[i].Name() != name {
			t.Fatalf("adapter %d = %q, want %q", i, sources[i].Name(), name)
		}
	}
}

func TestRuntimePreservesNegativeEvidence(t *testing.T) {
	now := time.Now()
	r := NewRuntime()
	signals := r.Collect(SignalInput{Now: now, CandidateExists: true, CandidateText: "candidate", CandidateConfidence: .9, UserAvailability: AvailabilityBusy, Privacy: InformationRestricted, Interruptibility: InterruptNotRecommended, Channel: "voice"})
	seen := map[string]bool{}
	for _, s := range signals {
		seen[s.Type] = true
	}
	for _, typ := range []string{"question_candidate", "user_availability", "information_policy", "interruptibility", "channel"} {
		if !seen[typ] {
			t.Fatalf("missing evidence signal %q", typ)
		}
	}
}
