package interaction

import "strings"

// SafetyLevel is a domain-neutral estimate of how consequential an interaction
// may be. It is deliberately not a capability/provider classification.
type SafetyLevel string

const (
	SafetyNone     SafetyLevel = "none"
	SafetyLow      SafetyLevel = "low"
	SafetyElevated SafetyLevel = "elevated"
	SafetyCritical SafetyLevel = "critical"
)

// PermissionRequirement describes what must be true before an interaction may
// cross from meaning into a consequential operation.
type PermissionRequirement string

const (
	PermissionNone                     PermissionRequirement = "none"
	PermissionConfirmation             PermissionRequirement = "confirmation"
	PermissionAuthority                PermissionRequirement = "authority"
	PermissionConfirmationAndAuthority PermissionRequirement = "confirmation_and_authority"
)

// Confirmation is separate from identity and authority. Saying "yes" is not
// authentication, and authentication is not consent for every operation.
type Confirmation string

const (
	ConfirmationNone    Confirmation = "none"
	ConfirmationGranted Confirmation = "granted"
	ConfirmationDenied  Confirmation = "denied"
)

// SafetyPermissionRequest is the input to the interaction safety boundary.
// The boundary consumes semantic meaning plus explicit authority/confirmation
// evidence; it never chooses a tool or executes anything.
type SafetyPermissionRequest struct {
	Semantic     SemanticRequest
	Identity     IdentityContext
	Scope        AuthorityScope
	Confirmation Confirmation
}

// SafetyPermissionDecision is an auditable boundary decision. It intentionally
// contains no provider, tool, executable path, or capability implementation.
type SafetyPermissionDecision struct {
	Safety            SafetyLevel
	Requirement       PermissionRequirement
	Allowed           bool
	NeedsConfirmation bool
	NeedsAuthority    bool
	Authenticated     bool
	PrimaryAuthority  bool
	Reason            string
}

// SafetyPermissionPolicy is the interaction-layer policy. It answers only
// whether a turn is safe/permitted to continue toward a consequential layer.
type SafetyPermissionPolicy struct {
	authority *AuthorityService
}

// NewSafetyPermissionPolicy optionally receives the authority boundary. A nil
// authority service is acceptable for low-risk interactions but causes
// critical operations to fail closed.
func NewSafetyPermissionPolicy(authority ...*AuthorityService) *SafetyPermissionPolicy {
	p := &SafetyPermissionPolicy{}
	if len(authority) > 0 {
		p.authority = authority[0]
	}
	return p
}

// Evaluate applies a fail-closed policy for consequential interaction. A
// confirmation can satisfy consent, but never identity/authority. Likewise,
// authentication does not silently satisfy a separate confirmation request.
func (p *SafetyPermissionPolicy) Evaluate(req SafetyPermissionRequest) SafetyPermissionDecision {
	if p == nil {
		return SafetyPermissionDecision{Safety: SafetyCritical, Requirement: PermissionConfirmationAndAuthority, Reason: "safety policy unavailable"}
	}

	confirmation := req.Confirmation
	if confirmation == "" {
		confirmation = ConfirmationNone
	}
	if confirmation != ConfirmationNone && confirmation != ConfirmationGranted && confirmation != ConfirmationDenied {
		return SafetyPermissionDecision{Safety: SafetyCritical, Requirement: PermissionConfirmationAndAuthority, Reason: "invalid confirmation state"}
	}

	acts := CanonicalActs(req.Semantic)
	if !acts.Has(ActCommand) {
		return SafetyPermissionDecision{Safety: SafetyNone, Requirement: PermissionNone, Allowed: true, Reason: "interaction is not a consequential operation"}
	}

	action := strings.ToLower(strings.TrimSpace(req.Semantic.Action.Name))
	if action == "" && strings.TrimSpace(req.Semantic.RequestedAction) == "" {
		return SafetyPermissionDecision{Safety: SafetyNone, Requirement: PermissionNone, Allowed: true, Reason: "no executable operation established"}
	}

	level := classifySafety(req.Semantic)
	decision := SafetyPermissionDecision{Safety: level, Requirement: PermissionNone}

	switch level {
	case SafetyLow:
		// Ordinary low-impact interaction may continue. Domain-specific
		// authorization remains downstream when a capability is resolved.
		decision.Allowed = true
		decision.Reason = "low-impact operation may continue to capability authorization"
		return decision
	case SafetyElevated:
		decision.Requirement = PermissionConfirmation
		decision.NeedsConfirmation = true
	case SafetyCritical:
		decision.Requirement = PermissionConfirmationAndAuthority
		decision.NeedsConfirmation = true
		decision.NeedsAuthority = true
	}

	if req.Identity.Status == IdentityAuthenticated {
		decision.Authenticated = true
	}
	if decision.NeedsAuthority && p.authority != nil {
		authority := p.authority.EvaluateAuthority(AuthorityRequest{Identity: req.Identity, Scope: req.Scope})
		decision.Authenticated = authority.Authenticated
		decision.PrimaryAuthority = authority.Primary
	}

	if decision.NeedsAuthority {
		if !decision.Authenticated || !decision.PrimaryAuthority {
			decision.Reason = "critical operation requires authenticated authority"
			return decision
		}
	}
	if decision.NeedsConfirmation {
		switch confirmation {
		case ConfirmationDenied:
			decision.Reason = "explicit confirmation was denied"
			return decision
		case ConfirmationNone:
			decision.Reason = "explicit confirmation required before proceeding"
			return decision
		case ConfirmationGranted:
			// continue
		}
	}

	decision.Allowed = true
	decision.Reason = "safety and required interaction permission conditions satisfied"
	return decision
}

func classifySafety(r SemanticRequest) SafetyLevel {
	text := strings.ToLower(strings.TrimSpace(r.Raw + " " + r.Action.Name + " " + r.RequestedAction))

	critical := []string{
		"borrar todo", "elimina todo", "eliminar todo", "formatear", "apagar el sistema",
		"shutdown", "factory reset", "restablecer de fábrica", "restablecer de fabrica",
		"desactivar seguridad", "desactiva seguridad", "quitar seguridad", "cambiar permisos",
		"dar permisos", "revocar permisos", "credenciales", "contraseña", "contrasena",
	}
	for _, token := range critical {
		if strings.Contains(text, token) {
			return SafetyCritical
		}
	}

	elevated := []string{
		"borrar ", "borra ", "elimina ", "eliminar ", "sobrescribir", "reemplazar ",
		"instalar ", "desinstalar ", "publicar ", "enviar ", "compartir ",
		"cerrar todo", "mover ", "renombrar ", "modificar permisos",
	}
	for _, token := range elevated {
		if strings.Contains(text, token) {
			return SafetyElevated
		}
	}

	return SafetyLow
}
