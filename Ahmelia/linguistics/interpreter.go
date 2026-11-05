package linguistics

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"ahmelia/comprehension"
)

// Interpretation representa únicamente la interpretación lingüística
// de una entrada del usuario.
//
// IMPORTANTE:
// Esta estructura NO representa una decisión cognitiva.
// El LLM puede ayudar a identificar la forma lingüística, entidades,
// referencias y posibles restricciones, pero Ahmelia decide qué hacer
// posteriormente mediante su propio modelo cognitivo.
type Relation struct {
	Source     string  `json:"source"`
	Predicate  string  `json:"predicate"`
	Target     string  `json:"target"`
	Value      string  `json:"value,omitempty"`
	Confidence float64 `json:"confidence"`
}

type Interpretation struct {
	// Intent representa la intención lingüística principal.
	Intent comprehension.IntentType `json:"intent"`

	// Entities son elementos mencionados explícitamente por el usuario.
	// Ejemplo: "abre Blender" -> ["Blender"]
	Entities  []string   `json:"entities"`
	Relations []Relation `json:"relations,omitempty"`

	// References son referencias cuyo significado depende del contexto.
	// Ejemplo: "continúa con lo de ayer" -> ["lo de ayer"]
	References []string `json:"references"`

	// Constraints contiene condiciones o restricciones expresadas
	// lingüísticamente.
	// Ejemplo: "hazlo mañana si tengo tiempo".
	Constraints []string `json:"constraints"`

	// Meaning es una descripción breve y neutral del significado literal
	// de la petición. No debe contener decisiones ni acciones inventadas.
	Meaning string `json:"meaning"`

	// Confidence expresa la confianza lingüística declarada por el modelo.
	// Ahmelia no debe usar este valor como verdad absoluta.
	Confidence float64 `json:"confidence"`

	// Resolutions son las referencias contextuales que el modelo propone
	// resolver usando SOLO hechos del contexto autorizado. Cada una cita IDs;
	// el arnés descarta las que no pueda verificar.
	Resolutions []ReferenceResolution `json:"resolutions,omitempty"`

	// Rich linguistic signals consumed by Ahmelia; they do not decide actions.
	SpeechAct        string   `json:"speech_act,omitempty"`
	Topic            string   `json:"topic,omitempty"`
	QuestionType     string   `json:"question_type,omitempty"`
	Directness       string   `json:"directness,omitempty"`
	UserStateSignals []string `json:"user_state_signals,omitempty"`
	CorrectionTarget string   `json:"correction_target,omitempty"`
}

// Interpreter transforma lenguaje natural en una representación
// lingüística estructurada.
// LanguageClient is the narrow contract required by the linguistic adapter.
// Implementations may be local LLMs, test doubles, or future linguistic
// services. The Interpreter knows nothing about capabilities, authority,
// memory, goals, or execution.
type LanguageClient interface {
	Generate(prompt string) (string, error)
	Available() bool
}

// ContextLanguageClient extends LanguageClient with a cancellable request
// boundary. The optional interface preserves backward compatibility with
// existing test doubles while allowing the runtime to impose a short live
// interaction budget and a longer deferred budget on the same Interpreter.
type ContextLanguageClient interface {
	LanguageClient
	GenerateContext(ctx context.Context, prompt string) (string, error)
}

type Interpreter struct {
	client    LanguageClient
	validator *Validator
}

// NewInterpreter crea un intérprete lingüístico usando el cliente
// proporcionado.
//
// La dependencia se recibe desde fuera para que Interpreter no dependa
// directamente de cómo se crea o configura Ollama.
func NewInterpreter(client LanguageClient) *Interpreter {
	return &Interpreter{
		client:    client,
		validator: NewValidator(),
	}
}

// Available reports whether the optional local linguistic model is reachable.
func (i *Interpreter) Available() bool {
	return i != nil && i.client != nil && i.client.Available()
}

