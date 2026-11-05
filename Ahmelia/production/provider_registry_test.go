package production

import (
	"context"
	"testing"

	"ahmelia/capability"
)

type testProvider struct{ id string }

func (p testProvider) ID() string { return p.id }
func (p testProvider) Execute(context.Context, ProviderRequest) ProviderResult {
	return ProviderResult{Success: true, Summary: "ok"}
}

func TestProviderRegistryResolvesCapabilityBeforeDomain(t *testing.T) {
	r := NewProviderRegistry()
	domain := capability.Domain("test")
	c := capability.Capability{ID: "x.one", Domain: domain, Action: "one", Description: "one", Permission: capability.PermissionAutomatic}
	p1, p2 := testProvider{"p1"}, testProvider{"p2"}
	if err := r.RegisterDomain(domain, p1); err != nil {
		t.Fatal(err)
	}
	if err := r.RegisterCapabilities(p2, c.ID); err != nil {
		t.Fatal(err)
	}
	got, ok := r.Resolve(c)
	if !ok || got.ID() != "p2" {
		t.Fatalf("got provider %v, want p2", got)
	}
}

func TestProviderRegistryFallsBackToDomain(t *testing.T) {
	r := NewProviderRegistry()
	domain := capability.Domain("test")
	c := capability.Capability{ID: "x.one", Domain: domain, Action: "one", Description: "one", Permission: capability.PermissionAutomatic}
	p := testProvider{"p1"}
	if err := r.RegisterDomain(domain, p); err != nil {
		t.Fatal(err)
	}
	got, ok := r.Resolve(c)
	if !ok || got.ID() != "p1" {
		t.Fatalf("got provider %v, want p1", got)
	}
}

func TestProviderRegistryKeepsMultipleInstrumentsAndRequiresExplicitChoice(t *testing.T) {
	r := NewProviderRegistry()
	c := capability.Capability{ID: "3d.model_object", Domain: capability.Domain("3d"), Action: "model_object", Permission: capability.PermissionAutomatic}
	primary, alternate := testProvider{"blender"}, testProvider{"alternate-3d"}
	if err := r.RegisterCapabilities(primary, c.ID); err != nil {
		t.Fatal(err)
	}
	if err := r.RegisterCapabilities(alternate, c.ID); err != nil {
		t.Fatalf("a second instrument for the same capability must register: %v", err)
	}
	if got := r.Candidates(c); len(got) != 2 || got[0].ID() != "alternate-3d" || got[1].ID() != "blender" {
		t.Fatalf("candidate instruments should be complete and stable-sorted: %+v", got)
	}
	if _, ok := r.Resolve(c); ok {
		t.Fatal("ambiguous instruments must not be chosen implicitly")
	}
	got, ok := r.ResolveFor(c, "blender")
	if !ok || got.ID() != "blender" {
		t.Fatalf("explicit instrument selection failed: provider=%v ok=%t", got, ok)
	}
	if _, ok := r.ResolveFor(c, "missing"); ok {
		t.Fatal("unknown instrument must not fall back to another provider")
	}
}

func TestAuthorityExecutionResultSeparatesControlAndExecutionState(t *testing.T) {
	pending := ActionResult{
		Executed: false, Success: false, Verified: false, AuthorizationRequired: true,
		AuthorizationReason: "approval required", AuthorizationCapabilityID: "demo.protected",
		WorkID: "work-1", OutcomeCode: "authorization_required",
	}
	got := AuthorityResultFromAction(pending)
	if !got.Handled || got.Executed || got.Succeeded || got.Verified || !got.AuthorizationRequired {
		t.Fatalf("authority result collapsed distinct states: %+v", got)
	}

	completed := ActionResult{Executed: true, Success: true, Observed: true, Verified: true, WorkID: "work-1", AttemptNumber: 7, OutcomeCode: "completed_verified"}
	got = AuthorityResultFromAction(completed)
	if !got.Handled || !got.Executed || !got.Succeeded || !got.Observed || !got.Verified {
		t.Fatalf("verified execution state not preserved: %+v", got)
	}
	if got.AttemptNumber != completed.AttemptNumber {
		t.Fatalf("attempt identity lost in ActionResult conversion: got %d, want %d", got.AttemptNumber, completed.AttemptNumber)
	}
}
