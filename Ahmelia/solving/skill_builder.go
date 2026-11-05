package solving

import (
	"ahmelia/actions/blender"
	"ahmelia/actions/unity"
	"ahmelia/actions/unreal"
	"ahmelia/actions/vscode"
	"ahmelia/interaction"
	"ahmelia/shared"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"
)

// ============================================================================
// SKILL_BUILDER.GO — Bloque 54, Fase 4.1 (rediseño — cierre de la cadena)
// ============================================================================
//
// Discover (52) decide DE DÓNDE. Fetch (Gateway, 51) trae el documento.
// Validate (53) dice si es confiable. Ninguno de los tres deja algo que
// Ahmelia pueda VOLVER A CONSULTAR después sin repetir todo el viaje a
// Internet — cada vez que alguien pregunta, se descarga todo de nuevo.
// Skill Builder es el que persiste el resultado: convierte un
// ValidationSet ya corroborado en una Skill guardada en disco, mismo
// criterio de persistencia que Knowledge Memory (49,
// memory/knowledge_memory.json).
//
// REDISEÑO respecto a la versión original de este bloque: antes, una
// "SkillNote" era un blob de texto curado (Content) sin forma — no
// respondía "¿cuándo se usa esto?" ni "¿qué pasos tiene?", solo "¿qué
// dice la fuente?". Ahora Build() además intenta extraer, con
// heurística de patrones (sin NLP, sin LLM — ver el fork documentado
// abajo), la estructura que el documento fuente YA traía: el problema
// que resuelve, cuándo usarlo, los pasos si eran una lista reconocible,
// y qué evitar. El Content curado se conserva tal cual — Semantic
// Search (57) y Update Detector (58) lo siguen usando como fuente de
// verdad para buscar y comparar — los campos nuevos son un ÍNDICE
// estructurado sobre ese mismo texto, no un reemplazo.
//
// EL FORK QUE SE RESOLVIÓ ACÁ (documentado para que no se re-litigue):
// había dos caminos honestos para extraer esa estructura — (A)
// heurística por patrones sobre el texto ya fetcheado, sin
// dependencias nuevas, o (B) agregar una capacidad nueva de
// comprensión de lenguaje real (llamar a un LLM). Se decidió A: es
// fiel al mismo criterio 100% determinista que rige desde el Bloque 1,
// no agrega red/costo/latencia nueva, y si el documento fuente es
// prosa narrativa sin estructura reconocible, la Skill sale con esos
// campos vacíos en vez de inventar procedimiento — fiel a la
// limitación en vez de fabricar algo. B queda como una posible fase
// futura separada, con su propio bloque, igual que "conectar a
// Internet" (47-52) fue su propia decisión explícita.
//
// DECISIÓN DE DISEÑO IMPORTANTE, distinta del borrador inicial de la
// Skill Spec: el UMBRAL DE PERSISTENCIA no cambió — sigue siendo
// exactamente el de hoy (al menos un documento confiable en el
// ValidationSet). El borrador original proponía descartar el
// candidato si no se lograba extraer ni WhenToUse ni Steps ("Candidate
// → no se persiste"). Se revirtió esa parte a propósito: la mayoría de
// la prosa real (artículos, documentación no tutorial) NO trae
// frases-gatillo tipo "usa X cuando Y" ni listas numeradas, así que
// ese umbral habría dejado a Knowledge Retrieval (56), Update Detector
// (58) y Versioning (59) sin nota para persistir/comparar/versionar en
// el caso común — una regresión real y silenciosa de "ya no vuelvo a
// salir a Internet" para la mayoría de los dominios, que es peor,
// dentro de la misma vara de honestidad del proyecto, que declarar
// campos vacíos. Por eso State clasifica CUÁNTA estructura se pudo
// extraer, pero nunca decide si la Skill se guarda o no — eso lo sigue
// decidiendo, únicamente, si hubo contenido confiable real.

// MaxNoteContentLength acota cuánto texto se guarda por nota — una nota
// de estudio no necesita el HTML completo normalizado de cada fuente,
// alcanza con lo suficiente para que quien la lea después entienda el
// tema; guardar documentos enteros sin límite haría crecer
// skills.json sin control.
const MaxNoteContentLength = 4000

// Topes sobre cuánta estructura se extrae por Skill — mismo espíritu
// que MaxNoteContentLength: una Skill no necesita CADA línea que
// matcheó un patrón, alcanza con lo suficiente para ser útil, sin que
// un documento largo infle sin límite los campos estructurados.
const (
	maxExtractedSteps     = 15
	maxExtractedListItems = 6
)

// SkillState clasifica CUÁNTA estructura real se pudo extraer del
// texto fuente — NO decide si la Skill se persiste (ver nota de
// diseño arriba). Solo dos valores reales: los demás estados del
// roadmap conceptual (NeedsUpdate, Superseded) ya existen como
// conceptos EXTERNOS a la Skill misma — UpdateCheck (58) y
// SkillVersionStore (59) — duplicarlos acá como campo sería el mismo
// error de raíz que el bloque original ya evitó una vez (mezclar
// responsabilidades de bloques distintos).
type SkillState string

const (
	// El zero-value ("") ES un estado válido y distinto, no un
	// descuido: significa que no se pudo extraer ni WhenToUse ni
	// Steps — el documento fuente era prosa narrativa sin ninguna
	// señal estructural reconocible. Antes del rediseño, Build()
	// asignaba StateStructured incondicionalmente como default, lo
	// cual contradecía esta misma documentación (decía "se extrajo al
	// menos una condición de uso" pero no lo garantizaba) — ver
	// TestBuild_ProsaSinEstructuraReconocibleQuedaSinCampos, que ya
	// existía escrito y en rojo antes de este fix. Describe() ya
	// maneja este caso ("sin procedimiento ni condición de uso
	// reconocibles en la fuente") vía su default de switch, así que no
	// hace falta una constante nombrada — el zero-value de Go alcanza.
	//
	// StateStructured: se extrajo al menos una condición de uso
	// (WhenToUse) del texto, pero el documento no traía una secuencia
	// de pasos reconocible.
	StateStructured SkillState = "structured"
	// StateProcedural: además de lo anterior (o en su lugar), el
	// documento traía una lista numerada/con viñetas reconocible como
	// procedimiento real.
	StateProcedural SkillState = "procedural"
)

