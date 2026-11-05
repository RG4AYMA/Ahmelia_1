package core

import (
	"context"
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"ahmelia/cognition"
	"ahmelia/integration"
	"ahmelia/interaction"
	"ahmelia/language"
	"ahmelia/linguistics"
	"ahmelia/programming"
)

type f9LanguageClient struct{}

func (f9LanguageClient) Available() bool { return true }

func (f9LanguageClient) GenerateContext(_ context.Context, prompt string) (string, error) {
	return f9LanguageClient{}.Generate(prompt)
}

func (f9LanguageClient) Generate(prompt string) (string, error) {
	input := f9PromptInput(prompt)
	lower := strings.ToLower(input)

	if strings.Contains(prompt, "Devuelve EXCLUSIVAMENTE JSON válido con esta forma: {\"response\":\"...\"}.") {
		switch {
		case strings.Contains(lower, "¿cómo se llamará el programa?") || strings.Contains(lower, "como se llamara el programa?"):
			if strings.Contains(strings.ToLower(prompt), "beta") {
				return `{"response":"El programa quedó registrado como beta."}`, nil
			}
			return `{"response":"El programa quedó registrado como alfa."}`, nil
		case strings.Contains(lower, "¿qué cambiaste?") || strings.Contains(lower, "que cambiaste?"):
			if strings.Contains(strings.ToLower(prompt), "nocturno") {
				return `{"response":"Apliqué la corrección y el objetivo quedó en modo nocturno."}`, nil
			}
			return `{"response":"La corrección quedó registrada."}`, nil
		case strings.Contains(lower, "¿qué habíamos decidido") || strings.Contains(lower, "que habiamos decidido") || strings.Contains(lower, "aurora"):
			return `{"response":"Decidimos usar Aurora principalmente para supervisar el estado del sistema."}`, nil
		case strings.Contains(lower, "historial relacionado con la supervisión") || strings.Contains(lower, "historial relacionado con la supervision"):
			return `{"response":"Tuvimos un problema con la supervisión. Lo solucionamos cambiando el mecanismo de actualización. Después acordamos mantener ese mecanismo. Zona horaria: UTC-05:00."}`, nil
		case strings.Contains(lower, "¿qué haces?") || strings.Contains(lower, "que haces?"):
			return `{"response":"Estoy aquí contigo."}`, nil
		case strings.Contains(lower, "civilización desconocida") || strings.Contains(lower, "civilizacion desconocida") || strings.Contains(lower, "en marte"):
			return `{"response":"No tengo evidencia suficiente para afirmar qué ocurrió."}`, nil
		case strings.Contains(lower, "no, me refería") || strings.Contains(lower, "no, me referia") || strings.Contains(lower, "cambia el enfoque"):
			return `{"response":"Entendido."}`, nil
		default:
			return `{"response":"Entendido."}`, nil
		}
	}

	// Semantic interpretation is deterministic test infrastructure. Reuse the
	// interaction layer's existing parser so the language stub can cover older
	// Engine.Process tests without depending on an external LLM server.
	r := interaction.Analyze(input, interaction.Context{})
	intent := "unknown"
	switch r.Kind {
	case interaction.KindCorrection:
		intent = "correction"
	case interaction.KindQuestion, interaction.KindSearch, interaction.KindReasoning:
		intent = "question"
	case interaction.KindLearning:
		intent = "learning"
	case interaction.KindFeedback:
		intent = "feedback"
	case interaction.KindAction, interaction.KindProduction, interaction.KindContinuation, interaction.KindStop, interaction.KindPause:
		intent = "command"
	}

	references := make([]string, 0, len(r.References))
	for _, ref := range r.References {
		if strings.TrimSpace(ref.Expression) != "" {
			references = append(references, ref.Expression)
		}
	}
	meaning := strings.TrimSpace(r.Objective.Text)
	if strings.TrimSpace(string(r.Meaning.Primary)) != "" {
		meaning = string(r.Meaning.Primary)
	}
	payload := map[string]any{
		"intent":             intent,
		"entities":           r.Entities,
		"references":         references,
		"constraints":        r.Objective.Constraints,
		"meaning":            meaning,
		"speech_act":         string(r.SpeechAct),
		"topic":              r.Topic,
		"question_type":      r.QuestionType,
		"directness":         r.Directness,
		"user_state_signals": []string{},
		"correction_target":  r.CorrectionTarget,
		"confidence":         r.Confidence,
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return `{"intent":"unknown","entities":[],"references":[],"constraints":[],"meaning":"","speech_act":"unknown","topic":"","question_type":"","directness":"","user_state_signals":[],"correction_target":"","confidence":0}`, nil
	}
	return string(encoded), nil
}
func f9PromptInput(prompt string) string {
	marker := "Entrada del usuario:"
	idx := strings.LastIndex(prompt, marker)
	if idx < 0 {
		return ""
	}
	rest := strings.TrimSpace(prompt[idx+len(marker):])
	if rest == "" {
		return ""
	}
	line := strings.TrimSpace(strings.SplitN(rest, "\n", 2)[0])
	if decoded, err := strconv.Unquote(line); err == nil {
		return decoded
	}
	return strings.Trim(line, `"`)
}

