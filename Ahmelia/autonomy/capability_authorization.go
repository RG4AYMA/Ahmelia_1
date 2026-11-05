package autonomy

import (
	"fmt"

	"ahmelia/capability"
)

// ============================================================================
// CAPABILITY_AUTHORIZATION.GO — Bloque 94.3 (Capability Authorization)
// ============================================================================
//
// B94.1 definió PermissionLevel dentro del contrato declarativo de
// capability.Capability y B94.2 creó CapabilityRegistry para descubrirlas.
// B94.3 cierra el puente que faltaba: traducir ese permiso declarado a la
// autoridad ejecutiva real de autonomy.DecisionAuthority.
//
// Regla central:
//
//   automatic -> Execute
//   notify    -> Execute (ejecuta, pero requiere notificación posterior)
//   ask_first -> Recommend mientras no exista aprobación explícita;
//                Execute únicamente después de ApprovalGranted.
//
// Esto evita un error especialmente peligroso: interpretar "ask_first" como
// si ya fuera permiso de ejecución solo porque la Capability existe en el
// Registry. Descubrimiento y autorización siguen siendo dos operaciones
// distintas.
//
// B94.3 NO ejecuta la Capability, NO modifica el Registry y NO pide por sí
// mismo una respuesta al usuario. Solo produce un veredicto auditable que
// ExecutorAdapter/ciclo ejecutivo podrá consumir en un bloque posterior.

// Approval representa el estado de una autorización explícita externa.
// ApprovalNone significa que todavía no existe respuesta; no equivale a
// aprobación.
type Approval string

const (
	ApprovalNone    Approval = "none"
	ApprovalGranted Approval = "granted"
	ApprovalDenied  Approval = "denied"
)

// IsValid comprueba el vocabulario cerrado de aprobación.
func (a Approval) IsValid() bool {
	switch a {
	case ApprovalNone, ApprovalGranted, ApprovalDenied:
		return true
	default:
		return false
	}
}

// CapabilityAuthorization es el resultado auditable de autorizar una
// Capability concreta. Allowed nunca se acepta como input: se calcula a
// partir del nivel de autoridad resultante mediante CanExecute().
type CapabilityAuthorization struct {
	CapabilityID  capability.ID
	Permission    capability.PermissionLevel
	Approval      Approval
	Authority     DecisionAuthority
	Allowed       bool
	NeedsApproval bool
	Reason        string
}

// AuthorizeCapability evalúa una Capability contra un estado de aprobación.
//
// Para automatic y notify, la Capability puede ejecutar sin aprobación previa.
// Para ask_first, ApprovalGranted es obligatorio. ApprovalNone queda pendiente
// y ApprovalDenied queda bloqueado.
func AuthorizeCapability(c capability.Capability, approval Approval) (CapabilityAuthorization, error) {
	if err := c.Validate(); err != nil {
		return CapabilityAuthorization{}, err
	}
	if !approval.IsValid() {
		return CapabilityAuthorization{}, fmt.Errorf(
			"capability authorization: approval %q no es válido",
			approval,
		)
	}

	result := CapabilityAuthorization{
		CapabilityID: c.ID,
		Permission:   c.Permission,
		Approval:     approval,
	}

	switch c.Permission {
	case capability.PermissionAutomatic:
		result.Authority = AuthorityExecute
		result.Allowed = CanExecute(result.Authority)
		result.Reason = "la capability declara permiso automático"

	case capability.PermissionNotify:
		result.Authority = AuthorityExecute
		result.Allowed = CanExecute(result.Authority)
		result.Reason = "la capability declara ejecución automática con notificación posterior"

	case capability.PermissionAskFirst:
		result.NeedsApproval = true
		switch approval {
		case ApprovalGranted:
			result.Authority = AuthorityExecute
			result.Allowed = CanExecute(result.Authority)
			result.Reason = "la autorización explícita fue concedida"
		case ApprovalDenied:
			result.Authority = AuthorityRecommend
			result.Allowed = CanExecute(result.Authority)
			result.Reason = "la autorización explícita fue denegada"
		case ApprovalNone:
			result.Authority = AuthorityRecommend
			result.Allowed = CanExecute(result.Authority)
			result.Reason = "la capability requiere autorización explícita antes de ejecutar"
		}
	}

	return result, nil
}

// AuthorizeCapabilityID descubre primero una Capability en el Registry y
// luego aplica exactamente el mismo contrato de autorización. No existe una
// segunda tabla de permisos: Permission vive en la Capability registrada.
func AuthorizeCapabilityID(r *capability.Registry, id capability.ID, approval Approval) (CapabilityAuthorization, error) {
	if r == nil {
		return CapabilityAuthorization{}, fmt.Errorf("capability authorization: registry nil")
	}

	c, ok := r.Get(id)
	if !ok {
		return CapabilityAuthorization{}, fmt.Errorf("capability authorization: capability %q no está registrada", id)
	}

	return AuthorizeCapability(c, approval)
}
