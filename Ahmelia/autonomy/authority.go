package autonomy

// ============================================================================
// AUTHORITY.GO — Bloque 88.5 (Decisional Authority Contract)
// ============================================================================
//
// Motivo real (auditoría previa a este bloque, confirmada leyendo el código,
// no supuesta): AutonomousLoop.runTick() llamaba a Cycle.RunStep() (B78-B88)
// en cada tick sin importar lo que InitiativeEngine (B75) hubiera decidido.
// El camino conversacional legado sí consultaba a InitiativeEngine, pero
// solo para decidir CÓMO hablarle al usuario — nunca para decidir si el
// ciclo ejecutivo debía avanzar. Es decir: WAIT y NO_ACTION existían en B75
// desde el principio, pero no frenaban nada. Dos caminos independientes,
// uno de los cuales ignoraba por completo al otro.
//
// Este archivo hace dos cosas, deliberadamente separadas:
//
//  1. Un vocabulario compartido de "niveles de autoridad" (DecisionAuthority)
//     que nombra y prueba una regla que ya debería cumplirse implícitamente
//     en todo el proyecto: recomendar/inferir/evaluar/planear NO es lo mismo
//     que autorizar, y autorizar no es lo mismo que ejecutar. No se
//     reestructura reasoning/, solving/, GoalSelector ni ExecutorAdapter
//     para que reporten su autoridad explícitamente — eso tocaría paquetes
//     que hoy no tienen ninguna razón para conocer autonomy/, a cambio de
//     un beneficio que ningún caller real necesita todavía. Lo único que
//     se formaliza acá es el contrato y su prueba: CanExecute().
//
//  2. Un puente real y concreto entre B75 (InitiativeEngine, que devuelve
//     DecisionAction: NO_ACTION/WAIT/ASK_USER/INFORM_USER/SUGGEST/EXECUTE/
//     RESUME_GOAL — los siete valores que initiative_engine.go declara hoy,
//     ni uno más) y el ciclo ejecutivo de B88. Esta es la parte que corrige
//     el hallazgo de la auditoría de verdad: AllowsCycleExecution(), usada
//     desde autonomous_loop.go para que WAIT/NO_ACTION/ASK_USER/INFORM_USER/
//     SUGGEST bloqueen a Cycle.RunStep() en ese tick, y solo RESUME_GOAL (o
//     EXECUTE, valor reservado que Evaluate() todavía no produce) lo dejen
//     avanzar.
//
// Deliberadamente AUSENTE de este archivo:
//   - Un sistema de permisos que envuelva cada llamada real de Reasoning,
//     Solving, GoalSelector o Executor. Esos paquetes no reciben ningún
//     cambio en este bloque.
//   - B89 (CapabilityResolver). La tabla de autoridad completa lo menciona
//     como destino futuro ("Task -> capacidad/acción"), pero ese bloque
//     decide qué significa eso — acá no se inventa su forma.
//   - Cualquier intento de que este archivo decida POR los componentes
//     reales qué autoridad tienen. AuthorityForInitiativeAction traduce el
//     valor que InitiativeEngine YA devuelve; no le exige a InitiativeEngine
//     que declare su propia autoridad, porque B75 no expone ese campo hoy.

// DecisionAuthority nombra cada nivel de la cadena real de autoridad del
// proyecto (Observación -> Recomendación -> Iniciativa -> Selección de
// Goal -> Planificación -> Resolución de capacidad (B89) -> Ejecución ->
// Verificación -> Recuperación). No todos los niveles tienen hoy un
// component real que los reporte explícitamente — ver AUSENTE arriba — pero
// nombrarlos permite que Decide()/Evaluate() de cualquier bloque futuro se
// ubique en la cadena sin inventar su propio vocabulario.
type DecisionAuthority string

