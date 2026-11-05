package capability

import (
	"ahmelia/actioncatalog"
	"fmt"
)

// ============================================================================
// CAPABILITY.GO — Bloque 94.1 (Capability Contract), primer bloque de la
// Fase 8 (Capability & Production Foundation)
// ============================================================================
//
// AUDITORÍA PREVIA (mismo criterio que B89 aplicó antes de escribir
// capability_resolver.go — no se reimplementa nada de lo que ya existe):
//
//   - "Capability" YA es un nombre usado en el proyecto, pero como CAMPO
//     string: solving.GoalResolution.Capability (goal_skill_resolver.go,
//     Bloque 62) es la frase de capacidad extraída de una petición en
//     lenguaje natural (ej. "crear carpeta proyectos_2026"). Ese campo
//     sigue existiendo tal cual — B94.1 no lo toca ni lo renombra. Lo que
//     este archivo agrega es un tipo NUEVO, capability.Capability, en un
//     paquete propio: el contrato declarativo de ALGO que Ahmelia puede
//     hacer, no el texto de una petición puntual. Son dos conceptos
//     distintos que hoy comparten nombre por coincidencia de vocabulario
//     en español — viven en paquetes distintos así que Go no los confunde,
//     pero se documenta acá para que una persona tampoco los confunda.
//
//   - autonomy/capability_resolver.go (B89) ya resuelve el camino
//     Goal -> Task -> Capability(frase) -> Skill/Procedure -> ActionRequest,
//     apoyado en solving.Decompose. Ese camino NO se reemplaza: B94 no
//     vuelve a implementar resolución de texto libre. Lo que falta, y que
//     B89 documenta como huecos nombrados a propósito (KindLocalAction
//     únicamente, un solo Subgoal ejecutable, sin capacidades de app
//     como Unity/Blender), es un lugar donde ALGO QUE AHMELIA PUEDE HACER
//     quede descrito como dato — con sus inputs, permisos y forma de
//     verificarse — en vez de vivir repartido entre shared.ActionPatterns
//     (triggers), shared.WiredActions (qué está conectado de verdad),
//     solving.executableActions (qué motor local lo corre) y
//     actions.Handler.Actions() (qué verbos entiende cada app). Ese es el
//     "Capability" que pide B94.1 — el nodo que hoy no existe como tipo,
//     solo como conocimiento disperso en cuatro sitios distintos.
//
//   - autonomy/authority.go (B88.5) YA define la escalera de autoridad real
//     (DecisionAuthority: None/Recommend/Initiate/SelectGoal/Plan/Resolve/
//     Execute/Verify/Recover) y su prueba CanExecute(). B94.1 NO duplica
//     ese contrato ni lo importa: capability/ es un paquete hoja (igual que
//     shared/ — sin dependencias internas del proyecto) porque
//     autonomy YA importa shared y solving hoy (ver capability_resolver.go),
//     y B94.2/B94.3 van a hacer que autonomy importe capability/ de la
//     misma forma. Si capability/ importara autonomy, autonomy importando
//     capability/ sería un ciclo. Por eso Permission acá abajo es un
//     vocabulario propio y liviano (PermissionLevel) — la traducción real
//     de "esta Capability pide autorización" a una DecisionAuthority
//     concreta es trabajo de B94.3, adentro de autonomy/, no de este
//     archivo.
//
//   - actions/dispatcher.go (esqueleto sin conectar, confirmado leyendo el
//     código: cada Handler.Execute() devuelve "todavía no implementado")
//     YA define el vocabulario de Domain que este archivo reutiliza tal
//     cual: los cuatro AppName() reales son "unity", "code" (vscode),
//     "blender", "unreal". Se agrega "local" para las 3 Action que
//     solving.Execute (B66) sí corre de verdad hoy (crear_carpeta,
//     mover_archivo, renombrar_archivo — ver executableActions en
//     execution_engine.go). No se inventa un quinto dominio sin evidencia.
//
// LO QUE ESTE ARCHIVO NO HACE (límites honestos, nombrados a propósito):
//   - No ejecuta nada. Execution acá es SOLO metadata (qué Domain/Action
//     identifica la ejecución real) — quien ejecuta sigue siendo
//     solving.Execute o actions.Handler, sin cambios.
//   - No registra ni descubre Capability — eso es CapabilityRegistry
//     (B94.2, capability/registry.go).
//   - No autoriza ni pide permiso — eso es B94.3, adentro de autonomy/,
//     que sabe traducir PermissionLevel a DecisionAuthority real.
//   - No verifica Preconditions ni corre Verification — son texto
//     declarativo en este bloque; un motor que los evalúe de verdad es
//     B94.4/B94.5.

// ID identifica una Capability de forma única dentro de un
// CapabilityRegistry (B94.2). Deliberadamente un string simple, mismo
// criterio que planning.Goal.ID/Task.ID: no se inventa un tipo con
// validación propia hasta que exista un caller real que la necesite.
type ID string

// SkillID identifies an Ahmelia-owned competence independently from the
// operation and provider used to express it. This metadata does not itself
// prove that the competence has been learned or demonstrated.
type SkillID string

