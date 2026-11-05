package solving

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ============================================================================
// VERIFICATION_ENGINE.GO — Bloque 67 (Verification Engine)
// ============================================================================
//
// Motivación real, la misma que trajiste: Execute() (66) devuelve
// OutcomeExecuted cuando files.CreateFolder/MoveFile/RenameFile no
// devolvieron error de Go — pero "no hubo error" y "el mundo real
// terminó como se quería" NO son la misma afirmación. os.Rename puede,
// en teoría, devolver nil y el estado del filesystem no ser el
// esperado por una carrera externa (otro proceso borra el destino
// entre la ejecución y ahora); y más importante para este proyecto:
// nada hasta este bloque vuelve a mirar el disco después de ejecutar.
// Verification Engine es exactamente eso — la segunda mirada.
//
// SOBRE ExpectedOutcome (54): es el campo correcto en espíritu, pero
// NO es lo que este bloque usa para verificar. Auditado en
// skill_builder.go: es texto libre extraído de prosa tutorial ("el
// resultado es...", "deberías ver...") — nunca estructurado, y la
// nota del propio campo ya admite que "va a quedar vacío la mayoría de
// las veces". No hay forma honesta de convertir esa prosa en un check
// programático sin fabricar NLP que el proyecto no tiene (mismo límite
// que ya se declaró en 54/57/61). Lo que este bloque hace en cambio:
// para cada Action ejecutable (66), define a mano la postcondición
// real y comprobable — "existe la carpeta", "el archivo está en el
// destino y ya no en el origen" — que es justamente lo que
// ExpectedOutcome describiría en prosa si la fuente lo hubiera dicho
// explícito. Cuando la Skill de origen SÍ trae ExpectedOutcome, se
// adjunta como nota informativa (SkillExpectedOutcome) — contexto para
// un humano, nunca el criterio que decide Confirmed/NotConfirmed.
//
// "Integrar, no crear": no reimplementa la lógica de resolución de
// destino de files.MoveFile (si destino es carpeta, el archivo final
// queda en destino/basename(origen)) — la RELEE acá para saber QUÉ
// ruta comprobar, porque después de una ejecución exitosa esa
// ambigüedad ya se resolvió sola: si destino sigue siendo una carpeta
// ahora, el archivo tiene que estar adentro; si no, destino mismo es
// el archivo.
//
// LÍMITE HONESTO: solo verifica lo que Execute() (66) puede ejecutar
// hoy — las mismas 3 Action de executableActions. No inventa checks
// para abrir_programa/volumen_*/ventanas porque este proyecto tampoco
// tiene forma de leer ese estado del sistema operativo todavía.

// VerificationOutcome documenta, sin ambigüedad, si se pudo confirmar
// el objetivo real o por qué no se intentó siquiera.
type VerificationOutcome string

const (
	// VerificationConfirmed — se ejecutó y la postcondición real en
	// disco coincide con el objetivo.
	VerificationConfirmed VerificationOutcome = "confirmado"
	// VerificationNotConfirmed — se ejecutó (Execute no devolvió
	// error) pero el estado real NO coincide con lo esperado. Esto es
	// precisamente el caso que "no devolvió error" solo, sin este
	// bloque, nunca detectaría.
	VerificationNotConfirmed VerificationOutcome = "no_confirmado"
	// VerificationNotApplicable — nunca se llegó a ejecutar (bloqueado
	// por autorización/confirmación, faltaban parámetros, Action no
	// ejecutable, o falló en Execute) — no hay nada que verificar
	// porque nunca se intentó cambiar el mundo real.
	VerificationNotApplicable VerificationOutcome = "no_aplica"
	// VerificationUnchecked — se ejecutó, pero este motor todavía no
	// tiene una postcondición definida para esta Action. Fail-safe
	// simétrico al de 65/66: JAMÁS se asume Confirmed por default
	// cuando no hay un check real corriendo.
	VerificationUnchecked VerificationOutcome = "sin_verificador"
)

// VerificationResult es la respuesta a "¿realmente ocurrió lo que
// queríamos?" — nunca una repetición de lo que ya decía
// ExecutionResult.
type PostconditionFact struct {
	Subject   string `json:"subject"`
	Predicate string `json:"predicate"`
	Expected  string `json:"expected"`
	Observed  string `json:"observed"`
}

