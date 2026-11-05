package solving

import (
	"fmt"

	"ahmelia/capability"
	"ahmelia/files"
	"ahmelia/shared"
)

// ============================================================================
// EXECUTION_ENGINE.GO — Bloque 66 (Execution Engine)
// ============================================================================
//
// FRONTERA REAL, auditada antes de escribir una sola línea acá (y por
// eso este bloque NO es el ejemplo de plan de 4 pasos con comodines
// que motivó el pedido):
//
//   - No existe ninguna estructura de "Plan" (lista ordenada de
//     Operations de una Skill) en el proyecto. planning.Plan es un
//     sistema distinto (objetivos/tareas con estimaciones de tiempo),
//     nada que ver con ejecutar los Operations de una Skill. Ejecutar
//     un Plan completo es un bloque futuro que depende de que ESTE
//     exista primero — un motor que sepa ejecutar UNA operación es el
//     paso anterior a poder ejecutar una secuencia.
//   - No existe coincidencia por comodines (`*.pdf`) en ningún lado.
//     files.MoveFile toma una ruta exacta. Inventar un resolutor de
//     glob acá sería fabricar una capacidad que no se pidió con
//     justificación real — se deja fuera a propósito.
//   - GoalResolution (62) resuelve UNA acción/capacidad, nunca un plan
//     de varios pasos — coherente con el alcance de este bloque.
//
// Lo que SÍ existe y este bloque conecta de verdad:
//   - shared.WiredActions / shared.ActionSafety (65) — para saber si
//     una Action tiene lógica real y qué tier de riesgo le corresponde.
//   - files.CreateFolder / files.MoveFile / files.RenameFile — las
//     únicas Action con una función Go ejecutable SIN depender del
//     estado de un *core.Engine (sesión de voz, último archivo
//     seleccionado, etc.). abrir_programa/volumen_*/minimizar/
//     cambiar_ventana/cerrar_ventana/reproducir_media siguen viviendo
//     únicamente en core/engine.go — ejecutarlas desde acá exigiría
//     inventar ese estado, así que este motor las rechaza explícita y
//     honestamente en vez de fingir que las soporta.
//
// renombrar_archivo ya tiene binding tanto en Engine como en el executor
// standalone. El catálogo canónico refleja ambos caminos; este motor solo
// proyecta el atributo StandaloneExecutable y no mantiene una lista propia.
//
// FLUJO, tal como se pidió:
//
//	Operation → operación identificada → acción concreta → parámetros
//	          → autorización (Bloque 65) → ejecución → resultado
//
// Deliberadamente NO interpreta texto de un tutorial como comandos:
// Execute() no recibe una frase suelta ni un Step de una Skill, recibe
// una Action ya identificada (mismo Action que Operation.Action, 54) y
// un mapa de parámetros YA EXTRAÍDOS por el llamador — este bloque no
// hace NLP ni adivina parámetros a partir de texto libre. Esa
// separación es la garantía real de "no ejecutar un tutorial
// directamente": entre "esto es lo que dice el texto" y "esto es lo
// que se va a ejecutar" siempre hay un paso explícito de resolución
// que un humano (o un bloque de resolución futuro) decide, no este.

// executableActions es una proyección del catálogo canónico. La lista de
// qué acciones puede ejecutar este motor no se mantiene en paralelo: la
// metadata StandaloneExecutable de actioncatalog/capability define la
// frontera y este mapa solo la materializa para compatibilidad con el motor.
var executableActions = capability.StandaloneExecutableActions()

// ExecutionRequest es una Operation ya resuelta a acción concreta +
// parámetros explícitos — nunca una frase de texto libre. Confirmed
// declara que el llamador YA obtuvo la confirmación/autorización que
// shared.RequiresConfirmation/RequiresAuthorization exijan para esta
// Action; Execute() nunca la pide por su cuenta (no tiene con qué
// preguntarle a nadie) — solo decide, con esa bandera, si ejecuta o
// devuelve por qué no.
type ExecutionRequest struct {
	Action    string
	Params    map[string]string
	Confirmed bool
}

// ExecutionOutcome documenta por qué se ejecutó o no — nunca deja
// ambigüedad entre "no se ejecutó porque falló" y "no se ejecutó
// porque hacía falta autorización que no llegó".
type ExecutionOutcome string

const (
	OutcomeExecuted           ExecutionOutcome = "ejecutado"
	OutcomeNeedsConfirmation  ExecutionOutcome = "necesita_confirmacion"
	OutcomeNeedsAuthorization ExecutionOutcome = "necesita_autorizacion"
	OutcomeNotExecutable      ExecutionOutcome = "no_ejecutable"
	OutcomeMissingParams      ExecutionOutcome = "faltan_parametros"
	OutcomeFailed             ExecutionOutcome = "fallo"
)

// ExecutionResult es lo único que Execute() devuelve — nunca un error
// crudo de Go sin contexto de qué tier de seguridad lo gateó.
type ExecutionResult struct {
	Action  string
	Tier    shared.SafetyTier
	Outcome ExecutionOutcome
	Detail  string // mensaje humano: qué pasó o por qué no se ejecutó
}

