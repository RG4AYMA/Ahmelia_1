package solving

import (
	"fmt"
	"strings"
)

// ============================================================================
// GOAL_DECOMPOSITION.GO — Bloque 69 (Goal Decomposition)
// ============================================================================
//
// AUDITORÍA REAL primero, porque el ejemplo que motivó este bloque
// ("prepara el entorno para desarrollar mi juego" → verificar Unity /
// VS Code / Git / crear estructura / comprobar que todo funciona)
// esconde una suposición grande: que Ahmelia puede mirar una frase
// abstracta NUNCA vista antes y inventar, de cero, que "entorno de
// juego" significa esos 5 pasos concretos. Esa capacidad —traducir un
// concepto de dominio a una lista de tareas sin haber visto antes esa
// lista en ningún lado— es comprensión semántica real. No existe en
// este proyecto (grep de NLP/inferencia de dominio: nada — cada bloque
// de 54 en adelante lo dice explícito) y fabricarla acá sería
// exactamente lo que "sin inventar" viene prohibiendo bloque tras
// bloque. Este motor NO la fabrica.
//
// LO QUE SÍ EXISTE, y es la descomposición real y honesta:
// SkillBuilder (54) YA parte los Steps de una fuente en Operations
// (detectOperations) — "verificar Unity" se vuelve un subobjetivo
// real SOLO si alguna vez se aprendió una Skill (una nota guardada,
// producto de 47-61) cuyo procedimiento tenía ese paso escrito. Es
// decir: la descomposición no nace de razonar sobre el pedido en
// abstracto, nace de que el conocimiento estructurado YA existía,
// guardado, de antes. Sin esa Skill, no hay 5 subobjetivos que
// inventar — hay que decirlo así de claro, no simular que se
// encontraron.
//
// Este bloque conecta lo que YA hay:
//   1. ResolveGoal (62) — resuelve el pedido a una Action local
//      (trivial, un solo "subobjetivo") o a una Skill con
//      Operations ya detectadas (54).
//   2. Cada Operation de esa Skill (Text/Action/Status, 54) se expone
//      como un Subgoal — la lista real que la fuente aprendida tenía,
//      nunca una lista fabricada por este motor.
//   3. Cada Subgoal, si su Action es de verdad ejecutable (66:
//      executableActions — un subconjunto de OpWired, ver nota
//      abajo), pasa por Execute (66) → Verify (67) → Diagnose (68) si
//      falla, EN SECUENCIA, deteniéndose en el primer subobjetivo que
//      no confirme. El resto queda marcado Skipped — nunca se asume
//      que un paso posterior habría funcionado.
//
// DISTINCIÓN IMPORTANTE, auditada contra 54/65/66: Operation.Status ==
// OpWired significa "tiene un case en core/engine.go"
// (shared.WiredActions) — un criterio MÁS AMPLIO que
// executableActions (66), que son solo las 3 Action con función Go
// invocable sin estado de Engine. Un Subgoal puede venir OpWired (54)
// y aun así no ser ejecutable por ESTE motor (ej. "abrir Unity" —
// wired en el sentido de 54 si "abrir_programa" matchea, pero
// requiere abrir_programa vía Engine.handleAction, no vía este
// pipeline). Se reporta esa distinción explícitamente en cada
// Subgoal, no se pisa una por la otra.
//
// PARÁMETROS: ni Decompose ni ExecuteDecomposition inventan Params
// para un Subgoal a partir de su Text — mismo límite que 66 (Execute
// nunca infiere parámetros de texto libre). Params se pasan desde
// afuera, uno por Subgoal, en el mismo orden — este bloque orquesta
// la secuencia, no adivina qué carpeta o qué archivo corresponde a
// cada paso.

// SubgoalStatus documenta, sin ambigüedad, qué pasó con cada paso de
// la descomposición.
type SubgoalStatus string

