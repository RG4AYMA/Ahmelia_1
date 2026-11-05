package capability

import (
	"os"
	"strings"
	"testing"
)

func TestNoPhraseGateCapabilityG4Resolver(t *testing.T) {
	data, err := os.ReadFile("router.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(data)
	// Whitelist: capability candidate matching lives in Discovery and may use
	// registered capability metadata; the G4 router itself never classifies the
	// original utterance with text gates.
	for _, bad := range []string{"Raw", "Normalized", "strings.Contains(", "HasPrefix(", "containsAny("} {
		if strings.Contains(src, bad) {
			t.Fatalf("G4 router contains forbidden raw/text gate %q", bad)
		}
	}
	if !strings.Contains(src, "Resolve(decision interaction.CapabilityInteractionDecision, req interaction.SemanticRequest)") {
		t.Fatal("G4 resolver does not consume the accepted typed capability decision")
	}
	if !strings.Contains(src, "DiscoverSemantic(req)") {
		t.Fatal("G4 production routing no longer uses semantic candidate discovery")
	}
}