func installF9LanguageStubs(t *testing.T, e *Engine) {
	t.Helper()
	client := f9LanguageClient{}
	interpreter := linguistics.NewInterpreter(client)
	e.linguisticInterpreter = interpreter
	e.linguisticPipeline = linguistics.NewPipeline(linguistics.NewRouter(), interpreter, linguistics.NewValidator())
	e.deferredLinguistic = linguistics.NewDeferredResolver(interpreter, 16)
	e.conversationGenerator = language.NewConversationalGenerator(client)
}

// Fase 9 is intentionally behavioral. These tests exercise the real Engine
// where practical and use semantic/domain boundaries for the parts that must
// not depend on a particular language-model wording.
func newF9Engine(t *testing.T) *Engine {
	isolateTestWorkspace(t)
	t.Helper()
	t.Setenv("AHMELIA_SKIP_PROGRAM_INDEX_SCAN", "1")
	t.Setenv("AHMELIA_DISABLE_TTS", "1")

	// Production tests must never use the repository working directory as the
	// project workspace. The runtime intentionally treats the nearest Go module
	// as the active workspace, so without this explicit test boundary a test can
	// snapshot Ahmelia's own source tree into the synthesis prompt, attempt to
	// mutate repository files, and trigger expensive recovery generations.
	// Keep the workspace beneath t.TempDir so project renames during repair are
	// also isolated and removed automatically when the test finishes.
	workspaceParent := t.TempDir()
	t.Setenv("AHMELIA_WORKSPACE_ROOT", filepath.Join(workspaceParent, "workspace"))

	// F9/F10 still materialize real projects through the production boundary;
	// only their filesystem authority is isolated from the developer's machine.

	for _, path := range []string{
		interaction.UserModelPath,
		interaction.ConversationStatePath,
		"memory/conversation_history.json",
		"memory/production_work.json",
	} {
		_ = os.Remove(path)
	}
	e := buildTestEngine(t)
	installF9LanguageStubs(t, e)
	t.Cleanup(func() {
		for _, path := range []string{
			interaction.UserModelPath,
			interaction.ConversationStatePath,
			"memory/conversation_history.json",
			"memory/production_work.json",
		} {
			_ = os.Remove(path)
		}
	})
	return e
}

func newF9ProductionEngine(t *testing.T) *Engine {
	t.Helper()
	e := newF9Engine(t)
	if err := integration.RegisterRuntimeProvidersWithSynthesizer(e.capabilityRegistry, e.productionProviders, e.world, f9ProductionSynthesizer{}); err != nil {
		t.Fatalf("register deterministic F9 production fixture: %v", err)
	}
	e.planner.SetGoalTaskDecomposer(nil)
	return e
}

func TestProductionRequestWithAccentsReachesCanonicalAuthority(t *testing.T) {
	input := "Crea una función Go Par que devuelva si un entero es par, añade pruebas y explícame cómo verificaste el objetivo"
	for _, channel := range []interaction.InputChannel{interaction.ChannelText, interaction.ChannelVoice} {
		t.Run(string(channel), func(t *testing.T) {
			e := newF9ProductionEngine(t)
			var responses []string
			e.SetResponseCallback(func(response string) { responses = append(responses, response) })
			e.ProcessWithChannel(input, channel)
			work, ok := e.workStore.Last()
			if !ok || work.LastActionResult == nil {
				t.Fatalf("request did not reach production authority: work=%+v exists=%t", work, ok)
			}
			result := work.LastActionResult
			if result.Producer != "programming" || result.ObjectiveStatus != "unassessed" || result.ObjectiveContractJSON == "" {
				t.Fatalf("production result did not preserve its route/objective contract: %+v", result)
			}
			if len(responses) == 0 || strings.Contains(strings.ToLower(responses[len(responses)-1]), "no supe qué hacer") {
				t.Fatalf("request fell through to generic unresolved-action response: %v", responses)
			}
		})
	}
}

