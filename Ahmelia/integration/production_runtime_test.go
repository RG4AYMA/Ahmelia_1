package integration

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ahmelia/autonomy"
	"ahmelia/capability"
	"ahmelia/interaction"
	"ahmelia/production"
	"ahmelia/tools"
)

type fakeProvider struct{}

func (fakeProvider) ID() string { return "fake" }
func (fakeProvider) Execute(_ context.Context, req production.ProviderRequest) production.ProviderResult {
	return production.ProviderResult{
		Success: true, Verified: true, Observed: true, Summary: "Trabajo terminado.", ProductionID: "p1", Producer: "fake",
		Project:   production.ProductionProject{ID: "p1", Name: "demo", Path: `C:\out\demo`},
		Artifacts: []production.ProductionArtifact{{ID: "a1", Name: "resultado.txt", Path: `C:\out\demo\resultado.txt`, Type: "artifact", Producer: "fake", ProjectID: "p1"}},
	}
}

func newFakeRuntime(t *testing.T) (*ProductionRuntime, *capability.Registry) {
	t.Helper()
	dir := t.TempDir()
	reg := capability.NewRegistry()
	c := capability.Capability{ID: "demo.create", Domain: capability.DomainLocal, Action: "create", Description: "crear un resultado", Permission: capability.PermissionAutomatic}
	if err := reg.Register(c); err != nil {
		t.Fatal(err)
	}
	providers := production.NewProviderRegistry()
	if err := providers.RegisterCapabilities(fakeProvider{}, c.ID); err != nil {
		t.Fatal(err)
	}
	runtime, err := NewProductionRuntime(capability.NewResolver(reg), reg, providers, &production.ProductionContext{}, production.NewWorkStore(filepath.Join(dir, "work.json")), production.NewTechnicalRecoveryService())
	if err != nil {
		t.Fatal(err)
	}
	return runtime, reg
}

func productionTestRequest() interaction.SemanticRequest {
	return interaction.SemanticRequest{
		Raw:             "Crear un resultado",
		Normalized:      "crear un resultado",
		Kind:            interaction.KindProduction,
		Objective:       interaction.Objective{Text: "Crear un resultado"},
		Language:        "es",
		Confidence:      0.95,
		Action:          interaction.ActionIntent{Name: "create", Confidence: 0.95},
		Topic:           "resultado",
		RequestedAction: "create",
		CommunicativeActs: interaction.CommunicativeActs{
			Primary:    interaction.ActCommand,
			Confidence: 0.95,
			Evidence:   []interaction.CommunicativeActEvidence{{Act: interaction.ActCommand, Confidence: 0.95, Source: "test:production_request"}},
		},
	}
}

func TestSemanticRequestIsDomainNeutral(t *testing.T) {
	r := interaction.Parse("Quiero que avances en esta escena hasta terminarla", interaction.ChannelVoice)
	if r.Kind != interaction.KindProduction {
		t.Fatalf("kind=%q", r.Kind)
	}
	if r.Objective.Text == "" {
		t.Fatal("objective empty")
	}
	if r.FollowUp {
		t.Fatal("unexpected follow-up classification")
	}
}

func TestProductionRuntimeReturnsStructuredActionResult(t *testing.T) {
	runtime, reg := newFakeRuntime(t)
	req := productionTestRequest()
	decision := interaction.ResolveCapabilityInteraction(req)
	route := capability.NewResolver(reg).Resolve(decision, req)
	result := runtime.ExecuteFromAuthority(context.Background(), req, route, production.ExecutionOptions{Approval: autonomy.ApprovalGranted, RecoveryAttempt: 1})
	if !result.Executed || !result.Success || !result.Verified || result.WorkID == "" || result.ProductionID != "p1" {
		t.Fatalf("result=%+v", result)
	}
	if result.Summary != "Trabajo terminado." {
		t.Fatalf("unexpected summary=%q", result.Summary)
	}
	if strings.Contains(result.Summary, "💬") {
		t.Fatal("ActionResult must not contain UI speech markers")
	}
}

type alternateFakeProvider struct{}

