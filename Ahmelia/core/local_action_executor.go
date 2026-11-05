package core

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"ahmelia/production"
	"ahmelia/solving"
	"ahmelia/system"
)

// ExecuteCapabilityAction is the provider-side adapter for the legacy local
// action implementation. The central execution authority never calls the
// physical Engine executor directly; it resolves this adapter through the
// generic production.Provider boundary.
func (e *Engine) ExecuteCapabilityAction(ctx context.Context, req production.ProviderRequest) production.ProviderResult {
	if e == nil {
		return production.ProviderResult{Success: false, FailureReason: "engine_unavailable"}
	}
	if err := ctx.Err(); err != nil {
		return production.ProviderResult{Success: false, FailureReason: "execution_context_cancelled", Err: err}
	}

	action := strings.TrimSpace(req.Operation)
	if action == "" {
		return production.ProviderResult{Success: false, FailureReason: "local_action_operation_missing"}
	}

	// Shutdown is itself a physical capability. It therefore crosses the same
	// provider boundary instead of remaining as an authority-side special case.
	if action == "salir" {
		e.Shutdown()
		// H7 (2026-10-17): pedir el cierre; la raíz viva termina el proceso.
		e.RequestExit()
		return production.ProviderResult{Success: true, Verified: false, Producer: "core.local_action", Summary: "Ahmelia cerró la sesión.", OutcomeCode: "completed", VerificationReason: "el cierre de sesión no tiene verificador de estado conectado"}
	}

	params := cloneStringMap(req.Parameters)
	if params == nil {
		params = make(map[string]string)
	}
	if params["raw"] == "" && strings.TrimSpace(req.Input) != "" {
		params["raw"] = req.Input
	}

	// ProviderRequest.Target is the canonical target of the capability.
	// Some legacy local rename operations predate ProviderRequest and encode the
	// source path as an explicit parameter. Normalize only that legacy contract
	// once at the provider adapter. Move operations intentionally keep origin and
	// destination separate because their Target is the destination.
	if action == "renombrar_archivo" && strings.TrimSpace(params["origen"]) == "" && strings.TrimSpace(params["source"]) == "" && strings.TrimSpace(req.Target) != "" {
		params["origen"] = strings.TrimSpace(req.Target)
	}

	// Volume operations now carry a before-state into the post-execution
	// observer. This is evidence, not a second decision authority.
	if isVolumeAction(action) {
		if volume, err := system.GetVolumePercent(); err == nil {
			params["__volume_before"] = fmt.Sprintf("%d", volume)
		}
		if muted, err := system.GetMuted(); err == nil {
			params["__muted_before"] = strconv.FormatBool(muted)
		}
	}

	// Paso 4.1: la expectativa se declara ANTES de cruzar la ejecución y el
	// veredicto se emite DESPUÉS de la observación independiente. El resultado
	// del ejecutor nunca se usa como prueba de sí mismo.
	declared, expected, declareErr := e.declareLocalActionExpectation(action, req, params)
	result := e.execute(action, req.Target, params)
	result = verifyLocalActionResult(action, params, result)
	return e.concludeLocalActionExpectation(action, params, result, declared, expected, declareErr)
}

