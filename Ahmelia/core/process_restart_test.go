package core

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"ahmelia/autonomy"
	"ahmelia/capability"
	"ahmelia/integration"
	"ahmelia/interaction"
	"ahmelia/memory"
	"ahmelia/production"
	"ahmelia/programming"
	"ahmelia/selfknowledge"
)

func newRestartTestEngine() *Engine {
	sess := memory.NewSession()
	longMem := memory.LoadLongMemory()
	shortMem := LoadShortMemory()
	history := selfknowledge.NewActionHistory(20)
	registry := selfknowledge.NewModuleRegistry()
	selfQuery := selfknowledge.NewSelfQuery(history, registry, "v3.1")
	return NewEngine(sess, longMem, &shortMem, history, selfQuery)
}

type crashAfterEffectProvider struct {
	root string
}

func (p *crashAfterEffectProvider) ID() string { return "crash-provider" }

func (p *crashAfterEffectProvider) Execute(_ context.Context, request production.ProviderRequest) production.ProviderResult {
	// The real provider persists the effect specification before mutating the
	// world, so reconciliation can later observe what was attempted.
	if request.EffectRecorder == nil {
		os.Exit(78)
	}
	spec, err := json.Marshal(restartPlan(p.root))
	if err != nil {
		os.Exit(78)
	}
	if err := request.EffectRecorder(spec); err != nil {
		os.Exit(78)
	}
	writeRestartProject(p.root)
	// Crash after the effect exists and before the provider returns its result.
	os.Exit(77)
	return production.ProviderResult{}
}

func restartProjectContents() map[string]string {
	return map[string]string{
		"go.mod":       "module restart\n\ngo 1.20\n",
		"main.go":      "package main\n\nfunc Add(a, b int) int { return a + b }\n\nfunc main() {}\n",
		"main_test.go": "package main\n\nimport \"testing\"\n\nfunc TestAdd(t *testing.T) { if Add(2, 3) != 5 { t.Fatal(\"bad sum\") } }\n",
	}
}

func writeRestartProject(root string) {
	_ = os.MkdirAll(root, 0o755)
	for name, content := range restartProjectContents() {
		_ = os.WriteFile(filepath.Join(root, name), []byte(content), 0o600)
	}
}

func restartPlan(root string) programming.MultiFilePlan {
	files := restartProjectContents()
	return programming.MultiFilePlan{
		ID: "plan-restart",
		Project: programming.CodeProject{
			ID: "restart", Name: "restart", Root: root, Language: "Go",
			BuildCommand: "go build ./...", TestCommand: "go test ./...",
		},
		Changes: []programming.MultiFileChange{
			{ID: "main", Operation: programming.SourceCreate, File: programming.SourceFile{Path: "main.go", Language: "Go", Role: programming.RoleSource, ProjectID: "restart"}, Content: files["main.go"]},
			{ID: "test", Operation: programming.SourceCreate, File: programming.SourceFile{Path: "main_test.go", Language: "Go", Role: programming.RoleTest, ProjectID: "restart"}, Content: files["main_test.go"]},
		},
	}
}