// Interpret analiza lingüísticamente una entrada.
//
// Este método:
//  1. Construye una instrucción estricta para el LLM.
//  2. Envía la entrada al modelo.
//  3. Espera exclusivamente JSON.
//  4. Convierte el JSON en Interpretation.
//  5. Valida y normaliza el resultado.
//
// No toma decisiones cognitivas.
// No ejecuta acciones.
// No consulta memoria.
// No resuelve referencias contextuales.
// No planifica.
//
// Todo eso sigue siendo responsabilidad de Ahmelia.
func (i *Interpreter) Interpret(input string) (Interpretation, error) {
	return i.InterpretContext(context.Background(), input)
}

// InterpretContext runs the exact same linguistic interpreter behind an
// explicit context boundary. This is the runtime seam used by synchronous
// conversational interpretation and the deferred worker; the interpreter
// itself remains a single cognitive-language capability with no decision or
// execution authority.
func (i *Interpreter) InterpretContext(ctx context.Context, input string) (Interpretation, error) {
	return i.InterpretContextWithSituation(ctx, input, nil)
}

// InterpretContextWithSituation is the same linguistic interpreter with a
// bounded, harness-owned projection of the situation. The snapshot only widens
// what the model may READ; it never grants decision, authority or execution.
// When snapshot is nil the behaviour is identical to InterpretContext.
func (i *Interpreter) InterpretContextWithSituation(ctx context.Context, input string, snapshot *SituationSnapshot) (Interpretation, error) {
	input = strings.TrimSpace(input)

	if input == "" {
		return Interpretation{
			Intent:      comprehension.IntentUnknown,
			Entities:    []string{},
			References:  []string{},
			Constraints: []string{},
			Meaning:     "",
			Confidence:  0,
		}, nil
	}

	if i == nil || i.client == nil {
		return Interpretation{}, fmt.Errorf("interpreter: cliente lingüístico no inicializado")
	}

	if ctx == nil {
		ctx = context.Background()
	}
	prompt := buildInterpretationPrompt(input, snapshot)

	var response string
	var err error
	if client, ok := i.client.(ContextLanguageClient); ok {
		response, err = client.GenerateContext(ctx, prompt)
	} else {
		// Legacy clients cannot observe cancellation. Keep the same interpreter
		// contract rather than silently spawning another linguistic authority.
		response, err = i.client.Generate(prompt)
	}
	if err != nil {
		return Interpretation{}, fmt.Errorf("interpreter: error consultando modelo lingüístico: %w", err)
	}

	interpretation, err := parseInterpretation(response)
	if err != nil {
		return Interpretation{}, fmt.Errorf("interpreter: respuesta lingüística inválida: %w", err)
	}

	normalizeInterpretation(&interpretation)
	interpretation.Resolutions = SanitizeResolutions(interpretation.Resolutions, snapshot)

	validation := i.validator.Validate(input, interpretation)

	return validation.Interpretation, nil
}

