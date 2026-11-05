package autonomy

import (
	"context"
	"fmt"
	"strings"

	"ahmelia/planning"
	"ahmelia/shared"
	"ahmelia/solving"
)

// ============================================================================
// CAPABILITY_RESOLVER.GO — Bloque 89 (Capability Resolution), primer bloque
// real de la Fase 7
// ============================================================================
//
// EL HUECO QUE AUDITAMOS ANTES DE ESCRIBIR ESTO:
//
//	Goal -> Task -> ??? -> ActionRequest -> Executor
//
// El "???" hoy es ExplicitGoalActionMapper (core/autonomy_b88.go, B88):
// solo acepta una Task cuyo Title/Description YA trae la sintaxis literal
// "action=X target=Y param.clave=valor". Nunca infiere una Action desde
// texto libre — hay un test que verifica exactamente eso como límite
// deliberado de B88 (TestB88ExplicitGoalActionMapperRejectsImplicitAction).
// Ese es el hueco que este bloque cierra, convirtiendo el diagrama en:
//
//	Goal -> Task -> Capability (62) -> Skill/Procedure (54/69) ->
//	Action Pattern (extractAutonomousParams, acá) -> ActionRequest -> Executor
//
// REGLA DE AUTORIDAD HEREDADA DE B88.5 (autonomy/authority.go,
// AuthorityResolve): este archivo NO decide qué Goal perseguir (eso ya lo
// hizo GoalSelector, B86, antes de que la Task llegue acá) y NO decide si
// actuar (eso ya lo autorizó InitiativeEngine + AllowsCycleExecution antes
// de que el ciclo llegue a Planning). CapabilityResolver responde
// EXCLUSIVAMENTE: "dada esta Task ya autorizada, ¿qué capacidad ejecutable
// permite realizarla?" — nunca ejecuta nada él mismo (eso sigue siendo
// AutonomyExecutor, B79, con AuthorityExecute).
//
// INTEGRAR, NO INVENTAR — auditoría real de lo que YA existe y este bloque
// conecta, sin reimplementar nada:
//
//   - solving.Decompose (B69, que a su vez envuelve ResolveGoal B62) YA
//     convierte un pedido en lenguaje natural en una lista de Subgoal, cada
//     uno con su Action, su OperationStatus (Wired/Named/Unknown, B54) y un
//     flag Executable calculado contra exactamente las 3 Action que
//     solving.Execute (B66) puede correr de verdad
//     (crear_carpeta/mover_archivo/renombrar_archivo). Ese es el nodo
//     "Capability -> Skill/Procedure" completo del diagrama — este bloque
//     no reescribe ni un fragmento de esa clasificación.
//
// LO QUE SÍ FALTA Y ESTE BLOQUE SÍ APORTA — el nodo "Action Pattern":
// ni ResolveGoal (62) ni Decompose (69) extraen parámetros concretos de
// texto libre — lo dicen explícito en sus propios headers ("Decompose no
// inventa Params"). Alguien tiene que convertir "crea carpeta
// proyectos_2026" en {"ruta": "proyectos_2026"} antes de que exista un
// ActionRequest real. extractAutonomousParams (acá abajo) es ese paso,
// deliberadamente acotado a las mismas 3 Action que solving.Execute sabe
// correr — ni una más.
//
// DOS LÍMITES HONESTOS, nombrados a propósito en vez de fingidos:
//
//  1. Solo Task de UN SOLO subobjetivo ejecutable. Si Decompose() devuelve
//     más de un Subgoal (una Skill aprendida con varios pasos), este
//     mapper lo rechaza explícitamente — ejecutar el primer subobjetivo y
//     descartar el resto sería mentir sobre qué Task se completó de
//     verdad. Correr una secuencia completa de Subgoals dentro del ciclo
//     autónomo (progreso multi-tick de una misma Task, en vez de un
//     ActionRequest por Task) es un cambio estructural de
//     GoalPlanningAdapter/Cycle que queda fuera de este bloque — B69 ya
//     resuelve la secuencia completa para el camino conversacional
//     (ExecuteDecomposition), pero eso corre sincrónico y de una sola vez,
//     no tick a tick con verificación/aprendizaje real de por medio como
//     exige el ciclo autónomo (B78/B80).
//
//  2. Solo KindLocalAction tiene parámetros extraíbles hoy. Cuando el
//     Subgoal viene de una Skill aprendida (KindProceduralFull/Partial/
//     Unbacked), Operation.Text (54) es SOLO el trigger matcheado (ej.
//     "mueve"), no la oración completa del Step original — el dato para
//     extraer un origen/destino real ya se perdió antes de llegar acá.
//     Ejecutar con parámetros inventados violaría el mismo principio
//     fail-safe que solving.Execute ya aplica (OutcomeMissingParams en vez
//     de adivinar) — así que este mapper rechaza el caso en vez de
//     fabricar un target. Arreglarlo de raíz exige que Skill/Operation
//     conserve el Step de origen de cada Operation — cambio de forma en
//     54/69, no de este bloque.
//
// B37 CIERRE DE CONVERGENCIA: el catálogo canónico ahora declara
// renombrar_archivo como Wired y el resolver lo puede consumir como
// KindLocalAction. La regla es intencional: una capability reconocida y
// ejecutable no puede quedar bloqueada por una lista histórica paralela.

