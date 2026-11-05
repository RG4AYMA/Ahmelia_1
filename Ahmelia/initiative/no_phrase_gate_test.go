package initiative

import (
	"os"
	"strings"
	"testing"
)

func TestNoPhraseGateInitiativeG2UsesStructuredFindings(t *testing.T) {
	for _, name := range []string{"source_adapters.go", "action_selection.go", "contract.go"} {
		data, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		src := string(data)
		// Whitelist: general TrimSpace calls and non-semantic string literals are
		// legitimate. G2's disagreement path must not inspect LastUserInput or
		// classify the utterance with phrase gates.
		if strings.Contains(src, "LastUserInput") && strings.Contains(src, "ReasonContradictsAuthoritativeEvidence") {
			t.Fatalf("G2 disagreement path in %s touches raw LastUserInput", name)
		}
	}
	data, err := os.ReadFile("source_adapters.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(data)
	if !strings.Contains(src, "DecisionEvidence.Findings") || !strings.Contains(src, "contradicts_authoritative_evidence") {
		t.Fatal("G2 disagreement adapter no longer consumes structured findings")
	}
}