// buildInterpretationPrompt construye la instrucción enviada al LLM.
//
// La instrucción es deliberadamente restrictiva: el modelo es solamente
// un analizador lingüístico y no debe decidir, aconsejar ni ejecutar.
func buildInterpretationPrompt(input string, snapshot *SituationSnapshot) string {
	contextBlock := ""
	if !snapshot.Empty() {
		contextBlock = fmt.Sprintf(`
CONTEXTO AUTORIZADO (proyección verificada del estado; incluye turnos previos y evidencia del sistema):
%s

Reglas del contexto autorizado:
- Puedes usar estos hechos para entender el turno y resolver referencias.
- Cada resolución debe citar en "evidence_ids" los IDs exactos que usó.
- Un ID que no aparezca arriba invalida la resolución y será descartado.
- Si ningún hecho sostiene la resolución, deja la referencia sin resolver.
- No cambies de dueño un dato por aparecer en este contexto.
`, snapshot.Render())
	}

	return fmt.Sprintf(`Eres exclusivamente el módulo lingüístico interno
de un sistema llamado Ahmelia.

Tu función es analizar lenguaje natural.

NO eres Ahmelia.
NO decides por Ahmelia.
NO razonas sobre qué acción es mejor.
NO ejecutas acciones.
NO inventas contexto.
NO resuelves referencias con información que no aparezca en el texto ni en el CONTEXTO AUTORIZADO.
NO respondas al usuario.
NO expliques tu análisis.

Analiza la entrada del usuario y devuelve EXCLUSIVAMENTE un objeto JSON válido.
No uses Markdown.
No uses bloques de código.
No escribas texto antes ni después del JSON.

Los únicos valores permitidos para "intent" son:
- "command"
- "question"
- "learning"
- "feedback"
- "correction"
- "unknown"

Significado de los campos:

Clasificación de "intent":

"command":
Usa "command" cuando el usuario expresa una instrucción,
petición operativa, orden o solicitud de que se realice algo.

Ejemplos:
- "abre Blender"
- "continúa con lo que hacíamos ayer"
- "abre el proyecto de Unity"
- "haz lo mismo que antes"
- "mueve el archivo"
- "revisa esto"

Una referencia contextual NO convierte una instrucción en "learning".
Si la estructura principal de la entrada es una instrucción para realizar
algo, el intent debe ser "command", aunque la instrucción contenga
referencias al pasado, aprendizaje o contexto previo.

"question":
Usa "question" cuando el usuario solicita información mediante una
pregunta.

Ejemplos:
- "¿qué hora es?"
- "¿dónde está Blender?"
- "¿cómo funciona esto?"

"learning":
Usa "learning" cuando el usuario expresa explícitamente una intención
de aprender, estudiar, practicar, comprender o adquirir conocimiento.

Ejemplos:
- "enséñame Go"
- "quiero aprender Python"
- "explícame cómo funciona una red"
- "quiero practicar inglés"

No uses "learning" solamente porque la entrada mencione trabajo previo,
experiencia anterior, un proyecto o una actividad que pueda implicar
aprendizaje.

"feedback":
Usa "feedback" cuando el usuario evalúa, comenta o proporciona una
valoración sobre un comportamiento, resultado o respuesta.

"correction":
Usa "correction" cuando el usuario corrige explícitamente información,
una interpretación o un resultado anterior.

"unknown":
Usa "unknown" cuando no sea posible determinar razonablemente la
categoría lingüística.

"entities":
Lista de entidades mencionadas explícitamente en el texto.

Una entidad debe representar un objeto, programa, archivo, lugar,
persona, proyecto u otro elemento que sea relevante como elemento
mencionado explícitamente por el usuario.

No inventes entidades.

No extraigas como entidad una palabra solamente porque sea un sustantivo.

Si un elemento aparece únicamente dentro de una restricción, prohibición
o condición, no lo conviertas automáticamente en entidad.

Ejemplo:

Entrada:
"abre el proyecto de Unity pero no ejecutes el juego"

Correcto:
"entities": ["proyecto de Unity"]
"constraints": ["no ejecutes el juego"]

Incorrecto:
"entities": ["proyecto de Unity", "juego"]
"constraints": ["no ejecutes el juego"]

"references":
Lista de expresiones referenciales cuyo significado completo NO puede
determinarse únicamente a partir de la entrada actual y necesita contexto
previo, memoria o información externa para ser resuelto.

Incluye únicamente referencias contextuales reales, por ejemplo:
- "eso"
- "esto"
- "aquello"
- "él"
- "ella"
- "ellos"
- "el anterior"
- "lo de ayer"
- "lo que hacíamos antes"
- "haz lo mismo"
- "allí"
- "aquí", cuando el lugar no está especificado en la entrada

NO incluyas como referencias:
- preguntas completas
- objetivos de una pregunta
- verbos o acciones explícitas
- entidades mencionadas explícitamente
- información que pueda entenderse completamente solo con la entrada actual

Ejemplo:

Entrada:
"¿qué hora es?"

Correcto:
"references": []

Incorrecto:
"references": ["qué hora es"]

Ejemplo:

Entrada:
"continúa con lo que hacíamos ayer en Blender"

Correcto:
"references": ["lo que hacíamos ayer"]

Puedes proponer una resolución SOLO con hechos del CONTEXTO AUTORIZADO.
Cada resolución debe citar los IDs usados en "evidence_ids".
Si ningún hecho lo sostiene, deja la referencia en "references" sin resolver.
No inventes información del contexto.

"resolutions":
Lista de resoluciones propuestas para las referencias contextuales.
Cada elemento tiene la forma:
{"reference":"la expresión del usuario","resolved":"a qué se refiere, en una frase","evidence_ids":["f1"],"confidence":0.0}
Omite este campo si no hay contexto autorizado o si no puedes resolver nada.
No inventes IDs.

"constraints":
Lista de condiciones, límites, prohibiciones o requisitos que restringen
cómo, cuándo, dónde o bajo qué condiciones debe realizarse una acción.

Incluye únicamente restricciones explícitas.

Ejemplos:
- "no ejecutes el juego"
- "hazlo rápido"
- "solo modifica el archivo"
- "sin conexión a internet"
- "antes de las 5"
- "no borres nada"

NO incluyas como restricciones:
- la acción principal solicitada
- el objetivo principal del usuario
- referencias contextuales
- entidades
- repeticiones del significado principal

Ejemplo:

Entrada:
"continúa con lo que hacíamos ayer en Blender"

Correcto:
"constraints": []

Porque "lo que hacíamos ayer" es una referencia contextual, no una
condición que limite la acción.

Ejemplo:

Entrada:
"continúa con lo que hacíamos ayer en Blender, pero no cambies los materiales"

Correcto:
"references": ["lo que hacíamos ayer"]
"constraints": ["no cambies los materiales"]
"meaning":
OBLIGATORIO.

Debes escribir siempre una descripción breve, neutral y literal
de lo que expresa el usuario.

Nunca dejes "meaning" vacío si la entrada contiene una petición
comprensible.

Ejemplos:

Entrada:
"abre Blender"

"meaning": "Instrucción para abrir Blender"

Entrada:
"¿qué hora es?"

"meaning": "Pregunta sobre la hora actual"

Entrada:
"continúa con lo que hacíamos ayer"

"meaning": "Continuar con el trabajo realizado anteriormente"

Entrada:
"abre el proyecto de Unity pero no ejecutes el juego"

"meaning": "Instrucción para abrir el proyecto de Unity sin ejecutar el juego"

No inventes información que no aparezca en la entrada.
No incluyas decisiones sobre cómo debe actuar Ahmelia.

Campos adicionales de comunicación:

"speech_act": acto comunicativo principal. Usa uno de:
"question", "command", "inform", "propose", "correct", "confirm",
"reject", "continue", "stop", "pause", "social", "feedback", "unknown".

"topic": tema explícito de la entrada si puede identificarse sin contexto;
si no, "".

"question_type": "quantity", "person", "location", "time",
"explanation", "procedure_or_explanation", "general" o "".

"directness": "direct_command", "polite_request", "question" o "statement".

"user_state_signals": lista de señales observables SOLO en el texto,
por ejemplo "frustration", "fatigue", "positive", "urgency".
No diagnostiques estados internos ni inventes emociones.

"correction_target": qué parte anterior está corrigiendo el usuario,
solo si aparece explícitamente; si no, "".

"relations": relaciones semánticas explícitas que estén expresadas en la
entrada. No son decisiones ni instrucciones para Ahmelia. Sirven para
representar vínculos entre actores, objetos, estados, objetivos o eventos.
Usa una o varias relaciones cuando la oración las exprese.
Predicados permitidos: "addresses", "asks_about", "asks_about_state",
"asks_about_opinion", "proposes", "anticipates", "gives_feedback_to",
"evaluates", "refers_to", "affects", "depends_on", "supports",
"commits_to", "enables", "declares_attribute", "declares_preference",
"asks_about_attribute", "asks_about_preference".
Las relaciones de atributo deben representar la estructura semántica, no una
lista cerrada de atributos. Por ejemplo, "mi nombre es Adrian" puede ser
user -> declares_attribute -> nombre con el valor "Adrian" en "value";
"mi color favorito es negro" puede ser user -> declares_preference ->
"color favorito" con "value": "negro"; y "cual es mi profesion?" puede
ser user -> asks_about_attribute -> "profesion". El mismo mecanismo debe
servir para atributos todavía no vistos.
Reconoce también paráfrasis, preguntas indirectas y preguntas embebidas: la
misma relación semántica puede expresarse con verbos o estructuras distintas
y no debe depender de una plantilla de frase concreta. Cuando una entrada
contenga una pregunta dentro de otra construcción, representa como Target el
objeto que realmente se pregunta y como Source el propietario de ese dato si
está expresado. Un pronombre usado para dirigirse al asistente no debe cambiar
el propietario del dato consultado.
No inventes relaciones que requieran contexto ausente.

"confidence":
OBLIGATORIO.

Debes proporcionar siempre un número entre 0.0 y 1.0.

Representa únicamente tu confianza en el análisis lingüístico realizado.

Si la entrada es clara y su estructura lingüística es evidente,
usa un valor alto, normalmente entre 0.9 y 1.0.

Si existe ambigüedad lingüística significativa, utiliza un valor menor.

Nunca omitas este campo.
Nunca lo dejes en 0.0 si la interpretación es claramente comprensible.

%s
Formato obligatorio:

{
  "intent": "unknown",
  "speech_act": "unknown",
  "entities": [],
  "references": [],
  "resolutions": [],
  "constraints": [],
  "meaning": "",
  "topic": "",
  "question_type": "",
  "directness": "",
  "user_state_signals": [],
  "correction_target": "",
  "relations": [],
  "confidence": 0.0
}

Entrada del usuario:
%q`, contextBlock, input)
}

