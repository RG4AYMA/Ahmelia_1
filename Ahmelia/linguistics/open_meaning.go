package linguistics

// open_meaning.go — Contrato de COMPRENSIÓN ABIERTA (directiva del propietario
// 2026-10-16 16:04 / [[Rediseño_Comprension_Abierta_Directiva_Propietario_2026-10-17]]).
//
// Qué cambiará cuando se integre: el rol de interpretationJSONSchema()+Interpretation
// en la cadena conversacional pasa a OpenMeaning. La discriminación fina (preferencia,
// corrección, comentario de tercero...) deja de ser un diccionario de 17 predicados:
// se razona después sobre purpose+meaning con los mecanismos de Ahmelia.
//
// INERTE hasta integración: nada del runtime llama a estas piezas (patrón T4:
// construir nuevo, cablear con UNA línea cuando toque). Extiende M1 (lenguaje).
// Sin literales de escena (R11). Sin autoridad: nunca decides ni ejecutas (0.1).

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// OpenPurpose es el enrutador mínimo de conducta. Es mucho más pequeño que el
// viejo intent de 6 valores+17 predicados: su único trabajo es decirle a los
// mecanismos a qué puerta va el significado; el CONTENIDO vive en Meaning libre.
type OpenPurpose string

const (
	OpenPurposeQuestion    OpenPurpose = "pregunta"
	OpenPurposeDeclaration OpenPurpose = "declaracion"
	OpenPurposeRequest     OpenPurpose = "pedido"
	OpenPurposeDelivery    OpenPurpose = "entrega" // entrega de contenido/artefacto
	OpenPurposeFeedback    OpenPurpose = "feedback"
	OpenPurposeSocial      OpenPurpose = "social"
	OpenPurposeOperational OpenPurpose = "operativa"
	OpenPurposeUnknown     OpenPurpose = "desconocido"
)

// OpenOwner es el propietario semántico del dato del turno (misma regla que ya
// gobierna el User Model: si es "user", nada del dato puede atribuirse a Ahmelia).
type OpenOwner string

const (
	OpenOwnerUser         OpenOwner = "user"
	OpenOwnerAhmelia      OpenOwner = "ahmelia"
	OpenOwnerExternal     OpenOwner = "externo"
	OpenOwnerUndetermined OpenOwner = "no_determinado"
)

// OpenMeaning es el resultado del nuevo contrato: significado en lengua natural
// (el contenido libre) + la señal mínima estable que necesitan los mecanismos.
type OpenMeaning struct {
	Purpose     OpenPurpose `json:"purpose"`
	OwnerOfData OpenOwner   `json:"owner_of_data"`
	Topic       string      `json:"topic"`
	Meaning     string      `json:"meaning"`
	Mentions    []string    `json:"mentions"`
	Confidence  float64     `json:"confidence"`

	// Resolutions son las referencias contextuales resueltas con hechos del
	// contexto autorizado. El arnés descarta las que citen IDs inexistentes.
	Resolutions []ReferenceResolution `json:"resolutions,omitempty"`

	// ProbeFlapRetry es honestidad para los mecanismos (BF1): registra que esta
	// comprensión avanzó DESPUÉS de cruzar el retry único del probe de
	// disponibilidad (DeepSeek 17:25). No entra al contrato JSON (json:"-").
	ProbeFlapRetry bool `json:"-"`

	// Marks es la representación explícita B2 (principio 2 del propietario);
	// opcional en el JSON: modelos antiguos sin marks dejan cero marcas y el
	// contracto no cambia de forma.
	Marks OpenMarks `json:"marks,omitempty"`
}

// ErrOpenMeaningProbeFlap separa, para el trazador diferido de DeepSeek, el caso
// en que el probe siguió inestable tras el retry único ("si el segundo probe
// falla, fallar rápido"): el consumidor lo detecta con errors.Is y pinta
// probe=flap retry=fail sin tocar su carril.
var ErrOpenMeaningProbeFlap = errors.New("open meaning: probe de disponibilidad inestable tras retry único")

// probeRetryPause es la pausa corta del retry único antes del segundo probe
// (DeepSeek 17:25: 300-500 ms; elegimos el centro). La puerta sync (~900 ms)
// no la paga: si el presupuesto del ctx es menor, se falla rápido y el turno
// queda sin enriquecimiento con la conducta sostenida por el carril determinista.
const probeRetryPause = 400 * time.Millisecond