// CapabilityResolver implementa GoalActionMapper (B81) resolviendo el
// texto ya autorizado de una Task contra el mismo vocabulario de
// capacidad que usa el camino conversacional desde B62/B69 — nunca un
// segundo parser inventado para el camino autónomo.
type CapabilityResolver struct {
	skills *solving.SkillLibrary
}

// NewCapabilityResolver arma el resolver sobre la SkillLibrary real del
// proyecto (memory/skill_library.json vía solving.LoadSkillLibrary).
// skills puede ser nil: solving.ResolveGoal/Decompose lo tratan como
// "sin Skills aprendidas todavía" (ver SemanticSkillSearcher.Search),
// nunca panickean — un caller sin Skills cargadas sigue pudiendo
// resolver KindLocalAction.
func NewCapabilityResolver(skills *solving.SkillLibrary) *CapabilityResolver {
	return &CapabilityResolver{skills: skills}
}

// MapTask implementa GoalActionMapper.
func (r *CapabilityResolver) MapTask(ctx context.Context, goal *planning.Goal, task *planning.Task) (ActionRequest, error) {
	if err := ctx.Err(); err != nil {
		return ActionRequest{}, err
	}
	if task == nil || goal == nil {
		return ActionRequest{}, fmt.Errorf("autonomy: goal/task no configurado")
	}

	text := strings.TrimSpace(task.Description)
	if text == "" {
		text = strings.TrimSpace(task.Title)
	}
	if text == "" {
		return ActionRequest{}, fmt.Errorf("autonomy: la tarea %q no tiene texto del que resolver ninguna capacidad", task.ID)
	}

	decomp := solving.Decompose(text, r.skills)

	switch len(decomp.Subgoals) {
	case 0:
		// KindKnowledgeOnly/KindAmbiguous/KindUnknown — Decompose ya
		// documentó por qué no hay subobjetivos; se reusa esa narrativa
		// en vez de fabricar una propia.
		return ActionRequest{}, fmt.Errorf(
			"autonomy: la tarea %q no resolvió a ninguna capacidad ejecutable: %s",
			task.ID, decomp.Resolution.Describe(),
		)
	case 1:
		// sigue abajo — es el único caso que este bloque sabe mapear.
	default:
		return ActionRequest{}, fmt.Errorf(
			"autonomy: la tarea %q resolvió a %d subobjetivos — B89 solo mapea tareas de un único paso ejecutable; una tarea multi-paso queda como límite nombrado para un bloque futuro, no se ejecuta parcialmente",
			task.ID, len(decomp.Subgoals),
		)
	}

	sg := decomp.Subgoals[0]

	if !sg.Executable {
		return ActionRequest{}, fmt.Errorf(
			"autonomy: la tarea %q identificó la capacidad %q (%s) pero no tiene ejecución autónoma real conectada hoy (ver solving.Execute, Bloque 66)",
			task.ID, sg.Text, sg.OperationStatus,
		)
	}

	if decomp.Resolution.Kind != solving.KindLocalAction {
		return ActionRequest{}, fmt.Errorf(
			"autonomy: la tarea %q resolvió contra un procedimiento aprendido (%s) — B89 todavía no extrae parámetros reales de un paso de Skill, solo de una acción local directa; límite nombrado para un bloque futuro",
			task.ID, decomp.Resolution.Skill.EffectiveLabel(),
		)
	}

	params, target, err := extractAutonomousParams(sg.Action, decomp.Resolution.Capability)
	if err != nil {
		return ActionRequest{}, fmt.Errorf("autonomy: tarea %q, capacidad %q: %w", task.ID, sg.Action, err)
	}

	objective := strings.TrimSpace(goal.Description)
	if objective == "" {
		objective = strings.TrimSpace(goal.Name)
	}
	return ActionRequest{
		ID:         task.ID,
		Action:     sg.Action,
		Objective:  objective,
		Target:     target,
		Parameters: params,
	}, nil
}