// parseInterpretation convierte la respuesta del LLM en una estructura Go.
func parseInterpretation(response string) (Interpretation, error) {
	response = strings.TrimSpace(response)

	if response == "" {
		return Interpretation{}, fmt.Errorf("respuesta vacía")
	}

	// En condiciones normales el prompt obliga a recibir JSON puro.
	// Esta limpieza adicional permite tolerar accidentalmente respuestas
	// encerradas en bloques Markdown.
	response = strings.TrimPrefix(response, "```json")
	response = strings.TrimPrefix(response, "```")
	response = strings.TrimSuffix(response, "```")
	response = strings.TrimSpace(response)

	var interpretation Interpretation

	if err := json.Unmarshal([]byte(response), &interpretation); err != nil {
		return Interpretation{}, fmt.Errorf("JSON inválido: %w", err)
	}

	return interpretation, nil
}

// normalizeInterpretation valida y normaliza los datos recibidos del LLM.
//
// Nunca confiamos ciegamente en que un modelo externo respetará el prompt.
func normalizeInterpretation(i *Interpretation) {
	if i == nil {
		return
	}
	if i.UserStateSignals == nil {
		i.UserStateSignals = []string{}
	}
	if strings.TrimSpace(i.SpeechAct) == "" {
		i.SpeechAct = "unknown"
	}
	i.Intent = normalizeIntent(i.Intent)

	if i.Entities == nil {
		i.Entities = []string{}
	}

	if i.References == nil {
		i.References = []string{}
	}

	if i.Constraints == nil {
		i.Constraints = []string{}
	}

	i.Meaning = strings.TrimSpace(i.Meaning)

	if i.Confidence < 0 {
		i.Confidence = 0
	}

	if i.Confidence > 1 {
		i.Confidence = 1
	}
}

// normalizeIntent garantiza que solo entren al sistema cognitivo los tipos
// de intención que Ahmelia ya reconoce oficialmente.
func normalizeIntent(intent comprehension.IntentType) comprehension.IntentType {
	switch intent {
	case comprehension.IntentCommand,
		comprehension.IntentQuestion,
		comprehension.IntentLearning,
		comprehension.IntentFeedback,
		comprehension.IntentCorrection,
		comprehension.IntentUnknown:
		return intent

	default:
		return comprehension.IntentUnknown
	}
}
