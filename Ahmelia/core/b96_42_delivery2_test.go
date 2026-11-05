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
	"ahmelia/production"
	"ahmelia/situation"
)

// B96.42.3 — Result, Observation and Verification must remain three distinct
// epistemic records even when they originate from one ProviderResult.
func TestB96423ActionOutcomeConvergesDistinctEpistemicStages(t *testing.T) {
	e := newB963334AuditEngine(t)
	e.convergeActionOutcome(interaction.SemanticRequest{
		Action: interaction.ActionIntent{Name: "crear_carpeta"},
	}, production.ActionResult{
		Success:            true,
		Observed:           true,
		Verified:           true,
		Producer:           "test.provider",
		ObservationDetails: "la carpeta existe en el entorno",
		VerificationReason: "el estado del entorno coincide con la postcondición esperada",
		VerificationSource: "environment",
		OutcomeCode:        "completed",
	})

	if len(e.pendingSituationObservations) != 3 {
		t.Fatalf("expected 3 independent outcome records, got %d: %#v", len(e.pendingSituationObservations), e.pendingSituationObservations)
	}

	kinds := map[string]situation.Observation{}
	for _, obs := range e.pendingSituationObservations {
		kinds[obs.Kind] = obs
	}
	for _, kind := range []string{"execution_result", "execution_observation", "execution_verification"} {
		if _, ok := kinds[kind]; !ok {
			t.Fatalf("missing converged stage %q: %#v", kind, kinds)
		}
	}
	if kinds["execution_result"].Epistemic != situation.EpistemicObserved {
		t.Fatalf("result record lost its provider evidence status: %+v", kinds["execution_result"])
	}
	if kinds["execution_verification"].Epistemic != situation.EpistemicDerived {
		t.Fatalf("verification must remain a derived verification claim: %+v", kinds["execution_verification"])
	}
	if !strings.Contains(kinds["execution_verification"].Object, "verified=true") {
		t.Fatalf("verification state not preserved: %+v", kinds["execution_verification"])
	}
}

func TestB96423SituationConsumesExecutionOutcomeOnNextConvergence(t *testing.T) {
	e := newB963334AuditEngine(t)
	e.convergeActionOutcome(interaction.SemanticRequest{
		Action: interaction.ActionIntent{Name: "abrir_programa"},
	}, production.ActionResult{
		Success:       false,
		Producer:      "core.local_action",
		FailureReason: "program_not_found",
		OutcomeCode:   "execution_failed",
	})

	e.situationPhase(&processTurn{semanticReq: interaction.SemanticRequest{Raw: "seguimos", Action: interaction.ActionIntent{Name: "continuar"}}})
	foundResult := false
	foundVerification := false
	for _, evidence := range e.lastSituation.Evidence {
		switch evidence.ID {
		default:
			if evidence.Predicate == "result" && evidence.Value == "success=false" {
				foundResult = true
			}
			if evidence.Predicate == "verification" && evidence.Object == "verified=false" {
				foundVerification = true
			}
		}
	}
	if !foundResult || !foundVerification {
		t.Fatalf("execution outcome did not enter the next Situation build: result=%t verification=%t evidence=%#v", foundResult, foundVerification, e.lastSituation.Evidence)
	}
	if len(e.pendingSituationObservations) != 0 {
		t.Fatalf("Situation must consume the pending outcome records: %#v", e.pendingSituationObservations)
	}
}

// B96.42.4 — all low-level speech emission must converge through the response
// orchestrator. This guards the class boundary, not individual messages.
func TestB96424OnlyResponseOrchestratorMayEmitRenderedSpeech(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") || name == "response_orchestrator.go" {
			continue
		}
		src, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		file, err := parser.ParseFile(fset, name, src, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			selector, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || selector.Sel == nil || (selector.Sel.Name != "say" && selector.Sel.Name != "sayAsync") {
				return true
			}
			if ident, ok := selector.X.(*ast.Ident); ok && ident.Name == "e" {
				t.Errorf("direct rendered speech remains outside ResponseOrchestrator: %s:%d", name, fset.Position(call.Pos()).Line)
			}
			return true
		})
	}
}

func TestB96424DeterministicSystemResponsePreservesExactGroundedText(t *testing.T) {
	e := newB963334AuditEngine(t)
	var responses []string
	e.SetResponseCallback(func(text string) { responses = append(responses, text) })
	req := interaction.SemanticRequest{Raw: "abre eso", Objective: interaction.Objective{Text: "abrir"}}
	if !e.renderCanonicalSystemResponse(req, "test_clarification", "¿A qué te refieres exactamente?", interaction.MoveClarify, interaction.ResponseIntentClarify, true) {
		t.Fatal("canonical system response did not render")
	}
	if len(responses) != 1 || responses[0] != "¿A qué te refieres exactamente?" {
		t.Fatalf("deterministic system response was reinterpreted by language: %#v", responses)
	}
}
