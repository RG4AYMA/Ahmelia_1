package core

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"ahmelia/interaction"
	"ahmelia/production"
)

func newF10Engine(t *testing.T) *Engine {
	t.Helper()
	t.Setenv("AHMELIA_FLOW_TRACE", "1")
	e := newF9Engine(t)
	// The Engine test fixture installs a deterministic production synthesizer;
	// adversarial tests can therefore retain real Work/repair ownership without
	// calling a live Ollama service.
	e.planner.SetGoalTaskDecomposer(nil)
	return e
}

func f10Trace(t *testing.T, e *Engine, input string) string {
	t.Helper()
	e.Process(input)
	if e.lastFlowTrace == nil {
		t.Fatalf("no flow trace retained for %q", input)
	}
	return strings.Join(e.lastFlowTrace.Snapshot(), "\n")
}

func requireTraceStages(t *testing.T, trace string, stages ...string) {
	t.Helper()
	last := -1
	for _, stage := range stages {
		idx := strings.Index(trace, "] "+stage+" |")
		if idx < 0 {
			t.Fatalf("flow trace missing %q:\n%s", stage, trace)
		}
		if idx < last {
			t.Fatalf("flow trace order regressed at %q:\n%s", stage, trace)
		}
		last = idx
	}
}

func assertNoTraceExecutionAcceptance(t *testing.T, trace string) {
	t.Helper()
	if strings.Contains(trace, "EXECUTION AUTHORITY | accepted:") || strings.Contains(trace, "repair accepted:") {
		t.Fatalf("unexpected execution authority acceptance:\n%s", trace)
	}
}

func TestF10AdversarialYesterdayRequiresEvidenceBeforeResponse(t *testing.T) {
	e := newF10Engine(t)
	var responses []string
	e.SetResponseCallback(func(msg string) { responses = append(responses, msg) })
	trace := f10Trace(t, e, "¿Recuerdas qué estaba haciendo ayer?")

	requireTraceStages(t, trace, "COGNITION", "DIALOGUE", "RESPONSE", "LANGUAGE")
	if len(responses) == 0 || strings.TrimSpace(responses[len(responses)-1]) == "" {
		t.Fatal("yesterday query produced no observable response")
	}
	got := strings.ToLower(responses[len(responses)-1])
	for _, forbidden := range []string{"work_", "productionid", "runtime", "internal state", "/home/oai"} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("historical query leaked runtime detail %q: %q", forbidden, got)
		}
	}
}

func TestF10AdversarialWhyDecisionUsesRecordedDecisionPath(t *testing.T) {
	e := newF10Engine(t)
	var responses []string
	e.SetResponseCallback(func(msg string) { responses = append(responses, msg) })
	f10Trace(t, e, "Quiero crear un programa Go llamado f10_reasoning")
	responses = nil
	trace := f10Trace(t, e, "¿Y por qué decidiste eso?")

	requireTraceStages(t, trace, "COGNITION", "DIALOGUE", "RESPONSE", "LANGUAGE")
	if len(responses) == 0 {
		t.Fatal("why-decision query produced no response")
	}
	got := strings.ToLower(responses[len(responses)-1])
	if strings.Contains(got, "decidí porque tú querías") || strings.Contains(got, "decidi porque tu querias") {
		t.Fatalf("language invented an unsupported decision rationale: %q", got)
	}
}

func TestF10AdversarialPauseDoesNotCrossExecutionAuthority(t *testing.T) {
	e := newF10Engine(t)
	work, err := e.workStore.Begin(interaction.Parse("Crear una escena base para supervivencia", interaction.ChannelText))
	if err != nil {
		t.Fatal(err)
	}
	if err := e.workStore.AttachProduction(work.ID, "goal-f10", "plan-f10", "test-provider"); err != nil {
		t.Fatal(err)
	}
	before, _ := e.workStore.Get(work.ID)
	trace := f10Trace(t, e, "Espera, no hagas eso todavía.")
	assertNoTraceExecutionAcceptance(t, trace)
	if after, _ := e.workStore.Get(work.ID); after.Status != before.Status || after.Objective.Text != before.Objective.Text {
		t.Fatalf("pause mutated work unexpectedly: before=%+v after=%+v", before, after)
	}
	if !strings.Contains(trace, "RESPONSE GATE") {
		t.Fatalf("pause did not converge to a response gate:\n%s", trace)
	}
}