// f9ProductionSynthesizer keeps this end-to-end behavior test independent of
// external model services while making a project-name correction observable
// in the generated artifact.
type f9ProductionSynthesizer struct{}

func (f9ProductionSynthesizer) Synthesize(_ context.Context, objective programming.CodeObjective) (*programming.MultiFilePlan, error) {
	project := programming.CodeProject{ID: objective.ProjectID, Name: objective.ProjectID, Root: objective.ProjectRoot, Language: objective.Language, SourceRoots: []string{"."}, TestRoots: []string{"."}, BuildCommand: "go build ./...", TestCommand: "go test ./...", RuntimeCommand: "go run ."}
	main := "package main\n\n// objective: " + strings.ReplaceAll(strings.TrimSpace(objective.Raw), "\n", " ") + "\nfunc main() {}\n"
	mainOp, moduleOp := programming.SourceCreate, programming.SourceCreate
	if objective.Existing {
		mainOp, moduleOp = programming.SourceWrite, programming.SourceWrite
	}
	return &programming.MultiFilePlan{ID: "f9-" + objective.ProjectID, Project: project, Changes: []programming.MultiFileChange{
		{ID: "main", Operation: mainOp, File: programming.SourceFile{Path: "main.go", Language: "Go", Role: programming.RoleEntrypoint, ProjectID: objective.ProjectID}, Content: main},
		{ID: "module", Operation: moduleOp, File: programming.SourceFile{Path: "go.mod", Language: "Go", Role: programming.RoleConfig, ProjectID: objective.ProjectID}, Content: "module " + objective.ProjectID + "\n\ngo 1.20\n"},
	}}, nil
}

// newF9ConversationEngine keeps the behavioral regression suite focused on
// conversation semantics. Tests that explicitly verify production execution
// use buildTestEngine plus a deterministic provider in their own fixture.
func newF9ConversationEngine(t *testing.T) *Engine {
	t.Helper()
	e := newF9Engine(t)
	e.planner.SetGoalTaskDecomposer(nil)
	return e
}

func TestF9EngineUsesIsolatedWorkspaceRoot(t *testing.T) {
	_ = newF9Engine(t)
	root := strings.TrimSpace(os.Getenv("AHMELIA_WORKSPACE_ROOT"))
	if root == "" {
		t.Fatal("F9/F10 fixture did not configure an isolated workspace root")
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		t.Fatal(err)
	}
	absCWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	rel, err := filepath.Rel(absRoot, absCWD)
	if err != nil {
		t.Fatal(err)
	}
	if rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))) {
		t.Fatalf("F9/F10 workspace root overlaps the repository working directory: root=%q cwd=%q", absRoot, absCWD)
	}
}

func TestF9ConversationDoesNotLeakRuntimeOrUserModelIntoGreeting(t *testing.T) {
	e := newF9Engine(t)
	// Seed a user-owned fact. A greeting should describe the current
	// interaction, not manufacture an answer from UserModel evidence.
	e.userModelStore = mustEmptyF9UserModelStore(t)
	var responses []string
	e.SetResponseCallback(func(msg string) { responses = append(responses, msg) })

	e.Process("Mi color favorito es negro.")
	responses = nil
	e.Process("Hola, ¿cómo estás?")

	if len(responses) == 0 || strings.TrimSpace(responses[len(responses)-1]) == "" {
		t.Fatalf("greeting produced no response: %#v", responses)
	}
	got := strings.ToLower(responses[len(responses)-1])
	for _, leaked := range []string{"runtime", "internalstate", "internal state", "usermodel", "user model"} {
		if strings.Contains(got, leaked) {
			t.Fatalf("greeting leaked implementation/user-model detail %q: %q", leaked, got)
		}
	}
	if strings.Contains(got, "negro") {
		t.Fatalf("greeting unexpectedly answered from a user-owned preference: %q", got)
	}
}

