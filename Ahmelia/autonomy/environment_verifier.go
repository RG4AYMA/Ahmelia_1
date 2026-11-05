package autonomy

import (
	"context"
	"fmt"
	"time"
)

// EnvironmentObservation representa evidencia observable del estado del entorno
// después de ejecutar una acción.
//
// B80 no interpreta texto libre como señal de éxito. El inspector que conozca
// el entorno real debe devolver evidencia estructurada.
type ObservationFact struct {
	Subject   string `json:"subject"`
	Predicate string `json:"predicate"`
	Expected  string `json:"expected"`
	Observed  string `json:"observed"`
}

type EnvironmentObservation struct {
	Confirmed bool
	Details   string
	Status    ObservationStatus
	Facts     []ObservationFact `json:"facts,omitempty"`

	// Provenance binds the evidence to the inspected operation and records
	// when the inspection was performed. These fields are metadata, not proof
	// by themselves; consumers must compare ActionID with the requested action.
	ActionID   string    `json:"action_id,omitempty"`
	Operation  string    `json:"operation,omitempty"`
	Target     string    `json:"target,omitempty"`
	Source     string    `json:"source,omitempty"`
	ObservedAt time.Time `json:"observed_at,omitempty"`
}

// HasSufficientConfirmedEvidence validates the structure of evidence, not its
// provenance or truthfulness. The caller must still bind the observation to
// the canonical action/work/attempt and apply resource-specific freshness rules.
func (o EnvironmentObservation) IsFresh(now time.Time, maxAge time.Duration) bool {
	if o.ObservedAt.IsZero() || maxAge <= 0 {
		return false
	}
	age := now.Sub(o.ObservedAt)
	return age >= -5*time.Second && age <= maxAge
}

func (o EnvironmentObservation) HasSufficientConfirmedEvidence() bool {
	if !o.Confirmed || o.Status != ObservationConfirmed || len(o.Facts) == 0 {
		return false
	}
	for _, fact := range o.Facts {
		if fact.Subject == "" || fact.Predicate == "" || fact.Expected == "" || fact.Observed == "" || fact.Expected != fact.Observed {
			return false
		}
	}
	return true
}

// EnvironmentInspector es el contrato entre Autonomía y el observador real
// del entorno. Su implementación concreta puede vivir en environment/.
//
// Ejemplos:
//   - comprobar que una carpeta existe después de crearla;
//   - comprobar que un archivo terminó en la ruta esperada;
//   - comprobar que un proceso/ventana está presente.
//
// El inspector NO ejecuta la acción.
type EnvironmentInspector interface {
	InspectAction(ctx context.Context, req ActionRequest) (EnvironmentObservation, error)
}

// EnvironmentVerificationResult contiene el resultado combinado de:
//  1. verificación técnica de B78;
//  2. evidencia observable de B80.
type EnvironmentVerificationResult struct {
	Success           bool
	Output            string
	ErrorReason       string
	Observed          bool
	Details           string
	ObservationStatus ObservationStatus
	// Observation carries the original structured evidence across the verifier boundary.
	// Details/status are projections; they must not replace the underlying facts/provenance.
	Observation EnvironmentObservation
}

// EnvironmentVerifier realiza la verificación posterior a la ejecución.
//
// La regla es deliberadamente estricta:
//   - si la ejecución técnica falló, no se consulta el entorno como si hubiera éxito;
//   - si el ejecutor dice que ejecutó pero el entorno no confirma el resultado,
//     la acción NO se considera exitosa;
//   - no se usa strings.Contains(output, "error") ni heurística textual.
type EnvironmentVerifier struct {
	inspector EnvironmentInspector
}

func NewEnvironmentVerifier(inspector EnvironmentInspector) *EnvironmentVerifier {
	return &EnvironmentVerifier{
		inspector: inspector,
	}
}

// Verify combina el resultado técnico de B78 con evidencia del entorno de B80.
func (v *EnvironmentVerifier) Verify(
	ctx context.Context,
	req ActionRequest,
	technical VerificationResult,
) EnvironmentVerificationResult {
	if !technical.Success {
		return EnvironmentVerificationResult{
			Success:           false,
			Output:            technical.Output,
			ErrorReason:       technical.ErrorReason,
			Observed:          false,
			ObservationStatus: ObservationNotObserved,
		}
	}

	if v == nil || v.inspector == nil {
		return EnvironmentVerificationResult{
			Success:           false,
			Output:            technical.Output,
			ErrorReason:       "no hay inspector de entorno configurado para B80",
			Observed:          false,
			ObservationStatus: ObservationInsufficient,
		}
	}

	observation, err := v.inspector.InspectAction(ctx, req)
	if err != nil {
		return EnvironmentVerificationResult{
			Success:           false,
			Output:            technical.Output,
			ErrorReason:       fmt.Sprintf("error al inspeccionar el entorno: %v", err),
			Observed:          false,
			ObservationStatus: ObservationInsufficient,
		}
	}

	status := observation.Status
	// An explicit not_observed status is meaningful evidence about the limits
	// of inspection and must not be overwritten by free-form Details. Likewise,
	// text being present does not establish that the expected condition is false.
	// Legacy inspectors that omit a status may only imply confirmation from the
	// structured Confirmed flag; otherwise the evidence is insufficient.
	if !status.Valid() {
		if observation.Confirmed {
			status = ObservationConfirmed
		} else {
			status = ObservationInsufficient
		}
	}

	// The legacy Confirmed flag and the structured status must agree before
	// the result can be accepted. Contradictory/insufficient/not-observed
	// evidence is never promoted to success by a true boolean alone.
	if status == ObservationConfirmed && !observation.Confirmed {
		status = ObservationInsufficient
	}
	if observation.Confirmed && status == ObservationConfirmed {
		provenanceMatches := observation.ActionID == req.ID && observation.Operation == req.Action && observation.Target == req.Target && observation.Source != ""
		if !provenanceMatches || !observation.IsFresh(time.Now().UTC(), 30*time.Second) {
			return EnvironmentVerificationResult{
				Success: false, Output: technical.Output,
				ErrorReason: "la evidencia ambiental no tiene identidad/procedencia coincidente o está obsoleta",
				Observed:    false, Details: observation.Details, ObservationStatus: ObservationInsufficient, Observation: observation,
			}
		}
	}
	if !observation.Confirmed || status != ObservationConfirmed || !observation.HasSufficientConfirmedEvidence() {
		return EnvironmentVerificationResult{
			Success:           false,
			Output:            technical.Output,
			ErrorReason:       "la ejecución técnica terminó correctamente, pero la evidencia ambiental no confirma el resultado",
			Observed:          true,
			Details:           observation.Details,
			ObservationStatus: status,
			Observation:       observation,
		}
	}

	return EnvironmentVerificationResult{
		Success:           true,
		Output:            technical.Output,
		Observed:          true,
		Details:           observation.Details,
		ObservationStatus: ObservationConfirmed,
		Observation:       observation,
	}
}