func TestF10AdversarialCompoundContinuationDoesNotForkWork(t *testing.T) {
	e := newF10Engine(t)
	e.Process("Quiero crear un programa Go llamado f10_compuesto")
	before, ok := e.workStore.Last()
	if !ok {
		t.Fatal("expected active materialized work")
	}
	beforeID := before.ID
	trace := f10Trace(t, e, "Continúa pero cambia el objetivo.")
	after, ok := e.workStore.Last()
	if !ok || after.ID != beforeID {
		t.Fatalf("compound continuation forked work ownership: before=%q after=%q ok=%t", beforeID, after.ID, ok)
	}
	requireTraceStages(t, trace, "COGNITION", "DIALOGUE")
	if strings.Contains(trace, "PRODUCTION RESULT") && !strings.Contains(trace, "EXECUTION AUTHORITY") {
		t.Fatalf("production result appeared without execution authority:\n%s", trace)
	}
}

func TestF10AdversarialUserKnowledgeQuestionDoesNotLeakInternalState(t *testing.T) {
	e := newF10Engine(t)
	var responses []string
	e.SetResponseCallback(func(msg string) { responses = append(responses, msg) })
	trace := f10Trace(t, e, "¿Qué sabes de mí?")
	requireTraceStages(t, trace, "COGNITION", "RESPONSE", "LANGUAGE")
	if len(responses) == 0 {
		t.Fatal("user-knowledge query produced no response")
	}
	got := strings.ToLower(responses[len(responses)-1])
	for _, forbidden := range []string{"memory/conversation_history", "production_work", "work_", "internalstate", "internal state", "ahmelia_core"} {
		if strings.Contains(got, strings.ToLower(forbidden)) {
			t.Fatalf("user-knowledge query leaked internal state %q: %q", forbidden, got)
		}
	}
}

func TestF10AdversarialProjectQuestionSeparatesWorkEvidenceFromInternalIDs(t *testing.T) {
	e := newF10Engine(t)
	e.Process("Quiero crear un programa Go llamado f10_project")
	var responses []string
	e.SetResponseCallback(func(msg string) { responses = append(responses, msg) })
	trace := f10Trace(t, e, "¿Qué sabes del proyecto?")
	requireTraceStages(t, trace, "COGNITION", "DIALOGUE", "RESPONSE", "LANGUAGE")
	if len(responses) == 0 {
		t.Fatal("project question produced no response")
	}
	got := strings.ToLower(responses[len(responses)-1])
	if strings.Contains(got, "work_") || strings.Contains(got, "/home/oai/") || strings.Contains(got, "internalstate") {
		t.Fatalf("project answer leaked internal work/runtime identifiers: %q", got)
	}
}

func TestF10AdversarialExecuteAndExplainPreservesExecutionResponseOrder(t *testing.T) {
	e := newF10Engine(t)
	var responses []string
	e.SetResponseCallback(func(msg string) { responses = append(responses, msg) })
	trace := f10Trace(t, e, "Hazlo y explícame mientras avanzas.")

	// Novel phrasing may fail to select a capability, but it must never fabricate
	// progress or bypass authority. If execution occurs, the trace must prove the
	// canonical capability -> authority -> production -> response route.
	if strings.Contains(trace, "PRODUCTION RESULT") {
		requireTraceStages(t, trace, "COGNITION→RUNTIME", "EXECUTION AUTHORITY", "PRODUCTION RESULT", "RESPONSE")
	}
	if len(responses) == 0 {
		t.Fatal("compound execution/explanation input produced no response")
	}
	got := strings.ToLower(responses[len(responses)-1])
	if strings.Contains(got, "ya avancé") || strings.Contains(got, "ya avance") || strings.Contains(got, "mientras avanzo") {
		if !strings.Contains(trace, "PRODUCTION RESULT") {
			t.Fatalf("response claimed progress without a production result: %q\n%s", got, trace)
		}
	}
}