func TestF9MemoryUsesHistoricalUserEvidence(t *testing.T) {
	e := newF9Engine(t)
	e.userModelStore = mustEmptyF9UserModelStore(t)
	var responses []string
	e.SetResponseCallback(func(msg string) { responses = append(responses, msg) })

	e.Process("Mi color favorito es negro.")
	responses = nil
	e.Process("¿Cuál es mi color favorito?")

	if len(responses) == 0 {
		t.Fatal("memory query produced no response")
	}
	if !strings.Contains(strings.ToLower(responses[len(responses)-1]), "negro") {
		t.Fatalf("historical user fact was not recovered: %#v", responses)
	}

	view, err := e.userModelStore.Read(interaction.UserModelAccessRequest{Actor: interaction.UserModelActorCognition, Mode: interaction.UserModelAccessRead})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, pref := range view.Model.Preferences {
		if strings.Contains(strings.ToLower(pref.Key), "color") && pref.Status == interaction.UserModelStatusActive && strings.EqualFold(pref.Value, "negro") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("canonical UserModel does not contain a color preference with value negro: %+v", view.Model.Preferences)
	}
}

func TestF9CorrectionChangesMeaningThroughFullEnginePipeline(t *testing.T) {
	e := newF9ProductionEngine(t)
	var responses []string
	e.SetResponseCallback(func(msg string) { responses = append(responses, msg) })
	e.Process("Quiero crear un programa Go llamado alfa")
	before, ok := e.workStore.Last()
	if !ok || before.LastActionResult == nil || !before.LastActionResult.Success || !before.LastActionResult.Verified {
		t.Fatalf("initial materialization was not successful and verified: %+v", before.LastActionResult)
	}
	oldProjectPath := before.LastActionResult.Project.Path
	workID := before.ID
	e.Process("No, me refería a beta")

	afterRepair, ok := e.workStore.Get(workID)
	if !ok || afterRepair.LastActionResult == nil {
		t.Fatalf("repair did not preserve the original work session: %+v ok=%t", afterRepair, ok)
	}
	if !afterRepair.LastActionResult.Success || !afterRepair.LastActionResult.Verified {
		t.Fatalf("repair result did not satisfy Success != Verified discipline: %+v", afterRepair.LastActionResult)
	}
	if !strings.EqualFold(afterRepair.Objective.Text, "beta") {
		t.Fatalf("repair did not update the work objective: objective=%q assessment=%+v work=%+v conversation=%+v", afterRepair.Objective.Text, e.lastConversationalCognition.Repair, afterRepair, e.conversationState.Snapshot())
	}
	if !strings.EqualFold(afterRepair.LastActionResult.Project.ID, "beta") {
		t.Fatalf("repair did not reconcile the existing artifact to beta: project=%+v", afterRepair.LastActionResult.Project)
	}
	if oldProjectPath != "" {
		if _, err := os.Stat(oldProjectPath); err == nil {
			t.Fatalf("old materialized project still exists after rename: %s", oldProjectPath)
		} else if !os.IsNotExist(err) {
			t.Fatalf("could not verify old project was removed: %v", err)
		}
	}
	if _, err := os.Stat(afterRepair.LastActionResult.Project.Path); err != nil {
		t.Fatalf("repaired project is not physically present: %v", err)
	}

	snap := e.conversationState.Snapshot()
	if len(snap.Turns) < 2 {
		t.Fatalf("expected correction to remain in conversation state, got %d turns", len(snap.Turns))
	}
	last := snap.Turns[len(snap.Turns)-1]
	if last.Relation != interaction.RelationCorrection && last.Kind != interaction.KindCorrection {
		t.Fatalf("correction did not survive Engine.Process(): relation=%q kind=%q", last.Relation, last.Kind)
	}
	if !strings.Contains(strings.ToLower(last.UserText), "beta") {
		t.Fatalf("correction turn lost replacement meaning: %q", last.UserText)
	}
	if len(responses) == 0 || strings.TrimSpace(responses[len(responses)-1]) == "" {
		t.Fatalf("correction produced no observable acknowledgement: %#v", responses)
	}
	if e.lastConversationalCognition.Action.Primary != cognition.ActionCorrect {
		t.Fatalf("cognition did not receive the corrected turn: action=%q", e.lastConversationalCognition.Action.Primary)
	}
	responses = nil
	e.Process("¿Cómo se llamará el programa?")
	if len(responses) == 0 || !strings.Contains(strings.ToLower(responses[len(responses)-1]), "beta") || strings.Contains(strings.ToLower(responses[len(responses)-1]), "alfa") {
		t.Fatalf("updated program name was not applied to the later query: %#v", responses)
	}
}

