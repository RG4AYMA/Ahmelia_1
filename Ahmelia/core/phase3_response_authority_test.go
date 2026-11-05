package core

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"
	"time"

	contextintelligence "ahmelia/contextintelligence"
	"ahmelia/dialogue"
	"ahmelia/interaction"
	"ahmelia/knowledge"
	"ahmelia/solving"
)

func TestPhase3TypedSpecialistResultsKeepDomainSemantics(t *testing.T) {
	e := newB963334AuditEngine(t)

	req := interaction.Analyze("¿cuánto es 2 + 2?", interaction.Context{})
	results := e.collectResponseSpecialistResults(req, false, contextintelligence.Result{}, interaction.TurnRecord{}, false, req.Raw, "cuanto es 2 + 2")
	if results.Deterministic == nil || results.Deterministic.Value != "4" {
		t.Fatalf("expected DeterministicAnswer=4, got %#v", results.Deterministic)
	}

	recall := dialogue.HistoricalRecallResult{Found: true, Response: "record histórico"}
	results.Historical = &recall
	if source, text, _, ok := results.Primary(); !ok || source != "historical_memory" || text != "record histórico" {
		t.Fatalf("historical result lost its domain semantics: %q %q %v", source, text, ok)
	}

	results = SpecialistResponseResults{Skill: &SkillResult{Search: &solving.SkillSearchResult{Query: "go"}}}
	if source, _, _, ok := results.Primary(); !ok || source != "skill_library" {
		t.Fatalf("skill result did not preserve skill authority: source=%q ok=%v", source, ok)
	}
}

func TestPhase3MultipleSpecialistResultsRemainVisibleToCommonCognition(t *testing.T) {
	results := SpecialistResponseResults{
		Deterministic: &DeterministicAnswer{Value: "4", Source: "deterministic_answer", Confidence: 1},
		Knowledge:     &knowledge.Result{Content: "dato adicional", Kind: knowledge.KindFact, Confidence: 0.8, Source: "knowledge"},
	}
	evidence := results.ResponseEvidence()
	if len(evidence) != 2 {
		t.Fatalf("expected both typed results as evidence, got %d: %#v", len(evidence), evidence)
	}
	contributions := results.Contributions(time.Now())
	if len(contributions) != 2 {
		t.Fatalf("expected both typed results as Situation contributions, got %d", len(contributions))
	}
}

func TestPhase3DeterministicAnswerUsesCanonicalResponsePath(t *testing.T) {
	e := newB963334AuditEngine(t)
	var responses []string
	e.SetResponseCallback(func(msg string) { responses = append(responses, msg) })

	e.Process("¿cuánto es 2 + 2?")

	if len(responses) == 0 || strings.TrimSpace(responses[len(responses)-1]) != "4" {
		t.Fatalf("deterministic response was not preserved: %#v", responses)
	}
	found := false
	for _, c := range e.lastSituationContributions {
		if strings.Contains(c.Perspective, "specialist_response:deterministic_answer") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("deterministic result did not enter Situation: %#v", e.lastSituationContributions)
	}
	if e.lastConversationalCognition.Situation.ID != e.lastSituation.ID {
		t.Fatal("deterministic response bypassed shared Situation")
	}
}

func TestPhase3HistoricalRecallUsesResultBeforeInitiativeAndLanguage(t *testing.T) {
	e := newB963334AuditEngine(t)
	now := time.Now()
	e.conversationHistory = dialogue.NewConversationHistory()
	e.conversationHistory.BeginSession("previous-session", now.Add(-48*time.Hour))
	e.conversationEpisodeID = "current"
	previousEpisode := e.conversationHistory.BeginSession("previous-session-2", now.Add(-24*time.Hour))
	e.conversationHistory.RecordTurn(previousEpisode, dialogue.HistoricalTurn{
		ID:        100,
		At:        now.Add(-24 * 60 * 60 * 1e9),
		UserText:  "Decidimos mantener Aurora para supervisar el estado del sistema.",
		Assistant: "Correcto.",
		Topic:     "Aurora",
		Objective: "supervisar estado",
	})
	var responses []string
	e.SetResponseCallback(func(msg string) { responses = append(responses, msg) })

	e.Process("¿Qué habíamos decidido sobre Aurora?")

	if len(responses) == 0 || !strings.Contains(strings.ToLower(responses[len(responses)-1]), "aurora") {
		t.Fatalf("historical result did not reach canonical output: %#v", responses)
	}
	found := false
	for _, c := range e.lastSituationContributions {
		if strings.Contains(c.Perspective, "specialist_response:historical_memory") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("historical result did not enter Situation: %#v", e.lastSituationContributions)
	}
}

func TestPhase3ProcessHasNoSpecialistOwnedResponseTerminals(t *testing.T) {
	src, err := os.ReadFile("engine.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(src)
	start := strings.Index(text, "func (e *Engine) Process(input string) {")
	end := strings.Index(text[start:], "\n/* =======================\n   EXECUTE")
	if start < 0 || end < 0 {
		t.Fatal("Process boundaries not found")
	}
	process := text[start : start+end]

	forbidden := []string{
		"e.sayAsync(recall.Response)",
		"e.sayAsync(answer)",
		"e.sayAsync(p.Describe())",
		"e.sayAsync(note.Describe())",
		"e.sayAsync(search.Matches[0].Note.Describe())",
		"e.sayAsync(search.Describe())",
		"e.sayAsync(k.Response)",
		"GenerateFromKnowledge(r)",
	}
	for _, marker := range forbidden {
		if strings.Contains(process, marker) {
			t.Fatalf("Phase 3 specialist response terminal remains in Process: %q", marker)
		}
	}
	phase, err := os.ReadFile("engine.go")
	if err != nil {
		t.Fatal(err)
	}
	phaseText := string(phase)
	specialist := strings.Index(phaseText, "e.renderSpecialistResponseResults(t.semanticReq, t.responseSpecialistResults)")
	gate := strings.Index(phaseText, "if conversationalResponseEligible(t.semanticReq, e.lastResponseIntent)")
	if specialist < 0 || gate < 0 || specialist > gate {
		t.Fatal("interactive decision no longer routes specialist results before the canonical response fallback")
	}
}

func TestPhase3NoDirectSpecialistSayAsyncCallsByAST(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "engine.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}

	specialistNames := map[string]bool{
		"Describe":              true,
		"SolveQuestion":         true,
		"GenerateFromKnowledge": true,
	}
	processOnly := false
	var processBody *ast.BlockStmt
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name == nil {
			continue
		}
		if fn.Name.Name == "Process" {
			processOnly = true
			processBody = fn.Body
			break
		}
	}
	if !processOnly || processBody == nil {
		t.Fatal("Process AST not found")
	}

	ast.Inspect(processBody, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "sayAsync" {
			return true
		}
		// Direct language calls are allowed only through the canonical render
		// helpers; a specialist method must never be the argument.
		if len(call.Args) == 1 {
			if inner, ok := call.Args[0].(*ast.CallExpr); ok {
				if innerSel, ok := inner.Fun.(*ast.SelectorExpr); ok && specialistNames[innerSel.Sel.Name] {
					t.Errorf("specialist call %s feeds sayAsync directly", innerSel.Sel.Name)
				}
			}
		}
		return true
	})
}