func (r *ExecutionResult) Executed() bool {
	return r.Outcome == OutcomeExecuted
}

// Execute resuelve una ExecutionRequest siguiendo el flujo declarado
// arriba. No hace NLP, no infiere parámetros, no decide por su cuenta
// si algo está "confirmado" — solo aplica la política ya decidida en
// Bloque 65 contra lo que el llamador le pasó.
func Execute(req ExecutionRequest) *ExecutionResult {
	action := req.Action
	tier := shared.SafetyTierOf(action)

	// 1. ¿Existe siquiera una función real para esta Action? Fail-safe:
	// una Action no reconocida acá NUNCA se ejecuta, sin importar
	// Confirmed — evita que un caller optimista dispare algo que este
	// motor en realidad no sabe hacer.
	if !executableActions[action] {
		return &ExecutionResult{
			Action:  action,
			Tier:    tier,
			Outcome: OutcomeNotExecutable,
			Detail:  fmt.Sprintf("%q no tiene una acción ejecutable conectada en este motor todavía", action),
		}
	}

	// 2. Autorización (Bloque 65) — el nivel más estricto, gatea antes
	// que confirmación.
	if shared.RequiresAuthorization(action) && !req.Confirmed {
		return &ExecutionResult{
			Action:  action,
			Tier:    tier,
			Outcome: OutcomeNeedsAuthorization,
			Detail:  fmt.Sprintf("%q es riesgo %q — requiere autorización explícita antes de ejecutar", action, tier),
		}
	}

	// 3. Confirmación (Bloque 65) — menos estricta que autorización,
	// pero igual bloquea si no llegó.
	if shared.RequiresConfirmation(action) && !req.Confirmed {
		return &ExecutionResult{
			Action:  action,
			Tier:    tier,
			Outcome: OutcomeNeedsConfirmation,
			Detail:  fmt.Sprintf("%q es riesgo %q — requiere confirmación antes de ejecutar", action, tier),
		}
	}

	// 4. Resolver acción real → parámetros → ejecutar.
	return executeAction(action, req.Params, tier)
}

// executeAction hace el despacho final a la función real de files/.
// Separado de Execute() para que la política de seguridad (arriba) y
// la ejecución concreta (acá) sean dos responsabilidades que se leen
// por separado — mismo motivo que Operation separa Status de Action.
func executeAction(action string, params map[string]string, tier shared.SafetyTier) *ExecutionResult {
	fail := func(format string, a ...any) *ExecutionResult {
		return &ExecutionResult{Action: action, Tier: tier, Outcome: OutcomeMissingParams, Detail: fmt.Sprintf(format, a...)}
	}

	switch action {
	case "crear_carpeta":
		path, ok := params["ruta"]
		if !ok || path == "" {
			return fail("crear_carpeta necesita el parámetro %q", "ruta")
		}
		if err := files.CreateFolder(path); err != nil {
			return &ExecutionResult{Action: action, Tier: tier, Outcome: OutcomeFailed, Detail: err.Error()}
		}
		return &ExecutionResult{Action: action, Tier: tier, Outcome: OutcomeExecuted, Detail: fmt.Sprintf("carpeta creada: %s", path)}

	case "mover_archivo":
		source, ok := params["origen"]
		if !ok || source == "" {
			return fail("mover_archivo necesita el parámetro %q", "origen")
		}
		dest, ok := params["destino"]
		if !ok || dest == "" {
			return fail("mover_archivo necesita el parámetro %q", "destino")
		}
		if err := files.MoveFile(source, dest); err != nil {
			return &ExecutionResult{Action: action, Tier: tier, Outcome: OutcomeFailed, Detail: err.Error()}
		}
		return &ExecutionResult{Action: action, Tier: tier, Outcome: OutcomeExecuted, Detail: fmt.Sprintf("movido: %s → %s", source, dest)}

	case "renombrar_archivo":
		oldPath, ok := params["ruta"]
		if !ok || oldPath == "" {
			return fail("renombrar_archivo necesita el parámetro %q", "ruta")
		}
		newName, ok := params["nombre_nuevo"]
		if !ok || newName == "" {
			return fail("renombrar_archivo necesita el parámetro %q", "nombre_nuevo")
		}
		if err := files.RenameFile(oldPath, newName); err != nil {
			return &ExecutionResult{Action: action, Tier: tier, Outcome: OutcomeFailed, Detail: err.Error()}
		}
		return &ExecutionResult{Action: action, Tier: tier, Outcome: OutcomeExecuted, Detail: fmt.Sprintf("renombrado: %s → %s", oldPath, newName)}

	default:
		// No debería llegar acá — executableActions ya filtró. Si
		// llega, es un bug de esta función (nueva Action agregada a
		// executableActions sin su case acá), no un input inválido.
		return &ExecutionResult{Action: action, Tier: tier, Outcome: OutcomeNotExecutable, Detail: fmt.Sprintf("%q está en executableActions pero no tiene case en executeAction — bug de este archivo", action)}
	}
}
