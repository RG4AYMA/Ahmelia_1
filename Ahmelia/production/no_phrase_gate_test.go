package production

import (
	"os"
	"strings"
	"testing"
)

func TestNoPhraseGateProductionD3Boundary(t *testing.T) {
	for _, name := range []string{"execution_guard.go", "recovery.go", "provider_registry.go"} {
		data, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		src := string(data)
		// Whitelist: production may contain machine-state/error text and
		// provider IDs, but D3 execution decisions must consume typed contracts.
		if strings.Contains(src, "HasAnyPhrase") || strings.Contains(src, "strings.Contains(input") {
			t.Fatalf("D3 production boundary contains phrase-gated routing in %s", name)
		}
	}
}