func TestF9CorrectionWithoutMaterializedWorkDoesNotEnterRepairPath(t *testing.T) {
	e := newF9ConversationEngine(t)
	e.Process("Mi color favorito es negro.")
	e.Process("No, me refería a azul.")
	if e.lastConversationalCognition.Repair.Required {
		t.Fatalf("ordinary conversational correction incorrectly entered repair path: %+v", e.lastConversationalCognition.Repair)
	}
	if _, ok := e.workStore.Last(); ok {
		t.Fatal("ordinary conversational correction unexpectedly created operational work")
	}
}

func TestF9UnrelatedCorrectionWithMaterializedWorkDoesNotEnterRepairPath(t *testing.T) {
	e := newF9ProductionEngine(t)
	e.Process("Quiero crear un programa Go llamado alfa")
	if _, ok := e.workStore.Last(); !ok {
		t.Fatal("expected materialized operational work")
	}
	e.Process("Mi color favorito es negro.")
	e.Process("No, me refería a azul.")
	if e.lastConversationalCognition.Repair.Required {
		t.Fatalf("unrelated correction incorrectly triggered repair of active work: %+v", e.lastConversationalCognition.Repair)
	}
}

func TestF9RepairRequiresVerifiedMaterializedResult(t *testing.T) {
	e := newF9ProductionEngine(t)
	e.Process("Quiero crear un programa Go llamado alfa")
	work, ok := e.workStore.Last()
	if !ok || work.LastActionResult == nil {
		t.Fatal("expected a materialized work result")
	}
	unverified := *work.LastActionResult
	unverified.Verified = false
	if err := e.workStore.RecordActionResult(work.ID, unverified); err != nil {
		t.Fatal(err)
	}
	e.Process("No, me refería a beta")
	if e.lastConversationalCognition.Repair.Required {
		t.Fatalf("repair activated from an unverified prior result: %+v", e.lastConversationalCognition.Repair)
	}
}

func TestF9ContextRecoversActiveWorkThroughEngineWithoutSecondOwner(t *testing.T) {
	e := newF9Engine(t)
	req := interaction.Parse("Crear una escena base para un juego de supervivencia", interaction.ChannelText)
	work, err := e.workStore.Begin(req)
	if err != nil {
		t.Fatal(err)
	}
	if err := e.workStore.AttachProduction(work.ID, "goal-test", "plan-test", "test-provider"); err != nil {
		t.Fatal(err)
	}
	if err := e.workStore.SetProgress(work.ID, "crear escena base", nil, []string{"crear enemigo", "verificar escena"}); err != nil {
		t.Fatal(err)
	}

	var responses []string
	e.SetResponseCallback(func(msg string) { responses = append(responses, msg) })
	e.Process("¿Qué estabas haciendo?")
	e.Process("Continúa.")

	last, ok := e.workStore.Last()
	if !ok || last.ID != work.ID {
		t.Fatalf("Engine.Process changed active work owner: got=%q want=%q ok=%t", last.ID, work.ID, ok)
	}
	if snap := e.conversationState.Snapshot(); snap.ActiveWorkID != work.ID {
		t.Fatalf("conversation lost active work link: %+v", snap)
	}
	if len(responses) < 2 {
		t.Fatalf("expected conversational answers for active-work recovery and continuation, got %d", len(responses))
	}
}

