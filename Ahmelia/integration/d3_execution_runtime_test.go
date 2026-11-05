package integration

import (
	"context"
	"path/filepath"
	"sync/atomic"
	"testing"

	"ahmelia/autonomy"
	"ahmelia/capability"
	"ahmelia/interaction"
	"ahmelia/production"
	"ahmelia/tools"
)

type recoveryTestProvider struct{ calls atomic.Int32 }

func (p *recoveryTestProvider) ID() string { return "recovery-test-provider" }
func (p *recoveryTestProvider) Execute(_ context.Context, req production.ProviderRequest) production.ProviderResult {
	p.calls.Add(1)
	if req.CapabilityID != req.Capability.ID {
		return production.ProviderResult{Success: false, FailureReason: "provider identity incoherent"}
	}
	if req.Operation != req.Capability.Action {
		return production.ProviderResult{Success: false, FailureReason: "provider operation incoherent"}
	}
	return production.ProviderResult{Success: false, Verified: false, FailureReason: "verified failure"}
}

type fixedRecoveryDecider struct{ action tools.RecoveryAction }

func (d fixedRecoveryDecider) Decide(context.Context, tools.Failure) tools.RecoveryAction {
	return d.action
}

func newRecoveryRuntime(t *testing.T, action tools.RecoveryAction) (*ProductionRuntime, *recoveryTestProvider, interaction.SemanticRequest, capability.Route) {
	t.Helper()
	reg := capability.NewRegistry()
	c := capability.Capability{ID: "demo.recovery", Domain: capability.DomainLocal, Action: "recover_action", Description: "recovery test", Permission: capability.PermissionAutomatic}
	if err := reg.Register(c); err != nil {
		t.Fatal(err)
	}
	provider := &recoveryTestProvider{}
	providers := production.NewProviderRegistry()
	if err := providers.RegisterCapabilities(provider, c.ID); err != nil {
		t.Fatal(err)
	}
	recovery := production.NewTechnicalRecoveryServiceWithDecider(fixedRecoveryDecider{action: action})
	runtime, err := NewProductionRuntime(capability.NewResolver(reg), reg, providers, &production.ProductionContext{}, production.NewWorkStore(filepath.Join(t.TempDir(), "work.json")), recovery)
	if err != nil {
		t.Fatal(err)
	}
	req := productionTestRequest()
	req.Action.Name = c.Action
	req.RequestedAction = c.Action
	req.Topic = "recovery"
	decision := interaction.ResolveCapabilityInteraction(req)
	route := capability.NewResolver(reg).Resolve(decision, req)
	return runtime, provider, req, route
}

func TestProductionRuntimePermissionAskFirstFailsClosedBeforeProvider(t *testing.T) {
	reg := capability.NewRegistry()
	c := capability.Capability{ID: "demo.ask", Domain: capability.DomainLocal, Action: "ask_action", Description: "ask first", Permission: capability.PermissionAskFirst}
	if err := reg.Register(c); err != nil {
		t.Fatal(err)
	}
	provider := &recoveryTestProvider{}
	providers := production.NewProviderRegistry()
	if err := providers.RegisterCapabilities(provider, c.ID); err != nil {
		t.Fatal(err)
	}
	runtime, err := NewProductionRuntime(capability.NewResolver(reg), reg, providers, &production.ProductionContext{}, production.NewWorkStore(filepath.Join(t.TempDir(), "work.json")), production.NewTechnicalRecoveryService())
	if err != nil {
		t.Fatal(err)
	}
	req := productionTestRequest()
	req.Action.Name = c.Action
	req.RequestedAction = c.Action
	decision := interaction.ResolveCapabilityInteraction(req)
	route := capability.NewResolver(reg).Resolve(decision, req)
	result := runtime.ExecuteFromAuthority(context.Background(), req, route, production.ExecutionOptions{Approval: autonomy.ApprovalNone})
	if result.Success || !result.AuthorizationRequired {
		t.Fatalf("expected fail-closed authorization handoff, got %+v", result)
	}
	if result.AuthorizationCapabilityID != c.ID {
		t.Fatalf("authorization capability=%q want %q", result.AuthorizationCapabilityID, c.ID)
	}
	if provider.calls.Load() != 0 {
		t.Fatalf("provider was invoked despite missing authorization: calls=%d", provider.calls.Load())
	}
}