// Skill es una capacidad reutilizable persistida: no solo QUÉ dice la
// fuente (Content, heredado de la SkillNote original) sino, cuando el
// texto fuente lo permitía, CUÁNDO y CÓMO usarlo. Sigue sin ser una
// Action ejecutable — ese salto ("tengo una nota confiable" → "puedo
// operar esto") sigue fuera de lo que este bloque, o cualquier otro
// hasta ahora, puede hacer con honestidad.
type Skill struct {
	SkillID string // TaskID del LearningItem que originó esto (Curriculum, 47)
	Label   string // TaskDesc del LearningItem — descripción legible

	// Problem es, honestamente, el mismo TaskDesc que Label — no hay
	// NLP real en el proyecto (ver fork arriba) para reformular "qué
	// problema resuelve esto" desde prosa libre sin fabricar una
	// interpretación. Queda como campo propio (en vez de reusar Label
	// directamente en la lógica que lo consume) para que el día que
	// exista una Fase B real de comprensión de lenguaje, el cambio sea
	// llenar este campo distinto — no agregar un campo nuevo.
	Problem string

	WhenToUse     []string // condiciones de activación, ej. "usa X cuando Y" — vacío si el texto no traía ninguna reconocible
	Steps         []string // pasos, SOLO si el documento fuente tenía una lista reconocible — vacío si no
	Pitfalls      []string // errores a evitar (negaciones/advertencias — "no hagas X"), NO qué hacer si ya pasó — ver FailureRecovery
	Prerequisites []string // qué necesita, SOLO si se menciona explícitamente en el texto

	// FailureRecovery: qué hacer SI el error ya ocurrió ("si falla X,
	// hacé Y") — distinto de Pitfalls, que es qué NO hacer para
	// evitarlo. No se infiere de Pitfalls ni se inventa: vacío si el
	// texto fuente no trae una frase de recuperación explícita.
	FailureRecovery []string
	// ExpectedOutcome: cómo se ve el éxito ("el resultado es...",
	// "deberías ver..."). Va a quedar vacío la mayoría de las veces —
	// la prosa tutorial rara vez lo dice explícito — y eso es correcto,
	// no un fallo de extracción: mismo criterio que el resto de estos
	// campos, fiel a la fuente en vez de fabricar un criterio de éxito.
	ExpectedOutcome []string

	// Limitations — Integration Fix (Skill Model Refinement): qué la
	// fuente dice explícitamente que este procedimiento NO puede
	// garantizar o NO cubre ("no funciona en Windows", "esto no
	// garantiza X", "solo aplica a proyectos nuevos"). Distinto de
	// Pitfalls (qué NO HACER para evitar un error durante la
	// ejecución) y de FailureRecovery (qué hacer SI el error YA
	// ocurrió) — Limitations es sobre el ALCANCE del procedimiento en
	// sí, no sobre cómo ejecutarlo bien. Vacío si la fuente no declara
	// ninguna limitación explícita — no se infiere de la ausencia de
	// ExpectedOutcome ni de nada más, eso sería fabricar una limitación
	// que la fuente nunca dijo.
	Limitations []string

	// Operations: qué operación requiere cada Step y si Ahmelia la
	// reconoce hoy. Una por cada match encontrado en cada Step — un
	// Step puede aportar 0 (si no matchea nada, ver detectOperations),
	// 1, o varias Operations. Esto NO dice si la Skill completa es
	// ejecutable — ver CapabilityCoverage/AllOperationsWired más abajo
	// y su advertencia de interpretación.
	Operations []Operation

	// ProcedureTitle: SOLO cuando hay Steps, la tarea concreta que esa
	// lista de pasos realiza — extraída de la línea que introduce la
	// lista (ver extractProcedureTitle), NO fabricada a partir de
	// Label. Vacío la mayoría de las veces (la prosa no siempre nombra
	// la tarea justo antes de la lista) — eso es correcto, no un fallo
	// de extracción. Esto es deliberadamente Nivel B (Procedural
	// Knowledge: "esta lista de pasos hace X"), no Nivel C
	// (Capability: una Skill separada llamada "X", con su propio
	// Problem/WhenToUse/Prerequisites propios) — ese salto requeriría
	// partir UN documento en VARIAS Skills, que es un cambio de forma,
	// no de campo, y queda fuera de este bloque.
	ProcedureTitle string

	Content string // Texto curado completo, recortado a MaxNoteContentLength — fuente de verdad para 57 (búsqueda) y 58 (diff de cambio)

	State        SkillState // clasificación de cuánta estructura se extrajo — ver comentario del tipo
	Sources      []string   // Document.Source (Name de AuthorizedSource) que aportaron contenido
	Corroborated bool       // true si ValidationSet.Corroborated era true al construirla
	BuiltAt      string     // RFC3339 — cuándo se armó esta nota
	UpdatedAt    string     // RFC3339 — última vez que se reconstruyó (Remember la pisa)
}

// OperationStatus clasifica si Ahmelia reconoce/puede respaldar una
// operación requerida por un Step — tres niveles, no dos, porque
// "reconocido" y "ejecutable" NO son lo mismo (ver shared.WiredActions).
type OperationStatus string

const (
	// OpWired: existe una Action con lógica real conectada en
	// core/engine.go (shared.WiredActions[Action] == true).
	OpWired OperationStatus = "wired"
	// OpNamed: el sistema reconoce el CONCEPTO — o bien está en
	// shared.ActionPatterns pero shared.WiredActions no lo confirma
	// (ej. renombrar_archivo), o bien es un verbo de actions/ (blender,
	// unity, unreal, vscode — siempre Named, ese paquete es un
	// esqueleto sin Execute() real) — pero no hay implementación
	// funcional detrás todavía.
	OpNamed OperationStatus = "named"
	// OpUnknown: no matchea ningún Action ni verbo conocido. El Step
	// entero (no un fragmento) se usa como Text — sin NLP no se puede
	// segmentar con confianza un Step no reconocido en sub-operaciones.
	OpUnknown OperationStatus = "unknown"
)

// Operation es una necesidad de ejecución detectada dentro de un Step.
// Text es siempre lo que efectivamente se encontró en el texto (el
// trigger matcheado, el verbo reconocido, o el Step completo si
// Unknown) — nunca se inventa. Action es el identificador interno
// (clave de ActionPatterns o verbo de Handler.Actions()) si matcheó
// algo, "" si Unknown.
type Operation struct {
	Text   string
	Action string
	Status OperationStatus
}

// capUniqueList saca duplicados exactos (pueden aparecer si dos
// documentos distintos repiten la misma frase) preservando el orden
// de aparición, y recorta a max — mismo tope que ya regía por
// documento individual, ahora aplicado al resultado ya combinado de
// varios documentos.
func capUniqueList(items []string, max int) []string {
	if len(items) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(items))
	var out []string
	for _, it := range items {
		if seen[it] {
			continue
		}
		seen[it] = true
		out = append(out, it)
		if len(out) >= max {
			break
		}
	}
	return out
}

// HasProcedure es true si se extrajeron pasos reales del documento
// fuente — atajo booleano, mismo patrón que HasLead()/HasAnalogy() en
// el resto del proyecto.
func (s *Skill) HasProcedure() bool {
	return len(s.Steps) > 0
}

// EffectiveLabel devuelve ProcedureTitle si el texto fuente lo traía
// (la tarea concreta que resuelven los Steps), o Label si no (el
// dominio del LearningItem) — el mismo criterio "más específico gana
// si existe, si no cae al genérico" que ya usa el resto del proyecto
// (ej. Describe() con Corroborated). Quien necesite mostrarle al
// usuario UN nombre para la Skill debería usar esto, no Label
// directamente, para no anunciar "Unity" cuando en realidad se sabe
// "Crear un proyecto en Unity".
func (s *Skill) EffectiveLabel() string {
	if s.ProcedureTitle != "" {
		return s.ProcedureTitle
	}
	return s.Label
}