func TestF9ExecutionAuthorityChainRemainsSeparatedFromMemoryAndLearning(t *testing.T) {
	src, err := os.ReadFile("engine.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(src)

	if !strings.Contains(text, "func (e *Engine) authorityExecute") || !strings.Contains(text, "e.productionRuntime.ExecuteFromAuthority(") {
		t.Fatal("central execution authority boundary is missing")
	}
	if strings.Contains(text, "longMem.Execute(") || strings.Contains(text, "learningState.Execute(") {
		t.Fatal("memory/learning appears to own physical execution")
	}

	providerSrc, err := os.ReadFile("local_action_executor.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	providerFile, err := parser.ParseFile(fset, "local_action_executor.go", providerSrc, 0)
	if err != nil {
		t.Fatal(err)
	}
	var executeCallOwners []string
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
				if x, ok := sel.X.(*ast.Ident); ok && x.Name == "e" {
					executeCallOwners = append(executeCallOwners, fn.Name.Name)
				}
			}
			return true
		})
	}
	if len(executeCallOwners) != 1 || executeCallOwners[0] != "ExecuteCapabilityAction" {
		t.Fatalf("physical Engine.execute must be owned by the local capability provider: %v", executeCallOwners)
	}
	turnPipeline, err := os.ReadFile("turn_pipeline.go")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(turnPipeline), "RepairFromAuthority(") {
		t.Fatal("turn_pipeline crossed the ProductionRuntime repair boundary directly")
	}
	if !strings.Contains(text, "e.productionRuntime.RepairFromAuthority(") {
		t.Fatal("post-execution repair does not cross ProductionRuntime from central authority")
	}

	for _, marker := range []string{
		"e.cognitionPhase(t)",
		"e.dialoguePhase(t)",
		"e.decisionPhase(t)",
		"e.preExecutionPhase(t)",
		"e.executionLifecyclePhase()",
	} {
		if !strings.Contains(text, marker) {
			t.Fatalf("Process pipeline missing stage %q", marker)
		}
	}
}

func TestF9FullConversationScenarioIsEndToEndAndPreservesCorrection(t *testing.T) {
	workDir := t.TempDir()
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(workDir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWD) })

	t.Setenv("AHMELIA_DISABLE_TTS", "1")
	e := newInteractionE2EEngine(t)
	installF9LanguageStubs(t, e)
	var responses []string
	e.SetResponseCallback(func(msg string) { responses = append(responses, msg) })

	inputs := []string{
		"Quiero hacer un pequeño juego de supervivencia.",
		"Empieza creando un programa base para el juego de supervivencia.",
		"Mientras haces eso, ¿qué necesitamos para el enemigo?",
		"¿Qué estabas haciendo?",
		"Continúa.",
		"No, espera. Cambia el enfoque: quiero que sea nocturno.",
		"¿Qué cambiaste?",
	}
	for _, input := range inputs {
		e.Process(input)
	}

	if len(responses) < len(inputs) {
		t.Fatalf("full-system scenario lost observable turns: got=%d want>=%d", len(responses), len(inputs))
	}
	work, ok := e.workStore.Last()
	if !ok {
		t.Fatal("real production turn did not create a WorkSession")
	}
	if work.ID == "" {
		t.Fatal("production work has no stable owner")
	}
	if work.LastActionResult == nil || !work.LastActionResult.Success || !work.LastActionResult.Verified {
		t.Fatalf("final work does not contain a successfully verified repair result: %+v", work.LastActionResult)
	}
	if !strings.Contains(strings.ToLower(work.Objective.Text), "nocturno") {
		t.Fatalf("full scenario did not apply the corrected objective to the existing work: %q", work.Objective.Text)
	}
	if !strings.Contains(strings.ToLower(work.LastActionResult.Summary), "correcci") {
		t.Fatalf("final action result does not identify the materialized repair: %q", work.LastActionResult.Summary)
	}
	if snap := e.conversationState.Snapshot(); snap.ActiveWorkID != work.ID {
		t.Fatalf("conversation/work ownership diverged: snapshot=%+v work=%s", snap, work.ID)
	}
	lastTurn, ok := e.conversationState.LastTurn()
	if !ok || lastTurn.UserText == "" {
		t.Fatal("final conversational turn was not persisted")
	}
	correctionFound := false
	for _, turn := range e.conversationState.Snapshot().Turns {
		if turn.Relation == interaction.RelationCorrection || turn.Kind == interaction.KindCorrection {
			if strings.Contains(strings.ToLower(turn.UserText), "nocturno") {
				correctionFound = true
			}
		}
	}
	if !correctionFound {
		t.Fatal("nighttime correction was not preserved as semantic conversation evidence")
	}
	if e.lastResponseIntent.Kind != interaction.ResponseIntentAnswer && e.lastResponseIntent.Kind != interaction.ResponseIntentAcknowledge {
		t.Fatalf("final response did not converge to a conversational response intent: %q", e.lastResponseIntent.Kind)
	}
	finalResponse := strings.ToLower(responses[len(responses)-1])
	objective := strings.ToLower(work.Objective.Text)
	if !strings.Contains(finalResponse, "nocturno") && !strings.Contains(objective, "nocturno") {
		t.Fatalf("nighttime correction did not propagate into observable state or final response: response=%q objective=%q", responses[len(responses)-1], work.Objective.Text)
	}
}