func (alternateFakeProvider) ID() string { return "alternate-instrument" }
func (alternateFakeProvider) Execute(context.Context, production.ProviderRequest) production.ProviderResult {
	return production.ProviderResult{
		Success: true, Verified: true, Observed: true, Producer: "alternate-instrument",
		Summary:      "resultado producido por el instrumento alternativo",
		ProductionID: "alternate-project",
		Project:      production.ProductionProject{ID: "alternate-project", Name: "alternate", Path: `C:\out\alternate`},
	}
}

func TestProductionRuntimeUsesExplicitInstrumentForSharedCapability(t *testing.T) {
	runtime, reg := newFakeRuntime(t)
	c, ok := reg.Get("demo.create")
	if !ok {
		t.Fatal("missing shared capability")
	}
	if err := runtime.providers.RegisterCapabilities(alternateFakeProvider{}, c.ID); err != nil {
		t.Fatal(err)
	}
	req := productionTestRequest()
	route := capability.NewResolver(reg).Resolve(interaction.ResolveCapabilityInteraction(req), req)
	blocked := runtime.ExecuteFromAuthority(context.Background(), req, route, production.ExecutionOptions{Approval: autonomy.ApprovalGranted, RecoveryAttempt: 1})
	if blocked.Success || !strings.Contains(blocked.FailureReason, "explicit provider selection required") {
		t.Fatalf("ambiguous implementation must not be selected silently: %+v", blocked)
	}
	selected := runtime.ExecuteFromAuthority(context.Background(), req, route, production.ExecutionOptions{
		Approval: autonomy.ApprovalGranted, RecoveryAttempt: 1, SelectedProviderID: "alternate-instrument",
	})
	if !selected.Success || selected.Producer != "alternate-instrument" {
		t.Fatalf("selected instrument did not produce the shared capability: %+v", selected)
	}
	work, ok := runtime.work.Get(selected.WorkID)
	if !ok || len(work.Attempts) != 1 || work.Attempts[0].Provider != "alternate-instrument" {
		t.Fatalf("selected instrument identity was not persisted with the attempt: %+v", work)
	}
}

func TestProductionRuntimePersistsAndQueriesStructuredWorkEvidence(t *testing.T) {
	runtime, reg := newFakeRuntime(t)
	req := productionTestRequest()
	decision := interaction.ResolveCapabilityInteraction(req)
	route := capability.NewResolver(reg).Resolve(decision, req)
	result := runtime.ExecuteFromAuthority(context.Background(), req, route, production.ExecutionOptions{Approval: autonomy.ApprovalGranted, RecoveryAttempt: 1})
	if !result.Success {
		t.Fatalf("production=%+v", result)
	}

	followReq := interaction.Analyze("¿Dónde está lo que acabas de crear?", interaction.Context{ActiveWorkID: result.WorkID, Language: "es"})
	query, ok := runtime.QueryWork(followReq)
	if !ok || !query.HasArtifact || query.Artifact.Name != "resultado.txt" {
		t.Fatalf("followup=%+v ok=%v", query, ok)
	}
	if query.Kind != production.WorkQueryLocation {
		t.Fatalf("query kind=%q", query.Kind)
	}
}

func TestProductionRuntimeHandlesGenericContinuationWithoutGeneratingLanguage(t *testing.T) {
	runtime, reg := newFakeRuntime(t)
	req := productionTestRequest()
	decision := interaction.ResolveCapabilityInteraction(req)
	route := capability.NewResolver(reg).Resolve(decision, req)
	result := runtime.ExecuteFromAuthority(context.Background(), req, route, production.ExecutionOptions{Approval: autonomy.ApprovalGranted, RecoveryAttempt: 1})
	if !result.Success {
		t.Fatalf("production=%+v", result)
	}
	followReq := interaction.Analyze("Continue with what you were doing.", interaction.Context{ActiveWorkID: result.WorkID, Language: "en"})
	query, ok := runtime.QueryWork(followReq)
	if !ok || query.Kind != production.WorkQueryContinuation || query.Work.ID != result.WorkID {
		t.Fatalf("continuation=%+v ok=%v", query, ok)
	}
	if strings.Contains(fmt.Sprintf("%+v", query), "💬") {
		t.Fatal("structured query must not generate conversational output")
	}
}

