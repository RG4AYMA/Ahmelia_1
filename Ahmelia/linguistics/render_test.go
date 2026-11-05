package linguistics

// render_test.go — pruebas de la superficie determinista. Cada escena tiene
// UNA salida correcta; las familias de defectos del propietario se assertan
// negativamente (inversión de sujeto, caracteres arrastrados, inventos).

import (
	"strings"
	"testing"
)

func TestRender_EscenasCanonicas(t *testing.T) {
	casos := []struct {
		kw       RenderSpec
		esperada string
	}{
		{RenderSpec{Move: MoveSaludo, Sujeto: "ahmelia"}, "Hola. Estoy aquí para ayudarte."},
		{RenderSpec{Move: MoveDatoFaltante, Sujeto: "user"}, "Todavía no tengo ese dato guardado. ¿Me lo dices?"},
		{RenderSpec{Move: MoveResponderValor, Sujeto: "user", Atributo: "color favorito", Valor: "negro"}, "Tu color favorito es negro."},
		{RenderSpec{Move: MoveResponderValor, Sujeto: "ahmelia", Atributo: "bebida favorita", Valor: "gaseosa de piña"}, "Mi bebida favorita es gaseosa de piña."},
		{RenderSpec{Move: MovePropioFaltante, Sujeto: "ahmelia", Atributo: "color favorito"}, "No tengo color favorito todavía. Dime qué elegirías tú y lo guardo."},
		{RenderSpec{Move: MoveRegistroOkey}, "Entendido. Lo he registrado."},
		{RenderSpec{Move: MoveFeedbackRecibido}, "Entendido. Gracias por decírmelo; lo tengo en cuenta."},
		{RenderSpec{Move: MoveNoEntendido}, "No entendí del todo. ¿Puedes repetírmelo de otra forma?"},
	}
	for _, c := range casos {
		out, ok := Render(c.kw)
		if !ok {
			t.Fatalf("escena %q con spec %+v debería poder hablar", c.kw.Move, c.kw)
		}
		if out != c.esperada {
			t.Fatalf("escena %q: got %q, want %q", c.kw.Move, out, c.esperada)
		}
	}
}

func TestRender_SujetoNoInvertible(t *testing.T) {
	// Fallo observado en vivo del propietario: preguntó por el color de
	// Ahmelia y salió "Sí, tu color favorito es el negro" (sujeto invertido).
	// Con el contrato viajando el sujeto, la inversión es imposible.
	outUser, _ := Render(RenderSpec{Move: MoveResponderValor, Sujeto: "user", Atributo: "color favorito", Valor: "negro"})
	outAhmelia, _ := Render(RenderSpec{Move: MoveResponderValor, Sujeto: "ahmelia", Atributo: "color favorito", Valor: "negro"})

	if !strings.HasPrefix(outUser, "Tu ") || strings.Contains(outUser, "Mi ") {
		t.Fatalf("sujeto user invertido: %q", outUser)
	}
	if !strings.HasPrefix(outAhmelia, "Mi ") || strings.Contains(outAhmelia, "Tu ") {
		t.Fatalf("sujeto ahmelia invertido: %q", outAhmelia)
	}
}

func TestRender_HonestoSinContenido(t *testing.T) {
	// El contrato no trae lo que la escena exige → nadie habla; jamás un invento.
	falsos := []RenderSpec{
		{Move: MoveResponderValor, Sujeto: "user", Atributo: "color favorito", Valor: ""},
		{Move: MoveResponderValor, Sujeto: "user", Atributo: "", Valor: "negro"},
		{Move: MoveResponderValor, Sujeto: "tercero", Atributo: "color favorito", Valor: "negro"}, // v1 sin terceros
		{Move: MoveDatoFaltante, Sujeto: "ahmelia"},                                               // el dato faltante es del user
		{Move: MoveSaludo, Sujeto: "user"},
	}
	for _, spec := range falsos {
		if out, ok := Render(spec); ok {
			t.Fatalf("spec inválida habló: %q (%+v)", out, spec)
		}
	}
}

func TestRender_TextoSanoAunSlotsSucios(t *testing.T) {
	// Defecto observado en vivo: '}' dangling del modelo dentro del habla.
	out, ok := Render(RenderSpec{Move: MoveResponderValor, Sujeto: "user", Atributo: "color favorito", Valor: "negro }"})
	if !ok {
		t.Fatal("slot sucio no debe impedir el render")
	}
	if strings.ContainsAny(out, "{}|") {
		t.Fatalf("texto hablado con caracteres sueltos: %q", out)
	}
	if !strings.Contains(out, "negro") {
		t.Fatalf("sanidad corrió el valor: %q", out)
	}
	if strings.Contains(out, "  ") {
		t.Fatalf("espacios dobles tras sanear: %q", out)
	}
}

func TestRender_SinEcoDeTema(t *testing.T) {
	// Escena observada: feedback recibido y salió "¿te gustaría VOLVER AL
	// TEMA de tu color favorito?" — el movimiento de feedback no menciona
	// ningún tema aunque el contrato lo tenga cargado.
	out, ok := Render(RenderSpec{Move: MoveFeedbackRecibido, Sujeto: "user", Atributo: "color favorito", Valor: "negro"})
	if !ok {
		t.Fatal("feedback debe hablar")
	}
	if strings.Contains(out, "color") || strings.Contains(out, "negro") {
		t.Fatalf("el renderizador resucitó el tema en feedback: %q", out)
	}
}

func TestMoveEnFase2(t *testing.T) {
	if !MoveEnFase2(MoveResponderValor) || !MoveEnFase2(MoveDatoFaltante) {
		t.Fatal("escenas de dato personal deben ser fase 2")
	}
	if MoveEnFase2("iniciativa_propuesta") {
		t.Fatal("escena no definida no es fase 2")
	}
}