// HasExecutableOperations es true si AL MENOS UNA Operation es Wired.
// No dice que la Skill completa se pueda realizar — ver
// AllOperationsWired para eso.
func (s *Skill) HasExecutableOperations() bool {
	for _, op := range s.Operations {
		if op.Status == OpWired {
			return true
		}
	}
	return false
}

// AllOperationsWired es true únicamente si TODAS las Operations
// detectadas son Wired (y hay al menos una). Describe un hecho sobre
// el estado ACTUAL de shared.WiredActions, calculado en el momento en
// que se llama — no una garantía de que ejecutar la Skill vaya a
// funcionar (eso pertenece al futuro bloque de Execution, no al 54).
func (s *Skill) AllOperationsWired() bool {
	if len(s.Operations) == 0 {
		return false
	}
	for _, op := range s.Operations {
		if op.Status != OpWired {
			return false
		}
	}
	return true
}

// CapabilityCoverage devuelve (wired, total) — cuántas de las
// Operations detectadas son Wired, sobre el total.
//
// ADVERTENCIA DE INTERPRETACIÓN: esta proporción NO mide qué tan
// "completa" o "aprendida" está la Skill. Una Skill con 2 Operations
// (2 wired = 100%) no es más útil ni más avanzada que una con 10
// Operations (8 wired = 80%) — la segunda simplemente describe una
// tarea más compleja. Sirve solo para responder "de lo que esta Skill
// puntual necesita, cuánto tengo disponible hoy", nunca para comparar
// Skills entre sí.
func (s *Skill) CapabilityCoverage() (wired, total int) {
	total = len(s.Operations)
	for _, op := range s.Operations {
		if op.Status == OpWired {
			wired++
		}
	}
	return wired, total
}

// OperabilityLabel da una etiqueta legible del estado de Operations —
// deliberadamente sin la palabra "Executable" en ningún valor: esa
// palabra implica una garantía de ejecución que este bloque no puede
// dar (queda reservada para el futuro Bloque 61, Skill Execution).
func (s *Skill) OperabilityLabel() string {
	wired, total := s.CapabilityCoverage()
	switch {
	case total == 0 || wired == 0:
		return "KnowledgeOnly"
	case wired == total:
		return "OperationsFullyMapped"
	default:
		return "OperationsPartiallyMapped"
	}
}

// Describe arma la narrativa de una Skill puntual.
func (s *Skill) Describe() string {
	corrob := "sin corroborar"
	if s.Corroborated {
		corrob = "corroborada"
	}
	kind := "sin procedimiento ni condición de uso reconocibles en la fuente"
	switch {
	case s.State == StateProcedural:
		kind = fmt.Sprintf("procedural (%d paso(s) reconocidos)", len(s.Steps))
	case s.State == StateStructured:
		kind = fmt.Sprintf("estructurada (%d condición(es) de uso, sin pasos)", len(s.WhenToUse))
	}
	return fmt.Sprintf("%s — skill %s, %s, fuentes: %s.",
		s.EffectiveLabel(), kind, corrob, strings.Join(s.Sources, ", "))
}

// SkillLibrary persiste las Skills construidas, por SkillID — mismo
// criterio exacto que KnowledgeMemory (49): un mapa cargado de disco al
// arrancar, guardado de inmediato en cada actualización.
//
// El tag JSON se mantiene "notes" a propósito, aunque el tipo Go se
// llame Skills: es compatibilidad hacia atrás con cualquier
// memory/skill_library.json ya persistido por la versión anterior de
// este bloque — cambiar la clave rompería la carga de datos reales ya
// guardados en disco sin ganar nada a cambio. Los campos nuevos
// (Problem, WhenToUse, Steps, etc.) simplemente no estaban en los
// archivos viejos y quedan en su zero-value al cargarlos — mismo
// criterio defensivo de todo el proyecto: sin datos, vacío, nunca
// inventado.
type SkillLibrary struct {
	Skills map[string]*Skill `json:"notes"`
}

// SkillLibraryPath — mismo criterio que KnowledgeMemoryPath (49) y
// LearningStatePath (memory_intelligence).
const SkillLibraryPath = "memory/skill_library.json"

// NewSkillLibrary arma una biblioteca vacía.
func NewSkillLibrary() *SkillLibrary {
	return &SkillLibrary{Skills: make(map[string]*Skill)}
}

// LoadSkillLibrary carga skill_library.json si existe; si no, arranca
// vacía — mismo patrón que LoadKnowledgeMemory.
func LoadSkillLibrary(path string) *SkillLibrary {
	if data, err := ioutil.ReadFile(path); err == nil {
		var sl SkillLibrary
		if err := json.Unmarshal(data, &sl); err == nil && sl.Skills != nil {
			return &sl
		}
	}
	return NewSkillLibrary()
}

// Save persiste la biblioteca a disco.
func (sl *SkillLibrary) Save(path string) error {
	data, err := json.MarshalIndent(sl, "", "  ")
	if err != nil {
		return err
	}
	return ioutil.WriteFile(path, data, 0644)
}

// Has es true si ya existe una Skill para este SkillID — para que quien
// llame pueda decidir si vale la pena re-fetchear en vez de reconstruir
// siempre.
func (sl *SkillLibrary) Has(skillID string) bool {
	_, ok := sl.Skills[skillID]
	return ok
}

// Get devuelve la Skill guardada, si existe.
func (sl *SkillLibrary) Get(skillID string) (*Skill, bool) {
	n, ok := sl.Skills[skillID]
	return n, ok
}

// SkillBuilder construye Skills a partir de un ValidationSet ya
// evaluado (Bloque 53). Sin estado propio — la persistencia vive en
// SkillLibrary, no acá, mismo criterio que Validator (48) vs
// KnowledgeMemory (49): quien evalúa no es quien guarda.
type SkillBuilder struct{}

// NewSkillBuilder arma el constructor de skills.
func NewSkillBuilder() *SkillBuilder {
	return &SkillBuilder{}
}

