package core

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"ahmelia/memory"
	"ahmelia/production"
	"ahmelia/programming"
	"ahmelia/selfknowledge"
)

func TestB96ProgrammingFailureReachesB95Recovery(t *testing.T) {
	svc := production.NewTechnicalRecoveryService()
	got, err := svc.Recover(context.Background(), "test-action", "tests failed: main_test.go:5", 1)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Decision.Action.Valid() {
		t.Fatalf("invalid recovery action: %q", got.Decision.Action)
	}
}

func TestProgrammingPlanFileChangeIsIdempotent(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("old"), 0644); err != nil {
		t.Fatal(err)
	}
	c := programmingPlanFileChange(root, "p", "go.mod", programming.RoleConfig, "new", "01")
	if c.Operation != programming.SourceWrite {
		t.Fatalf("existing managed file must become write, got %q", c.Operation)
	}
	c = programmingPlanFileChange(root, "p", "main.go", programming.RoleEntrypoint, "new", "02")
	if c.Operation != programming.SourceCreate {
		t.Fatalf("missing file must remain create, got %q", c.Operation)
	}
}

func TestEngineStillConstructsWithRuntimeIntegration(t *testing.T) {
	isolateTestWorkspace(t)
	t.Setenv("AHMELIA_SKIP_PROGRAM_INDEX_SCAN", "1")
	longMem := memory.LoadLongMemory()
	shortMem := LoadShortMemory()
	sess := memory.NewSession()
	history := selfknowledge.NewActionHistory(4)
	registry := selfknowledge.NewModuleRegistry()
	selfQuery := selfknowledge.NewSelfQuery(history, registry, "test")
	e := NewEngine(sess, longMem, &shortMem, history, selfQuery)
	if e.programmingIntegration == nil || e.capabilityRegistry == nil || e.capabilityDiscovery == nil {
		t.Fatal("B94-B96 runtime integration missing")
	}
}
