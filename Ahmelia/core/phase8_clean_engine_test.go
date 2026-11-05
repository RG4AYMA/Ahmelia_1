package core

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"
)

// Phase 8 — Process is a coordinator, not a second decision engine.
func TestPhase8ProcessIsCoordinatorOnly(t *testing.T) {
	src, err := os.ReadFile("engine.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "engine.go", src, 0)
	if err != nil {
		t.Fatal(err)
	}
	var process *ast.FuncDecl
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if ok && fn.Name != nil && fn.Name.Name == "Process" {
			process = fn
			break
		}
	}
	if process == nil {
		t.Fatal("Process AST not found")
	}

	forbiddenSelectors := map[string]bool{
		"say":                          true,
		"sayAsync":                     true,
		"authorityExecute":             true,
		"ResolveCapabilityInteraction": true,
		"Execute":                      true,
		"QueryWork":                    true,
		"evaluateWorkflow":             true,
		"evaluateMetacognition":        true,
		"renderResponseContract":       true,
	}
	ast.Inspect(process.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if sel, ok := call.Fun.(*ast.SelectorExpr); ok && forbiddenSelectors[sel.Sel.Name] {
			t.Errorf("Process contains terminal/domain decision call %s", sel.Sel.Name)
		}
		return true
	})
}

func TestPhase8ProcessHasCanonicalOrderAndReasonableSize(t *testing.T) {
	src, err := os.ReadFile("engine.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(src)
	start := strings.Index(text, "func (e *Engine) Process(input string) {")
	end := start + strings.Index(text[start:], "\n}\n\n/* =======================\n   EXECUTE")
	if start < 0 || end <= start {
		t.Fatal("Process boundaries not found")
	}
	process := text[start:end]
	lines := strings.Count(process, "\n") + 1
	if lines > 100 {
		t.Fatalf("Process remains too large for coordinator role: %d lines", lines)
	}
	ordered := []string{
		"e.inputContextPhase(t)",
		"e.evidencePhase(t)",
		"e.situationPhase(t)",
		"e.cognitionPhase(t)",
		"e.dialoguePhase(t)",
		"e.decisionPhase(t)",
		"e.preExecutionPhase(t)",
		"e.executionLifecyclePhase()",
		"e.finishExecutionPhase(t)",
	}
	last := -1
	for _, marker := range ordered {
		pos := strings.Index(process, marker)
		if pos < 0 {
			t.Fatalf("Process missing phase marker %q", marker)
		}
		if pos <= last {
			t.Fatalf("phase order regressed at %q", marker)
		}
		last = pos
	}
}

func TestPhase8WorkflowAndMetacognitionRemainOutsideProcess(t *testing.T) {
	for _, file := range []string{"workflow_runtime.go", "metacognitive_runtime.go", "turn_pipeline.go"} {
		if _, err := os.Stat(file); err != nil {
			t.Fatalf("required runtime boundary file missing: %s: %v", file, err)
		}
	}
	workflow, err := os.ReadFile("workflow_runtime.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(workflow), "func (e *Engine) evaluateWorkflow") || !strings.Contains(string(workflow), "func (e *Engine) applyWorkflowDecision") {
		t.Fatal("workflow authority boundary was not preserved")
	}
	meta, err := os.ReadFile("metacognitive_runtime.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(meta), "func (e *Engine) collectMetacognitiveDiagnostics") || !strings.Contains(string(meta), "func (e *Engine) evaluateMetacognition") {
		t.Fatal("metacognition boundary was not preserved")
	}
}
