package integration

import (
	"context"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"ahmelia/autonomy"
	"ahmelia/capability"
	"ahmelia/environment"
	"ahmelia/interaction"
	"ahmelia/production"
	"ahmelia/programming"
)

// adaptiveBehavioralSynthesizer models the boundary where the configured LLM
// would supply the next code proposal. The first proposal is deliberately bad;
// once Ahmelia observes the real build failure, the second proposal repairs the
// same project instead of creating a new one.
type adaptiveBehavioralSynthesizer struct {
	mu    sync.Mutex
	calls int
}

func (s *adaptiveBehavioralSynthesizer) Synthesize(_ context.Context, objective programming.CodeObjective) (*programming.MultiFilePlan, error) {
	s.mu.Lock()
	s.calls++
	call := s.calls
	s.mu.Unlock()

	source := "package main\n\nfunc Add(a, b int) int { return a + b }\n\nfunc main() {}\n"
	if call == 1 {
		source = "package main\n\nfunc Add(a, b int) int { return a + }\n\nfunc main() {}\n"
	}

	// On retry the objective must contain the already materialized project.
	// That proves recovery is repairing the observed artifact, not silently
	// starting a second project.
	if call == 2 && !objective.Existing {
		panic("adaptive synthesizer: retry did not observe existing project")
	}
	if call == 2 && objective.RecoveryContext == nil {
		panic("adaptive synthesizer: retry did not receive structured recovery context")
	}
	if call == 2 && len(objective.RecoveryContext.Evidence) == 0 {
		panic("adaptive synthesizer: structured recovery context lost domain evidence")
	}
	if call == 2 && !strings.Contains(string(objective.RecoveryContext.Evidence[0].Payload), "f10") && objective.ProjectID == "f10_repair" {
		// The behavioral test has a different project ID; keep this assertion out of
		// the generic adaptive fixture while still requiring non-empty structured payload.
	}
	if call == 2 && len(objective.RecoveryContext.Evidence[0].Payload) == 0 {
		panic("adaptive synthesizer: structured recovery payload is empty")
	}

	op := programming.SourceCreate
	if objective.Existing {
		op = programming.SourceWrite
	}
	files := []programming.MultiFileChange{
		{ID: "main", Operation: op, File: programming.SourceFile{Path: "main.go", Language: "Go", Role: programming.RoleEntrypoint, ProjectID: objective.ProjectID}, Content: source},
		{ID: "test", Operation: op, File: programming.SourceFile{Path: "main_test.go", Language: "Go", Role: programming.RoleTest, ProjectID: objective.ProjectID}, Content: "package main\n\nimport \"testing\"\n\nfunc TestAdd(t *testing.T) { if Add(2, 3) != 5 { t.Fatal(\"unexpected result\") } }\n"},
	}
	if call == 1 {
		files = append(files, programming.MultiFileChange{ID: "mod", Operation: programming.SourceCreate, File: programming.SourceFile{Path: "go.mod", Language: "Go", Role: programming.RoleConfig, ProjectID: objective.ProjectID}, Content: "module recoverybehavior\n\ngo 1.20\n"})
	}
	return &programming.MultiFilePlan{
		ID:      "adaptive-" + objective.ProjectID,
		Project: programming.CodeProject{ID: objective.ProjectID, Name: objective.ProjectID, Root: objective.ProjectRoot, Language: "Go", SourceRoots: []string{"."}, TestRoots: []string{"."}, BuildCommand: "go build ./...", TestCommand: "go test ./...", RuntimeCommand: "go run ."},
		Changes: files,
	}, nil
}

func TestProgrammingAgentRecoversObservedBadCodeAndVerifiesRepair(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("AHMELIA_WORKSPACE_ROOT", filepath.Join(home, "workspace"))

	world := environment.NewManager()
	synthesizer := &adaptiveBehavioralSynthesizer{}
	provider, err := NewProgrammingProviderWithSynthesizer(world, synthesizer)
	if err != nil {
		t.Fatal(err)
	}

	reg := capability.NewRegistry()
	cap := capability.Capability{ID: "programming.create", Domain: capability.DomainLocal, Action: "create", Description: "crear programa", Permission: capability.PermissionAutomatic}
	if err := reg.Register(cap); err != nil {
		t.Fatal(err)
	}
	providers := production.NewProviderRegistry()
	if err := providers.RegisterCapabilities(provider, cap.ID); err != nil {
		t.Fatal(err)
	}
	runtime, err := NewProductionRuntime(capability.NewResolver(reg), reg, providers, &production.ProductionContext{}, production.NewWorkStore(filepath.Join(t.TempDir(), "work.json")), production.NewTechnicalRecoveryService())
	if err != nil {
		t.Fatal(err)
	}

	req := interaction.SemanticRequest{
		Raw:        "Crea un programa Go que sume dos enteros y corrige cualquier fallo hasta que compile y pase los tests.",
		Normalized: "crear programa go sumar enteros corregir fallos",
		Kind:       interaction.KindProduction,
		Objective:  interaction.Objective{Text: "Crear un programa Go que sume dos enteros y corregir cualquier fallo hasta que compile y pase los tests."},
		Language:   "es", Confidence: 0.99,
		Action: interaction.ActionIntent{Name: "create", Confidence: 0.99},
		Topic:  "programa Go", RequestedAction: "create",
		CommunicativeActs: interaction.CommunicativeActs{Primary: interaction.ActCommand, Confidence: 0.99, Evidence: []interaction.CommunicativeActEvidence{{Act: interaction.ActCommand, Confidence: 0.99, Source: "test:recovery_e2e"}}},
	}
	route := capability.NewResolver(reg).Resolve(interaction.ResolveCapabilityInteraction(req), req)

	first := runtime.ExecuteFromAuthority(context.Background(), req, route, production.ExecutionOptions{Approval: autonomy.ApprovalGranted, RecoveryAttempt: 1})
	if first.Success || first.Verified {
		t.Fatalf("bad first proposal was accepted: %+v", first)
	}
	if first.Recovery == nil || first.Recovery.Action != "retry" {
		t.Fatalf("expected retry recovery decision after observed build failure: %+v", first)
	}
	if first.FailedStage != "build" {
		t.Fatalf("expected build failure, got %q: %+v", first.FailedStage, first)
	}
	if first.WorkID == "" || first.Project.ID == "" {
		t.Fatalf("first attempt lost continuity: %+v", first)
	}

	second := runtime.ExecuteFromAuthority(context.Background(), req, route, production.ExecutionOptions{Approval: autonomy.ApprovalGranted, RecoveryAttempt: 2, WorkID: first.WorkID})
	if !second.Success || !second.Verified || !second.Observed {
		t.Fatalf("repaired project was not verified: %+v", second)
	}
	if second.Project.ID != first.Project.ID {
		t.Fatalf("repair created a different project: first=%q second=%q", first.Project.ID, second.Project.ID)
	}

	work, ok := runtime.work.Get(first.WorkID)
	if !ok || work.Status != production.WorkCompleted {
		t.Fatalf("work did not close only after verified repair: ok=%v work=%+v", ok, work)
	}
	if len(work.Attempts) != 2 {
		t.Fatalf("expected exactly two attempts, got %d", len(work.Attempts))
	}
	if work.Attempts[0].Status == "succeeded" || work.Attempts[1].Status != "succeeded" {
		t.Fatalf("unexpected attempt states: %+v", work.Attempts)
	}

	synthesizer.mu.Lock()
	calls := synthesizer.calls
	synthesizer.mu.Unlock()
	if calls != 2 {
		t.Fatalf("expected one initial synthesis plus one corrective synthesis, got %d", calls)
	}
}