func mustEmptyF9UserModelStore(t *testing.T) *interaction.UserModelStore {
	t.Helper()
	store, err := interaction.NewUserModelStore(interaction.UserModel{UserID: "default-user"})
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func TestF91GenericProductionGoalCrossesCapabilityDecisionWithoutProviderSelection(t *testing.T) {
	req := interaction.Parse("Empieza creando un programa base para el juego de supervivencia", interaction.ChannelText)
	decision := interaction.ResolveCapabilityInteraction(req)
	if !decision.Executable {
		t.Fatalf("generic production goal did not produce an executable capability decision: %+v", decision)
	}
	if decision.Request.Operation != "produce" {
		t.Fatalf("generic production operation=%q, want produce", decision.Request.Operation)
	}
	if decision.Request.Source != "interaction.production_intent" {
		t.Fatalf("generic production request source=%q", decision.Request.Source)
	}
}

func TestF91LegacySemanticFallbackCannotCreateExecutableDecision(t *testing.T) {
	src, err := os.ReadFile("turn_pipeline.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(src)
	start := strings.Index(text, "func (e *Engine) finishExecutionPhase(t *processTurn) {")
	end := start + strings.Index(text[start:], "\n}\n\n// executionLifecyclePhase")
	if start < 0 || end <= start {
		t.Fatal("finishExecutionPhase boundaries not found")
	}
	body := text[start:end]
	for _, forbidden := range []string{
		"CapabilityInteractionDecision{",
		"Executable: true",
		"legacy semantic action compatibility path",
		"e.authorityExecute(",
	} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("legacy execution decision still exists in finishExecutionPhase: %q", forbidden)
		}
	}
}

func TestF10ExplicitRecentWorkReferencesRecoverLostConversationOwner(t *testing.T) {
	cases := []string{
		"¿Qué estábamos haciendo?",
		"¿Qué hacíamos?",
		"¿Qué estábamos trabajando?",
		"¿Qué falta?",
		"¿Qué queda?",
		"What were we working on?",
	}
	for _, input := range cases {
		t.Run(input, func(t *testing.T) {
			e := newF9Engine(t)
			installF9LanguageStubs(t, e)
			req := interaction.Parse("Crear una escena base para un juego de supervivencia", interaction.ChannelText)
			work, err := e.workStore.Begin(req)
			if err != nil {
				t.Fatal(err)
			}
			if err := e.workStore.AttachProduction(work.ID, "goal-test", "plan-test", "test-provider"); err != nil {
				t.Fatal(err)
			}
			if snap := e.conversationState.Snapshot(); snap.ActiveWorkID != "" {
				t.Fatalf("fixture unexpectedly starts with an active owner: %q", snap.ActiveWorkID)
			}

			e.Process(input)

			if snap := e.conversationState.Snapshot(); snap.ActiveWorkID != work.ID {
				t.Fatalf("explicit recent-work reference failed to restore owner: input=%q active=%q want=%q", input, snap.ActiveWorkID, work.ID)
			}
			links := e.conversationState.Snapshot().WorkLinks
			found := false
			for _, link := range links {
				if link.WorkID == work.ID {
					found = true
					break
				}
			}
			if !found {
				t.Fatalf("owner was restored without recording a conversation-work link: %+v", links)
			}
		})
	}
}

func TestF10UnrelatedQuestionDoesNotAdoptHistoricalWorkAsActive(t *testing.T) {
	e := newF9Engine(t)
	installF9LanguageStubs(t, e)
	req := interaction.Parse("Crear una escena base para un juego de supervivencia", interaction.ChannelText)
	work, err := e.workStore.Begin(req)
	if err != nil {
		t.Fatal(err)
	}
	if err := e.workStore.AttachProduction(work.ID, "goal-test", "plan-test", "test-provider"); err != nil {
		t.Fatal(err)
	}

	e.Process("¿Qué tiempo hace mañana?")

	if snap := e.conversationState.Snapshot(); snap.ActiveWorkID != "" {
		t.Fatalf("unrelated question silently adopted historical work: active=%q work=%q", snap.ActiveWorkID, work.ID)
	}
}
