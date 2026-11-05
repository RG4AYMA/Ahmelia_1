package autonomy

import "context"

// ObservationStatus preserves the epistemic state of environmental evidence
// instead of collapsing it into VerificationResult.Success.
type ObservationStatus string

const (
	ObservationNotObserved  ObservationStatus = "not_observed"
	ObservationConfirmed    ObservationStatus = "confirmed"
	ObservationContradicted ObservationStatus = "contradicted"
	ObservationInsufficient ObservationStatus = "insufficient"
)

func (s ObservationStatus) Valid() bool {
	switch s {
	case ObservationNotObserved, ObservationConfirmed, ObservationContradicted, ObservationInsufficient:
		return true
	default:
		return false
	}
}

// VerificationResult contiene el dictamen técnico que usa el ciclo para
// decidir entre el camino de éxito y el de recuperación/diagnóstico.
// AuthorityOutcome transports the canonical authority result across the
// autonomy boundary without importing production or duplicating its result model.
type AuthorityOutcome interface {
	AuthorityHandled() bool
	AuthorityExecuted() bool
	AuthoritySucceeded() bool
	AuthorityOutcomeStatus() string
	AuthorityObserved() bool
	AuthorityVerified() bool
	AuthorityWorkID() string
	AuthorityAttemptNumber() int
	AuthorityOutcomeCode() string
	AuthorityFailureReason() string
	AuthoritySummary() string
}

type VerificationResult struct {
	Success            bool
	Output             string
	ErrorReason        string
	ObservationStatus  ObservationStatus
	ObservationDetails string
	Observation        EnvironmentObservation
	AuthorityResult    AuthorityOutcome
}

// ActionVerifier realiza únicamente verificación técnica liviana.
// La inspección real del entorno pertenece a un bloque posterior (B80).
type ActionVerifier struct{}

func NewActionVerifier() *ActionVerifier {
	return &ActionVerifier{}
}

func (v *ActionVerifier) Verify(ctx context.Context, req ActionRequest, res ActionResponse) VerificationResult {
	if err := ctx.Err(); err != nil {
		return VerificationResult{
			Success:     false,
			Output:      res.Output,
			ErrorReason: "contexto cancelado: " + err.Error(),
		}
	}

	if req.ID != "" && res.RequestID != "" && req.ID != res.RequestID {
		return VerificationResult{
			Success:         false,
			Output:          res.Output,
			ErrorReason:     "el resultado no corresponde a la solicitud ejecutada",
			AuthorityResult: res.AuthorityResult,
		}
	}

	if res.AuthorityResult != nil {
		authority := res.AuthorityResult
		status := authority.AuthorityOutcomeStatus()
		// Explicit epistemic outcomes dominate optimistic booleans. A provider
		// cannot report OutcomeUnknown and simultaneously turn that uncertainty
		// into technical success by setting Succeeded/Verified to true.
		statusAllowsSuccess := status != "unknown" && status != "failed"
		success := authority.AuthorityHandled() && authority.AuthorityExecuted() && authority.AuthoritySucceeded() && authority.AuthorityVerified() && statusAllowsSuccess
		reason := authority.AuthorityFailureReason()
		if !success && reason == "" {
			reason = authority.AuthorityOutcomeCode()
		}
		if !success && reason == "" {
			reason = authority.AuthorityOutcomeStatus()
		}
		if !success && reason == "" && !authority.AuthorityHandled() {
			reason = "la autoridad no gestionó la operación"
		}
		return VerificationResult{
			Success:         success,
			Output:          authority.AuthoritySummary(),
			ErrorReason:     reason,
			AuthorityResult: authority,
		}
	}

	if res.Error != nil {
		return VerificationResult{
			Success:     false,
			Output:      res.Output,
			ErrorReason: "error técnico de ejecución: " + res.Error.Error(),
		}
	}

	if !res.Executed {
		return VerificationResult{
			Success:     false,
			Output:      res.Output,
			ErrorReason: "la acción no fue ejecutada por el motor",
		}
	}

	return VerificationResult{
		Success: true,
		Output:  res.Output,
	}
}
