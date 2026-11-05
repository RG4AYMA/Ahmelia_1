package interaction

import "testing"

func TestAuthorityBoundaryRequiresAuthenticatedIdentity(t *testing.T) {
	a := NewAuthorityService(NewUserProfile().PrimaryAuthority)

	cases := []struct {
		name string
		ctx  IdentityContext
	}{
		{"unknown", IdentityContext{IdentityID: "ayma240", Status: IdentityUnknown}},
		{"claimed", IdentityContext{IdentityID: "ayma240", Status: IdentityClaimed}},
		{"recognized", IdentityContext{IdentityID: "ayma240", Status: IdentityRecognized}},
		{"other authenticated", IdentityContext{IdentityID: "someone_else", Status: IdentityAuthenticated}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := a.EvaluateAuthority(AuthorityRequest{
				Identity: tc.ctx,
				Scope:    AuthorityOperate,
			})
			if d.Authorized || d.Primary {
				t.Fatalf("identity must not escalate to authority: %+v", d)
			}
		})
	}
}

func TestAuthorityBoundarySeparatesPrimaryIdentityFromMechanism(t *testing.T) {
	a := NewAuthorityService(NewUserProfile().PrimaryAuthority)

	d := a.EvaluateAuthority(AuthorityRequest{
		Identity: IdentityContext{
			IdentityID: "ayma240",
			Status:     IdentityAuthenticated,
			Source:     "future-sensor",
			Confidence: 0.91,
		},
		Scope: AuthorityInteract,
	})

	if !d.Authenticated || !d.Authorized || !d.Primary {
		t.Fatalf("authenticated primary identity should cross boundary: %+v", d)
	}
	if d.IdentityID != "ayma240" {
		t.Fatalf("unexpected identity: %q", d.IdentityID)
	}
}

func TestAuthorityBoundaryDoesNotTreatIdentityAsCapabilitySelection(t *testing.T) {
	a := NewAuthorityService(NewUserProfile().PrimaryAuthority)

	d := a.EvaluateAuthority(AuthorityRequest{
		Identity: IdentityContext{
			IdentityID: "ayma240",
			Status:     IdentityAuthenticated,
		},
		Scope: AuthorityOperate,
	})

	if !d.Authorized {
		t.Fatalf("expected authority decision, got %+v", d)
	}
	// The authority boundary intentionally has no provider/tool/capability
	// field. Compilation of this test therefore protects the architecture:
	// authority answers "who may authorize?", not "what should execute?".
}
