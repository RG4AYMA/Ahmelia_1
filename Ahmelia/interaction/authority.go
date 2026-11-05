package interaction

import "strings"

// Authority separates identity from permission. Recognition mechanisms such
// as face, voice or iris belong above this contract and never grant authority
// merely by producing a match.
type AuthorityDecision struct {
	Authenticated bool
	Authorized    bool
	Primary       bool
	IdentityID    string
	Reason        string
}

type AuthorityService struct {
	primary AuthorityIdentity
}

func NewAuthorityService(primary AuthorityIdentity) *AuthorityService {
	return &AuthorityService{primary: primary}
}

func (a *AuthorityService) PrimaryIdentity() AuthorityIdentity {
	if a == nil {
		return AuthorityIdentity{}
	}
	return a.primary
}

// AuthorizePrimary is intentionally credential-agnostic. Future biometric
// adapters must prove identity before this service is allowed to authorize it.
func (a *AuthorityService) AuthorizePrimary(identityID string, authenticated bool) AuthorityDecision {
	if a == nil {
		return AuthorityDecision{Reason: "authority service unavailable"}
	}
	id := strings.ToLower(strings.TrimSpace(identityID))
	expected := strings.ToLower(strings.TrimSpace(a.primary.ID))
	if !authenticated {
		return AuthorityDecision{Reason: "identity not authenticated"}
	}
	if id == "" || expected == "" || id != expected {
		return AuthorityDecision{Authenticated: true, Reason: "authenticated identity is not the primary authority"}
	}
	return AuthorityDecision{
		Authenticated: true, Authorized: true, Primary: true,
		IdentityID: a.primary.ID, Reason: "primary authority authenticated",
	}
}