func TestProductionRuntimeRecoveryDecisionRemainsWithOwner(t *testing.T) {
	for _, action := range []tools.RecoveryAction{tools.RecoveryRetry, tools.RecoveryReplan, tools.RecoveryBlock, tools.RecoveryAskUser} {
		t.Run(string(action), func(t *testing.T) {
			runtime, provider, req, route := newRecoveryRuntime(t, action)
			result := runtime.ExecuteFromAuthority(context.Background(), req, route, production.ExecutionOptions{Approval: autonomy.ApprovalGranted, RecoveryAttempt: 1})
			if result.Success {
				t.Fatalf("expected provider failure, got %+v", result)
			}
			if result.Recovery == nil || result.Recovery.Action != action {
				t.Fatalf("runtime did not return canonical recovery decision: %+v", result)
			}
			if provider.calls.Load() != 1 {
				t.Fatalf("ProductionRuntime applied recovery itself; provider calls=%d", provider.calls.Load())
			}
			if result.WorkID == "" {
				t.Fatal("failed production attempt must remain attributable to a WorkStore record")
			}
		})
	}
}

func TestProductionRuntimeUsesOwnerApprovalResolverAsAuthoritativeEvidence(t *testing.T) {
	reg := capability.NewRegistry()
	c := capability.Capability{ID: "demo.owner-approval", Domain: capability.DomainLocal, Action: "ask_action", Description: "owner approval", Permission: capability.PermissionAskFirst}
	if err := reg.Register(c); err != nil {
		t.Fatal(err)
	}
	provider := &recoveryTestProvider{}
	providers := production.NewProviderRegistry()
	if err := providers.RegisterCapabilities(provider, c.ID); err != nil {
		t.Fatal(err)
	}
	runtime, err := NewProductionRuntime(capability.NewResolver(reg), reg, providers, &production.ProductionContext{}, production.NewWorkStore(filepath.Join(t.TempDir(), "work.json")), production.NewTechnicalRecoveryService())
	if err != nil {
		t.Fatal(err)
	}
	req := productionTestRequest()
	req.Action.Name = c.Action
	req.RequestedAction = c.Action
	decision := interaction.ResolveCapabilityInteraction(req)
	route := capability.NewResolver(reg).Resolve(decision, req)

	blocked := runtime.ExecuteFromAuthority(context.Background(), req, route, production.ExecutionOptions{
		Approval: autonomy.ApprovalGranted,
		ApprovalResolver: func(capability.ID) autonomy.Approval {
			return autonomy.ApprovalNone
		},
	})
	if blocked.Success || !blocked.AuthorizationRequired {
		t.Fatalf("owner denial must override caller-local approval evidence: %+v", blocked)
	}
	if provider.calls.Load() != 0 {
		t.Fatalf("provider executed despite owner withholding approval: calls=%d", provider.calls.Load())
	}
}

func TestProductionRuntimeProviderBoundaryCarriesCoherentCapabilityIdentity(t *testing.T) {
	runtime, provider, req, route := newRecoveryRuntime(t, tools.RecoveryBlock)
	result := runtime.ExecuteFromAuthority(context.Background(), req, route, production.ExecutionOptions{Approval: autonomy.ApprovalGranted, RecoveryAttempt: 1})
	if result.Recovery == nil {
		t.Fatalf("expected failure recovery: %+v", result)
	}
	if provider.calls.Load() != 1 {
		t.Fatalf("provider call count=%d", provider.calls.Load())
	}
}

type authorizationResumeProvider struct{ calls atomic.Int32 }

func (p *authorizationResumeProvider) ID() string { return "authorization-resume-provider" }
func (p *authorizationResumeProvider) Execute(_ context.Context, req production.ProviderRequest) production.ProviderResult {
	p.calls.Add(1)
	return production.ProviderResult{Success: true, Verified: true, Observed: true, Summary: "authorized execution completed", Producer: p.ID()}
}