type VerificationResult struct {
	Action  string
	Outcome VerificationOutcome
	Detail  string
	Facts   []PostconditionFact

	// SkillExpectedOutcome — presente solo si se pasó una *Skill con
	// ExpectedOutcome no vacío. Puramente informativo: nunca participa
	// en el cálculo de Outcome, ver nota arriba sobre por qué prosa
	// libre no es un criterio verificable.
	SkillExpectedOutcome []string
}

func (v *VerificationResult) Confirmed() bool {
	return v.Outcome == VerificationConfirmed
}

// verifiableActions — deliberadamente un mapa separado de
// executableActions (66), aunque hoy tengan las mismas 3 claves.
// Mismo motivo que WiredActions vs ActionSafety: agregar una Action
// nueva a executableActions NO debe hacer que de repente "se pueda
// verificar" sin que alguien haya escrito, a propósito, el check de
// postcondición correspondiente acá abajo.
var verifiableActions = map[string]bool{
	"crear_carpeta":     true,
	"mover_archivo":     true,
	"renombrar_archivo": true,
}

// ExpectedFacts returns the checkable postconditions of an action, resolved
// from params *before* execution. It is the single source of the expectation:
// Verify consumes it after execution, and the durable expectation ledger
// consumes it before execution. It never mutates the world.
//
// A nil/empty result means the harness has no checkable expectation for this
// action instance. Callers must not manufacture one.
func ExpectedFacts(action string, params map[string]string) ([]PostconditionFact, string) {
	if !verifiableActions[action] {
		return nil, fmt.Sprintf("%q no tiene una postcondición verificable definida para esta acción todavía", action)
	}
	switch action {
	case "crear_carpeta":
		return expectedCarpetaCreada(params)
	case "mover_archivo":
		return expectedArchivoMovido(params)
	case "renombrar_archivo":
		return expectedArchivoRenombrado(params)
	}
	return nil, fmt.Sprintf("%q está en verifiableActions pero no tiene expectativa definida — bug de este archivo", action)
}

// ObserveExpected inspects the world for an already-declared expectation. It
// never re-declares the expectation: the subjects and expected values are the
// ones fixed before execution. For mover_archivo it also verifies that the
// pre-execution destination resolution still holds; if the destination changed
// type between declaration and observation, the result is unknown
// (insufficient) with an explicit reason, never a retroactive confirmation.
func ObserveExpected(action string, params map[string]string, expected []PostconditionFact) *VerificationResult {
	res := &VerificationResult{Action: action}
	if len(expected) == 0 {
		res.Outcome = VerificationUnchecked
		res.Detail = "no hay expectativa declarada que observar"
		return res
	}

	if action == "mover_archivo" {
		current, reason := ExpectedFacts(action, params)
		if len(current) == 0 {
			res.Outcome = VerificationUnchecked
			res.Detail = "no se pudo revalidar la resolución del destino: " + reason
			return res
		}
		if current[len(current)-1].Subject != expected[len(expected)-1].Subject {
			res.Outcome = VerificationUnchecked
			res.Detail = fmt.Sprintf("la resolución del destino cambió entre la declaración (%s) y la observación (%s); no se puede confirmar retroactivamente",
				expected[len(expected)-1].Subject, current[len(current)-1].Subject)
			return res
		}
	}

	for i, want := range expected {
		observed, err := observePostconditionFact(want)
		fact := want
		fact.Observed = observed
		res.Facts = append(res.Facts, fact)
		if observed == "" || observed == "unknown" {
			res.Outcome = VerificationUnchecked
			res.Detail = unknownDetail(action, want, err)
			return res
		}
		if !strings.EqualFold(observed, want.Expected) {
			res.Outcome = VerificationNotConfirmed
			res.Detail = contradictedDetail(action, want, observed, i)
			return res
		}
	}
	res.Outcome = VerificationConfirmed
	res.Detail = confirmedDetail(action, expected)
	return res
}

