package initiative

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestB963935DeferredLinguisticEvidenceBecomesInitiativeCandidate(t *testing.T) {
	now := time.Now()
	input := SignalInput{
		Now: now,
		DeferredLinguistic: []DeferredLinguisticEvidence{{
			JobID: "ling-deferred-test", RawInput: "pregunta", Meaning: "Pregunta comprendida", Confidence: .92, CompletedAt: now,
		}},
	}
	var sourceSignal []InitiativeSignal
	for _, src := range RuntimeSourceAdapters() {
		if src.Name() == "deferred_linguistic" {
			sourceSignal = src.CollectSnapshot(RuntimeSnapshot{Input: input, Now: now})
		}
	}
	if len(sourceSignal) != 1 {
		t.Fatalf("expected one deferred linguistic signal, got %d", len(sourceSignal))
	}
	sig := sourceSignal[0]
	if sig.Source != SourceDeferredLinguistic || sig.Reason != ReasonUsefulInformation || sig.Target != "ling-deferred-test" {
		t.Fatalf("unexpected deferred signal: %+v", sig)
	}
	ops := BuildCandidateOpportunities(sourceSignal, input)
	if len(ops) != 1 || ops[0].Source != SourceDeferredLinguistic {
		t.Fatalf("deferred evidence did not become an initiative opportunity: %+v", ops)
	}
}

func TestB963935DeferredWorkerHasNoResponseOrExecutionImports(t *testing.T) {
	data, err := os.ReadFile("../linguistics/deferred.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, forbidden := range []string{
		"ahmelia/tts", "ahmelia/language", "ahmelia/capability", "ahmelia/autonomy",
		"tts.Speak", "onResponse", "Respond(", "Execute(",
	} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("deferred resolver contains forbidden response/execution dependency: %q", forbidden)
		}
	}
}

func TestB963935DeferredEvidenceUsesExistingInitiativePolicy(t *testing.T) {
	now := time.Now()
	input := SignalInput{
		Now:                now,
		UserAvailability:   AvailabilityAvailable,
		Privacy:            InformationAllowed,
		Interruptibility:   InterruptSafe,
		ConversationLoad:   LoadLow,
		RequiresDirectTurn: false,
		DeferredLinguistic: []DeferredLinguisticEvidence{{
			JobID: "ling-deferred-policy", RawInput: "pregunta", Meaning: "Resolución completada", Confidence: .92, CompletedAt: now,
		}},
	}
	sources := []SignalSource{DeferredLinguisticSignalSource{}}
	engine := NewSignalEngine(sources...)
	signals := engine.Collect(input)
	ops := BuildCandidateOpportunities(signals, input)
	if len(ops) != 1 {
		t.Fatalf("expected one deferred opportunity, got %d: %+v", len(ops), ops)
	}
	decision := DefaultPolicy().Evaluate(ops[0], now)
	if decision.Source != SourceDeferredLinguistic || decision.Decision != Intervene {
		t.Fatalf("deferred evidence did not reach existing initiative policy: %+v", decision)
	}
}