func TestProductionRuntimeResolvesTestResultAndActiveWorkGenerically(t *testing.T) {
	runtime, reg := newFakeRuntime(t)
	req := productionTestRequest()
	decision := interaction.ResolveCapabilityInteraction(req)
	route := capability.NewResolver(reg).Resolve(decision, req)
	result := runtime.ExecuteFromAuthority(context.Background(), req, route, production.ExecutionOptions{Approval: autonomy.ApprovalGranted, RecoveryAttempt: 1})
	if !result.Success {
		t.Fatalf("production=%+v", result)
	}
	store := runtime.work
	last, ok := store.Last()
	if !ok {
		t.Fatal("missing work")
	}
	artifact := production.ProductionArtifact{ID: "t1", Name: "main_test.go", Path: `C:\out\demo\main_test.go`, Type: "test", Producer: "fake", ProjectID: "p1"}
	if err := store.RecordArtifact(last.ID, artifact); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		input string
		kind  production.WorkQueryKind
	}{
		{"¿Y el test?", production.WorkQueryTest},
		{"¿Y qué resultado tuvo?", production.WorkQueryRecentResult},
		{"¿Qué estábamos haciendo?", production.WorkQueryActive},
	} {
		queryReq := interaction.Analyze(tc.input, interaction.Context{ActiveWorkID: last.ID, ActiveObjective: last.Objective.Text, Language: "es"})
		query, ok := runtime.QueryWork(queryReq)
		if !ok || query.Kind != tc.kind {
			t.Fatalf("%q => %+v ok=%v", tc.input, query, ok)
		}
	}
}

func TestProductionRuntimeAcceptsQuotedGreetingLiteral(t *testing.T) {
	runtime, reg := newFakeRuntime(t)
	input := `Crea un pequeño programa Go llamado prueba_contexto que muestre "Hola contexto" y añade un test.`
	req := interaction.Analyze(input, interaction.Context{Language: "es"})
	if req.Kind != interaction.KindProduction {
		t.Fatalf("semantic kind=%q, want production", req.Kind)
	}
	decision := interaction.ResolveCapabilityInteraction(req)
	route := capability.NewResolver(reg).Resolve(decision, req)
	result := runtime.ExecuteFromAuthority(context.Background(), req, route, production.ExecutionOptions{Approval: autonomy.ApprovalGranted, RecoveryAttempt: 1})
	if !result.Success || !result.Verified || !strings.Contains(result.Summary, "Trabajo terminado") {
		t.Fatalf("result=%+v", result)
	}
}

func TestProductionRuntimeDoesNotInferVerificationFromExecutionSuccess(t *testing.T) {
	runtime, reg := newFakeRuntime(t)
	// Replace the provider with one that reports successful execution but no verification.
	providers2 := production.NewProviderRegistry()
	if err := providers2.RegisterCapabilities(unverifiedProvider{}, "demo.create"); err != nil {
		t.Fatal(err)
	}
	runtime.providers = providers2
	runtime.guard, _ = production.NewExecutionGuard(reg, providers2)
	req := productionTestRequest()
	decision := interaction.ResolveCapabilityInteraction(req)
	route := capability.NewResolver(reg).Resolve(decision, req)
	result := runtime.ExecuteFromAuthority(context.Background(), req, route, production.ExecutionOptions{Approval: autonomy.ApprovalGranted, RecoveryAttempt: 1})
	if !result.Success || result.Verified || result.OutcomeStatus != production.OutcomeUnknown {
		t.Fatalf("unverified execution was not represented as unknown: %+v", result)
	}
	work, ok := runtime.work.Get(result.WorkID)
	if !ok || work.Status == production.WorkCompleted || work.Status == production.WorkFailed {
		t.Fatalf("unverified execution incorrectly closed Work: %+v", work)
	}
}

type unverifiedProvider struct{}

func (unverifiedProvider) ID() string { return "unverified" }
func (unverifiedProvider) Execute(context.Context, production.ProviderRequest) production.ProviderResult {
	return production.ProviderResult{Success: true, Verified: false, Summary: "Trabajo ejecutado, pendiente de verificación.", OutcomeCode: "completed_unverified"}
}