// Verify confirma si el objetivo real de una ExecutionRequest ya
// ejecutada se cumplió de verdad, mirando el filesystem — no el
// resultado que Execute() ya reportó. skill es opcional (nil si no
// aplica); si no es nil y trae ExpectedOutcome, se adjunta como nota,
// nunca como criterio de decisión.
func Verify(action string, params map[string]string, execResult *ExecutionResult, skill *Skill) *VerificationResult {
	res := &VerificationResult{Action: action}
	if skill != nil && len(skill.ExpectedOutcome) > 0 {
		res.SkillExpectedOutcome = skill.ExpectedOutcome
	}

	// 1. Si nunca se ejecutó de verdad, no hay nada que mirar en el
	// mundo real — verificar acá sería inventar una comprobación sobre
	// un cambio que nunca ocurrió.
	if execResult == nil || !execResult.Executed() {
		res.Outcome = VerificationNotApplicable
		res.Detail = "no se verificó porque la acción nunca se ejecutó"
		return res
	}

	// 2. Fail-safe: Action ejecutada pero sin check de postcondición
	// definido acá. No se asume éxito solo porque Execute() no dio
	// error — ese es exactamente el problema que motivó este bloque.
	expected, reason := ExpectedFacts(action, params)
	if len(expected) == 0 {
		res.Outcome = VerificationUnchecked
		res.Detail = reason
		return res
	}

	// 3. La expectativa ya está fijada. Solo se observa y compara.
	observed := ObserveExpected(action, params, expected)
	observed.SkillExpectedOutcome = res.SkillExpectedOutcome
	return observed
}

func expectedCarpetaCreada(params map[string]string) ([]PostconditionFact, string) {
	path := strings.TrimSpace(params["ruta"])
	if path == "" {
		return nil, "falta la ruta esperada de la carpeta; no hay estado que observar"
	}
	return []PostconditionFact{{
		Subject: path, Predicate: "filesystem.object_type", Expected: "directory",
	}}, ""
}

func expectedArchivoRenombrado(params map[string]string) ([]PostconditionFact, string) {
	oldPath := strings.TrimSpace(params["ruta"])
	newName := strings.TrimSpace(params["nombre_nuevo"])
	if oldPath == "" || newName == "" {
		return nil, "faltan la ruta original o el nombre nuevo; no se puede evaluar el renombrado"
	}
	newPath := filepath.Join(filepath.Dir(oldPath), newName) // misma lógica que files.RenameFile
	return []PostconditionFact{
		{Subject: oldPath, Predicate: "filesystem.exists", Expected: "false"},
		{Subject: newPath, Predicate: "filesystem.exists", Expected: "true"},
	}, ""
}

func expectedArchivoMovido(params map[string]string) ([]PostconditionFact, string) {
	source := strings.TrimSpace(params["origen"])
	dest := strings.TrimSpace(params["destino"])
	if source == "" || dest == "" {
		return nil, "faltan origen o destino; no se puede evaluar el movimiento"
	}

	// Misma resolución que files.MoveFile, leída antes de ejecutar: si
	// destino es carpeta, el archivo final queda en destino/basename;
	// si no, destino mismo es el archivo final. Esta lectura no muta nada.
	expected := dest
	if info, err := os.Stat(dest); err == nil && info.IsDir() {
		expected = filepath.Join(dest, filepath.Base(source))
	} else if err != nil && !definitelyAbsent(dest, err) {
		return nil, fmt.Sprintf("no se pudo determinar si el destino es carpeta o archivo en %s: %v", dest, err)
	}

	return []PostconditionFact{
		{Subject: source, Predicate: "filesystem.exists", Expected: "false"},
		{Subject: expected, Predicate: "filesystem.exists", Expected: "true"},
	}, ""
}

func observePostconditionFact(fact PostconditionFact) (string, error) {
	switch fact.Predicate {
	case "filesystem.object_type":
		info, err := os.Stat(fact.Subject)
		switch {
		case err != nil && definitelyAbsent(fact.Subject, err):
			return "absent", err
		case err != nil:
			return "unknown", err
		case info.IsDir():
			return "directory", nil
		default:
			return "non_directory", nil
		}
	case "filesystem.exists":
		_, err := os.Stat(fact.Subject)
		switch {
		case err == nil:
			return "true", nil
		case definitelyAbsent(fact.Subject, err):
			return "false", err
		default:
			return "unknown", err
		}
	}
	return "unknown", fmt.Errorf("predicado %q no observable", fact.Predicate)
}

func unknownDetail(action string, fact PostconditionFact, err error) string {
	switch action {
	case "crear_carpeta":
		return fmt.Sprintf("no se pudo determinar el estado de la carpeta %s: %v", fact.Subject, err)
	case "mover_archivo":
		return fmt.Sprintf("no se pudo determinar el estado de %s: %v", fact.Subject, err)
	case "renombrar_archivo":
		return fmt.Sprintf("no se pudo determinar el estado de %s: %v", fact.Subject, err)
	default:
		return fmt.Sprintf("no se pudo observar %s: %v", fact.Subject, err)
	}
}

