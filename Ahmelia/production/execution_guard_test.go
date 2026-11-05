package production

import (
	"context"
	"testing"

	"ahmelia/autonomy"
	"ahmelia/capability"
)

type guardTestProvider struct{}

func (guardTestProvider) ID() string { return "guard-provider" }
func (guardTestProvider) Execute(context.Context, ProviderRequest) ProviderResult {
	return ProviderResult{Success: true, Verified: true}
}

func guardCapability(permission capability.PermissionLevel) capability.Capability {
	return capability.Capability{
		ID:          "test.capability",
		Domain:      capability.DomainLocal,
		Action:      "guard_action",
		Description: "execution guard test",
		Permission:  permission,
	}
}

func newGuard(t *testing.T, c capability.Capability) (*ExecutionGuard, *capability.Registry, *ProviderRegistry) {
	t.Helper()
	registry := capability.NewRegistry()
	if err := registry.Register(c); err != nil {
		t.Fatal(err)
	}
	providers := NewProviderRegistry()
	if err := providers.RegisterCapabilities(guardTestProvider{}, c.ID); err != nil {
		t.Fatal(err)
	}
	guard, err := NewExecutionGuard(registry, providers)
	if err != nil {
		t.Fatal(err)
	}
	return guard, registry, providers
}

func TestExecutionGuardPermissionAskFirstFailsClosedWithoutApproval(t *testing.T) {
	c := guardCapability(capability.PermissionAskFirst)
	guard, _, _ := newGuard(t, c)

	preflight, err := guard.Check(c, c.ID, c.Action, autonomy.ApprovalNone)
	if err != nil {
		t.Fatal(err)
	}
	if preflight.Allowed {
		t.Fatal("PermissionAskFirst must fail closed without explicit approval")
	}
	if !preflight.NeedsAuthorization {
		t.Fatalf("expected authorization handoff, got %+v", preflight)
	}
	if preflight.AuthorizationReason == "" {
		t.Fatal("authorization reason must be structured before execution")
	}
}

func TestExecutionGuardPermissionAskFirstAllowsOnlyExplicitApproval(t *testing.T) {
	c := guardCapability(capability.PermissionAskFirst)
	guard, _, _ := newGuard(t, c)

	preflight, err := guard.Check(c, c.ID, c.Action, autonomy.ApprovalGranted)
	if err != nil {
		t.Fatal(err)
	}
	if !preflight.Allowed {
		t.Fatalf("explicitly approved AskFirst capability should pass preflight: %+v", preflight)
	}
	if preflight.Authorization.Decision.Approval != autonomy.ApprovalGranted {
		t.Fatalf("unexpected authorization evidence: %+v", preflight.Authorization)
	}
}

func TestExecutionGuardRequiresInstrumentSelectionBeforeAuthorization(t *testing.T) {
	c := guardCapability(capability.PermissionAutomatic)
	c.Action = "abrir_programa"
	guard, _, providers := newGuard(t, c)
	if err := providers.RegisterCapabilities(testProvider{"alternate"}, c.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := guard.Check(c, c.ID, c.Action, autonomy.ApprovalNone); err == nil {
		t.Fatal("ambiguous instruments must require an explicit selection before execution")
	}
	preflight, err := guard.Check(c, c.ID, c.Action, autonomy.ApprovalNone, "alternate")
	if err != nil {
		t.Fatal(err)
	}
	if !preflight.Allowed || preflight.RegisteredProviderID != "alternate" {
		t.Fatalf("selected instrument was not preserved through preflight: %+v", preflight)
	}
	if _, err := guard.Check(c, c.ID, c.Action, autonomy.ApprovalNone, "not-registered"); err == nil {
		t.Fatal("an unregistered instrument must fail closed")
	}
}

func TestExecutionGuardRejectsCapabilityIdentityMismatch(t *testing.T) {
	registered := guardCapability(capability.PermissionAutomatic)
	guard, _, _ := newGuard(t, registered)
	candidate := registered
	candidate.ID = "other.capability"

	if _, err := guard.Check(candidate, registered.ID, registered.Action, autonomy.ApprovalGranted); err == nil {
		t.Fatal("identity mismatch must fail closed")
	}
}

func TestExecutionGuardRejectsCapabilityRegistryInconsistency(t *testing.T) {
	registered := guardCapability(capability.PermissionAutomatic)
	guard, _, _ := newGuard(t, registered)
	candidate := registered
	candidate.Action = "different_action"

	if _, err := guard.Check(candidate, candidate.ID, candidate.Action, autonomy.ApprovalGranted); err == nil {
		t.Fatal("candidate/registry disagreement must fail closed")
	}
}

func TestExecutionGuardRejectsInvalidCapabilityBeforeProviderResolution(t *testing.T) {
	registered := guardCapability(capability.PermissionAutomatic)
	guard, _, _ := newGuard(t, registered)
	candidate := registered
	candidate.Permission = capability.PermissionLevel("invalid")

	if _, err := guard.Check(candidate, candidate.ID, candidate.Action, autonomy.ApprovalGranted); err == nil {
		t.Fatal("invalid capability contract must fail closed")
	}
}

func TestExecutionGuardRejectsUnknownActionWithoutExplicitApproval(t *testing.T) {
	c := guardCapability(capability.PermissionAutomatic)
	c.Action = "unclassified_action"
	guard, _, _ := newGuard(t, c)

	preflight, err := guard.Check(c, c.ID, c.Action, autonomy.ApprovalNone)
	if err != nil {
		t.Fatal(err)
	}
	if preflight.Allowed || !preflight.NeedsAuthorization {
		t.Fatalf("unknown actions must fail closed before provider execution: %+v", preflight)
	}
}

func TestExecutionGuardRejectsOperationMismatch(t *testing.T) {
	c := guardCapability(capability.PermissionAutomatic)
	guard, _, _ := newGuard(t, c)

	if _, err := guard.Check(c, c.ID, "different_operation", autonomy.ApprovalGranted); err == nil {
		t.Fatal("operation mismatch must fail closed")
	}
}
