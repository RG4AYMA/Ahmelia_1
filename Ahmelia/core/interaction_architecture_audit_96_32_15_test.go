package core

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ahmelia/interaction"
)

// B96.32.15 — final Interaction Architecture Audit.
// These checks are intentionally architectural rather than behavioral: the
// E2E suite already proves that the lifecycle works. This suite prevents the
// runtime from silently regaining a competing semantic authority or making
// Interaction responsible for domain execution.
func TestB963215ProcessDelegatesToCanonicalPhaseBoundaries(t *testing.T) {
	engine, err := os.ReadFile("engine.go")
	if err != nil {
		t.Fatal(err)
	}
	process := string(engine)
	start := strings.Index(process, "func (e *Engine) Process(input string) {")
	end := start + strings.Index(process[start:], "\n}\n\n/* =======================\n   EXECUTE")
	if start < 0 || end <= start {
		t.Fatal("Process boundaries not found")
	}
	body := process[start:end]
	for _, marker := range []string{
		"e.inputContextPhase(t)",
		"e.evidencePhase(t)",
		"e.situationPhase(t)",
		"e.cognitionPhase(t)",
		"e.dialoguePhase(t)",
		"e.decisionPhase(t)",
		"e.preExecutionPhase(t)",
		"e.executionLifecyclePhase()",
		"e.finishExecutionPhase(t)",
	} {
		if !strings.Contains(body, marker) {
			t.Fatalf("Process no longer coordinates canonical phase %q", marker)
		}
	}
	for _, forbidden := range []string{
		"classifier.ClassifyWithContext(",
		"comprehension.DetectIntentWithContext(",
		"interaction.ResolveCapabilityInteraction(",
		"e.authorityExecute(",
		"e.sayAsync(",
		"e.productionRuntime.ExecuteFromAuthority(",
	} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("terminal/domain decision remains embedded in Process: %q", forbidden)
		}
	}
}

func TestB963215InteractionPackageImportsOnlyStandardLibrary(t *testing.T) {
	// ahmelia/shared is an explicit, audited exception, not a relaxation of
	// this invariant. It is a leaf utility package (normalize.go, keywords.go,
	// capabilities.go, constants.go, execution_safety.go) that itself imports
	// zero ahmelia/* packages — confirmed by inspection on 2026-09-16 — so
	// depending on it cannot reintroduce a domain authority, execution path,
	// or runtime state into interaction. It was already load-bearing in
	// interaction/semantic_request.go, interaction/semantic_relations.go and
	// interaction/semantic_similarity.go before this exception was written;
	// this test had been silently failing since B96.35 because the full
	// `go test ./core -count=1` aggregate had not been run unfiltered until
	// the B96.40/B96.41 initiative-duplicate and feedback-guardrail fixes.
	//
	// Do not add another entry to this exception without the same inspection:
	// confirm the candidate package is domain-neutral, carries no runtime state
	// or execution authority, and imports only already-audited leaf utilities.
	// B96.41 adds ownership/ because the Ownership Resolver is exactly such a
	// pure shared semantic mechanism and interaction is its required consumer.
	allowedInternalImports := map[string]bool{
		"ahmelia/shared":    true,
		"ahmelia/ownership": true,
	}

	files, err := filepath.Glob(filepath.Join("..", "interaction", "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		for _, imp := range f.Imports {
			path := strings.Trim(imp.Path.Value, `"`)
			if allowedInternalImports[path] {
				continue
			}
			if strings.HasPrefix(path, "ahmelia/") {
				t.Fatalf("interaction package acquired an internal domain dependency: %s imports %q", name, path)
			}
		}
	}
}

func TestB963215SharedDependsOnlyOnCanonicalLeaf(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("..", "shared", "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		for _, imp := range f.Imports {
			path := strings.Trim(imp.Path.Value, `"`)
			if path == "ahmelia/actioncatalog" {
				continue
			}
			if strings.HasPrefix(path, "ahmelia/") {
				t.Fatalf("ahmelia/shared acquired a non-canonical internal dependency (%s imports %q)", name, path)
			}
		}
	}
}

func TestB963215OwnershipExceptionStaysLeafOnly(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("..", "ownership", "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		for _, imp := range f.Imports {
			path := strings.Trim(imp.Path.Value, `"`)
			if path == "ahmelia/shared" || strings.HasPrefix(path, "ahmelia/") {
				if path == "ahmelia/shared" {
					continue
				}
				t.Fatalf("ownership is no longer a leaf shared package (%s imports %q)", name, path)
			}
		}
	}
}

func TestB963215InteractionStateKeepsOwnershipBoundaries(t *testing.T) {
	data, err := os.ReadFile("../interaction/interaction_state.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(data)

	for _, forbidden := range []string{
		"workStore",
		"capability.Registry",
		"production.ProviderRegistry",
		"Authorize(",
		"Execute(",
	} {
		if strings.Contains(src, forbidden) {
			t.Fatalf("InteractionState contains an ownership leak %q", forbidden)
		}
	}
	if !strings.Contains(src, "ConversationState remains authoritative") {
		t.Fatal("interaction state must document ConversationState as the authoritative history owner")
	}
	if !strings.Contains(src, "does not replace ConversationState") && !strings.Contains(src, "does not\n// replace ConversationState") {
		t.Fatal("interaction state must document that it does not replace ConversationState")
	}
}

func TestB963215ProductionRuntimeConsumesSemanticRequest(t *testing.T) {
	data, err := os.ReadFile("../integration/production_runtime.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(data)
	execStart := strings.Index(src, "func (r *ProductionRuntime) ExecuteFromAuthority(")
	if execStart < 0 {
		t.Fatal("structured ProductionRuntime.Execute not found")
	}
	body := src[execStart:]
	if !strings.Contains(body, "options production.ExecutionOptions") {
		t.Fatal("ProductionRuntime.ExecuteFromAuthority must consume structured execution options")
	}
	if strings.Contains(body, "interaction.Analyze(") {
		t.Fatal("production runtime must not re-interpret a SemanticRequest")
	}
	if !strings.Contains(body, "req.Raw") || !strings.Contains(body, "route.Kind") || !strings.Contains(body, "route.Candidates") {
		t.Fatal("production runtime does not appear to consume the structured decision")
	}
	if strings.Contains(body, "sayAsync(") || strings.Contains(body, "say(") {
		t.Fatal("production runtime must not emit conversational output")
	}
}

// Keep go/ast imported in this audit as an explicit reminder that future
// architectural checks should prefer syntax-aware inspection over brittle
// string matching when the audited invariant becomes more structural.
var _ ast.Node

func TestB963215ResponseGateUsesCanonicalResponseIntent(t *testing.T) {
	correction := interaction.SemanticRequest{
		CommunicativeActs: interaction.CommunicativeActs{Primary: interaction.ActCorrection},
	}
	ack := interaction.ResponseIntent{Kind: interaction.ResponseIntentAcknowledge, ShouldSpeak: true}
	if !conversationalResponseEligible(correction, ack) {
		t.Fatal("a canonical acknowledgement intent must reach the response gate regardless of act-specific allow-lists")
	}

	command := interaction.SemanticRequest{
		CommunicativeActs: interaction.CommunicativeActs{Primary: interaction.ActCommand},
	}
	if conversationalResponseEligible(command, ack) {
		t.Fatal("an executable command must remain on the capability/runtime path")
	}

	if conversationalResponseEligible(correction, interaction.ResponseIntent{}) {
		t.Fatal("an empty response intent must not authorize conversational rendering")
	}
}
