//go:build ahmelia_live

package linguistics

// Batería v1 — eje F4 de la directiva del propietario: NEGACIÓN, ALCANCE Y
// RELACIONES LÓGICAS (la familia que invierte decisiones). Los 8 casos son los
// de su tabla exacta. La familia exige representación explícita (B2); hoy la
// única representación viva es el meaning libre, así que v0 evalúa la CAPTURA
// textual, no una garantía mecanística.
//
// Criterios aceptables / errores críticos (idénticos a la tabla del
// propietario, definidos antes de correr — ver tablero INICIO 18:26):
//
//	1 "No abras el archivo."              prohibición reconocida | CRÍTICO: meaning radica como pedido de abrir
//	2 "No todos los archivos..."          parcial, no general | CRÍTICO: ninguno/todos
//	3 "No es imposible resolverlo."       doble negación preservada | CRÍTICO: leerlo "imposible"
//	4 "Si termina la descarga..."         condición + acción condicionada | CRÍTICO: acción inmediata
//	5 "Todos pueden entrar excepto..."    excepción conservada dos vías | CRÍTICO: excepción abandonada
//	6 "Nunca he estado allí; fui ayer."   contradicción marcada | CRÍTICO: suavizar en un único hecho
//	7 "Se apagó porque..."                causa afirmada-como-afirmada | CRÍTICO: convertida en verdad endosada
//	8 "Es posible que el archivo esté corrupto." hipótesis≠hecho | CRÍTICO: "está corrupto"
//
// Ejecutar manualmente:
//	AHMELIA_LIVE_LLM=1 go test -tags ahmelia_live -run TestOpenMeaningNegationLive ./linguistics -count=1 -v

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestOpenMeaningNegationLive(t *testing.T) {
	if os.Getenv("AHMELIA_LIVE_LLM") != "1" {
		t.Skip("prueba solo con AHMELIA_LIVE_LLM=1")
	}
	client := NewOllamaClient()
	if !client.Available() {
		t.Skip("ollama no disponible")
	}
	interp := NewInterpreter(client)
	ctx := context.Background()

	cases := []struct {
		id     string
		frase  string
		evalua string
	}{
		{"1 prohibición", "No abras el archivo.", "se prohíbe la acción"},
		{"2 parcial", "No todos los archivos están dañados.", "no confundir «no todos» con «ninguno»"},
		{"3 doble negación", "No es imposible resolverlo.", "interpretar la estructura sin simplificarla incorrectamente"},
		{"4 condición", "Si termina la descarga, abre el documento.", "acción condicionada ≠ acción inmediata"},
		{"5 excepción", "Todos pueden entrar excepto los visitantes.", "aplicar la excepción correctamente"},
		{"6 contradicción", "Nunca he estado allí; fui ayer.", "detectar afirmaciones incompatibles (mismo lugar y período)"},
		{"7 causalidad", "Se apagó porque se quedó sin batería.", "causa afirmada ≠ coincidencia"},
		{"8 hipótesis", "Es posible que el archivo esté corrupto.", "no transformar hipótesis en hecho"},
	}

	for _, c := range cases {
		start := time.Now()
		om, err := interp.InterpretOpenContext(ctx, c.frase)
		d := time.Since(start).Round(time.Millisecond)
		if err != nil {
			t.Errorf("CASO %s | %q → ERROR: %v", c.id, c.frase, err)
			continue
		}
		t.Logf("CASO %s | %s", c.id, c.frase)
		t.Logf("\t evalúa: %s", c.evalua)
		t.Logf("\t (%s flap=%v) purpose=%s owner=%s topic=%q conf=%.2f mentions=%v",
			d, om.ProbeFlapRetry, om.Purpose, om.OwnerOfData, om.Topic, om.Confidence, om.Mentions)
		t.Logf("\t marks=%+v", om.Marks)
		t.Logf("\t meaning=%q", om.Meaning)
	}
}