// openMeaningJSONSchema devuelve la gramática para la decodificación restringida.
// Nada aquí discrimina contenido: la única taxonomía finita son los 8 propósitos
// de conducta.
func openMeaningJSONSchema() json.RawMessage {
	return json.RawMessage(`{
		"type":"object",
		"properties":{
			"purpose":{"type":"string","enum":["pregunta","declaracion","pedido","entrega","feedback","social","operativa","desconocido"]},
			"owner_of_data":{"type":"string","enum":["user","ahmelia","externo","no_determinado"]},
			"topic":{"type":"string"},
			"meaning":{"type":"string"},
			"mentions":{"type":"array","items":{"type":"string"}},
			"resolutions":{"type":"array","items":{"type":"object","properties":{"reference":{"type":"string"},"resolved":{"type":"string"},"evidence_ids":{"type":"array","items":{"type":"string"}},"confidence":{"type":"number","minimum":0,"maximum":1}},"required":["reference","resolved","evidence_ids"]}},
			"confidence":{"type":"number","minimum":0,"maximum":1},
			"marks":{
				"type":"object",
				"properties":{
					"negation":{"type":"object","properties":{"scope":{"type":"string"},"target":{"type":"string"}},"required":["scope","target"]},
					"condition":{"type":"object","properties":{"antecedent":{"type":"string"},"consequent":{"type":"string"}},"required":["antecedent","consequent"]},
					"exception":{"type":"object","properties":{"set":{"type":"string"},"excluded":{"type":"string"}},"required":["set","excluded"]},
					"modality":{"type":"string","enum":["hypothesis","affirmed","reported","observed","historical","unknown"]},
					"provenance":{"type":"string","enum":["user","llm","observed","external"]},
					"incompatibility":{"type":"array","items":{"type":"object","properties":{"a":{"type":"string"},"b":{"type":"string"},"reason":{"type":"string"}},"required":["a","b","reason"]}}
				}
			}
		},
		"required":["purpose","owner_of_data","topic","meaning","mentions","confidence"]
	}`)
}

// buildOpenMeaningPrompt construye la instrucción de comprensión abierta.
// Más corta que la del diccionario: no tiene que explicar 17 predicados porque
// ya no los devuelve. Se prohíbe decidir y ejecutar (0.1) y guardar silencios.
func buildOpenMeaningPrompt(input string) string {
	return buildOpenMeaningPromptWithSituation(input, nil)
}