// verifyLocalActionResult attaches the canonical postcondition evidence to the
// same physical execution result. It does not execute anything a second time.
// The solving verification engine is the owner of filesystem postconditions.
func verifyLocalActionResult(action string, params map[string]string, result production.ProviderResult) production.ProviderResult {
	if !result.Success {
		result.OutcomeStatus = production.OutcomeFailed
		return result
	}

	if action == "crear_carpeta" || action == "mover_archivo" || action == "renombrar_archivo" {
		execResult := &solving.ExecutionResult{Action: action, Outcome: solving.OutcomeExecuted, Detail: result.Summary}
		verification := solving.Verify(action, params, execResult, nil)
		result.Observed = true
		result.Verified = verification.Confirmed()
		result.VerificationSource = "solving.verification_engine"
		result.VerificationReason = verification.Detail
		result.ObservationDetails = verification.Detail
		if result.Verified {
			result.Success = true
			result.OutcomeStatus = production.OutcomeSucceeded
			result.OutcomeCode = "completed_verified"
			return result
		}
		// The filesystem was actually observed and contradicted the expected
		// postcondition. This is a known failure, not an unknown observation.
		result.Success = false
		result.OutcomeStatus = production.OutcomeFailed
		result.OutcomeCode = "behavioral_verification_failed"
		result.FailureReason = verification.Detail
		return result
	}

	if isVolumeAction(action) {
		before, beforeErr := strconv.Atoi(strings.TrimSpace(params["__volume_before"]))
		beforeOK := beforeErr == nil
		after, afterErr := system.GetVolumePercent()
		muted, muteErr := system.GetMuted()
		result.Observed = afterErr == nil && muteErr == nil
		result.VerificationSource = "system.audio_observer"
		if afterErr != nil || muteErr != nil {
			result.Verified = false
			result.OutcomeStatus = production.OutcomeUnknown
			result.OutcomeCode = "completed_unverified"
			result.VerificationReason = "no se pudo leer suficientemente el estado real del audio"
			result.ObservationDetails = result.VerificationReason
			return result
		}

		verified, knownFailure, detail := evaluateVolumeObservation(action, before, beforeOK, after, muted, func() int { v, _ := extractExpectedVolume(params); return v }(), func() bool { _, ok := extractExpectedVolume(params); return ok }())
		result.Verified = verified
		result.ObservationDetails = detail
		result.VerificationReason = detail
		if verified {
			result.Success = true
			result.OutcomeStatus = production.OutcomeSucceeded
			result.OutcomeCode = "completed_verified"
		} else if knownFailure {
			// The observer read the real state but it contradicts the requested
			// postcondition. The outcome is therefore known-failed.
			result.Success = false
			result.OutcomeStatus = production.OutcomeFailed
			result.OutcomeCode = "behavioral_verification_failed"
			result.FailureReason = detail
		} else {
			result.Success = true
			result.OutcomeStatus = production.OutcomeUnknown
			result.OutcomeCode = "completed_unverified"
		}
		return result
	}

	// Legacy local actions that have no environment observer remain explicitly
	// unverified. The physical provider result is preserved instead of being
	// rewritten as a failure merely because evidence is unavailable.
	result.Success = true
	result.OutcomeStatus = production.OutcomeUnknown
	result.OutcomeCode = "completed_unverified"
	result.Verified = false
	result.Observed = false
	if result.VerificationReason == "" {
		result.VerificationReason = "el proveedor local no tiene un observador de estado conectado"
	}
	return result
}

func evaluateVolumeObservation(action string, before int, beforeOK bool, after int, muted bool, expected int, expectedOK bool) (verified bool, knownFailure bool, detail string) {
	switch action {
	case "volumen_bajar":
		detail = fmt.Sprintf("volumen antes=%d despues=%d; se esperaba una disminución", before, after)
		if !beforeOK {
			return false, false, "no se pudo obtener el volumen anterior para determinar si disminuyó"
		}
		return after < before, after >= before, detail
	case "volumen_subir":
		detail = fmt.Sprintf("volumen antes=%d despues=%d; se esperaba un aumento", before, after)
		if !beforeOK {
			return false, false, "no se pudo obtener el volumen anterior para determinar si aumentó"
		}
		return after > before, after <= before, detail
	case "volumen_set":
		detail = fmt.Sprintf("volumen observado=%d; esperado=%d", after, expected)
		if !expectedOK {
			return false, false, "no se pudo determinar el porcentaje de volumen solicitado"
		}
		return after == expected, after != expected, detail
	case "volumen_max":
		detail = fmt.Sprintf("volumen observado=%d; esperado=100", after)
		return after == 100, after != 100, detail
	case "volumen_silencio":
		detail = fmt.Sprintf("mute observado=%t; esperado=true", muted)
		return muted, !muted, detail
	default:
		return false, false, "acción de volumen no reconocida"
	}
}

func isVolumeAction(action string) bool {
	switch action {
	case "volumen_subir", "volumen_bajar", "volumen_silencio", "volumen_max", "volumen_set":
		return true
	default:
		return false
	}
}

func extractExpectedVolume(params map[string]string) (int, bool) {
	raw := ""
	if params != nil {
		raw = params["percent"]
		if raw == "" {
			raw = params["raw"]
		}
	}
	return system.ExtractVolumePercent(raw)
}
