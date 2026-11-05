package core

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ahmelia/cognition"
	"ahmelia/interaction"
	"ahmelia/memory"
	"ahmelia/shared"
)

// FASE 1 architectural invariant: Engine.execute must have exactly one real
// call site, owned by the central authority gateway. Comments/documentation do
// not count; this parses the source AST rather than doing substring matching.
func TestPhase1SingleEngineExecuteEntryPoint(t *testing.T) {
	providerPath := "local_action_executor.go"
	providerSrc, err := os.ReadFile(providerPath)
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	providerFile, err := parser.ParseFile(fset, providerPath, providerSrc, 0)
	if err != nil {
		t.Fatal(err)
	}

	var owners []string
	for _, decl := range providerFile.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil || fn.Name == nil {
			continue
		}
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "execute" {
				if ident, ok := sel.X.(*ast.Ident); ok && ident.Name == "e" {
					owners = append(owners, fn.Name.Name)
				}
			}
			return true
		})
	}

	if len(owners) != 1 || owners[0] != "ExecuteCapabilityAction" {
		t.Fatalf("Engine.execute must have exactly one provider-owned entry point, got %v", owners)
	}

	engineSrc, err := os.ReadFile("engine.go")
	if err != nil {
		t.Fatal(err)
	}
	engineFile, err := parser.ParseFile(token.NewFileSet(), "engine.go", engineSrc, 0)
	if err != nil {
		t.Fatal(err)
	}
	var authorityProviderCalls []string
	for _, decl := range engineFile.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil || fn.Name == nil || fn.Name.Name != "authorityExecuteWithOptions" {
			continue
		}
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "ExecuteFromAuthority" {
				authorityProviderCalls = append(authorityProviderCalls, fn.Name.Name)
			}
			return true
		})
	}
	if len(authorityProviderCalls) != 1 {
		t.Fatalf("central authority must delegate exactly once to ProductionRuntime, got %d", len(authorityProviderCalls))
	}
}

func TestPhase1AuthorityFailsClosedWithoutCognitiveExecutionPosture(t *testing.T) {
	e := newB963334AuditEngine(t)
	decision := interaction.CapabilityInteractionDecision{
		Executable: true,
		Request: interaction.CapabilityRequest{
			Operation: "saludo",
			Source:    "phase1-test",
		},
	}
	if e.authorityExecute(decision).Handled {
		t.Fatal("execution authority accepted a capability without cognitive execution posture")
	}
	if got := e.history.GetLastAction(); got != nil {
		t.Fatalf("blocked authority crossed execution boundary: %#v", got)
	}
}

func TestPhase1LongMemoryActionBecomesSituationProposalAndCannotExecute(t *testing.T) {
	e := newB963334AuditEngine(t)
	input := "ejecuta mi rutina delta"
	e.longMem.Data[shared.Normalize(input)] = &memory.Knowledge{
		ID:         shared.Normalize(input),
		Type:       memory.KnowCommand,
		Input:      input,
		Action:     "saludo",
		Target:     "",
		Confidence: 0.92,
		Source:     memory.SourceLearned,
	}
	e.SetResponseCallback(func(string) {})

	e.Process(input)

	foundProposal := false
	for _, ev := range e.lastSituation.Evidence {
		if ev.Authority == "memory.LongMemory" && ev.Predicate == "proposes_action" && ev.Object == "saludo" {
			foundProposal = true
			break
		}
	}
	if !foundProposal {
		t.Fatalf("LongMemory proposal did not converge into Situation evidence: %+v", e.lastSituation.Evidence)
	}
	if got := e.history.GetLastAction(); got != nil && got.Action == "saludo" {
		t.Fatalf("LongMemory proposal reached execution history: %#v", got)
	}
}

func TestPhase1AuthorizedExecutionStillReachesEngineExecutor(t *testing.T) {
	e := newB963334AuditEngine(t)
	e.SetResponseCallback(func(string) {})

	// Real Engine E2E: this command must cross Cognition -> CapabilityDecision
	// -> authorityExecute -> Capability Provider -> Engine.execute.
	e.Process("cambia a chrome")

	if !e.lastConversationalCognition.Valid {
		t.Fatalf("cognition did not produce a valid result: %+v", e.lastConversationalCognition.Errors)
	}
	if e.lastConversationalCognition.Action.Primary != cognition.ActionRequestExecution {
		t.Fatalf("real command did not produce execution posture: %s", e.lastConversationalCognition.Action.Primary)
	}
	if !capabilityHandoffReady(e.lastConversationalCognition) {
		t.Fatal("real command did not satisfy the cognition-to-authority contract")
	}

	last := e.history.GetLastAction()
	if last == nil || last.Action != "cambiar_ventana" || last.Target != "chrome" {
		t.Fatalf("authorized execution did not reach the canonical local capability provider: %#v", last)
	}
}

func TestPhase1SessionBranchesCannotTouchPhysicalExecutionAPIsDirectly(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "engine.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	forbidden := map[string]bool{
		"OpenFile":       true,
		"MoveFile":       true,
		"RenameFile":     true,
		"SetVolume":      true,
		"MinimizeAll":    true,
		"CloseWindow":    true,
		"SwitchToWindow": true,
		"SwitchWindow":   true,
		"Command":        true,
	}
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || (fn.Name.Name != "Process" && fn.Name.Name != "handleFileAction") {
			continue
		}
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			if sel, ok := call.Fun.(*ast.SelectorExpr); ok && forbidden[sel.Sel.Name] {
				t.Fatalf("%s contains a direct physical execution API call: %s", fn.Name.Name, sel.Sel.Name)
			}
			return true
		})
	}
}

// Ensure the Phase 1 test itself does not accidentally bind to another copy of
// engine.go when run from a different working directory.
func TestPhase1EngineSourceIsLocal(t *testing.T) {
	info, err := os.Stat(filepath.Join(".", "engine.go"))
	if err != nil {
		t.Fatal(err)
	}
	if info.IsDir() || !strings.HasSuffix(info.Name(), ".go") {
		t.Fatalf("unexpected engine source: %+v", info)
	}
}