const (
	// SubgoalPending — identificado, todavía no se intentó ejecutar
	// (estado inicial de Decompose(), antes de ExecuteDecomposition()).
	SubgoalPending SubgoalStatus = "pendiente"
	// SubgoalConfirmed — se ejecutó y Verify (67) confirmó la
	// postcondición real.
	SubgoalConfirmed SubgoalStatus = "confirmado"
	// SubgoalFailed — se ejecutó pero falló (Execute, 66) o se
	// ejecutó sin error pero no se pudo confirmar el resultado real
	// (Verify, 67). Detiene la secuencia.
	SubgoalFailed SubgoalStatus = "fallido"
	// SubgoalBlocked — nunca llegó a ejecutarse: requiere
	// confirmación/autorización (65), le faltan parámetros, o su
	// Action no es ejecutable por este motor (66). Detiene la
	// secuencia igual que Failed — un bloqueo no resuelto es tan
	// parada como un error.
	SubgoalBlocked SubgoalStatus = "bloqueado"
	// SubgoalSkipped — nunca se intentó porque un Subgoal anterior en
	// la secuencia no confirmó. No es "no ejecutable" ni "fallido" —
	// es honesto sobre POR QUÉ nunca se llegó a intentar.
	SubgoalSkipped SubgoalStatus = "omitido"
)

// Subgoal es un paso identificado de la descomposición — nunca
// inventado: viene de una Operation real (54) de una Skill guardada,
// o (para KindLocalAction) del pedido completo tratado como un único
// paso trivial.
type Subgoal struct {
	Text            string          // Operation.Text (54), o la Capability completa si es acción local
	Action          string          // Operation.Action (54), o GoalResolution.LocalAction
	OperationStatus OperationStatus // Wired/Named/Unknown según 54 — NO implica ejecutable por este motor, ver nota arriba
	Executable      bool            // true solo si Action está en executableActions (66) — el criterio real de "este motor lo puede correr"

	Status SubgoalStatus
	Detail string

	ExecResult *ExecutionResult
	VerResult  *VerificationResult
	Diagnosis  *FailureDiagnosis
}

// GoalDecomposition es el resultado completo — la resolución original
// (62) más la lista de Subgoals identificados y, después de llamar
// ExecuteDecomposition, su resultado real.
type GoalDecomposition struct {
	Request      string
	Resolution   *GoalResolution
	Subgoals     []Subgoal
	AllConfirmed bool // true solo si TODOS los Subgoals terminaron Confirmed — false si la lista está vacía (nada que confirmar no es lo mismo que todo confirmado)
}

// Decompose resuelve el pedido (62) e identifica los Subgoals reales
// — no ejecuta nada todavía (mismo principio que ResolveGoal: separar
// "qué hace falta" de "hacerlo"). Con KindKnowledgeOnly/KindUnknown/
// KindAmbiguous no hay Subgoals que identificar — se dice así de
// claro en vez de fabricar una lista.
func Decompose(request string, skills *SkillLibrary) *GoalDecomposition {
	resolution := ResolveGoal(request, skills)
	decomp := &GoalDecomposition{Request: request, Resolution: resolution}

	switch resolution.Kind {
	case KindLocalAction:
		// Descomposición trivial: el pedido YA es una sola acción, no
		// hace falta partirlo en más de un Subgoal.
		decomp.Subgoals = []Subgoal{{
			Text:            resolution.Capability,
			Action:          resolution.LocalAction,
			OperationStatus: OpWired, // por definición: KindLocalAction exige WiredActions[LocalAction] (62)
			Executable:      executableActions[resolution.LocalAction],
			Status:          SubgoalPending,
		}}

	case KindProceduralFull, KindProceduralPartial, KindProceduralUnbacked:
		// Los Subgoals reales son las Operations que 54 ya detectó al
		// construir la Skill — nunca se recalculan ni se inventan acá.
		for _, op := range resolution.Skill.Operations {
			decomp.Subgoals = append(decomp.Subgoals, Subgoal{
				Text:            op.Text,
				Action:          op.Action,
				OperationStatus: op.Status,
				// Executable usa executableActions (66) como única
				// autoridad — NO op.Status == OpWired. Son criterios
				// distintos a propósito (ver header): renombrar_archivo
				// es OpNamed en 54 (nunca tuvo case en engine.go) pero
				// SÍ ejecutable acá porque este motor llama a files/
				// directo. Filtrar por OpWired perdería ese caso real.
				Executable: executableActions[op.Action],
				Status:     SubgoalPending,
			})
		}

	default: // KindKnowledgeOnly, KindAmbiguous, KindUnknown
		// Sin Subgoals — límite honesto: no hay procedimiento
		// estructurado (o ni siquiera una Skill identificada sin
		// ambigüedad) del cual derivar pasos reales.
	}

	return decomp
}