func contradictedDetail(action string, fact PostconditionFact, observed string, index int) string {
	switch {
	case fact.Predicate == "filesystem.object_type" && observed == "absent":
		return fmt.Sprintf("la carpeta esperada no existe en %s", fact.Subject)
	case fact.Predicate == "filesystem.object_type" && observed == "non_directory":
		return fmt.Sprintf("existe algo en %s, pero no es una carpeta", fact.Subject)
	case fact.Predicate == "filesystem.exists" && fact.Expected == "false":
		return fmt.Sprintf("el archivo original todavía existe en %s", fact.Subject)
	case action == "renombrar_archivo" && index > 0:
		return fmt.Sprintf("no existe el nombre nuevo esperado (%s)", fact.Subject)
	case action == "mover_archivo" && index > 0:
		return fmt.Sprintf("el origen ya no existe, pero tampoco existe el destino esperado (%s)", fact.Subject)
	default:
		return fmt.Sprintf("el estado observado en %s no coincide: esperaba %q, observé %q", fact.Subject, fact.Expected, observed)
	}
}

func confirmedDetail(action string, facts []PostconditionFact) string {
	switch action {
	case "crear_carpeta":
		if len(facts) > 0 {
			return fmt.Sprintf("carpeta confirmada en %s", facts[0].Subject)
		}
	case "mover_archivo":
		if len(facts) == 2 {
			return fmt.Sprintf("movimiento confirmado: ya no existe %s, existe %s", facts[0].Subject, facts[1].Subject)
		}
	case "renombrar_archivo":
		if len(facts) == 2 {
			return fmt.Sprintf("renombrado confirmado: ya no existe %s, existe %s", facts[0].Subject, facts[1].Subject)
		}
	}
	return "postcondición confirmada"
}

// ExecuteAndVerify encadena 66 y 67 — ejecuta y, solo si de verdad se
// ejecutó, confirma la postcondición real. Conveniencia para el
// llamador que quiere la respuesta completa ("¿pasó, y de verdad
// ocurrió lo que quería?") en un solo paso, sin tener que acordarse de
// llamar a Verify por separado.
func ExecuteAndVerify(req ExecutionRequest, skill *Skill) (*ExecutionResult, *VerificationResult) {
	execResult := Execute(req)
	verResult := Verify(req.Action, req.Params, execResult, skill)
	return execResult, verResult
}

// definitelyAbsent distinguishes a genuinely absent path from an OS lookup
// that reports a missing child because traversal was blocked by a non-directory
// ancestor (notably ERROR_PATH_NOT_FOUND on Windows). In the latter case the
// inspector did not establish the target state, so the result must stay unknown.
func definitelyAbsent(path string, err error) bool {
	if err == nil || !os.IsNotExist(err) {
		return false
	}
	current := filepath.Dir(path)
	for {
		info, parentErr := os.Stat(current)
		if parentErr == nil {
			return info.IsDir()
		}
		if !os.IsNotExist(parentErr) {
			return false
		}
		parent := filepath.Dir(current)
		if parent == current {
			return false
		}
		current = parent
	}
}

// DescribeVerification arma una narrativa humana de un
// VerificationResult — el equivalente, para 67, de lo que Describe()
// es para GoalResolution (62). Nunca dice "hecho" sin más: distingue
// con claridad confirmado / no confirmado / no aplica / sin verificar.
func (v *VerificationResult) Describe() string {
	var b strings.Builder
	switch v.Outcome {
	case VerificationConfirmed:
		fmt.Fprintf(&b, "Confirmado: %s.", v.Detail)
	case VerificationNotConfirmed:
		fmt.Fprintf(&b, "Se ejecutó sin error, pero al revisar el resultado real no coincide con lo esperado: %s.", v.Detail)
	case VerificationUnchecked:
		fmt.Fprintf(&b, "Se ejecutó, pero no puedo confirmar el resultado todavía: %s.", v.Detail)
	default: // VerificationNotApplicable
		fmt.Fprintf(&b, "No hay nada que verificar: %s.", v.Detail)
	}
	if len(v.SkillExpectedOutcome) > 0 {
		fmt.Fprintf(&b, " (La fuente original describe el resultado esperado como: %s.)", strings.Join(v.SkillExpectedOutcome, "; "))
	}
	return b.String()
}