func TestProductionRuntimeAuthorizationContinuationReusesWorkAndAttempt(t *testing.T) {
	reg := capability.NewRegistry()
	c := capability.Capability{ID: "demo.authorization-resume", Domain: capability.DomainLocal, Action: "protected_action", Description: "authorization continuation", Permission: capability.PermissionAskFirst}
	if err := reg.Register(c); err != nil {
		t.Fatal(err)
	}
	provider := &authorizationResumeProvider{}
	providers := production.NewProviderRegistry()
	if err := providers.RegisterCapabilities(provider, c.ID); err != nil {
		t.Fatal(err)
	}
	work := production.NewWorkStore(filepath.Join(t.TempDir(), "work.json"))
	runtime, err := NewProductionRuntime(capability.NewResolver(reg), reg, providers, &production.ProductionContext{}, work, production.NewTechnicalRecoveryService())
	if err != nil {
		t.Fatal(err)
	}
	req := productionTestRequest()
	req.Action.Name = c.Action
	req.RequestedAction = c.Action
	decision := interaction.ResolveCapabilityInteraction(req)
	route := capability.NewResolver(reg).Resolve(decision, req)

	blocked := runtime.ExecuteFromAuthority(context.Background(), req, route, production.ExecutionOptions{Approval: autonomy.ApprovalNone})
	if blocked.Success || !blocked.AuthorizationRequired || blocked.WorkID == "" {
		t.Fatalf("expected persisted authorization suspension, got %+v", blocked)
	}
	pending, ok := work.PendingExecution(blocked.WorkID)
	if !ok {
		t.Fatal("expected pending execution in canonical Work")
	}
	if pending.CapabilityID != string(c.ID) || pending.AttemptNumber != 1 {
		t.Fatalf("unexpected pending contract: %+v", pending)
	}
	w1, _ := work.Get(blocked.WorkID)
	if len(w1.Attempts) != 1 || w1.Attempts[0].Status != "awaiting_authorization" {
		t.Fatalf("unexpected attempt state: %+v", w1.Attempts)
	}

	resumed := runtime.ExecuteFromAuthority(context.Background(), pending.Request, route, production.ExecutionOptions{
		Approval: autonomy.ApprovalGranted, WorkID: blocked.WorkID, AttemptNumber: pending.AttemptNumber,
		ResumePendingAuthorization: true, ExpectedCapabilityID: capability.ID(pending.CapabilityID),
		ExpectedOperation: pending.Operation, ExpectedProviderID: pending.ProviderID, ExpectedRouteKind: capability.IntentKind(pending.RouteKind),
	})
	if !resumed.Success || !resumed.Verified {
		t.Fatalf("expected resumed verified execution, got %+v", resumed)
	}
	if resumed.WorkID != blocked.WorkID {
		t.Fatalf("resume created a different Work: first=%s second=%s", blocked.WorkID, resumed.WorkID)
	}
	if provider.calls.Load() != 1 {
		t.Fatalf("provider calls=%d want 1", provider.calls.Load())
	}
	w2, _ := work.Get(blocked.WorkID)
	if len(w2.Attempts) != 1 {
		t.Fatalf("resume created a second attempt: %+v", w2.Attempts)
	}
	if w2.Attempts[0].Status != "succeeded" {
		t.Fatalf("attempt status=%q want succeeded", w2.Attempts[0].Status)
	}
	if w2.PendingExecution != nil {
		t.Fatal("pending authorization was not cleared after successful continuation")
	}
}

func TestProductionRuntimeAuthorizationResumeRejectsProviderDriftBeforeEffect(t *testing.T) {
	reg := capability.NewRegistry()
	c := capability.Capability{ID: "demo.authorization-provider-drift", Domain: capability.DomainLocal, Action: "protected_action", Description: "provider drift", Permission: capability.PermissionAskFirst}
	if err := reg.Register(c); err != nil {
		t.Fatal(err)
	}
	provider := &authorizationResumeProvider{}
	providers := production.NewProviderRegistry()
	if err := providers.RegisterCapabilities(provider, c.ID); err != nil {
		t.Fatal(err)
	}
	work := production.NewWorkStore(filepath.Join(t.TempDir(), "work.json"))
	runtime, err := NewProductionRuntime(capability.NewResolver(reg), reg, providers, &production.ProductionContext{}, work, production.NewTechnicalRecoveryService())
	if err != nil {
		t.Fatal(err)
	}
	req := productionTestRequest()
	req.Action.Name, req.RequestedAction = c.Action, c.Action
	decision := interaction.ResolveCapabilityInteraction(req)
	route := capability.NewResolver(reg).Resolve(decision, req)
	blocked := runtime.ExecuteFromAuthority(context.Background(), req, route, production.ExecutionOptions{Approval: autonomy.ApprovalNone})
	if !blocked.AuthorizationRequired || blocked.WorkID == "" {
		t.Fatalf("expected authorization suspension, got %+v", blocked)
	}
	pending, ok := work.PendingExecution(blocked.WorkID)
	if !ok {
		t.Fatal("missing pending execution")
	}
	resumed := runtime.ExecuteFromAuthority(context.Background(), pending.Request, route, production.ExecutionOptions{
		Approval: autonomy.ApprovalGranted, WorkID: blocked.WorkID, AttemptNumber: pending.AttemptNumber,
		ResumePendingAuthorization: true, ExpectedCapabilityID: capability.ID(pending.CapabilityID),
		ExpectedOperation: pending.Operation, ExpectedProviderID: "provider-from-old-runtime",
		ExpectedRouteKind: capability.IntentProduction,
	})
	if resumed.Success || resumed.FailureReason != "authorization_resume_identity_mismatch" {
		t.Fatalf("provider identity drift must fail closed, got %+v", resumed)
	}
	if provider.calls.Load() != 0 {
		t.Fatalf("provider effect occurred after identity drift: calls=%d", provider.calls.Load())
	}
}