// Build arma una Skill para un LearningItem a partir del ValidationSet
// que Bloque 53 ya calculó sobre los documentos traídos para ese
// objetivo. Devuelve nil si no hay NINGÚN documento confiable — no
// tiene sentido persistir una Skill sin contenido real detrás; eso
// sería fabricar una capacidad de la nada, exactamente lo que este
// bloque existe para evitar. Ese es el ÚNICO criterio que decide si
// hay Skill o no — ver la nota de diseño al principio del archivo
// sobre por qué State no gatilla el descarte. item y vs pueden ser
// nil — defensivo, mismo criterio de todo el proyecto.
func (sb *SkillBuilder) Build(item LearningItem, vs *ValidationSet) *Skill {
	if vs == nil || len(vs.Validations) == 0 {
		return nil
	}

	var reliable []*SourceValidation
	for _, v := range vs.Validations {
		if v.IsReliable() {
			reliable = append(reliable, v)
		}
	}
	if len(reliable) == 0 {
		return nil
	}

	// Orden estable por Source, para que el contenido armado (y por
	// lo tanto el recorte a MaxNoteContentLength y la extracción de
	// estructura) sea determinístico entre corridas, no dependa del
	// orden en que FetchAll devolvió los documentos.
	sort.Slice(reliable, func(i, j int) bool {
		return reliable[i].Document.Source < reliable[j].Document.Source
	})

	var sourceNames []string
	seenSource := make(map[string]bool)
	var content strings.Builder

	// Integration Fix — bug real encontrado en vivo: Discovery a veces
	// trae más de un documento para el mismo objetivo (ej. Go trae
	// go.dev Y pkg.go.dev siempre, sin importar cuál tenga el Resource
	// relevante). Antes, toda la extracción de estructura corría sobre
	// el texto YA CONCATENADO de todos los documentos — así que
	// extractProcedureTitle, al buscar "la línea de antes" de la
	// primera lista de pasos, podía terminar mirando la última línea
	// de un documento COMPLETAMENTE DISTINTO (el anterior en el orden
	// alfabético), y devolvía un fragmento sin sentido ("way, you
	// will" salió así, contaminado de la portada de pkg.go.dev). Ahora
	// cada documento se analiza POR SEPARADO, nunca cruzando el límite
	// entre fuentes.
	var whenToUse, pitfalls, prereqs, failureRecovery, expectedOutcome, limitations []string
	var steps []string
	var procedureTitle string
	for _, v := range reliable {
		if !seenSource[v.Document.Source] {
			sourceNames = append(sourceNames, v.Document.Source)
			seenSource[v.Document.Source] = true
		}
		if content.Len() > 0 {
			content.WriteString("\n\n")
		}

		// analysisText: el texto crudo tal como lo dio el Gateway,
		// CON los marcadores shared.HeadingMarker que
		// extractProcedureTitle necesita — nunca se persiste así, es
		// solo insumo de extracción. Bug encontrado antes (probando 5
		// fuentes de ejemplo): si la extracción corre sobre texto que
		// YA tiene el tag "[Source] " pegado, un trigger que matchea
		// la primera línea de un documento termina con el tag adentro
		// del campo extraído. Separar analysisText de lo que se
		// guarda como Content lo resuelve de raíz.
		analysisText := v.Document.Text

		// displayText: lo que se guarda en Content — con el tag de
		// fuente para atribución, y SIN los marcadores de heading (son
		// metadata interna del pipeline; la fuente nunca escribió
		// bytes \x00, no tiene sentido mostrárselos al usuario).
		displayText := strings.ReplaceAll(v.Document.Text, shared.HeadingMarker, "")
		docText := fmt.Sprintf("[%s] %s", v.Document.Source, displayText)
		content.WriteString(docText)

		whenToUse = append(whenToUse, extractByTriggers(analysisText, whenToUseTriggers, maxExtractedListItems)...)
		pitfalls = append(pitfalls, extractByTriggers(analysisText, pitfallTriggers, maxExtractedListItems)...)
		prereqs = append(prereqs, extractByTriggers(analysisText, prerequisiteTriggers, maxExtractedListItems)...)
		failureRecovery = append(failureRecovery, extractByTriggers(analysisText, failureRecoveryTriggers, maxExtractedListItems)...)
		expectedOutcome = append(expectedOutcome, extractByTriggers(analysisText, expectedOutcomeTriggers, maxExtractedListItems)...)
		limitations = append(limitations, extractByTriggers(analysisText, limitationTriggers, maxExtractedListItems)...)

		// Steps se queda con el PRIMER documento (en el orden estable
		// ya ordenado por Source) que tenga evidencia procedural real
		// — mezclar los pasos de dos páginas distintas en una sola
		// lista sería inventar un procedimiento que ninguna fuente
		// individual describe así.
		if len(steps) == 0 {
			if docSteps := extractSteps(analysisText); len(docSteps) > 0 {
				steps = docSteps
				procedureTitle = extractProcedureTitle(analysisText, docSteps)
			}
		}
	}
	whenToUse = capUniqueList(whenToUse, maxExtractedListItems)
	pitfalls = capUniqueList(pitfalls, maxExtractedListItems)
	prereqs = capUniqueList(prereqs, maxExtractedListItems)
	failureRecovery = capUniqueList(failureRecovery, maxExtractedListItems)
	expectedOutcome = capUniqueList(expectedOutcome, maxExtractedListItems)
	limitations = capUniqueList(limitations, maxExtractedListItems)

	// El recorte a MaxNoteContentLength es solo para lo que se guarda
	// como Content legible — la extracción de estructura, arriba, ya
	// corrió por documento sobre el texto completo de cada uno, nunca
	// sobre esta versión recortada.
	text := content.String()
	if len(text) > MaxNoteContentLength {
		text = strings.TrimSpace(text[:MaxNoteContentLength]) + "…"
	}

	// Zero-value ("") si no hubo ninguna señal — ver comentario del
	// tipo SkillState. Procedural gana sobre Structured si ambas
	// señales están presentes (TestBuild_StepsYWhenToUseJuntosDanProcedural).
	state := SkillState("")
	if len(whenToUse) > 0 {
		state = StateStructured
	}
	if len(steps) > 0 {
		state = StateProcedural
	}

	now := time.Now().Format(time.RFC3339)
	return &Skill{
		SkillID: item.TaskID,
		Label:   item.TaskDesc,
		Problem: item.TaskDesc,

		WhenToUse:     whenToUse,
		Steps:         steps,
		Pitfalls:      pitfalls,
		Prerequisites: prereqs,

		FailureRecovery: failureRecovery,
		ExpectedOutcome: expectedOutcome,
		Limitations:     limitations,
		Operations:      detectOperations(steps),
		ProcedureTitle:  procedureTitle,

		Content: text,

		State:        state,
		Sources:      sourceNames,
		Corroborated: vs.Corroborated,
		BuiltAt:      now,
		UpdatedAt:    now,
	}
}

// Install guarda (o actualiza) una Skill en la biblioteca. BuiltAt se
// conserva de la Skill anterior si ya existía — es la primera vez que
// se armó esta skill, no cambia porque se refresque el contenido;
// UpdatedAt sí se pisa siempre. Mismo espíritu que
// FirstValidatedAt/LastCheckedAt en Knowledge Memory (49). No-op si
// skill es nil.
func (sl *SkillLibrary) Install(skill *Skill) {
	if skill == nil || skill.SkillID == "" {
		return
	}
	if existing, ok := sl.Skills[skill.SkillID]; ok {
		skill.BuiltAt = existing.BuiltAt
	}
	skill.UpdatedAt = time.Now().Format(time.RFC3339)
	sl.Skills[skill.SkillID] = skill
}