// buildOpenMeaningPromptWithSituation includes the bounded, harness-owned
// context block. The empty/nil case is byte-identical to the legacy prompt.
func buildOpenMeaningPromptWithSituation(input string, snapshot *SituationSnapshot) string {
	contextBlock := ""
	if !snapshot.Empty() {
		contextBlock = "\nCONTEXTO AUTORIZADO (proyección verificada del estado; incluye turnos previos y evidencia del sistema):\n" +
			snapshot.Render() +
			"\n\nReglas del contexto autorizado:\n" +
			"- Puedes usar estos hechos para comprender el turno y resolver referencias.\n" +
			"- Cada resolución debe citar en \"evidence_ids\" los IDs exactos que usó.\n" +
			"- Un ID que no aparezca arriba invalida la resolución y será descartado.\n" +
			"- Si ningún hecho sostiene la resolución, no la propongas.\n" +
			"- No cambies de dueño un dato por aparecer en este contexto.\n"
	}

	return `Eres exclusivamente el módulo lingüístico interno de un sistema llamado Ahmelia.

Tu función es COMPRENDER la entrada del usuario para que Ahmelia razone sobre tu comprensión.

NO eres Ahmelia. NO decides por Ahmelia. NO razonas qué acción es mejor. NO ejecutas acciones.
NO inventes hechos, datos o contexto que no estén en el texto de la entrada.
NO resuelvas referencias con información que no aparezca en el texto ni en el CONTEXTO AUTORIZADO.
NO respondas al usuario. NO expliques tu análisis.

Devuelve EXCLUSIVAMENTE un objeto JSON válido. Sin Markdown, sin bloques de código, sin texto extra.

Campos:

"purpose" — el propósito de conducta de esta entrada, uno de:
- "pregunta": pide información
- "declaracion": afirma o comunica algo (hecho, preferencia, corrección incluida como "declaracion" de corrección cuando corrige algo anterior)
- "pedido": instrucción o solicitud de que se realice algo
- "entrega": entrega/aporta contenido, archivos o resultados
- "feedback": evalúa o valora el comportamiento de Ahmelia
- "social": saluda, conversa sin pedir nada
- "operativa": habla del sistema o del ambiente de trabajo
- "desconocido": si no es clasificable con honradez

"owner_of_data" — a quién pertenece el dato principal del turno:
- "user" si habla de sí o de algo suyo (sus dispositivos, sus archivos, sus sensaciones, su trabajo).
- Archivos, dispositivos, configuraciones o cosas del hogar SIN dueño explícito en el texto → "user" (es la conversación con su dueño).
- "externo" solo si se refiere a terceros ajenos o al mundo externo.
- "ahmelia" SOLO si el texto habla explícitamente de Ahmelia misma (lo que ella dijo, sabe o hace). NUNCA elijas "ahmelia" por defecto.
- "no_determinado" si no se puede honradamente.

"topic" — tema central EN PALABRAS del usuario.

"meaning" — el significado COMPLETO de la entrada en español natural, descripción neutral que capture TODO lo expreso incluyendo tono y correcciones, sin añadir nada ausente. Si la entrada es ambigua, significa la ambigüedad en lugar de elegir una rama.

"mentions" — entidades y nombres tal como aparecieron en el texto (verbatim). Si el dato es de un dispositivo o archivos del usuario, pertenecen al user.

"resolutions" — referencias contextuales resueltas con hechos del CONTEXTO AUTORIZADO. Cada elemento: {"reference":"la expresión del usuario","resolved":"a qué se refiere","evidence_ids":["f1"],"confidence":0.0}. Omite el campo si no hay contexto o no puedes resolver nada. No inventes IDs.

"confidence" — número entre 0 y 1: tu confianza en la comprensión. Nunca 0.0 si es comprensible.

"marks" — marcas explícitas del turno; omítelas si ninguna aplica. Son LECTURAS, no decisiones:

"negation" — si la entrada niega o prohíbe algo: {"scope":"accion|estado|cantidad","target":"aquello que NO debe o no es"}. Una orden negada es pedido que NO hace la acción.

"condition" — si una acción depende de algo: {"antecedent":"lo primero","consequent":"lo que pasa si el antecedent se cumple"}.

"exception" — si hay excepción: {"set":"a quién se aplica","excluded":"quién queda fuera"}.

"modality" — cómo se presenta el dato del turno: "hypothesis" (es posible que...), "affirmed" (el usuario lo afirma), "reported" (cuenta algo dicho por otro o de archivo), "observed", "historical", "unknown". NUNCA conviertas una hipótesis en hecho.

"provenance" — la fuente del hecho: "user"|"llm"|"observed"|"external".

"incompatibility" — si dos partes del mismo texto se contradicen: lista [{"a":"primera afirmación","b":"segunda afirmación","reason":"por qué chocan (mismo lugar y período, etc.)"}]. NO las suavices ni elijas una rama: déjalas MARCADAS.

Disciplina de marcas: emite TODAS las marcas que el texto expresa; una frase puede llevar varias a la vez. Ejemplos: una orden negada ("no abras el archivo") lleva negation; una acción condicionada ("si termina la descarga, abre el documento") lleva condition con sus dos partes; una exclusión ("todos excepto los visitantes") lleva exception y NO negation. No inventes marcas que el texto no tiene; no omitas las que sí tiene.

Formato:
{"purpose":"...","owner_of_data":"...","topic":"...","meaning":"...","mentions":[...],"resolutions":[],"confidence":0.0}

` + contextBlock + `
Entrada del usuario: ` + fmt.Sprintf("%q", input)
}

// ParseOpenMeaning valida y normaliza la salida del modelo. El validador es
// defensivo: la gramática garantiza la forma; aquí se garantiza el CONJUNTO de
// dominio y la higiene — nada entra al mecanismo con verdades descalabazadas.
// Exportada: core y variation la usan para fijar baterías sobre el mismo camino
// de producción.
func ParseOpenMeaning(raw string) (OpenMeaning, error) {
	var m OpenMeaning
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return OpenMeaning{}, fmt.Errorf("open meaning: json inválido: %w", err)
	}
	m.Purpose = normalizeOpenPurpose(m.Purpose)
	m.OwnerOfData = normalizeOpenOwner(m.OwnerOfData)
	m.Topic = strings.TrimSpace(m.Topic)
	m.Meaning = strings.TrimSpace(m.Meaning)
	if m.Meaning == "" {
		return OpenMeaning{}, fmt.Errorf("open meaning: el significado no puede quedar vacío")
	}
	kept := make([]string, 0, len(m.Mentions))
	for _, s := range m.Mentions {
		if t := strings.TrimSpace(s); t != "" {
			kept = append(kept, t)
		}
	}
	m.Mentions = kept
	m.Marks = normalizeOpenMarks(m.Marks)
	if m.Confidence < 0 {
		m.Confidence = 0
	}
	if m.Confidence > 1 {
		m.Confidence = 1
	}
	return m, nil
}