func TestProductionRuntimeReusesCoreOwnedWorkSession(t *testing.T) {
	runtime, reg := newFakeRuntime(t)
	req := productionTestRequest()
	decision := interaction.ResolveCapabilityInteraction(req)
	route := capability.NewResolver(reg).Resolve(decision, req)

	work, err := runtime.work.Begin(req)
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.work.AttachPlanning(work.ID, "goal-1", "plan-1"); err != nil {
		t.Fatal(err)
	}

	result := runtime.ExecuteFromAuthority(context.Background(), req, route, production.ExecutionOptions{
		Approval: autonomy.ApprovalGranted, RecoveryAttempt: 1, WorkID: work.ID,
	})
	if !result.Success || result.WorkID != work.ID {
		t.Fatalf("runtime did not reuse Core-owned work: result=%+v", result)
	}
	last, ok := runtime.work.Last()
	if !ok || last.ID != work.ID {
		t.Fatalf("unexpected last work: %+v ok=%t", last, ok)
	}
}

type retryThenSuccessProvider struct{ calls int }

func (p *retryThenSuccessProvider) ID() string { return "retry-then-success" }
func (p *retryThenSuccessProvider) Execute(_ context.Context, _ production.ProviderRequest) production.ProviderResult {
	p.calls++
	if p.calls == 1 {
		return production.ProviderResult{Success: false, Verified: false, FailedStage: "test", FailureReason: "tests failed"}
	}
	return production.ProviderResult{Success: true, Verified: true, Observed: true, Summary: "corregido y verificado", ProductionID: "p-retry", Producer: p.ID(), Project: production.ProductionProject{ID: "p-retry", Name: "demo", Path: `C:\out\retry`}}
}

func TestProductionRuntimeRetryAppendsAttemptToSameWorkOwner(t *testing.T) {
	reg := capability.NewRegistry()
	c := capability.Capability{ID: "demo.retry", SkillID: "demo.retry.skill", Domain: capability.DomainLocal, Action: "retry_action", Description: "retry test", Permission: capability.PermissionAutomatic}
	if err := reg.Register(c); err != nil {
		t.Fatal(err)
	}
	provider := &retryThenSuccessProvider{}
	providers := production.NewProviderRegistry()
	if err := providers.RegisterCapabilities(provider, c.ID); err != nil {
		t.Fatal(err)
	}
	recovery := production.NewTechnicalRecoveryServiceWithDecider(fixedRecoveryDecider{action: tools.RecoveryRetry})
	store := production.NewWorkStore(filepath.Join(t.TempDir(), "work.json"))
	runtime, err := NewProductionRuntime(capability.NewResolver(reg), reg, providers, &production.ProductionContext{}, store, recovery)
	if err != nil {
		t.Fatal(err)
	}

	req := productionTestRequest()
	req.Action.Name = c.Action
	req.RequestedAction = c.Action
	decision := interaction.ResolveCapabilityInteraction(req)
	route := capability.NewResolver(reg).Resolve(decision, req)

	first := runtime.ExecuteFromAuthority(context.Background(), req, route, production.ExecutionOptions{Approval: autonomy.ApprovalGranted, RecoveryAttempt: 1})
	if first.Success || first.Recovery == nil || first.Recovery.Action != tools.RecoveryRetry || first.WorkID == "" {
		t.Fatalf("first attempt did not produce retryable failure: %+v", first)
	}
	workID := first.WorkID
	second := runtime.ExecuteFromAuthority(context.Background(), req, route, production.ExecutionOptions{Approval: autonomy.ApprovalGranted, RecoveryAttempt: 2, WorkID: workID})
	if !second.Success || !second.Verified || second.WorkID != workID {
		t.Fatalf("second attempt did not reuse and verify same work: %+v", second)
	}
	work, ok := store.Get(workID)
	if !ok || len(work.Attempts) != 2 {
		t.Fatalf("expected two attempts in same work: %+v ok=%t", work, ok)
	}
	if work.Attempts[0].RecoveryAction != string(tools.RecoveryRetry) || work.Attempts[1].Status != "succeeded" {
		t.Fatalf("attempt chain not persisted: %+v", work.Attempts)
	}
	for i, attempt := range work.Attempts {
		if attempt.SkillID != string(c.SkillID) {
			t.Fatalf("attempt %d lost stable skill identity: got=%q want=%q", i+1, attempt.SkillID, c.SkillID)
		}
	}
	if work.Status != production.WorkCompleted {
		t.Fatalf("work must complete after verified second attempt: %q", work.Status)
	}
}