func TestF10AdversarialCorrectionWithoutKnownCueRequiresConcreteReplacement(t *testing.T) {
	cases := []string{
		"Eso no era lo que quería.",
		"No era eso.",
		"No es lo que quería.",
	}
	for _, input := range cases {
		t.Run(input, func(t *testing.T) {
			e := newF10Engine(t)
			projectRoot := seedVerifiedF10Project(t)

			// This test isolates conversational correction semantics. Its precondition is
			// a real, locally materialized and tested Go project recorded through the
			// canonical WorkStore; it must not call Ollama to manufacture that precondition.
			req := interaction.Parse("Quiero crear un programa Go llamado f10_repair", interaction.ChannelText)
			work, err := e.workStore.Begin(req)
			if err != nil {
				t.Fatal(err)
			}
			if err := e.workStore.AttachProduction(work.ID, "goal-f10-repair", "plan-f10-repair", "programming"); err != nil {
				t.Fatal(err)
			}
			if err := e.workStore.RecordActionResult(work.ID, production.ActionResult{
				Executed: true, Success: true, Verified: true, Observed: true,
				Summary: "Fixture project materialized and go test passed.",
				WorkID:  work.ID, ProductionID: "production-f10-repair-fixture", Producer: "programming",
				Project: production.ProductionProject{ID: "f10_repair", Name: "f10_repair", Path: projectRoot},
			}); err != nil {
				t.Fatal(err)
			}
			e.conversationState.SetActiveWork(work.ID)
			e.conversationState.SetActiveObjective(req.Objective.Text)

			trace := f10Trace(t, e, input)
			if e.lastConversationalCognition.Repair.Required {
				t.Fatalf("ambiguous correction activated repair without a concrete replacement: %+v", e.lastConversationalCognition.Repair)
			}
			assertNoTraceExecutionAcceptance(t, trace)
			requireTraceStages(t, trace, "COGNITION", "RESPONSE", "LANGUAGE")
			if after, ok := e.workStore.Get(work.ID); !ok || after.LastActionResult == nil || !after.LastActionResult.Verified {
				t.Fatalf("ambiguous correction corrupted the verified prior work: work=%+v ok=%t", after, ok)
			}
		})
	}
}

// TestF10ProductionCreatesVerifiedGoProject is the separate Ollama-backed
// production E2E. Set AHMELIA_RUN_OLLAMA_E2E=1 to opt in on a machine with Ollama
// running; ordinary conversational regression tests stay deterministic.
func TestF10ProductionCreatesVerifiedGoProject(t *testing.T) {
	if os.Getenv("AHMELIA_RUN_OLLAMA_E2E") != "1" {
		t.Skip("set AHMELIA_RUN_OLLAMA_E2E=1 to run the real Ollama production E2E")
	}
	e := newF10Engine(t)
	var recoveryPrompts []string
	e.linguisticClient.SetStructuredPromptObserver(func(prompt string, _ json.RawMessage, _ int) {
		if strings.Contains(prompt, "[Recovery context]") {
			recoveryPrompts = append(recoveryPrompts, prompt)
		}
	})
	e.Process("Quiero crear un programa Go llamado f10_repair")
	for i, prompt := range recoveryPrompts {
		t.Logf("LLM_RECOVERY_PROMPT_%d_BEGIN\n%s\nLLM_RECOVERY_PROMPT_%d_END", i+1, prompt, i+1)
	}
	work, ok := e.workStore.Last()
	if !ok || work.LastActionResult == nil || !work.LastActionResult.Success || !work.LastActionResult.Verified {
		t.Fatalf("expected successfully verified production, got work=%+v ok=%t", work, ok)
	}
	root := work.LastActionResult.Project.Path
	if strings.TrimSpace(root) == "" {
		t.Fatal("verified production has no project path")
	}
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("verified Go project is missing go.mod at %q: %v", root, err)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("cannot inspect generated project %q: %v", root, err)
	}
	hasGoTest := false
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), "_test.go") {
			hasGoTest = true
			break
		}
	}
	if !hasGoTest {
		t.Fatalf("verified Go project contains no top-level _test.go file: %q", root)
	}
}

