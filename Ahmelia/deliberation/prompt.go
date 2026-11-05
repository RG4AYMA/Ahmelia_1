package deliberation

// prompt.go — PROMPT ACOTADO DE DELIBERACIÓN.
//
// El modelo recibe un espacio de trabajo con IDs y una decisión ya tomada por
// el arnés. Su tarea es aportar juicio, no ejecutar ni decidir. El prompt
// prohíbe explícitamente inventar elementos, ampliar autoridad o responder al
// usuario. La salida es un contrato JSON cerrado.

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Schema is the structured-output contract. The enum keeps the strategy in the
// same closed vocabulary the harness already uses.
func Schema() json.RawMessage {
	return json.RawMessage(`{
		"type":"object",
		"properties":{
			"reading":{"type":"string"},
			"strategy":{"type":"string","enum":["answer","clarify","discuss","propose","correct","disagree","confirm","request_execution","wait","continue","change_topic","unknown"]},
			"critique":{"type":"string"},
			"expectation":{"type":"string"},
			"evidence_ids":{"type":"array","items":{"type":"string"}},
			"confidence":{"type":"number","minimum":0,"maximum":1},
			"abstain":{"type":"boolean"},
			"mode_suggestion":{"type":"string","enum":["focus","explore","rest","companion"]},
			"self_goal":{
				"type":"object",
				"properties":{
					"title":{"type":"string"},
					"motive":{"type":"string"},
					"next_step":{"type":"string"},
					"expected_minutes":{"type":"integer","minimum":1,"maximum":240},
					"risk":{"type":"string","enum":["low","medium","high"]},
					"mode":{"type":"string","enum":["focus","explore","rest","companion"]},
					"evidence_ids":{"type":"array","items":{"type":"string"}},
					"confidence":{"type":"number","minimum":0,"maximum":1}
				},
				"required":["title","motive","next_step","expected_minutes","risk","mode","evidence_ids","confidence"]
			}
		},
		"required":["reading","strategy","critique","expectation","evidence_ids","confidence","abstain"]
	}`)
}

// BuildPrompt renders the bounded deliberation request. The harness decision
// is shown as already-made context, never as a question to re-decide.
func BuildPrompt(ws *Workspace, harness HarnessDecision) string {
	workspace := "(vacío)"
	if !ws.Empty() {
		workspace = ws.Render()
	}

	var h strings.Builder
	h.WriteString("action: ")
	h.WriteString(orUnknown(harness.Action))
	h.WriteString("\nstrategy: ")
	h.WriteString(orUnknown(string(harness.Strategy)))
	h.WriteString("\nintent: ")
	h.WriteString(orUnknown(harness.Intent))
	h.WriteString(fmt.Sprintf("\nspeak_to_current_turn: %t\nuncertainty: %t", harness.Speak, harness.Uncertainty))
	if harness.Reason != "" {
		h.WriteString("\nreason: ")
		h.WriteString(harness.Reason)
	}
	if len(harness.Evidence) > 0 {
		h.WriteString("\nevidence: ")
		h.WriteString(strings.Join(harness.Evidence, " | "))
	}

	return `Eres el módulo deliberativo interno de Ahmelia.

No eres Ahmelia. No decides por ella. No ejecutas acciones. No autorizas nada.
No respondes al usuario. No inventas hechos, recuerdos ni contexto.

Tu única tarea es aportar un juicio acotado sobre el turno actual para que el
arnés pueda compararlo con la decisión que YA tomó. Esa decisión no se cambia
en este paso: tu propuesta se registra en modo sombra.

ESPACIO DE TRABAJO (hechos que el arnés posee; puedes leerlos, no ampliarlos):
` + workspace + `

DECISIÓN YA TOMADA POR EL ARNÉS (autoridad; no la reemplaces):
` + h.String() + `

Devuelve SOLO un objeto JSON con estos campos:

- "reading": una frase sobre qué está pasando en el turno, usando el espacio.
- "strategy": la postura que tú tomarías. Vocabulario cerrado: answer, clarify,
  discuss, propose, correct, disagree, confirm, request_execution, wait,
  continue, change_topic, unknown.
- "critique": qué podría estar pasando por alto la decisión del arnés. Una frase.
  Si no ves nada, "".
- "expectation": qué debería ser observable después del turno si tu lectura es
  correcta. Una frase verificable. Si no aplica, "".
- "evidence_ids": IDs exactos del espacio de trabajo que sostienen tu lectura.
  Si no hay ninguno, deja [] y marca "abstain": true.
- "confidence": número entre 0 y 1.
- "abstain": true si no puedes juzgar con lo que hay. Abstenerse es correcto;
  inventar no lo es.
- "mode_suggestion": opcional. Si crees que otro modo persistente serviría
  mejor, nómbralo aquí (focus, explore, rest o companion). Es una SUGERENCIA
  al propietario; no cambia el modo.
- "self_goal": opcional. Una meta propia SOLO si el espacio de trabajo contiene
  un elemento de tipo "impulso". Si no hay impulso, omite este campo.
  - "title": qué meta propia propones, en una frase.
  - "motive": por qué la propones, citando el impulso o un hecho del espacio.
  - "next_step": el primer paso concreto, sin ejecutarlo.
  - "expected_minutes": costo estimado, entero entre 1 y 240.
  - "risk": low, medium o high.
  - "mode": focus, explore, rest o companion.
  - "evidence_ids": IDs exactos del espacio que sostienen la meta (incluido el
    impulso). Sin cita válida, la meta se descarta.
  - "confidence": número entre 0 y 1.

Reglas:
- Cita solo IDs que aparezcan literalmente arriba (w1, w2, ...).
- Un ID inventado invalida tu propuesta y será descartado.
- No propongas ejecutar herramientas ni ampliar permisos.
- No conviertas una sugerencia en una orden.
- No uses conocimiento externo sobre el usuario.
- Una meta propia es una PROPUESTA para el propietario: no la ejecutes, no la
  des por aceptada y no la presentes como una orden.
- Un elemento de tipo "gusto_propio" es contexto derivado de experiencias que
  el propietario confirmó. No es una orden, no obliga a elegir esa actividad y
  no te autoriza a inventar otros gustos.
- El elemento "modo_persistente" es una decisión del propietario. Puedes
  sugerir otro modo en "mode_suggestion", pero no cambiarlo ni actuar como si
  ya hubiera cambiado.
- Responde en español.
`
}

func orUnknown(s string) string {
	if strings.TrimSpace(s) == "" {
		return "(desconocido)"
	}
	return strings.TrimSpace(s)
}
