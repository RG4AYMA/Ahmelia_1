package solving

import (
	"fmt"
	"strings"
)

// ============================================================================
// FAILURE_RECOVERY.GO — Bloque 68 (Failure Recovery)
// ============================================================================
//
// Motivación real: hoy, cuando executeAction() (66) falla, lo único
// que llega es OutcomeFailed + el error crudo de Go metido en Detail
// ("ya existe un archivo en: /x/y.pdf"). Eso alcanza para NO mentir
// ("no dijo que funcionó"), pero no alcanza para lo que este bloque
// pide: distinguir "no sé qué pasó" de "esto es un patrón conocido,
// y además la Skill de origen ya documentaba qué hacer si pasa".
//
// EL LÍMITE CENTRAL, y la frase que decidió todo el diseño de acá:
// "pero sin inventar recuperaciones". Auditado igual que ExpectedOutcome
// (67): FailureRecovery (54) es texto libre de una fuente tutorial
// ("si falla X, hacé Y") — nunca estructurado, y NUNCA atado a una
// Operation específica (es un campo de Skill entera, no de Operation
// — confirmado en skill_builder.go, no hay ExpectedOutcome/
// FailureRecovery por-Operation). Eso significa que este bloque NO
// PUEDE, honestamente:
//   - parsear esa prosa en una nueva ExecutionRequest y reintentar
//     solo — sería inventar un intérprete de lenguaje natural que el
//     proyecto no tiene (mismo límite ya declarado en 54/57/61/67).
//   - asumir que el FailureRecovery de la Skill aplica exactamente a
//     ESTA falla puntual — la Skill puede documentar recuperación
//     para un paso distinto del que en verdad fue Wired y falló acá.
//
// Lo que SÍ hace, con datos reales:
//   1. Clasifica la falla contra los mensajes de error REALES que
//      files/manager.go puede producir (auditados abajo, uno por
//      uno) — no una heurística inventada, es literalmente el
//      vocabulario de errores que el proyecto ya tiene.
//   2. Si la Skill trae FailureRecovery, lo adjunta tal cual, marcado
//      como "conocido, de la fuente, no verificado como específico a
//      esta falla" — información real para que un humano (o un
//      bloque de decisión futuro) decida, nunca una acción que este
//      motor ejecuta solo.
//   3. AutoRetryable es SIEMPRE false hoy — hallazgo real de la
//      auditoría (ver abajo), igual que Bloque 65 encontró que
//      ninguna Action Wired era Destructive/Sensitive todavía: los
//      tres modos de falla reales de files/ (ya existe / no existe /
//      error de IO) son todos deterministas con los mismos
//      parámetros — reintentar sin cambiar nada volvería a fallar
//      igual. Reintentar con OTROS parámetros exigiría que alguien
//      (humano o resolutor futuro) decida cuáles, no este bloque.
//
// AUDITORÍA de los 9 mensajes de error reales que files/manager.go
// puede producir para las 3 Action que executeAction() (66) conecta
// (MoveFile, RenameFile, CreateFolder — se ignoran a propósito
// SearchFile/OpenFile, que no pasan por Execute()):
//   "archivo origen no existe: %s"                         → FailureNotFound
//   "ya existe un archivo en: %s"                           → FailureAlreadyExists
//   "error moviendo archivo: %v"                            → FailureIO
//   "archivo copiado pero no se pudo eliminar el original"  → FailureIO
//   "archivo no existe: %s"                                 → FailureNotFound
//   "ya existe un archivo con el nombre: %s"                → FailureAlreadyExists
//   "error renombrando archivo: %v"                         → FailureIO
//   "la carpeta ya existe: %s"                               → FailureAlreadyExists
//   "error creando carpeta: %v"                             → FailureIO
// Las 9 caen en 3 categorías — no hay una cuarta real hoy, pero
// FailureUnclassified existe igual como fail-safe: si files/ agrega
// un error nuevo mañana sin actualizar este archivo, cae ahí en vez
// de mentir con una categoría que no le corresponde.

// FailureCategory clasifica la causa estructural de una falla — no un
// diagnóstico inventado, derivado literalmente de qué mensaje de
// error real produjo files/ (ver auditoría arriba).
type FailureCategory string

const (
	// FailureAlreadyExists — el destino/nombre ya está ocupado. No es
	// transitorio: reintentar con los mismos parámetros vuelve a fallar
	// siempre.
	FailureAlreadyExists FailureCategory = "ya_existe"
	// FailureNotFound — el origen no existe. Tampoco transitorio bajo
	// los mismos parámetros.
	FailureNotFound FailureCategory = "no_existe"
	// FailureIO — falló la operación de sistema de archivos en sí
	// (permisos, disco, etc.) — el único caso donde un reintento
	// podría, en teoría, tener un resultado distinto, pero este motor
	// no tiene forma de saber si la causa ya se resolvió, así que
	// tampoco lo asume.
	FailureIO FailureCategory = "error_io"
	// FailureUnclassified — el error no matchea ningún patrón
	// conocido. Fail-safe: nunca se inventa una categoría que no está
	// confirmada contra el vocabulario real de files/.
	FailureUnclassified FailureCategory = "sin_clasificar"
)