func normalizeOpenPurpose(p OpenPurpose) OpenPurpose {
	switch OpenPurpose(strings.TrimSpace(string(p))) {
	case OpenPurposeQuestion, OpenPurposeDeclaration, OpenPurposeRequest, OpenPurposeDelivery,
		OpenPurposeFeedback, OpenPurposeSocial, OpenPurposeOperational:
		return OpenPurpose(strings.TrimSpace(string(p)))
	default:
		return OpenPurposeUnknown
	}
}

func normalizeOpenOwner(o OpenOwner) OpenOwner {
	switch OpenOwner(strings.TrimSpace(string(o))) {
	case OpenOwnerUser, OpenOwnerAhmelia, OpenOwnerExternal:
		return OpenOwner(strings.TrimSpace(string(o)))
	default:
		return OpenOwnerUndetermined
	}
}

// structuredOpenClient es el límite opcional que ofrece el cliente real de
// producción (OllamaClient lo implementa con su timeout de 180 s).
type structuredOpenClient interface {
	GenerateStructuredContext(ctx context.Context, prompt string, schema json.RawMessage, numPredict int) (string, error)
	Available() bool
}

// InterpretOpenContext es la puerta de comprensión abierta. INERTE: ningún
// código del runtime la llama todavía; se cablea con UNA línea en el hito.
func (i *Interpreter) InterpretOpenContext(ctx context.Context, input string) (OpenMeaning, error) {
	return i.InterpretOpenContextWithSituation(ctx, input, nil)
}

// InterpretOpenContextWithSituation is the open-comprehension path with a
// bounded harness snapshot. Resolutions that cite unknown IDs are discarded
// before the meaning reaches any consumer.
func (i *Interpreter) InterpretOpenContextWithSituation(ctx context.Context, input string, snapshot *SituationSnapshot) (OpenMeaning, error) {
	if i == nil || i.client == nil {
		return OpenMeaning{}, fmt.Errorf("open meaning: intérprete no inicializado")
	}
	client, ok := i.client.(structuredOpenClient)
	if !ok {
		return OpenMeaning{}, fmt.Errorf("open meaning: el cliente no ofrece el límite estructurado")
	}
	if client == nil {
		return OpenMeaning{}, fmt.Errorf("open meaning: cliente nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	input = strings.TrimSpace(input)
	if input == "" {
		return OpenMeaning{}, fmt.Errorf("open meaning: entrada vacía")
	}
	var flapRetry bool
	if !client.Available() {
		flapRetry = true
		// H3 (DeepSeek 17:25): pausa corta + re-probe único + fail fast.
		// La puerta sync (~900 ms) no paga el retry: con presupuesto menor
		// el turno queda sin enriquecimiento y la conducta se sostiene por
		// el carril determinista (H5 verificado en corrrida viva 17:09).
		if dl, tienePresupuesto := ctx.Deadline(); tienePresupuesto && time.Until(dl) < probeRetryPause+300*time.Millisecond {
			return OpenMeaning{}, fmt.Errorf("open meaning: flap de probe con presupuesto corto del llamador: %w", ErrOpenMeaningProbeFlap)
		}
		time.Sleep(probeRetryPause)
		if !client.Available() {
			return OpenMeaning{}, fmt.Errorf("open meaning: flap persistente del probe tras retry único: %w", ErrOpenMeaningProbeFlap)
		}
	}
	raw, err := client.GenerateStructuredContext(ctx, buildOpenMeaningPromptWithSituation(input, snapshot), openMeaningJSONSchema(), 256)
	if err != nil {
		if flapRetry {
			return OpenMeaning{}, fmt.Errorf("open meaning: generación tras cruzar el retry del probe: %w", err)
		}
		return OpenMeaning{}, fmt.Errorf("open meaning: generación: %w", err)
	}
	m, err := ParseOpenMeaning(raw)
	if err != nil {
		return OpenMeaning{}, err
	}
	m.Resolutions = SanitizeResolutions(m.Resolutions, snapshot)
	m.ProbeFlapRetry = flapRetry
	return m, nil
}