// ExecuteDecomposition corre cada Subgoal EN ORDEN por Execute (66) →
// Verify (67) → Diagnose (68) si falla, deteniéndose en el primero
// que no termine Confirmed. paramsBySubgoal[i] son los parámetros
// explícitos para Subgoals[i] (mismo contrato que ExecutionRequest.Params
// en 66 — nunca inferidos de Text); un índice sin entrada o con params
// vacíos simplemente fallará como faltan_parametros si la Action los
// necesita, igual que Execute() sola. confirmed se propaga tal cual a
// cada ExecutionRequest.
func ExecuteDecomposition(decomp *GoalDecomposition, paramsBySubgoal []map[string]string, confirmed bool) *GoalDecomposition {
	stopped := false
	allConfirmed := len(decomp.Subgoals) > 0

	for i := range decomp.Subgoals {
		sg := &decomp.Subgoals[i]

		if stopped {
			sg.Status = SubgoalSkipped
			sg.Detail = "no se intentó: un subobjetivo anterior en la secuencia no se confirmó"
			allConfirmed = false
			continue
		}

		var params map[string]string
		if i < len(paramsBySubgoal) {
			params = paramsBySubgoal[i]
		}

		execResult := Execute(ExecutionRequest{Action: sg.Action, Params: params, Confirmed: confirmed})
		sg.ExecResult = execResult

		switch execResult.Outcome {
		case OutcomeExecuted:
			verResult := Verify(sg.Action, params, execResult, decomp.Resolution.Skill)
			sg.VerResult = verResult
			if verResult.Confirmed() {
				sg.Status = SubgoalConfirmed
				sg.Detail = verResult.Detail
			} else {
				sg.Status = SubgoalFailed
				sg.Detail = "se ejecutó sin error, pero no se pudo confirmar el resultado real: " + verResult.Detail
				stopped = true
				allConfirmed = false
			}

		case OutcomeFailed:
			sg.Diagnosis = Diagnose(execResult, decomp.Resolution.Skill)
			sg.Status = SubgoalFailed
			sg.Detail = sg.Diagnosis.Describe()
			stopped = true
			allConfirmed = false

		default: // NeedsConfirmation, NeedsAuthorization, MissingParams, NotExecutable
			sg.Status = SubgoalBlocked
			sg.Detail = execResult.Detail
			stopped = true
			allConfirmed = false
		}
	}

	decomp.AllConfirmed = allConfirmed
	return decomp
}

// Describe arma la narrativa completa — el árbol OBJETIVO → subpasos
// del diagrama, pero fiel a lo que de verdad se identificó/ejecutó,
// nunca a lo que "debería" haber si el objetivo fuera obvio.
func (d *GoalDecomposition) Describe() string {
	var b strings.Builder

	if len(d.Subgoals) == 0 {
		fmt.Fprintf(&b, "No puedo descomponer %q en subobjetivos concretos todavía: %s",
			d.Resolution.Capability, d.Resolution.Describe())
		return b.String()
	}

	fmt.Fprintf(&b, "Para %q identifiqué %d subobjetivo(s):\n", d.Resolution.Capability, len(d.Subgoals))
	for i, sg := range d.Subgoals {
		fmt.Fprintf(&b, "%d. %s → %s", i+1, sg.Text, subgoalStatusLabel(sg))
		if sg.Detail != "" && sg.Status != SubgoalPending {
			fmt.Fprintf(&b, " (%s)", sg.Detail)
		}
		b.WriteString("\n")
	}

	if d.AllConfirmed {
		b.WriteString("Todos los subobjetivos se confirmaron.")
	}
	return b.String()
}

func subgoalStatusLabel(sg Subgoal) string {
	if sg.Status == SubgoalPending {
		if sg.Executable {
			return "identificado, ejecutable"
		}
		return fmt.Sprintf("identificado, sin ejecución conectada (%s)", sg.OperationStatus)
	}
	return string(sg.Status)
}