func TestProgrammingProviderDoesNotOwnRecoveryDecision(t *testing.T) {
	src, err := os.ReadFile("runtime.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(src)
	start := strings.Index(text, "type ProgrammingProvider struct")
	end := strings.Index(text, "func runProgrammingRequest(")
	if start < 0 || end <= start {
		t.Fatal("could not isolate ProgrammingProvider source")
	}
	segment := text[start:end]
	if strings.Contains(segment, "TechnicalRecoveryService") || strings.Contains(segment, ".Recover(") {
		t.Fatal("ProgrammingProvider must produce evidence, not own the recovery decision")
	}
}

type frameCaptureProvider struct{ frame interaction.ObjectiveFrame }

func (p *frameCaptureProvider) ID() string { return "frame-capture" }
func (p *frameCaptureProvider) Execute(_ context.Context, req production.ProviderRequest) production.ProviderResult {
	p.frame = req.ObjectiveFrame
	return production.ProviderResult{Success: true, Verified: true, Observed: true, Summary: "frame received", Producer: p.ID()}
}

func TestProductionRuntimeCarriesCanonicalObjectiveFrameToProvider(t *testing.T) {
	reg := capability.NewRegistry()
	capabilityDef := capability.Capability{ID: "demo.frame", Domain: capability.DomainLocal, Action: "create", Description: "frame propagation", Permission: capability.PermissionAutomatic}
	if err := reg.Register(capabilityDef); err != nil {
		t.Fatal(err)
	}
	provider := &frameCaptureProvider{}
	providers := production.NewProviderRegistry()
	if err := providers.RegisterCapabilities(provider, capabilityDef.ID); err != nil {
		t.Fatal(err)
	}
	runtime, err := NewProductionRuntime(capability.NewResolver(reg), reg, providers, &production.ProductionContext{}, production.NewWorkStore(filepath.Join(t.TempDir(), "work.json")), production.NewTechnicalRecoveryService())
	if err != nil {
		t.Fatal(err)
	}
	req := productionTestRequest()
	req.Frame = interaction.ObjectiveFrame{PrimaryGoal: "crear un resultado", PrimaryOperation: "create", RequiredActions: []string{"test"}, Verification: []string{"behavioral_result"}, Recovery: []string{"repair_and_retry"}, Workspace: "current_workspace", Confidence: .95, Provenance: "test"}
	req.Action.Name = "create"
	req.RequestedAction = "create"
	decision := interaction.ResolveCapabilityInteraction(req)
	route := capability.NewResolver(reg).Resolve(decision, req)
	result := runtime.ExecuteFromAuthority(context.Background(), req, route, production.ExecutionOptions{Approval: autonomy.ApprovalGranted, RecoveryAttempt: 1})
	if !result.Success || !result.Verified {
		t.Fatalf("execution=%+v", result)
	}
	if provider.frame.PrimaryGoal != req.Frame.PrimaryGoal || len(provider.frame.RequiredActions) != 1 || provider.frame.RequiredActions[0] != "test" || len(provider.frame.Verification) != 1 || provider.frame.Workspace != "current_workspace" {
		t.Fatalf("canonical objective frame was lost at provider boundary: got=%+v want=%+v", provider.frame, req.Frame)
	}
}

func TestProductionRuntimeDoesNotUseLastWorkAsImplicitContinuityOwner(t *testing.T) {
	runtime, _ := newFakeRuntime(t)
	firstReq := productionTestRequest()
	first, err := runtime.work.Begin(firstReq)
	if err != nil {
		t.Fatal(err)
	}
	second, err := runtime.work.Begin(interaction.SemanticRequest{Objective: interaction.Objective{Text: "otro trabajo"}})
	if err != nil {
		t.Fatal(err)
	}
	if first.ID == second.ID {
		t.Fatal("test requires distinct work IDs")
	}
	queryReq := interaction.Analyze("Continúa.", interaction.Context{})
	if _, ok := runtime.QueryWork(queryReq); ok {
		t.Fatal("continuation resolved implicitly to WorkStore.Last instead of requiring conversational ownership")
	}
}

type unknownRepairProvider struct{}

func (unknownRepairProvider) ID() string { return "unknown-repair" }
func (unknownRepairProvider) Execute(context.Context, production.ProviderRequest) production.ProviderResult {
	return production.ProviderResult{
		Success: true, OutcomeStatus: production.OutcomeSucceeded, Verified: true, Observed: true,
		Summary: "initial materialization verified", Producer: "unknown-repair", ProductionID: "repair-project",
		Project: production.ProductionProject{ID: "repair-project", Name: "repair project", Path: "test://repair-project"},
	}
}
func (unknownRepairProvider) Repair(context.Context, production.RepairIntent) production.ProviderResult {
	return production.ProviderResult{
		Success: true, OutcomeStatus: production.OutcomeUnknown, Verified: false, Observed: false,
		Summary: "repair effect may have occurred, but is not observable", OutcomeCode: "repair_unknown",
	}
}

func TestRepairUnknownOutcomeRemainsOpenAndGetsDistinctAttemptIdentity(t *testing.T) {
	dir := t.TempDir()
	reg := capability.NewRegistry()
	c := capability.Capability{ID: "demo.create", SkillID: "demo.create.skill", Domain: capability.DomainLocal, Action: "create", Description: "crear un resultado", Permission: capability.PermissionAutomatic}
	if err := reg.Register(c); err != nil {
		t.Fatal(err)
	}
	providers := production.NewProviderRegistry()
	if err := providers.RegisterCapabilities(unknownRepairProvider{}, c.ID); err != nil {
		t.Fatal(err)
	}
	store := production.NewWorkStore(filepath.Join(dir, "work.json"))
	runtime, err := NewProductionRuntime(capability.NewResolver(reg), reg, providers, &production.ProductionContext{}, store, production.NewTechnicalRecoveryService())
	if err != nil {
		t.Fatal(err)
	}
	req := productionTestRequest()
	decision := interaction.ResolveCapabilityInteraction(req)
	route := capability.NewResolver(reg).Resolve(decision, req)
	initial := runtime.ExecuteFromAuthority(context.Background(), req, route, production.ExecutionOptions{Approval: autonomy.ApprovalGranted, RecoveryAttempt: 1})
	if !initial.Success || !initial.Verified || initial.OutcomeStatus != production.OutcomeSucceeded {
		t.Fatalf("initial materialization should be verified: %+v", initial)
	}
	repair := runtime.RepairFromAuthority(context.Background(), production.RepairIntent{
		WorkID: initial.WorkID, Target: "repair-project", Replacement: "revised project", PreviousResult: initial,
	})
	if repair.OutcomeStatus != production.OutcomeUnknown || repair.Verified || repair.AttemptNumber != 2 {
		t.Fatalf("repair uncertainty/identity not preserved: %+v", repair)
	}
	work, ok := store.Get(initial.WorkID)
	if !ok {
		t.Fatal("work disappeared after uncertain repair")
	}
	if work.Status == production.WorkCompleted || work.Status == production.WorkFailed {
		t.Fatalf("uncertain repair incorrectly closed Work: status=%s", work.Status)
	}
	if len(work.Attempts) != 2 || work.Attempts[0].Status != "succeeded" || work.Attempts[1].Status != "unverified" {
		t.Fatalf("repair attempt lifecycle not preserved: %+v", work.Attempts)
	}
	for i, attempt := range work.Attempts {
		if attempt.SkillID != string(c.SkillID) {
			t.Fatalf("repair chain attempt %d lost stable skill identity: got=%q want=%q", i+1, attempt.SkillID, c.SkillID)
		}
	}
}
