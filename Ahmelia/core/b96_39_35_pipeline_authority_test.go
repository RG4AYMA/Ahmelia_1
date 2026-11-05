package core

import (
	"os"
	"strings"
	"testing"
)

// B96.39.35-3 — the live Engine must have one linguistic orchestration path.
// Core may own the shared Pipeline and call it from the semantic boundary, but
// it must not call Interpreter directly, because that would create a second
// orchestration path that can drift from Router/Validator behavior.
func TestB963935CoreUsesSingleLiveLinguisticPipeline(t *testing.T) {
	data, err := os.ReadFile("engine.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(data)

	start := strings.Index(src, "func (e *Engine) analyzeSemanticBoundary")
	end := strings.Index(src[start:], "func linguisticSyncBudget")
	if start < 0 || end < 0 {
		t.Fatal("semantic boundary function not found")
	}
	boundary := src[start : start+end]
	// The canonical live path is ProcessRequestWithSituation (bounded snapshot
	// of harness state). The legacy ProcessRequest remains accepted for
	// compatibility, but both must route through the same Pipeline.
	if !strings.Contains(boundary, "pipeline.ProcessRequestWithSituation(") &&
		!strings.Contains(boundary, "pipeline.ProcessRequest(") {
		t.Fatal("live semantic boundary does not call the canonical linguistic Pipeline")
	}
	if strings.Contains(boundary, "linguisticInterpreter.Interpret(") || strings.Contains(boundary, "linguisticInterpreter.InterpretContext(") {
		t.Fatal("Core directly calls Interpreter; linguistic orchestration would have two live paths")
	}

	newEngine := src[strings.Index(src, "func NewEngine("):]
	if !strings.Contains(newEngine, "linguisticPipeline := linguistics.NewPipeline(") {
		t.Fatal("NewEngine does not construct the shared linguistic Pipeline")
	}
}
