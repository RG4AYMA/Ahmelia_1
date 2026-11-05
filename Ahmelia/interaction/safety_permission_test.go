package interaction

import "testing"

func TestSafetyPermissionConversationIsNotBlocked(t *testing.T) {
	p := NewSafetyPermissionPolicy()
	d := p.Evaluate(SafetyPermissionRequest{Semantic: SemanticRequest{Kind: KindQuestion}})
	if !d.Allowed || d.Requirement != PermissionNone {
		t.Fatalf("conversation should pass unchanged: %#v", d)
	}
}

func TestSafetyPermissionLowImpactDoesNotRequireConsent(t *testing.T) {
	p := NewSafetyPermissionPolicy()
	d := p.Evaluate(SafetyPermissionRequest{Semantic: SemanticRequest{
		Kind: KindAction, Raw: "abre chrome", Action: ActionIntent{Name: "abrir_programa", Target: "chrome"},
	}})
	if !d.Allowed || d.NeedsConfirmation || d.NeedsAuthority {
		t.Fatalf("low impact action should continue without interaction-level escalation: %#v", d)
	}
}

func TestSafetyPermissionElevatedRequiresExplicitConfirmation(t *testing.T) {
	p := NewSafetyPermissionPolicy()
	r := SemanticRequest{Kind: KindAction, Raw: "borra el archivo", Action: ActionIntent{Name: "borrar_archivo"}}
	if d := p.Evaluate(SafetyPermissionRequest{Semantic: r}); d.Allowed || !d.NeedsConfirmation {
		t.Fatalf("elevated operation must wait for confirmation: %#v", d)
	}
	if d := p.Evaluate(SafetyPermissionRequest{Semantic: r, Confirmation: ConfirmationDenied}); d.Allowed {
		t.Fatal("denied confirmation must remain blocked")
	}
	if d := p.Evaluate(SafetyPermissionRequest{Semantic: r, Confirmation: ConfirmationGranted}); !d.Allowed {
		t.Fatalf("granted confirmation should satisfy elevated consent: %#v", d)
	}
}

func TestSafetyPermissionCriticalRequiresAuthorityAndConfirmation(t *testing.T) {
	p := NewSafetyPermissionPolicy(NewAuthorityService(NewUserProfile().PrimaryAuthority))
	r := SemanticRequest{Kind: KindAction, Raw: "desactivar seguridad", Action: ActionIntent{Name: "configurar_seguridad"}}

	d := p.Evaluate(SafetyPermissionRequest{Semantic: r, Confirmation: ConfirmationGranted})
	if d.Allowed || !d.NeedsAuthority || d.Authenticated {
		t.Fatalf("critical operation must fail closed without authenticated authority: %#v", d)
	}

	d = p.Evaluate(SafetyPermissionRequest{
		Semantic:     r,
		Identity:     IdentityContext{IdentityID: "ayma240", Status: IdentityAuthenticated},
		Scope:        AuthorityOperate,
		Confirmation: ConfirmationGranted,
	})
	if !d.Allowed || !d.Authenticated || !d.PrimaryAuthority {
		t.Fatalf("authenticated authority plus confirmation should cross boundary: %#v", d)
	}
}

func TestSafetyPermissionConfirmationNeverAuthenticates(t *testing.T) {
	p := NewSafetyPermissionPolicy()
	r := SemanticRequest{Kind: KindAction, Raw: "desactivar seguridad", Action: ActionIntent{Name: "configurar_seguridad"}}
	d := p.Evaluate(SafetyPermissionRequest{Semantic: r, Confirmation: ConfirmationGranted})
	if d.Authenticated || d.PrimaryAuthority || d.Allowed {
		t.Fatalf("confirmation must not become identity or authority: %#v", d)
	}
}