// TestF10IntegratedProductionThenAmbiguousCorrection verifies the full sequence
// against real production: generate and verify a Go project, then receive an
// ambiguous correction without inventing a replacement or corrupting prior work.
func TestF10IntegratedProductionThenAmbiguousCorrection(t *testing.T) {
	if os.Getenv("AHMELIA_RUN_OLLAMA_E2E") != "1" {
		t.Skip("set AHMELIA_RUN_OLLAMA_E2E=1 to run the integrated Ollama E2E")
	}
	e := newF10Engine(t)
	e.Process("Quiero crear un programa Go llamado f10_repair")
	before, ok := e.workStore.Last()
	if !ok || before.LastActionResult == nil || !before.LastActionResult.Success || !before.LastActionResult.Verified {
		t.Fatalf("production precondition is not verified: work=%+v ok=%t", before, ok)
	}
	workID := before.ID
	productionID := before.LastActionResult.ProductionID
	projectPath := before.LastActionResult.Project.Path

	trace := f10Trace(t, e, "Eso no era lo que quería.")
	if e.lastConversationalCognition.Repair.Required {
		t.Fatalf("ambiguous correction activated repair without a concrete replacement: %+v", e.lastConversationalCognition.Repair)
	}
	assertNoTraceExecutionAcceptance(t, trace)
	requireTraceStages(t, trace, "COGNITION", "RESPONSE", "LANGUAGE")
	after, ok := e.workStore.Get(workID)
	if !ok || after.ID != workID || after.LastActionResult == nil || !after.LastActionResult.Success || !after.LastActionResult.Verified {
		t.Fatalf("ambiguous correction changed or invalidated verified work: work=%+v ok=%t", after, ok)
	}
	if after.LastActionResult.ProductionID != productionID || after.LastActionResult.Project.Path != projectPath {
		t.Fatalf("ambiguous correction replaced production identity: before=(%q,%q) after=(%q,%q)", productionID, projectPath, after.LastActionResult.ProductionID, after.LastActionResult.Project.Path)
	}
}

func seedVerifiedF10Project(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		"go.mod":       "module ahmelia.local/project-f10_repair\n\ngo 1.20\n",
		"main.go":      "package main\nfunc Add(a, b int) int { return a + b }\nfunc main() {}\n",
		"main_test.go": "package main\nimport \"testing\"\nfunc TestAdd(t *testing.T) { if got := Add(2, 3); got != 5 { t.Fatalf(\"got %d\", got) } }\n",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o600); err != nil {
			t.Fatalf("write verified fixture %s: %v", name, err)
		}
	}
	cmd := exec.Command("go", "test", "./...")
	cmd.Dir = root
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("verified fixture project does not pass go test: %v\n%s", err, output)
	}
	return root
}

func TestF10AdversarialMixedSequenceIsStateSafe(t *testing.T) {
	e := newF10Engine(t)
	var responses []string
	e.SetResponseCallback(func(msg string) { responses = append(responses, msg) })

	inputs := []string{
		"Quiero crear un programa Go llamado f10_mix",
		"¿Qué sabes del proyecto?",
		"Continúa pero cambia el objetivo.",
		"Eso no era lo que quería",
		"Espera, no hagas eso todavía.",
		"¿Y por qué decidiste eso?",
	}
	for _, input := range inputs {
		f10Trace(t, e, input)
	}
	if len(responses) < len(inputs) {
		t.Fatalf("mixed adversarial sequence lost turns: got=%d want>=%d", len(responses), len(inputs))
	}
	work, ok := e.workStore.Last()
	if !ok {
		t.Fatal("mixed adversarial sequence lost operational ownership")
	}
	if work.ID == "" {
		t.Fatal("mixed adversarial work has no stable owner")
	}
	if snap := e.conversationState.Snapshot(); snap.ActiveWorkID != work.ID {
		t.Fatalf("conversation/work ownership diverged after mixed sequence: snapshot=%+v work=%s", snap, work.ID)
	}
	if e.lastConversationalCognition.Repair.Required {
		t.Fatalf("mixed sequence left an executable repair from an ambiguous correction: %+v", e.lastConversationalCognition.Repair)
	}
}