const (
	// AuthorityNone: no hay nada que recomendar, decidir ni ejecutar.
	AuthorityNone DecisionAuthority = "none"

	// AuthorityRecommend: inferir o sugerir sin comprometer ninguna acción
	// (reasoning.Decider, solving cuando solo evalúa viabilidad, y las
	// salidas conversacionales de InitiativeEngine: ASK_USER/INFORM_USER/
	// SUGGEST — todas piden hablarle al usuario, ninguna autoriza trabajo
	// autónomo).
	AuthorityRecommend DecisionAuthority = "recommend"

	// AuthorityInitiate: decidir que HAY motivo para que el ciclo ejecutivo
	// avance ahora — sin elegir qué Goal, qué Task ni qué Action. Es
	// exactamente lo que InitiativeEngine.Evaluate() ya hace hoy con
	// RESUME_GOAL (y, si algún día lo produce, EXECUTE).
	AuthorityInitiate DecisionAuthority = "initiate"

	// AuthoritySelectGoal: elegir CUÁL objetivo activo trabajar (GoalSelector,
	// B86) sin ejecutarlo.
	AuthoritySelectGoal DecisionAuthority = "select_goal"

	// AuthorityPlan: descomponer un Goal en Tasks (planning.Planner) sin
	// ejecutarlas.
	AuthorityPlan DecisionAuthority = "plan"

	// AuthorityResolve: traducir una Task a una capacidad/acción concreta
	// (B89, todavía no implementado) sin ejecutarla.
	AuthorityResolve DecisionAuthority = "resolve"

	// AuthorityExecute: el único nivel que autoriza tocar el mundo real
	// (ExecutorAdapter/AutonomyExecutor, B79).
	AuthorityExecute DecisionAuthority = "execute"

	// AuthorityVerify: confirmar evidencia de una ejecución ya ocurrida
	// (ActionVerifier/EnvironmentVerifier, B78/B80) sin decidir nuevos
	// objetivos a partir de esa evidencia.
	AuthorityVerify DecisionAuthority = "verify"

	// AuthorityRecover: explicar/decidir cómo seguir tras un fallo
	// (Replanner, B87) sin ejecutar directamente esa recuperación — la
	// ejecución de lo que Replanner decide vuelve a pasar por
	// AuthorityExecute en la siguiente vuelta del ciclo.
	AuthorityRecover DecisionAuthority = "recover"
)

// AuthorityDecision es un veredicto auditable: qué componente (Source)
// ejerció qué nivel de autoridad, si eso alcanza para ejecutar (Allowed) y
// por qué (Reason). Se arma siempre con NewAuthorityDecision para que Allowed
// no pueda declararse a mano — ver comentario de esa función.
type AuthorityDecision struct {
	Source    string
	Authority DecisionAuthority
	Allowed   bool
	Reason    string
}

// CanExecute es la regla central del contrato de B88.5:
//
//	Ningún componente puede ejecutar una acción solo porque produjo una
//	recomendación.
//
// Solo AuthorityExecute autoriza a tocar el mundo real. Todo lo demás —por
// más convencido que esté un componente de su propia recomendación—
// se queda en eso: una recomendación, hasta que algo con AuthorityExecute
// la tome.
func CanExecute(authority DecisionAuthority) bool {
	return authority == AuthorityExecute
}

// NewAuthorityDecision arma un veredicto auditable. Allowed lo calcula
// CanExecute, nunca el caller: ningún Source puede auto-otorgarse permiso
// de ejecución declarando Allowed:true a mano al construir el struct.
func NewAuthorityDecision(source string, authority DecisionAuthority, reason string) AuthorityDecision {
	return AuthorityDecision{
		Source:    source,
		Authority: authority,
		Allowed:   CanExecute(authority),
		Reason:    reason,
	}
}

// ----------------------------------------------------------------------------
// Puente real: InitiativeEngine (B75) -> ciclo ejecutivo (B88).
// ----------------------------------------------------------------------------

// AuthorityForInitiativeAction traduce el veredicto real de
// InitiativeEngine.Evaluate() a un nivel de autoridad. Cubre EXACTAMENTE los
// siete DecisionAction que initiative_engine.go declara hoy. Si B75 agrega
// un valor nuevo sin que alguien decida a propósito qué autoridad le
// corresponde, el default (AuthorityNone) lo deja bloqueado — no hereda
// ejecución por accidente.
func AuthorityForInitiativeAction(action DecisionAction) DecisionAuthority {
	switch action {
	case ActionResumeGoal, ActionExecute:
		// InitiativeEngine solo decide SI hay motivo para actuar ahora — no
		// elige la acción ni la ejecuta. Que esto sea AuthorityInitiate y NO
		// AuthorityExecute es la regla central de este puente: la autoridad
		// de ejecutar la sigue ejerciendo ExecutorAdapter dentro de Cycle,
		// después de Planning.
		return AuthorityInitiate
	case ActionAskUser, ActionInformUser, ActionSuggest:
		// Las tres son, por definición, para hablarle al usuario — no
		// autorizan ningún trabajo autónomo sobre el entorno real.
		return AuthorityRecommend
	case ActionWait, ActionNoAction:
		return AuthorityNone
	default:
		return AuthorityNone
	}
}

// AllowsCycleExecution decide si una InitiativeDecision de B75 habilita que
// el ciclo ejecutivo (Cycle.RunStep, B78-B88) avance en el tick actual.
//
// Es la corrección concreta del hallazgo de auditoría de B88.5: antes de
// este bloque, AutonomousLoop.runTick() invocaba Cycle.RunStep() sin
// importar la decisión de InitiativeEngine — WAIT y NO_ACTION nunca
// bloqueaban nada. Ver autonomous_loop.go, runIntegratedTick.
func AllowsCycleExecution(decision InitiativeDecision) bool {
	return AuthorityForInitiativeAction(decision.Action) == AuthorityInitiate
}