// ----------------------------------------------------------------------------
// Action Pattern -> parámetros concretos.
// ----------------------------------------------------------------------------

// autonomousParamSeparator es el mismo criterio que classifier.go ya usa
// para partir origen/destino en "mueve X a Y" (ver
// classifier.extractParameters, mover_archivo) — se reusa tal cual en vez
// de inventar un segundo separador para el camino autónomo.
const autonomousParamSeparator = " a "

// extractAutonomousParams traduce una Capability en texto libre (ya
// resuelta a una de las 3 Action ejecutables autónomamente) a los
// parámetros EXACTOS que solving.Execute (Bloque 66) espera para esa
// Action — ver execution_engine.go: crear_carpeta necesita "ruta",
// mover_archivo necesita "origen"/"destino", renombrar_archivo necesita
// "ruta"/"nombre_nuevo" (no "destino": auditado contra
// executeAction()).
//
// Determinista, sin NLP — mismo espíritu que classifier.go y
// skill_builder.go: matchea el TRIGGER contra shared.ActionPatterns
// (única fuente de verdad, la misma que usa ResolveGoal/detectOperations)
// case-insensitive, pero corta el remainder sobre el texto ORIGINAL, no
// sobre shared.Normalize(capability). Motivo real, encontrado escribiendo
// este bloque: Normalize() borra ".", "," y ";" para limpiar texto
// conversacional — pero acá el remainder es una ruta/nombre de archivo,
// donde el punto es dato, no puntuación ("informe.pdf" normalizado
// queda "informepdf", un nombre de archivo distinto e inválido). Ningún
// trigger de estas 3 Action lleva acentos ni los signos que Normalize()
// quita, así que buscarlo en minúsculas sobre el original es seguro y
// preserva may/min y puntuación reales en el valor devuelto.
//
// LÍMITE CONOCIDO, documentado en vez de disimulado: separa origen/destino
// por " a " literal — un origen o nombre que contenga la subcadena " a "
// en medio (ej. una ruta con la palabra "casa") puede partirse en el
// lugar equivocado. Mismo tipo de límite que classifier.extractParameters
// ya acepta hoy para mover_archivo, no uno nuevo que este bloque inventa.
func extractAutonomousParams(action, capability string) (map[string]string, string, error) {
	lower := strings.ToLower(capability)

	trigger := longestMatchingTrigger(action, lower)
	if trigger == "" {
		return nil, "", fmt.Errorf("no reconozco ningún patrón de %q dentro de %q", action, capability)
	}
	idx := strings.Index(lower, trigger)
	remainder := strings.TrimSpace(capability[idx+len(trigger):])
	if remainder == "" {
		return nil, "", fmt.Errorf("%q no dejó ningún parámetro después de quitar %q", capability, trigger)
	}

	switch action {
	case "crear_carpeta":
		ruta := stripNamedPrefix(remainder)
		if ruta == "" {
			return nil, "", fmt.Errorf("crear_carpeta necesita un nombre/ruta de carpeta, %q quedó vacío", capability)
		}
		return map[string]string{"ruta": ruta}, ruta, nil

	case "mover_archivo":
		origen, destino, ok := splitOriginDestination(remainder)
		if !ok {
			return nil, "", fmt.Errorf("mover_archivo necesita \"<origen> a <destino>\" — no encontré ese patrón en %q", capability)
		}
		return map[string]string{"origen": origen, "destino": destino}, destino, nil

	case "renombrar_archivo":
		ruta, nombreNuevo, ok := splitOriginDestination(remainder)
		if !ok {
			return nil, "", fmt.Errorf("renombrar_archivo necesita \"<ruta> a <nombre_nuevo>\" — no encontré ese patrón en %q", capability)
		}
		return map[string]string{"ruta": ruta, "nombre_nuevo": nombreNuevo}, nombreNuevo, nil

	default:
		// No debería llegar acá: sg.Executable (66) ya filtró a estas 3
		// Action. Si llega, es un bug de ESTE archivo (una Action nueva se
		// agregó a executableActions sin su case acá) — mismo criterio que
		// el default de execution_engine.go.executeAction.
		return nil, "", fmt.Errorf("%q está marcada ejecutable pero B89 no tiene un extractor de parámetros para esa Action — bug de capability_resolver.go", action)
	}
}