// Describe arma el resumen de toda la biblioteca — orden estable por
// SkillID, mismo criterio que All() en cualquier módulo con mapa
// interno de este proyecto.
func (sl *SkillLibrary) Describe() string {
	if len(sl.Skills) == 0 {
		return "Todavía no hay ninguna nota de estudio guardada."
	}
	ids := make([]string, 0, len(sl.Skills))
	for id := range sl.Skills {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	var b strings.Builder
	fmt.Fprintf(&b, "%d nota(s) de estudio guardadas:\n", len(ids))
	for _, id := range ids {
		b.WriteString("  — " + sl.Skills[id].Describe() + "\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

// ── Detección de Operations (Bloque 54, rediseño) ───────────────────
//
// namedActionsVocabulary: verbos de Handler.Actions() de TODOS los
// paquetes actions/ registrados — leídos directo de cada Handler, SIN
// pasar por actions.Dispatcher (que no está instanciado en ningún
// lugar real del proyecto todavía, ver actions/dispatcher.go). Esto es
// lectura de metadata (qué verbos existen), no activación del sistema
// de dispatch — no ejecuta nada, no importa environment.Manager.
// Calculado una sola vez al cargar el paquete: la lista de Handlers
// registrados es estática en el código, no cambia en runtime.
var namedActionsVocabulary = collectNamedActionsVocabulary()

func collectNamedActionsVocabulary() map[string]bool {
	type actionsSource interface{ Actions() []string }
	handlers := []actionsSource{blender.New(), unity.New(), unreal.New(), vscode.New()}
	out := make(map[string]bool)
	for _, h := range handlers {
		for _, verb := range h.Actions() {
			out[shared.Normalize(verb)] = true
		}
	}
	return out
}

// detectOperations clasifica, para cada Step, qué Operations requiere
// y si Ahmelia las reconoce/respalda hoy. Sin NLP: matchea el Step
// normalizado contra dos vocabularios conocidos (ActionPatterns +
// verbos de actions/*). Un Step puede aportar más de una Operation si
// contiene varios matches reales (ej. "abre el editor y exporta el
// proyecto" matchea abrir_programa Y exportar). Si NO matchea nada, se
// agrega una única Operation Unknown con el Step completo como Text —
// sin NLP no se puede segmentar con confianza cuántas "operaciones
// desconocidas" hay dentro de un Step no reconocido.
func detectOperations(steps []string) []Operation {
	var ops []Operation
	// Orden estable de claves — ActionPatterns y namedActionsVocabulary
	// son maps, iterarlos directo daría un orden distinto en cada
	// corrida (mismo criterio que el resto del proyecto, ej.
	// Describe() en SkillLibrary).
	actionKeys := make([]string, 0, len(shared.ActionPatterns))
	for k := range shared.ActionPatterns {
		actionKeys = append(actionKeys, k)
	}
	sort.Strings(actionKeys)

	verbs := make([]string, 0, len(namedActionsVocabulary))
	for v := range namedActionsVocabulary {
		verbs = append(verbs, v)
	}
	sort.Strings(verbs)

	for _, step := range steps {
		normalized := shared.Normalize(step)
		found := false

		// 1. Contra shared.ActionPatterns — Wired si shared.WiredActions
		// lo confirma, Named si el trigger matchea pero no hay
		// implementación real detrás (ej. renombrar_archivo).
		for _, actionKey := range actionKeys {
			for _, trigger := range shared.ActionPatterns[actionKey] {
				// A trigger is an operation only when it appears in an
				// instruction-like position. Plain substring matching is too
				// permissive for procedural text: e.g. "Use the go command to
				// run your code" contains "run", but the step is describing a
				// command being used, not issuing an independent "run" action
				// to Ahmelia. Requiring a clause boundary prevents incidental
				// mentions from inflating capability coverage.
				if !isInstructionTrigger(normalized, shared.Normalize(trigger)) {
					continue
				}
				status := OpNamed
				if shared.WiredActions[actionKey] {
					status = OpWired
				}
				ops = append(ops, Operation{Text: trigger, Action: actionKey, Status: status})
				found = true
				break
			}
		}

		// 2. Contra los verbos de actions/* — siempre Named, ese
		// paquete es un esqueleto sin Execute() real (ver comentario de
		// namedActionsVocabulary arriba).
		for _, verb := range verbs {
			if strings.Contains(normalized, verb) {
				ops = append(ops, Operation{Text: verb, Action: verb, Status: OpNamed})
				found = true
			}
		}

		if !found {
			ops = append(ops, Operation{Text: step, Action: "", Status: OpUnknown})
		}
	}
	return ops
}

// isInstructionTrigger distinguishes an operation from an incidental
// mention of the same word. The check is intentionally syntactic and
// deterministic: accept the beginning of the step or a clear clause
// boundary ("and", "y", punctuation), but reject infinitive/reporting
// contexts such as "to run" or "para ejecutar". This keeps capability
// coverage honest without requiring an NLP model.
func isInstructionTrigger(text, trigger string) bool {
	text = strings.TrimSpace(shared.Normalize(text))
	trigger = strings.TrimSpace(shared.Normalize(trigger))
	if text == "" || trigger == "" {
		return false
	}
	for start := 0; ; {
		rel := strings.Index(text[start:], trigger)
		if rel < 0 {
			return false
		}
		pos := start + rel
		end := pos + len(trigger)
		if (pos == 0 || isClauseBoundary(text, pos)) &&
			(end == len(text) || isWordBoundary(text, end)) {
			return true
		}
		start = pos + 1
		if start >= len(text) {
			return false
		}
	}
}

func isWordBoundary(text string, pos int) bool {
	if pos >= len(text) {
		return true
	}
	return text[pos] == ' ' || text[pos] == ',' || text[pos] == '.' ||
		text[pos] == ';' || text[pos] == ':' || text[pos] == '!' || text[pos] == '?'
}

func isClauseBoundary(text string, pos int) bool {
	if pos <= 0 {
		return true
	}
	prefix := strings.TrimSpace(text[:pos])
	if prefix == "" {
		return true
	}
	if strings.HasSuffix(prefix, ",") || strings.HasSuffix(prefix, ";") ||
		strings.HasSuffix(prefix, ":") || strings.HasSuffix(prefix, ".") ||
		strings.HasSuffix(prefix, "!") || strings.HasSuffix(prefix, "?") {
		return true
	}
	for _, connector := range []string{"and", "y", "then", "luego", "despues", "despues"} {
		if strings.HasSuffix(prefix, connector) {
			return true
		}
	}
	return false
}

// ── Extracción heurística de estructura ────────────────────────────
//
// Sin NLP, sin LLM (ver el fork documentado al principio del
// archivo) — mismo patrón "lista de triggers + strings.Contains" que
// ya usan ~30 bloques del proyecto (shared.MatchesAny,
// shared.InferTags), aplicado acá a nivel de ORACIÓN en vez de a
// nivel de comando de voz completo.

// stepLinePattern reconoce una línea con forma de paso de
// procedimiento: numerada ("1. ", "2) ") o con viñeta ("- ", "* ",
// "• "). No interpreta el contenido de la línea, solo su forma.
var stepLinePattern = regexp.MustCompile(`^(?:\d{1,2}[\.\)]|[-*•])\s+(.{3,200})$`)

// extractSentences separa texto corrido en unidades simples — por
// salto de línea y por signo de cierre de oración. No es un parser de
// lenguaje real, es el mismo criterio simple y determinista de
// splitting que el resto del proyecto usa para todo lo demás.
func extractSentences(text string) []string {
	unified := strings.NewReplacer("\n", ". ", "\r", "").Replace(text)
	raw := strings.FieldsFunc(unified, func(r rune) bool {
		return r == '.' || r == '!' || r == '?'
	})
	var sentences []string
	for _, s := range raw {
		s = strings.TrimSpace(s)
		if s != "" {
			sentences = append(sentences, s)
		}
	}
	return sentences
}

// extractSteps busca líneas con forma de paso de procedimiento. Se
// exigen AL MENOS 2 líneas que matcheen para considerarlo una
// secuencia real — una sola línea suelta que matchee el patrón puede
// ser una referencia aislada (ej. una cita "[1] ...") y no un
// procedimiento, así que una sola coincidencia no cuenta como
// estructura.
// actionVerbs es evidencia léxica de que una línea es una INSTRUCCIÓN
// a ejecutar, no un ítem de lista cualquiera — hallazgo real contra
// datos en vivo: una tabla de contenidos también es una lista de
// <li>, pero ningún ítem arranca con un verbo en modo imperativo
// ("Descarga...", no "2.1.1. Argument Passing"). Deliberadamente
// chica y ampliable, mismo criterio que el resto de las listas de
// triggers del proyecto — no pretende ser exhaustiva, solo suficiente
// para distinguir "instrucción" de "título/enlace/sección".
var actionVerbs = []string{
	"descarga", "descargá", "ejecuta", "ejecutá", "corre", "corré",
	"abre", "abrí", "crea", "creá", "instala", "instalá",
	"configura", "configurá", "selecciona", "seleccioná",
	"escribe", "escribí", "guarda", "guardá", "copia", "copiá",
	"pega", "pegá", "agrega", "agregá", "elimina", "eliminá",
	"reinicia", "reiniciá", "confirma", "confirmá", "verifica", "verificá",
	"ve a", "andá a", "dirigite a", "dirígete a", "haz clic", "hacé clic",
	"download", "run", "open", "create", "install", "configure",
	"select", "click", "type", "enter", "save", "copy", "navigate",
	"go to", "choose", "add", "remove", "start", "stop", "verify",
}

// hierarchicalNumberPattern reconoce numeración de VARIOS niveles
// ("2.1.1.", "3.1.") — la forma típica de una entrada de tabla de
// contenidos (capítulo.sección.subsección), distinta de la numeración
// plana de una lista de pasos ("1.", "2.", "3."). Hallazgo real: el
// índice del tutorial de Python usaba EXACTAMENTE esta forma
// ("2.1.1. Argument Passing") para sus entradas — una línea así se
// descarta como candidata a Step antes de llegar a evaluarse
// siquiera, no compite con las líneas de un procedimiento real.
var hierarchicalNumberPattern = regexp.MustCompile(`^\d+(\.\d+)+\.?\s`)

// startsWithActionVerb compara, normalizado (shared.Normalize — sin
// acentos, minúsculas), si candidate arranca con alguno de
// actionVerbs.
func startsWithActionVerb(candidate string) bool {
	norm := shared.Normalize(candidate)
	for _, v := range actionVerbs {
		if norm == v || strings.HasPrefix(norm, v+" ") {
			return true
		}
	}
	return false
}

// hasProceduralEvidence decide si una lista de candidatos a Step
// REALMENTE parece un procedimiento — el refinamiento central de este
// cambio: antes, "es una lista con forma de <li>" alcanzaba para
// llamarla Steps. Eso es demasiado débil — un <li> solo significa
// "elemento de una lista", puede ser un paso, pero también una tabla
// de contenidos, navegación, una lista de características, requisitos
// o enlaces relacionados. Se exige AL MENOS una de dos evidencias
// independientes:
//   - Contexto: la línea justo antes de la lista termina en ":" (la
//     misma señal que ya usa extractProcedureTitle — "para instalar
//     esto, seguí estos pasos:")
//   - Léxico: la MITAD o más de los candidatos arrancan con un verbo
//     en modo imperativo reconocible (actionVerbs)
//
// Sin ninguna de las dos, se descarta la lista entera — es un límite
// honesto real, no solo teórico: preferimos declarar "sin
// procedimiento reconocible" antes que etiquetar una tabla de
// contenidos como si fuera una capacidad. El costo conocido es que
// una lista de pasos genuina, escrita SIN verbo imperativo al
// principio y SIN una frase introductoria con ":", puede quedar
// afuera (falso negativo) — se acepta ese costo a cambio de no volver
// a repetir el hallazgo real de hoy (falsos positivos de navegación y
// tabla de contenidos).
func hasProceduralEvidence(text string, lines []string, firstIdx int, candidates []string) bool {
	if firstIdx > 0 {
		lead := strings.TrimSpace(lines[firstIdx-1])
		if lead != "" && strings.HasSuffix(lead, ":") {
			return true
		}
	}
	verbCount := 0
	for _, c := range candidates {
		if startsWithActionVerb(c) {
			verbCount++
		}
	}
	return verbCount*2 >= len(candidates)
}

// extractSteps busca líneas con forma de paso de procedimiento y
// exige evidencia real de que la lista es un procedimiento (ver
// hasProceduralEvidence) — no solo su forma. Se exigen además AL
// MENOS 2 líneas con esa forma para considerarlo una secuencia real —
// una sola línea suelta que matchee el patrón puede ser una
// referencia aislada, no un procedimiento.
func extractSteps(text string) []string {
	lines := strings.Split(text, "\n")
	var candidates []string
	firstIdx := -1
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if hierarchicalNumberPattern.MatchString(trimmed) {
			// Numeración de tabla de contenidos ("2.1.1. ..."), no de
			// procedimiento — ni siquiera entra a competir como
			// candidato.
			continue
		}
		m := stepLinePattern.FindStringSubmatch(trimmed)
		if m == nil {
			continue
		}
		if firstIdx == -1 {
			firstIdx = i
		}
		candidates = append(candidates, strings.TrimSpace(m[1]))
	}
	if len(candidates) < 2 {
		return nil
	}
	if !hasProceduralEvidence(text, lines, firstIdx, candidates) {
		return nil
	}
	if len(candidates) > maxExtractedSteps {
		candidates = candidates[:maxExtractedSteps]
	}
	return candidates
}

// sourceTagPattern reconoce el prefijo "[Fuente] " — ya NO debería
// aparecer en analysisText (Build() separa analysisText de displayText
// desde el fix del bug de contaminación de tag, ver comentario en
// Build()), así que esto es defensa en profundidad, no el mecanismo
// principal: si algún llamador futuro le pasa a extractProcedureTitle
// texto que todavía trae el tag, igual sale limpio.
var sourceTagPattern = regexp.MustCompile(`^\[[^\]]{1,80}\]\s*`)

// startsLowercase es true si s empieza con una letra minúscula — señal
// de que la línea es la continuación de una oración cortada (por un
// <br>, por ejemplo), no el principio de una oración nueva. Un s que
// empieza con dígito, símbolo, o está vacío no cuenta como minúscula
// (defensivo: solo afirma "sí" cuando hay evidencia real de letra).
func startsLowercase(s string) bool {
	for _, r := range s {
		return unicode.IsLower(r)
	}
	return false
}

// negativeTitlePhrases son frases que NO deben aceptarse solas como
// ProcedureTitle cuando la línea EMPIEZA con ellas — son
// introducciones genéricas a una lista ("estas son las cosas que vas
// a hacer"), no el nombre de la tarea que esa lista describe.
// Encontrado en vivo probando con Go Docs real: "Along the way, you
// will:" se aceptó como ProcedureTitle de una Skill de Go, cuando el
// nombre real de la tarea ("Tutorial: Get started with Go") era un
// heading varias líneas antes — ver extractProcedureTitle.
//
// Deliberadamente NO se usa "contiene en cualquier posición": un buen
// título real como "Para instalar Unity, seguí estos pasos" TERMINA
// con una frase parecida ("estos pasos") sin ser genérico — lo
// específico ("instalar Unity") va adelante. Solo cuenta como señal
// de genericidad si la frase abre la línea, que es el patrón real que
// se vio en vivo.
var negativeTitlePhrases = []string{
	"along the way", "the following", "a continuacion", "lo siguiente",
	"esto es lo que", "you will",
}

// isGenericListIntro compara normalizado (shared.Normalize — mismo
// criterio que el resto del bloque) para no depender de mayúsculas o
// acentos exactos de la fuente, y solo rechaza si la línea EMPIEZA con
// una frase genérica — ver comentario de negativeTitlePhrases.
func isGenericListIntro(s string) bool {
	normalized := shared.Normalize(s)
	for _, phrase := range negativeTitlePhrases {
		if strings.HasPrefix(normalized, phrase) {
			return true
		}
	}
	return false
}

// extractProcedureTitle busca, SOLO cuando ya hay Steps reales, un
// nombre para la tarea concreta que esos Steps resuelven — el lugar
// donde la prosa tutorial casi siempre lo dice, en vez de quedarse con
// el dominio genérico que trae el LearningItem ("Unity" en vez de
// "Crear un proyecto en Unity"). Es la respuesta heurística, sin NLP,
// a la distinción entre Nivel A (Knowledge sobre un dominio) y Nivel B
// (Procedural Knowledge — una tarea concreta con procedimiento).
//
// Dos prioridades, en orden:
//
//  1. El heading real (shared.HeadingMarker) más cercano ANTES de la
//     lista, buscando hacia atrás hasta maxHeadingLookback líneas. Es
//     más confiable que cualquier heurística de puntuación porque es
//     una señal que el propio HTML fuente ya declaraba ("esto era un
//     <h1>-<h6>"), no una inferencia sobre dónde cae un ":".
//  2. Si no hay heading en la ventana (o el que hay es genérico/vacío):
//     la línea inmediatamente anterior a la lista, SOLO si termina en
//     ":" y no es una introducción genérica (negativeTitlePhrases) —
//     mismo criterio de siempre, con el filtro nuevo.
//
// Sin ninguna de las dos señales, devuelve "" — fiel a la fuente en
// vez de adivinar cuál de las oraciones anteriores es el título real.
// No hay Nivel C acá: esto sigue siendo UN título por Skill, no una
// separación de la Skill en varias capacidades distintas — eso queda
// fuera de alcance (ver nota en el encabezado del archivo).
func extractProcedureTitle(text string, steps []string) string {
	if len(steps) == 0 {
		return ""
	}
	lines := strings.Split(text, "\n")

	firstStepIdx := -1
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if hierarchicalNumberPattern.MatchString(trimmed) {
			// Mismo criterio que extractSteps: una línea con
			// numeración de tabla de contenidos no cuenta como "la
			// primera línea de la lista" — si no, esta función podría
			// mirar una línea distinta a la que extractSteps
			// realmente usó como primer Step.
			continue
		}
		if stepLinePattern.MatchString(trimmed) {
			firstStepIdx = i
			break
		}
	}
	if firstStepIdx <= 0 {
		return ""
	}

	// Prioridad 1 — heading real, ventana acotada hacia atrás (mismo
	// espíritu que maxLeadLines más abajo: un límite explícito en vez
	// de escanear el documento entero, para no terminar agarrando un
	// heading de una sección completamente distinta muy arriba).
	const maxHeadingLookback = 40
	for i, scanned := firstStepIdx-1, 0; i >= 0 && scanned < maxHeadingLookback; i, scanned = i-1, scanned+1 {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, shared.HeadingMarker) {
			heading := strings.TrimPrefix(trimmed, shared.HeadingMarker)
			heading = sourceTagPattern.ReplaceAllString(heading, "")
			heading = strings.TrimSpace(heading)
			if heading != "" && !isGenericListIntro(heading) {
				return heading
			}
			// Heading real encontrado pero vacío/genérico — no seguir
			// subiendo más allá de él, sería una señal todavía peor.
			// Cae a prioridad 2.
			break
		}
	}

	// Prioridad 2 — heurística original: línea(s) inmediatamente
	// anterior(es) a la lista, si terminan en ":" y no son una
	// introducción genérica.
	lastLine := strings.TrimSpace(lines[firstStepIdx-1])
	leadParts := []string{lastLine}
	if startsLowercase(lastLine) {
		const maxLeadLines = 3
		for j := firstStepIdx - 2; j >= 0 && len(leadParts) < maxLeadLines; j-- {
			l := strings.TrimSpace(lines[j])
			if l == "" {
				break
			}
			leadParts = append([]string{l}, leadParts...)
			if !startsLowercase(l) {
				break // acá arranca la oración real
			}
		}
	}
	lead := strings.Join(leadParts, " ")
	if lead == "" || !strings.HasSuffix(lead, ":") {
		return ""
	}
	lead = strings.TrimSuffix(lead, ":")
	lead = sourceTagPattern.ReplaceAllString(lead, "")
	lead = strings.TrimSpace(lead)
	if lead == "" || isGenericListIntro(lead) {
		return ""
	}
	return lead
}

// extractByTriggers devuelve, en orden de aparición y sin duplicados,
// las oraciones de text que contienen alguno de triggers (comparación
// normalizada: minúsculas, sin acentos ni puntuación — shared.Normalize,
// la misma función que usa el resto del proyecto). Devuelve como
// mucho max resultados. text vacío o triggers vacío → nil, defensivo.
func extractByTriggers(text string, triggers []string, max int) []string {
	if text == "" || len(triggers) == 0 {
		return nil
	}
	var found []string
	seen := make(map[string]bool)
	for _, sentence := range extractSentences(text) {
		normalized := shared.Normalize(sentence)
		if normalized == "" || seen[normalized] {
			continue
		}
		if shared.MatchesAny(normalized, triggers) {
			found = append(found, sentence)
			seen[normalized] = true
			if len(found) >= max {
				break
			}
		}
	}
	return found
}

// whenToUseTriggers — frases que indican una condición de activación
// explícita ("usa X cuando Y", "si necesitás Z"). Todas en minúsculas
// y sin acentos porque se comparan contra shared.Normalize(oración).
var whenToUseTriggers = []string{
	"usa esto cuando", "usalo cuando", "se usa cuando", "se utiliza cuando",
	"conviene usar cuando", "conviene usarlo cuando", "se recomienda cuando",
	"se recomienda usar cuando", "ideal cuando", "es ideal cuando",
	"cuando necesites", "cuando necesitas", "si necesitas", "si necesitas",
	"si queres", "si quieres", "sirve cuando", "sirve para cuando",
	"aplica cuando", "es util cuando", "es util para cuando",
	"recomendado cuando", "usalo si necesitas", "usa x cuando",
}

// pitfallTriggers — frases de negación/advertencia explícita sobre
// errores a evitar.
var pitfallTriggers = []string{
	"no hagas", "no uses", "nunca hagas", "nunca uses", "evita",
	"evitar", "el error comun es", "un error comun es", "error comun",
	"cuidado con", "ten cuidado con", "atencion", "advertencia",
	"no confundas", "no olvides evitar",
}

// prerequisiteTriggers — frases que mencionan explícitamente qué hace
// falta ANTES de poder usar lo descrito.
var prerequisiteTriggers = []string{
	"necesitas tener", "necesitas instalar", "requiere tener",
	"requiere que tengas", "antes de esto necesitas", "es necesario tener",
	"debes tener instalado", "hace falta tener", "prerequisito",
	"requisito previo", "primero necesitas",
}

// failureRecoveryTriggers — frases que describen qué hacer SI un error
// ya ocurrió. Distinto de pitfallTriggers (qué NO hacer para evitarlo):
// acá el error ya pasó y el texto da un paso de recuperación.
var failureRecoveryTriggers = []string{
	"si falla", "si esto falla", "si no funciona", "si esto no funciona",
	"en caso de error", "si sigue fallando", "si continua fallando",
	"si no ves el resultado", "si aparece un error", "si aparece este error",
}

// expectedOutcomeTriggers — frases que describen cómo se ve el éxito.
// Va a quedar vacío la mayoría de las veces (ver comentario del campo
// ExpectedOutcome en el struct Skill) — eso es correcto, no un fallo.
var expectedOutcomeTriggers = []string{
	"el resultado es", "el resultado deberia ser", "deberias ver",
	"como resultado obtienes", "sabras que funciono si", "esto deberia mostrar",
	"resultado esperado", "al finalizar deberias", "el objetivo es que",
}

// limitationTriggers — Integration Fix (Skill Model Refinement): frases
// que declaran explícitamente el ALCANCE del procedimiento, no un
// error a evitar (pitfallTriggers) ni una recuperación (failureRecoveryTriggers).
var limitationTriggers = []string{
	"no funciona en", "no funciona con", "no esta pensado para",
	"no esta disponible en", "no soporta", "no es compatible con",
	"no garantiza", "no cubre", "no incluye", "solo aplica a",
	"solo funciona en", "solo esta disponible en", "esta limitado a",
	"limitacion conocida", "ten en cuenta que no",
}

// ── Triggers de consulta ──────────────────────────────────────────────

// skillBuilderTriggers consulta la biblioteca YA guardada — sin tocar
// la red, mismo espíritu que Recall() en Knowledge Memory (49).
var skillBuilderTriggers = []string{
	"que notas de estudio tienes",
	"tienes apuntes de",
	"tu biblioteca de conocimiento",
	"que hay guardado sobre",
}

// ParseSkillBuilderQuery detecta si se pregunta por las notas de
// estudio YA guardadas (Skill Builder/Library) — distinto de
// ParseSourceValidationQuery (53), que pregunta si una fuente ES
// confiable AHORA MISMO, sin persistir nada, y de
// ParseSkillBuildQuery, que SÍ dispara la cadena completa de I/O.
func ParseSkillBuilderQuery(req interaction.SemanticRequest) bool {
	return interaction.HasSemanticQuery(req, interaction.QuerySolvingSkillBuilder)
}

// skillBuildTriggers dispara la cadena completa Discover → Fetch →
// Validate → Build → Install — I/O real, distinto de solo consultar
// lo que ya existe (skillBuilderTriggers, arriba).
var skillBuildTriggers = []string{
	"guarda una nota sobre",
	"guarda lo que aprendiste sobre",
	"construye una nota de estudio sobre",
	"instala una nota sobre",
	// Integration Fix — forma imperativa ("aprende X"), la natural en
	// un comando de voz, distinta de "aprender" (infinitivo, la única
	// forma que cubría todo Fase 3/4 hasta acá — ver
	// dialogue.learningGoalHints). "aprende sobre" va primero para que
	// stripTrigger() la prefiera cuando esté presente y no deje
	// "sobre" pegado al MainGoal. El espacio final en "aprende " es
	// obligatorio: sin él, "aprende" matchea como substring de
	// "aprender" (aprendER unity) y stripTrigger corta mal, dejando
	// "R unity" en vez de "Unity" — el espacio fuerza el límite de
	// palabra sin tocar el mecanismo general de Contains() que usan
	// las otras ~30 listas de triggers.
	"aprende sobre",
	"aprende ",
	// Integration Fix — forma infinitiva ("aprender X"), la que usa
	// PendingClarification al fusionar objetivo original + aclaración
	// (memory/session.go, core/engine.go) cuando el objetivo original
	// vino dicho así ("aprender blender", no "aprende blender").
	// los antiguos learningGoalHints del Dialogue Manager ya cubrían esta
	// forma para decidir SI preguntar — pero esta lista, la que
	// dispara la ejecución real, nunca la tuvo: sin este trigger, un
	// objetivo fusionado en infinitivo llegaba hasta acá y no activaba
	// Discover→Fetch→Validate→Build, aunque ya tuviera dominio.
	// "aprender sobre" antes que "aprender " por el mismo motivo que
	// "aprende sobre"/"aprende " arriba: preferencia de stripTrigger().
	"aprender sobre",
	"aprender ",
}

// ParseSkillBuildQuery detecta si se pide construir (y persistir) una
// nota nueva a partir de fuentes reales — no solo consultar lo ya
// guardado.
func ParseSkillBuildQuery(req interaction.SemanticRequest) bool {
	return interaction.HasSemanticQuery(req, interaction.QuerySolvingSkillBuild)
}