type authorizationUnknownProvider struct{ calls atomic.Int32 }

func (p *authorizationUnknownProvider) ID() string { return "authorization-unknown-provider" }
func (p *authorizationUnknownProvider) Execute(_ context.Context, _ production.ProviderRequest) production.ProviderResult {
	p.calls.Add(1)
	return production.ProviderResult{Success: true, Observed: false, Verified: false, Summary: "effect may have occurred; outcome is unknown", Producer: p.ID()}
}

func TestAuthorizationConsumptionPreventsReplayWhenOutcomeRemainsUnknown(t *testing.T) {
	reg := capability.NewRegistry()
	c := capability.Capability{ID: "demo.authorization-unknown", Domain: capability.DomainLocal, Action: "protected_action", Description: "unknown outcome authorization", Permission: capability.PermissionAskFirst}
	if err := reg.Register(c); err != nil {
		t.Fatal(err)
	}
	provider := &authorizationUnknownProvider{}
	providers := production.NewProviderRegistry()
	if err := providers.RegisterCapabilities(provider, c.ID); err != nil {
		t.Fatal(err)
	}
	work := production.NewWorkStore(filepath.Join(t.TempDir(), "work.json"))
	runtime, err := NewProductionRuntime(capability.NewResolver(reg), reg, providers, &production.ProductionContext{}, work, production.NewTechnicalRecoveryService())
	if err != nil {
		t.Fatal(err)
	}
	req := productionTestRequest()
	req.Action.Name, req.RequestedAction = c.Action, c.Action
	decision := interaction.ResolveCapabilityInteraction(req)
	route := capability.NewResolver(reg).Resolve(decision, req)
	blocked := runtime.ExecuteFromAuthority(context.Background(), req, route, production.ExecutionOptions{Approval: autonomy.ApprovalNone})
	if !blocked.AuthorizationRequired || blocked.WorkID == "" {
		t.Fatalf("expected authorization suspension, got %+v", blocked)
	}
	pending, ok := work.PendingExecution(blocked.WorkID)
	if !ok {
		t.Fatal("missing pending execution")
	}
	options := production.ExecutionOptions{
		Approval: autonomy.ApprovalGranted, WorkID: blocked.WorkID, AttemptNumber: pending.AttemptNumber,
		ResumePendingAuthorization: true, ExpectedCapabilityID: capability.ID(pending.CapabilityID),
		ExpectedOperation: pending.Operation, ExpectedProviderID: pending.ProviderID, ExpectedRouteKind: capability.IntentProduction,
	}
	first := runtime.ExecuteFromAuthority(context.Background(), pending.Request, route, options)
	if first.OutcomeStatus != production.OutcomeUnknown || first.Verified {
		t.Fatalf("missing observation must remain unknown, got %+v", first)
	}
	if _, stillPending := work.PendingExecution(blocked.WorkID); stillPending {
		t.Fatal("authorization remained pending after it was consumed and execution began")
	}
	second := runtime.ExecuteFromAuthority(context.Background(), pending.Request, route, options)
	if second.Success || provider.calls.Load() != 1 {
		t.Fatalf("repeated confirmation replayed an uncertain effect: second=%+v calls=%d", second, provider.calls.Load())
	}
	stored, _ := work.Get(blocked.WorkID)
	if len(stored.Attempts) != 1 || stored.Attempts[0].Status != "unverified" {
		t.Fatalf("uncertain attempt must be durably marked unverified for reconciliation, got %+v", stored.Attempts)
	}
}