// longestMatchingTrigger busca, entre los triggers reales de
// shared.ActionPatterns[action] (única fuente de verdad, la misma que usa
// ResolveGoal/detectOperations/classifier.extractAction), cuál aparece en
// lowerText (ya en minúsculas, SIN pasar por shared.Normalize — ver nota
// de extractAutonomousParams sobre por qué el punto de un nombre de
// archivo no se puede perder acá) — y si hay más de uno, el más
// largo/específico gana, mismo criterio de desempate que
// classifier.extractAction ya aplica para no depender del orden de
// iteración de un map. Ninguno de los triggers de estas 3 Action lleva
// acentos, así que comparar solo en minúsculas (sin normalizar acentos)
// es seguro y suficiente.
func longestMatchingTrigger(action, lowerText string) string {
	best := ""
	for _, trigger := range shared.ActionPatterns[action] {
		t := strings.ToLower(trigger)
		if t != "" && strings.Contains(lowerText, t) && len(t) > len(best) {
			best = t
		}
	}
	return best
}

// stripNamedPrefix saca el conector "llamada/llamado/de nombre" cuando la
// Capability lo trae explícito (ej. "crea carpeta llamada proyectos_2026"
// -> "proyectos_2026"); si no aparece ninguno, devuelve remainder tal
// cual (ej. "crea carpeta proyectos_2026" -> "proyectos_2026", ya sin el
// trigger porque quien llama ya lo quitó). Busca case-insensitive pero
// corta sobre el remainder ORIGINAL — mismo motivo que
// extractAutonomousParams: no perder may/min ni puntuación real del
// nombre de carpeta.
func stripNamedPrefix(remainder string) string {
	lower := strings.ToLower(remainder)
	for _, marker := range []string{"llamada ", "llamado ", "de nombre "} {
		if idx := strings.Index(lower, marker); idx != -1 {
			return strings.TrimSpace(remainder[idx+len(marker):])
		}
	}
	return strings.TrimSpace(remainder)
}

// splitOriginDestination parte remainder en dos mitades no vacías por la
// primera aparición de autonomousParamSeparator. ok=false si no hay
// separador o si alguna mitad queda vacía — nunca se completa una mitad
// vacía con un valor inventado (mismo fail-safe que
// solving.Execute/OutcomeMissingParams). Busca " a " case-insensitive
// pero corta sobre remainder ORIGINAL, mismo motivo que arriba.
func splitOriginDestination(remainder string) (origen string, destino string, ok bool) {
	lower := strings.ToLower(remainder)
	idx := strings.Index(lower, autonomousParamSeparator)
	if idx == -1 {
		return "", "", false
	}
	origen = strings.TrimSpace(remainder[:idx])
	destino = strings.TrimSpace(remainder[idx+len(autonomousParamSeparator):])
	if origen == "" || destino == "" {
		return "", "", false
	}
	return origen, destino, true
}