// TestCrashHelperProcess is spawned as a subprocess. It crashes after the
// provider writes the effect but before the result can be finalized.
func TestCrashHelperProcess(t *testing.T) {
	if os.Getenv("AHMELIA_CRASH_HELPER") != "1" {
		t.Skip("helper only")
	}
	cwd := os.Getenv("AHMELIA_CRASH_CWD")
	project := os.Getenv("AHMELIA_CRASH_PROJECT")
	if cwd == "" || project == "" {
		t.Fatal("missing helper paths")
	}
	if err := os.Chdir(cwd); err != nil {
		t.Fatal(err)
	}
	e := newRestartTestEngine()

	registry := capability.NewRegistry()
	c := capability.Capability{
		ID: "programming.restart", Domain: capability.DomainLocal, Action: "produce",
		Description: "restart crash test", Lane: capability.LaneProduction,
		Permission: capability.PermissionAutomatic,
	}
	if err := registry.Register(c); err != nil {
		t.Fatal(err)
	}
	providers := production.NewProviderRegistry()
	if err := providers.RegisterCapabilities(&crashAfterEffectProvider{root: project}, c.ID); err != nil {
		t.Fatal(err)
	}
	runtime, err := integration.NewProductionRuntime(
		capability.NewResolver(registry), registry, providers,
		&production.ProductionContext{}, e.workStore.Raw(), production.NewTechnicalRecoveryService(),
	)
	if err != nil {
		t.Fatal(err)
	}
	e.capabilityRegistry = registry
	e.capabilityResolver = capability.NewResolver(registry)
	e.productionRuntime = runtime

	req := interaction.Analyze("crear un proyecto que sobreviva al crash", interaction.Context{})
	req.Kind = interaction.KindProduction
	req.Action = interaction.ActionIntent{Name: c.Action, Target: project, Confidence: 1}
	req.RequestedAction = c.Action
	req.CommunicativeActs = interaction.CommunicativeActs{Primary: interaction.ActCommand, Confidence: 1}
	work, err := e.workStore.Begin(req)
	if err != nil {
		t.Fatal(err)
	}
	// The attempt is created by the production runtime, as in the real path.
	// Pre-creating it here would force ResumeAttempt on an attempt that never
	// passed through authorization, which is a different state transition.
	if err := os.WriteFile(filepath.Join(cwd, "work_id.txt"), []byte(work.ID), 0o600); err != nil {
		t.Fatal(err)
	}

	decision := interaction.ResolveCapabilityInteraction(req)
	if !decision.Executable {
		t.Fatalf("decision not executable: %+v", decision)
	}
	if e.capabilityResolver != nil {
		route := e.capabilityResolver.Resolve(decision, req)
		t.Logf("decision operation=%q executable=%t route kind=%s candidates=%d score=%d", decision.Request.Operation, decision.Executable, route.Kind, len(route.Candidates), route.Score)
	}
	result := e.authorityExecuteWithOptions(decision, production.ExecutionOptions{
		Context: context.Background(), Approval: autonomy.ApprovalGranted,
		WorkID: work.ID, ExperimentalBypassCognition: true,
	}, req)
	t.Fatalf("helper did not crash: result=%+v", result)
}

func TestProcessCrashRecoveryWithRealEngine(t *testing.T) {
	t.Setenv("AHMELIA_SKIP_PROGRAM_INDEX_SCAN", "1")
	t.Setenv("AHMELIA_DISABLE_TTS", "1")
	root := t.TempDir()
	cwd := filepath.Join(root, "cwd")
	project := filepath.Join(root, "project")
	if err := os.MkdirAll(cwd, 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(os.Args[0], "-test.run=TestCrashHelperProcess", "-test.v")
	cmd.Dir = cwd
	cmd.Env = append(os.Environ(),
		"AHMELIA_CRASH_HELPER=1",
		"AHMELIA_CRASH_CWD="+cwd,
		"AHMELIA_CRASH_PROJECT="+project,
		"AHMELIA_DISABLE_TTS=1",
		"AHMELIA_SKIP_PROGRAM_INDEX_SCAN=1",
	)
	out, err := cmd.CombinedOutput()
	exitErr, ok := err.(*exec.ExitError)
	if !ok || exitErr.ExitCode() != 77 {
		t.Fatalf("helper exit=%v output=%s", err, out)
	}
	workBytes, err := os.ReadFile(filepath.Join(cwd, "work_id.txt"))
	if err != nil {
		t.Fatalf("read work id: %v", err)
	}
	workID := string(workBytes)
	if workID == "" {
		t.Fatal("empty work id")
	}

	oldDir, _ := os.Getwd()
	if err := os.Chdir(cwd); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldDir)

	restarted := newRestartTestEngine()
	reconciled, blocked := restarted.RecoverPendingWork(context.Background())
	if reconciled != 1 || blocked != 0 {
		t.Fatalf("after crash: reconciled=%d blocked=%d", reconciled, blocked)
	}
	work, ok := restarted.workStore.Get(workID)
	if !ok || work.Status != production.WorkCompleted || work.LastActionResult == nil || !work.LastActionResult.Success {
		t.Fatalf("work was not recovered: ok=%t work=%+v", ok, work)
	}
	if len(work.Attempts) != 1 {
		t.Fatalf("crash recovery must not repeat the effect: attempts=%d", len(work.Attempts))
	}
	attempt := work.Attempts[0]
	if attempt.Status != "succeeded" || len(attempt.EffectSpecification) == 0 {
		t.Fatalf("attempt was not reconciled from its durable specification: %+v", attempt)
	}
	if work.LastActionResult.VerificationSource != "programming.reconciliation" {
		t.Fatalf("result was not produced by reconciliation: %+v", work.LastActionResult)
	}
	for _, name := range []string{"main.go", "main_test.go", "go.mod"} {
		if _, err := os.Stat(filepath.Join(project, name)); err != nil {
			t.Fatalf("recovered project missing %s: %v", name, err)
		}
	}
}
