package linguistics

// render.go — Superficie determinista para las escenas de Fase 2.
//
// Directiva del propietario (2026-10-18): en informática solo hay dos estados,
// se cumple o no se cumple. La cadena de decisión ya es verificable (trazas
// SEMANTIC / USER MODEL / RESPONSE GATE de sus logs); el tramo que fallaba a
// sus ojos es el FINAL: el modelo pequeño redactando la frase hablada turno a
// turno invertía sujetos ("Sí, tu color favorito es el negro" cuando él
// preguntaba por el de Ahmelia), arrastraba "}" al habla, repetía contenido
// y resucitaba temas viejos. Esta pieza elimina esas familias POR
// CONSTRUCCIÓN:
//
//   1. El sujeto viaja en el contrato y el renderizador solo emite el
//      pronombre que ese sujeto autoriza (no puede invertir tu/mi).
//   2. Solo habla de atributos y valores que el contrato le entrega: jamás
//      resucita temas por sí misma.
//   3. Texto saneado mecánicamente (sin llaves ni caracteres sueltos).
//   4. Si el contrato no trae lo que la escena exige, devuelve ok=false y
//      nadie habla — honesto, nada disfrazado.
//
// El LLM queda en la INTERPRETACIÓN de la entrada (comprehension=open), nunca
// en la redacción final de estas escenas: interpretación abierta SÍ, salida
// abierta NO. Alcance v1 honesto: sujeto user|ahmelia; terceros quedan para la
// fase de compañera (informe del propietario, sección 3.3).

import "strings"

// RenderMove — escena de salida con enunciado canónico.
type RenderMove string

const (
	MoveSaludo           RenderMove = "saludo"
	MoveDatoFaltante     RenderMove = "dato_faltante"
	MoveResponderValor   RenderMove = "responder_valor"
	MovePropioFaltante   RenderMove = "propio_faltante"
	MoveRegistroOkey     RenderMove = "registro_ok"
	MoveFeedbackRecibido RenderMove = "feedback_recibido"
	MoveNoEntendido      RenderMove = "no_entendido"
)

// RenderSpec — lo que el contrato de conducta entrega al renderizador.
type RenderSpec struct {
	Move     RenderMove
	Sujeto   string // "user"|"ahmelia": de quién es el contenido del turno
	Atributo string // "color favorito", "bebida favorita" (solo escenas de atributo)
	Valor    string // "negro", "gaseosa de piña" (solo responder_valor)
}

// renderSanitize — limpieza mecánica del slot: retira caracteres que el
// modelo haya arrastrado y colapsa espacios. No interpreta nada.
func renderSanitize(s string) string {
	cleaned := strings.Map(func(r rune) rune {
		switch r {
		case '{', '}', '|', '`', 0:
			return -1 // eliminado
		}
		return r
	}, s)
	return strings.Join(strings.Fields(cleaned), " ")
}

func renderHabilita(s string) bool {
	return strings.TrimSpace(s) != ""
}

// Render compone el enunciado final. ok=false significa "el contrato no trae
// lo que la escena exige": el turno queda sin habla en vez de inventar.
// Toda salida pasa por el saneador mecánico: la superficie JAMÁS puede
// arrastrar caracteres extraños, aunque un slot venga sucio del dato o del
// modelo.
func Render(spec RenderSpec) (string, bool) {
	switch spec.Move {
	case MoveSaludo:
		if spec.Sujeto != "ahmelia" {
			return "", false
		}
		return renderSanitize("Hola. Estoy aquí para ayudarte."), true

	case MoveDatoFaltante:
		// Cadena canónica ya verificada EN VIVO por el propietario (re-demo,
		// turn 1); frase fija: cero piezas que puedan romperla.
		if spec.Sujeto != "user" {
			return "", false
		}
		return renderSanitize("Todavía no tengo ese dato guardado. ¿Me lo dices?"), true

	case MoveResponderValor:
		if spec.Sujeto != "user" && spec.Sujeto != "ahmelia" {
			return "", false
		}
		if !renderHabilita(spec.Atributo) || !renderHabilita(spec.Valor) {
			return "", false
		}
		if spec.Sujeto == "user" {
			// Forma canónica ya escuchada correcta por el propietario:
			// "Tu color favorito es negro." Sin artículo: segura en género.
			return renderSanitize("Tu " + spec.Atributo + " es " + spec.Valor + "."), true
		}
		return renderSanitize("Mi " + spec.Atributo + " es " + spec.Valor + "."), true

	case MovePropioFaltante:
		if spec.Sujeto != "ahmelia" || !renderHabilita(spec.Atributo) {
			return "", false
		}
		return renderSanitize("No tengo " + spec.Atributo + " todavía. Dime qué elegirías tú y lo guardo."), true

	case MoveRegistroOkey:
		return renderSanitize("Entendido. Lo he registrado."), true

	case MoveFeedbackRecibido:
		// Fija y SIN arrastrar el tema previo (el eco de tema muere aquí).
		return renderSanitize("Entendido. Gracias por decírmelo; lo tengo en cuenta."), true

	case MoveNoEntendido:
		return renderSanitize("No entendí del todo. ¿Puedes repetírmelo de otra forma?"), true

	default:
		return "", false
	}
}

// MoveEnFase2 — el conjunto de escenas cuya superficie DEBE salir de Render
// (integración en core: el camino renderResponseContract usa Render cuando el
// acto cae aquí). Una sola condición para DeepSeek, sin listas de frases.
func MoveEnFase2(m RenderMove) bool {
	switch m {
	case MoveSaludo, MoveDatoFaltante, MoveResponderValor, MovePropioFaltante,
		MoveRegistroOkey, MoveFeedbackRecibido, MoveNoEntendido:
		return true
	}
	return false
}
