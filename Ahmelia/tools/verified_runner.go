package tools

import (
	"context"
	"fmt"
)

// ============================================================================
// VERIFIED_RUNNER — Bloque 94.5 (Execute -> Observe -> Verify)
// ============================================================================
//
// B94.4 creó la frontera de ejecución. B94.5 añade el cierre epistemológico:
// ejecutar algo NO significa que haya ocurrido correctamente. Cada intento
// pasa por Execute -> Observe -> Verify y solo un Verify positivo produce
// éxito.
//
// El paquete mantiene las responsabilidades separadas:
//   - Runner resuelve Tool -> Executor.
//   - Observer obtiene evidencia del entorno después de ejecutar.
//   - Verifier decide si esa evidencia satisface el resultado esperado.
//   - VerifiedRunner orquesta los pasos y, opcionalmente, reintenta.
//
// No contiene heurísticas sobre texto, no diagnostica ni modifica acciones.
// Un diagnoser/replanner superior puede decidir cómo construir el siguiente
// Action cuando un intento no verifica.

// Observation es evidencia devuelta por el entorno. El contenido es deliberadamente
// genérico: un entorno concreto puede representar filesystem, Unity, Blender,
// terminal, etc.
type Observation struct {
	Confirmed bool
	Details   string
}

// Observer inspecciona el entorno después de un intento de ejecución.
type Observer interface {
	Observe(context.Context, Action, Result) (Observation, error)
}

// Verification contiene el dictamen del verificador sobre una observación.
type Verification struct {
	Success     bool
	Reason      string
	Observation Observation
}

// Verifier determina si la evidencia observada demuestra que la Action
// consiguió su resultado esperado.
type Verifier interface {
	Verify(context.Context, Action, Result, Observation) Verification
}

// Attempt registra un intento completo y permite auditar qué ocurrió en cada
// vuelta sin inferir éxito desde Result.Success.
type Attempt struct {
	Number       int
	Execution    Result
	Observation  Observation
	Verification Verification
}

// VerifiedResult es el resultado final de una ejecución verificada.
type VerifiedResult struct {
	ActionID string
	Success  bool
	Attempts []Attempt
	Reason   string
}

// VerifiedRunner conecta B94.4 con la observación y verificación de B94.5.
type VerifiedRunner struct {
	runner     *Runner
	observer   Observer
	verifier   Verifier
	maxRetries int
}

// NewVerifiedRunner crea un runner con un máximo de reintentos. maxRetries=0
// significa exactamente un intento; nunca hay reintentos implícitos.
func NewVerifiedRunner(runner *Runner, observer Observer, verifier Verifier, maxRetries int) *VerifiedRunner {
	if maxRetries < 0 {
		maxRetries = 0
	}
	return &VerifiedRunner{
		runner:     runner,
		observer:   observer,
		verifier:   verifier,
		maxRetries: maxRetries,
	}
}

// Execute ejecuta, observa y verifica. Un intento fallido puede repetirse,
// pero siempre con la misma Action: cambiar parámetros, reparar o replantear
// corresponde a una capa superior y no se inventa aquí.
func (r *VerifiedRunner) Execute(ctx context.Context, action Action, env Environment) VerifiedResult {
	result := VerifiedResult{ActionID: action.ID}
	if r == nil || r.runner == nil {
		result.Reason = "verified runner: runner nil"
		return result
	}
	if r.observer == nil {
		result.Reason = "verified runner: observer nil"
		return result
	}
	if r.verifier == nil {
		result.Reason = "verified runner: verifier nil"
		return result
	}

	attempts := r.maxRetries + 1
	for i := 1; i <= attempts; i++ {
		if err := ctx.Err(); err != nil {
			result.Reason = "contexto cancelado: " + err.Error()
			return result
		}

		execution := r.runner.Execute(ctx, action, env)
		observation, observeErr := r.observer.Observe(ctx, action, execution)
		if observeErr != nil {
			result.Attempts = append(result.Attempts, Attempt{
				Number:      i,
				Execution:   execution,
				Observation: observation,
				Verification: Verification{
					Success:     false,
					Reason:      fmt.Sprintf("error al observar: %v", observeErr),
					Observation: observation,
				},
			})
			result.Reason = fmt.Sprintf("error al observar después del intento %d: %v", i, observeErr)
			continue
		}

		verification := r.verifier.Verify(ctx, action, execution, observation)
		if verification.Observation != observation {
			verification.Observation = observation
		}

		result.Attempts = append(result.Attempts, Attempt{
			Number:       i,
			Execution:    execution,
			Observation:  observation,
			Verification: verification,
		})

		if verification.Success {
			result.Success = true
			result.Reason = verification.Reason
			return result
		}

		result.Reason = verification.Reason
	}

	return result
}
