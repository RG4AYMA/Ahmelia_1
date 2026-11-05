package production

import (
	"ahmelia/capability"
	"testing"
)

func TestSafetyGateSafeAction(t *testing.T) {
	g := NewSafetyGate()
	c := capability.Capability{ID: "fs.read", Permission: capability.PermissionAutomatic}
	a, err := g.Assess(c, "buscar_archivo")
	if err != nil {
		t.Fatal(err)
	}
	if !a.Allowed {
		t.Fatalf("expected safe action to be allowed, got %#v", a)
	}
	if a.RequiresConfirmation || a.RequiresAuthorization {
		t.Fatalf("unexpected gate requirements: %#v", a)
	}
}

func TestSafetyGateUnknownActionFailsClosed(t *testing.T) {
	g := NewSafetyGate()
	c := capability.Capability{ID: "x", Permission: capability.PermissionAutomatic}
	a, err := g.Assess(c, "__unclassified_action__")
	if err != nil {
		t.Fatal(err)
	}
	if a.Allowed {
		t.Fatal("unclassified action must not be allowed")
	}
	if !a.RequiresConfirmation || !a.RequiresAuthorization {
		t.Fatalf("expected fail-closed requirements: %#v", a)
	}
}

func TestSafetyGateAskFirstAddsAuthorization(t *testing.T) {
	g := NewSafetyGate()
	c := capability.Capability{ID: "x", Permission: capability.PermissionAskFirst}
	a, err := g.Assess(c, "buscar_archivo")
	if err != nil {
		t.Fatal(err)
	}
	if a.Allowed {
		t.Fatal("ask_first must not be directly allowed")
	}
	if !a.RequiresAuthorization {
		t.Fatal("ask_first must require authorization")
	}
}

func TestSafetyGateRejectsInvalidCapability(t *testing.T) {
	g := NewSafetyGate()
	_, err := g.Assess(capability.Capability{Permission: capability.PermissionAutomatic}, "buscar_archivo")
	if err == nil {
		t.Fatal("expected invalid capability error")
	}
}
