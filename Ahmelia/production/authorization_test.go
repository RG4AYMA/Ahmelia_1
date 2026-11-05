package production

import (
	"ahmelia/autonomy"
	"ahmelia/capability"
	"ahmelia/planning"
	"testing"
)

func ar(t *testing.T, p capability.PermissionLevel) *capability.Registry {
	t.Helper()
	r := capability.NewRegistry()
	if e := r.Register(capability.Capability{ID: "fs.write", Domain: capability.DomainLocal, Action: "write", Permission: p}); e != nil {
		t.Fatal(e)
	}
	return r
}
func TestAuthorizationAutomatic(t *testing.T) {
	g, _ := NewAuthorizationGate(ar(t, capability.PermissionAutomatic))
	x, e := g.Authorize("fs.write", autonomy.ApprovalNone)
	if e != nil || !x.CanExecute() {
		t.Fatalf("%v %+v", e, x.Decision)
	}
}
func TestAuthorizationAskFirst(t *testing.T) {
	g, _ := NewAuthorizationGate(ar(t, capability.PermissionAskFirst))
	x, _ := g.Authorize("fs.write", autonomy.ApprovalNone)
	if x.CanExecute() {
		t.Fatal("pending allowed")
	}
	x, _ = g.Authorize("fs.write", autonomy.ApprovalDenied)
	if x.CanExecute() {
		t.Fatal("denied allowed")
	}
	x, _ = g.Authorize("fs.write", autonomy.ApprovalGranted)
	if !x.CanExecute() {
		t.Fatal("granted blocked")
	}
}
func TestAuthorizationUnknown(t *testing.T) {
	g, _ := NewAuthorizationGate(ar(t, capability.PermissionAutomatic))
	if _, e := g.Authorize("missing", autonomy.ApprovalNone); e == nil {
		t.Fatal("expected error")
	}
}
func TestAuthorizationBindingValidation(t *testing.T) {
	g, _ := NewAuthorizationGate(ar(t, capability.PermissionAutomatic))
	b := CapabilityBinding{Task: planning.NewTask("g", "x", "", 1, 0), Capability: capability.Capability{ID: "fs.write", Domain: capability.DomainLocal, Action: "write", Permission: capability.PermissionAutomatic}, Action: Action{ID: "a", CapabilityID: "fs.other", Name: "write"}}
	if _, e := g.AuthorizeBinding(b, autonomy.ApprovalNone); e == nil {
		t.Fatal("expected mismatch")
	}
}
