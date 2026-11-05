package integration

import (
	"os"
	"strings"
	"testing"
)

func TestNoPhraseGateIntegrationD3Boundary(t *testing.T) {
	data, err := os.ReadFile("production_runtime.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(data)
	// Whitelist: Integration may forward SemanticRequest.Raw to a provider as
	// payload, but it must never classify authorization/execution from phrases.
	if strings.Contains(src, "HasAnyPhrase") || strings.Contains(src, "strings.Contains(input") {
		t.Fatal("ProductionRuntime contains phrase-gated execution routing")
	}
}