func TestF10InvariantAuthorityAndBoundaryStaticAudit(t *testing.T) {
	// I6/I4/I5: physical execution remains behind one Engine authority.
	engineSrc := mustReadF10(t, "engine.go")
	turnSrc := mustReadF10(t, "turn_pipeline.go")
	if strings.Contains(turnSrc, "productionRuntime.ExecuteFromAuthority(") || strings.Contains(turnSrc, "productionRuntime.RepairFromAuthority(") {
		t.Fatal("turn pipeline crossed the concrete production authority boundary directly")
	}
	if !strings.Contains(engineSrc, "e.productionRuntime.ExecuteFromAuthority(") || !strings.Contains(engineSrc, "e.productionRuntime.RepairFromAuthority(") {
		t.Fatal("central authority no longer owns production entry points")
	}
	for _, root := range []string{"../memory", "../memory_intelligence", "../learning", "../knowledge", "../situation", "../contextintelligence", "../interaction"} {
		for _, path := range mustGoFilesUnderF10(t, root) {
			src := mustReadF10(t, path)
			if strings.Contains(src, ".execute(") && !strings.Contains(path, "_test.go") {
				t.Fatalf("evidence/memory boundary %s contains physical execute call", path)
			}
		}
	}

	// I3: language/linguistics do not own capability or execution authority.
	for _, root := range []string{"../language", "../linguistics"} {
		for _, path := range mustGoFilesUnderF10(t, root) {
			src := mustReadF10(t, path)
			for _, forbidden := range []string{"authorityExecute", "ExecuteFromAuthority", "RepairFromAuthority", "ResolveCapabilityInteraction"} {
				if strings.Contains(src, forbidden) {
					t.Fatalf("language boundary %s contains authority symbol %q", path, forbidden)
				}
			}
		}
	}

	// I1/I2/I7: Process remains orchestration, while normal dialogue is expected
	// to converge through cognition -> dialogue -> response -> language.
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "engine.go", engineSrc, 0)
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
		t.Fatal("Engine.Process AST not found")
	}
	forbidden := map[string]bool{"say": true, "sayAsync": true, "authorityExecute": true, "ResolveCapabilityInteraction": true, "renderResponseContract": true}
	ast.Inspect(process.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if sel, ok := call.Fun.(*ast.SelectorExpr); ok && forbidden[sel.Sel.Name] {
			t.Errorf("I1/I7 violation: Process directly calls terminal/authority function %s", sel.Sel.Name)
		}
		return true
	})

	// I8: the structured production result crosses the response-contract boundary.
	productionSrc := mustReadF10(t, "production_result.go")
	if !strings.Contains(productionSrc, "production_action_result") || !strings.Contains(productionSrc, "renderResponseContractWithStrategy") {
		t.Fatal("production result path lacks canonical response-contract handoff")
	}

	// Bind the static audit to a live flow trace rather than relying on source text alone.
	e := newF10Engine(t)
	trace := f10Trace(t, e, "¿Qué sabes del proyecto?")
	requireTraceStages(t, trace, "COGNITION", "DIALOGUE", "RESPONSE", "LANGUAGE")
}

func mustReadF10(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func mustGoFilesUnderF10(t *testing.T, root string) []string {
	t.Helper()
	var out []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || filepath.Ext(path) != ".go" {
			return nil
		}
		out = append(out, path)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}