// Domain agrupa Capability por el entorno/aplicación al que aplican —
// reutiliza tal cual el vocabulario de actions.Handler.AppName()
// (dispatcher.go) y agrega DomainLocal para las Action que solving.Execute
// corre sin ninguna app externa de por medio.
type Domain string

const (
	DomainLocal   Domain = "local"
	DomainUnity   Domain = "unity"
	DomainVSCode  Domain = "code"
	DomainBlender Domain = "blender"
	DomainUnreal  Domain = "unreal"
)

// ExecutionLane separates capabilities that represent direct interactive/local
// actions from capabilities that represent production workflows. The lane is
// routing metadata, not authority: authorization and execution still belong
// to the canonical runtime boundary.
type ExecutionLane string

const (
	LaneUnspecified ExecutionLane = ""
	LaneAction      ExecutionLane = "action"
	LaneProduction  ExecutionLane = "production"
)

// Parameter describe un input o output nombrado que una Capability
// necesita o produce. No es una revalidación de map[string]string
// (ActionRequest.Parameters, autonomy/executor_adapter.go, o
// solving.ExecutionRequest) — es la DESCRIPCIÓN de esas claves, para que
// algo (un futuro selector, o una persona preguntando "qué necesita
// CreateUnityScript") pueda leerla sin adivinar mirando el código de
// extractAutonomousParams.
type Parameter struct {
	Name        string
	Description string
	Required    bool
}

// PermissionLevel is owned by the leaf action catalog. Keeping it as an
// alias means capability validation and downstream catalogs share exactly one
// semantic type without importing runtime authority packages.
type PermissionLevel = actioncatalog.PermissionLevel

const (
	PermissionAutomatic = actioncatalog.PermissionAutomatic
	PermissionNotify    = actioncatalog.PermissionNotify
	PermissionAskFirst  = actioncatalog.PermissionAskFirst
)

// FailureMode nombra una forma conocida en la que una Capability puede
// fallar. Existir acá como dato no reemplaza core.FailureDiagnosis (B92):
// le da vocabulario contra el que un diagnóstico futuro podría matchear,
// en vez de parsear el string de un error.
type FailureMode struct {
	Code        string
	Description string
	Recoverable bool
}

// Capability es el contrato declarativo que B94.1 formaliza: todo lo que
// Ahmelia necesita SABER sobre algo que puede hacer, antes de intentar
// hacerlo. Una Capability nunca se ejecuta a sí misma — Action/Domain acá
// son metadata de A QUÉ ejecutor real corresponde (solving.Execute para
// DomainLocal, actions.Handler para los demás), no una función embebida:
// mismo espíritu que CapabilityResolver (B89), que resuelve pero no
// ejecuta.
type Capability struct {
	ID ID
	// SkillID groups executable operations under a stable competence identity.
	// Domain and provider remain replaceable instrument bindings.
	SkillID     SkillID
	Domain      Domain
	Description string
	Lane        ExecutionLane

	Inputs  []Parameter
	Outputs []Parameter

	Permission PermissionLevel

	// Preconditions son condiciones legibles que deberían cumplirse antes
	// de Execution. B94.1 solo las documenta — evaluarlas de verdad es
	// trabajo de un bloque posterior (B94.4/B94.5), igual que
	// EnvironmentInspector (autonomy/environment_verifier.go) ya separa
	// "declarar qué se espera observar" de "observarlo de verdad".
	Preconditions []string

	// Action es la clave de vocabulario que identifica la ejecución real:
	// una key de shared.ActionPatterns cuando Domain==DomainLocal (ver
	// shared/capabilities.go), o uno de los verbos que devuelve
	// Handler.Actions() cuando Domain es un dominio de aplicación (ver
	// actions/dispatcher.go). B94.1 no valida contra esas fuentes —
	// hacerlo acá acoplaría capability/ a shared/ y actions/ sin que
	// exista todavía un caller real que lo necesite; queda como límite
	// nombrado para B94.2 (Registry), que sí puede validar al registrar.
	Action string

	FailureModes []FailureMode
}

// Validate confirma que una Capability trae lo mínimo indispensable para
// que el resto de B94 pueda apoyarse en ella — ID, Domain, Action y un
// Permission válido. No valida Inputs/Outputs/Preconditions/FailureModes:
// una Capability sin ellos es incompleta pero no inválida (ej. una acción
// sin parámetros, como "detener", legítimamente no tiene Inputs).
func (c Capability) Validate() error {
	if c.ID == "" {
		return fmt.Errorf("capability: ID vacío")
	}
	if c.Domain == "" {
		return fmt.Errorf("capability: %q no declara Domain", c.ID)
	}
	if c.Action == "" {
		return fmt.Errorf("capability: %q no declara Action", c.ID)
	}
	if c.Permission == "" {
		return fmt.Errorf("capability: %q no declara Permission", c.ID)
	}
	if !c.Permission.IsValid() {
		return fmt.Errorf("capability: %q declara Permission %q, no es uno de los niveles válidos", c.ID, c.Permission)
	}
	return nil
}
