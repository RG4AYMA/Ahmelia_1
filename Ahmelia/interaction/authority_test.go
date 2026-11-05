package interaction

import "testing"

func TestPrimaryAuthorityRequiresAuthenticationAndIdentity(t *testing.T) {
	a := NewAuthorityService(NewUserProfile().PrimaryAuthority)
	if d := a.AuthorizePrimary("ayma240", false); d.Authorized {
		t.Fatal("unauthenticated identity must not be authorized")
	}
	if d := a.AuthorizePrimary("someone_else", true); d.Authorized {
		t.Fatal("non-primary identity must not be authorized")
	}
	if d := a.AuthorizePrimary("ayma240", true); !d.Authorized || !d.Primary {
		t.Fatalf("primary identity should be authorized: %+v", d)
	}
}