// failurePatterns mapea substrings REALES de files/manager.go a su
// categoría — mantenido a mano, mismo criterio que ActionSafety (65):
// agregar un error nuevo en files/ implica agregar (o decidir a
// propósito no agregar) su patrón acá. Orden importa: "ya existe" se
// chequea antes que "no existe" porque ningún mensaje real combina
// ambos, pero mantiene la intención explícita en vez de depender de
// que nunca colisionen por casualidad.
var failurePatterns = []struct {
	substr   string
	category FailureCategory
}{
	{"ya existe", FailureAlreadyExists},
	{"no existe", FailureNotFound},
	{"error moviendo", FailureIO},
	{"error renombrando", FailureIO},
	{"error creando", FailureIO},
	{"no se pudo eliminar el original", FailureIO},
}

func classifyFailure(detail string) FailureCategory {
	for _, p := range failurePatterns {
		if strings.Contains(detail, p.substr) {
			return p.category
		}
	}
	return FailureUnclassified
}

// FailureDiagnosis es la respuesta completa a "¿qué pasó, y qué
// sabemos hacer al respecto?" — nunca "la función falló" sin más.
type FailureDiagnosis struct {
	Action   string
	Category FailureCategory
	RawError string

	// HasSkillRecovery / SkillRecovery — presente solo si la Skill
	// pasada trae FailureRecovery no vacío. Ver nota central: esto es
	// texto de la Skill ENTERA, no verificado como específico a esta
	// Operation puntual — se marca explícitamente como tal en
	// Describe().
	HasSkillRecovery bool
	SkillRecovery    []string

	// AutoRetryable — hoy siempre false (ver auditoría arriba). Campo
	// separado (en vez de simplemente omitir la idea de reintentar)
	// para que quede documentado que SE CONSIDERÓ, no que se olvidó.
	AutoRetryable bool

	Diagnosis string // narrativa estructural, sin el texto de la Skill (eso va aparte en SkillRecovery)
}

// Diagnose es el punto de entrada del bloque. Solo tiene sentido
// llamarlo sobre un ExecutionResult que de verdad falló — para
// cualquier otro Outcome, devuelve un diagnóstico vacío en vez de
// fabricar una causa para algo que ni siquiera se rompió.
func Diagnose(execResult *ExecutionResult, skill *Skill) *FailureDiagnosis {
	if execResult == nil || execResult.Outcome != OutcomeFailed {
		return &FailureDiagnosis{
			Category:  "",
			Diagnosis: "no hay nada que diagnosticar: la ejecución no falló (o nunca se intentó)",
		}
	}

	diag := &FailureDiagnosis{
		Action:        execResult.Action,
		RawError:      execResult.Detail,
		Category:      classifyFailure(execResult.Detail),
		AutoRetryable: false, // ver auditoría — ninguna categoría real de hoy lo amerita
	}

	switch diag.Category {
	case FailureAlreadyExists:
		diag.Diagnosis = "el destino o nombre ya está ocupado — reintentar con los mismos parámetros va a fallar igual; hace falta un destino/nombre distinto o confirmar si sobreescribir es aceptable (esta Action no sobreescribe)"
	case FailureNotFound:
		diag.Diagnosis = "el origen indicado no existe — reintentar con los mismos parámetros va a fallar igual; hace falta confirmar la ruta correcta"
	case FailureIO:
		diag.Diagnosis = "falló la operación de sistema de archivos en sí (no un problema de nombres/rutas ocupadas) — puede ser permisos u otra condición del sistema que este motor no puede diagnosticar más a fondo"
	default:
		diag.Diagnosis = "el mensaje de error no matchea ningún patrón conocido de files/ — no se inventa una causa"
	}

	if skill != nil && len(skill.FailureRecovery) > 0 {
		diag.HasSkillRecovery = true
		diag.SkillRecovery = skill.FailureRecovery
	}

	return diag
}

// Describe arma la narrativa pedida — "la función falló" nunca es
// suficiente, pero tampoco se afirma más de lo que se sabe de verdad.
func (d *FailureDiagnosis) Describe() string {
	if d.Category == "" {
		return d.Diagnosis
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%q falló: %s. %s.", d.Action, d.RawError, d.Diagnosis)

	if d.HasSkillRecovery {
		fmt.Fprintf(&b, " La Skill de origen documenta esta recuperación conocida (a nivel del procedimiento completo, no verificada como específica de este paso): %s. No la aplico automáticamente — es texto de la fuente, no una acción estructurada que este motor pueda ejecutar solo; hace falta que la traduzcas en un nuevo intento con parámetros concretos.",
			strings.Join(d.SkillRecovery, "; "))
	} else {
		b.WriteString(" No hay ninguna recuperación conocida documentada en la Skill de origen para este procedimiento.")
	}

	if d.AutoRetryable {
		b.WriteString(" Se puede reintentar automáticamente.")
	} else {
		b.WriteString(" No corresponde un reintento automático con los mismos parámetros.")
	}

	return b.String()
}
