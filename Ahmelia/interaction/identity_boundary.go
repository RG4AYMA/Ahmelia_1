package interaction

import "strings"

// IdentityStatus describes what the interaction layer actually knows about
// the speaker. Recognition and authentication are intentionally distinct:
// seeing a name or matching a weak signal is not the same as establishing
// an authenticated identity.
type IdentityStatus string

const (
	IdentityUnknown       IdentityStatus = "unknown"
	IdentityClaimed       IdentityStatus = "claimed"
	IdentityRecognized    IdentityStatus = "recognized"
	IdentityAuthenticated IdentityStatus = "authenticated"
)

// IdentityContext is the identity evidence attached to an interaction turn.
// It is deliberately mechanism-agnostic: voice, face, device credentials,
// UI sessions and future sensors can all populate the same boundary.
type IdentityContext struct {
	IdentityID string
	Status     IdentityStatus
	Source     string
	Confidence float64
}

// AuthorityScope identifies the authority being requested. Identity alone
// never implies unrestricted authority.
type AuthorityScope string

const (
	AuthorityNone      AuthorityScope = ""
	AuthorityIdentify  AuthorityScope = "identify"
	AuthorityInteract  AuthorityScope = "interact"
	AuthorityOperate   AuthorityScope = "operate"
	AuthorityConfigure AuthorityScope = "configure"
)

// AuthorityRequest is the domain-neutral request for an authority decision.
// It contains no provider/tool information and does not execute anything.
type AuthorityRequest struct {
	Identity IdentityContext
	Scope    AuthorityScope
}

// AuthorityBoundaryDecision records the result of the identity/authority
// boundary. Downstream systems can require Authenticated/Authorized before
// performing consequential operations.
type AuthorityBoundaryDecision struct {
	IdentityID    string
	Status        IdentityStatus
	Authenticated bool
	Authorized    bool
	Primary       bool
	Scope         AuthorityScope
	Reason        string
}

// EvaluateAuthority is stricter than a mere identity match. A primary
// authority decision requires an authenticated identity and an explicit
// authority scope. Unknown, claimed or merely recognized identities cannot
// silently escalate into primary authority.
//
// This function does not know how identity was established and does not grant
// any domain capability. It only evaluates the already-proven identity
// evidence against the configured primary identity.
func (a *AuthorityService) EvaluateAuthority(req AuthorityRequest) AuthorityBoundaryDecision {
	if a == nil {
		return AuthorityBoundaryDecision{Status: IdentityUnknown, Scope: req.Scope, Reason: "authority service unavailable"}
	}

	id := strings.TrimSpace(req.Identity.IdentityID)
	expected := strings.TrimSpace(a.primary.ID)
	status := req.Identity.Status
	if status == "" {
		status = IdentityUnknown
	}

	out := AuthorityBoundaryDecision{
		IdentityID: id,
		Status:     status,
		Scope:      req.Scope,
	}

	if id == "" {
		out.Reason = "no identity established"
		return out
	}

	if status != IdentityAuthenticated {
		out.Reason = "identity is not authenticated"
		return out
	}

	out.Authenticated = true

	if !strings.EqualFold(id, expected) || expected == "" {
		out.Reason = "authenticated identity is not the primary authority"
		return out
	}

	out.Primary = true

	// Identification itself does not need an elevated scope. Interaction is
	// the ordinary conversational authority; operational/configuration
	// authority must still be requested explicitly by the caller.
	switch req.Scope {
	case AuthorityNone, AuthorityIdentify, AuthorityInteract, AuthorityOperate, AuthorityConfigure:
		out.Authorized = true
		out.Reason = "authenticated primary authority for requested scope"
	default:
		out.Reason = "unknown authority scope"
	}

	return out
}

// IdentityFromLegacyAuthentication adapts the older boolean contract without
// pretending that authentication mechanisms are known by the interaction
// layer. It exists to keep the migration incremental.
func IdentityFromLegacyAuthentication(identityID string, authenticated bool) IdentityContext {
	status := IdentityClaimed
	if authenticated {
		status = IdentityAuthenticated
	}
	if strings.TrimSpace(identityID) == "" {
		status = IdentityUnknown
	}
	return IdentityContext{
		IdentityID: strings.TrimSpace(identityID),
		Status:     status,
		Source:     "legacy-authentication",
		Confidence: 1,
	}
}
