//go:build ahmelia_live

package linguistics

// Batería de FLEXIBILIDAD — grupo de equivalencia semántica del propietario
// (2026-10-17, chat): 5 formas sociales distintas con UNA intención compartida
// ("hacer que pare la música"). Prueba el núcleo de la directiva 16:04: el
// diccionario cerrado clasifica por FORMA y fracasa con variantes; el contrato
// abierto captura el SIGNIFICADO compartido y los mecanismos pueden derivar
// una misma acción desde variantes distintas.
//
// Ejecutar manualmente (jamás interfiere con suites):
//
//	AHMELIA_LIVE_LLM=1 go test -tags ahmelia_live -run TestOpenMeaningVariantsLive ./linguistics -count=1 -v
//
// Cada frase corre por AMBOS contratos con el mismo cliente vivo:
// abierto (InterpretOpenContext) y cerrado (InterpretContext).

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"
)

func TestOpenMeaningVariantsLive(t *testing.T) {
	if os.Getenv("AHMELIA_LIVE_LLM") != "1" {
		t.Skip("prueba solo con AHMELIA_LIVE_LLM=1")
	}
	client := NewOllamaClient()
	if !client.Available() {
		t.Skip("ollama no disponible")
	}

	// Grupo del propietario: 5 formas, intención compartida.
	variants := []struct {
		phrase   string
		facultad string // cómo la nombra el propietario
	}{
		{"Apaga la música.", "orden directa"},
		{"¿Puedes quitar la música?", "petición indirecta"},
		{"Ya no quiero escuchar música.", "preferencia expresada"},
		{"Está sonando demasiado fuerte.", "queja que podría implicar una acción"},
		{"Mejor dejemos de escuchar esto.", "propuesta compartida"},
	}

	interp := NewInterpreter(client)
	ctx := context.Background()

	for _, v := range variants {
		abierto := "---"
		cerrado := "---"
		start := time.Now()
		if om, err := interp.InterpretOpenContext(ctx, v.phrase); err == nil {
			abierto = fmt.Sprintf("purpose=%s owner=%s topic=%q conf=%.2f mentions=%v meaning=%q",
				om.Purpose, om.OwnerOfData, om.Topic, om.Confidence, om.Mentions, om.Meaning)
		} else {
			abierto = fmt.Sprintf("ERROR: %v", err)
		}
		dOpen := time.Since(start)

		start = time.Now()
		ic, err := interp.InterpretContext(ctx, v.phrase)
		if err == nil {
			cerrado = fmt.Sprintf("intent=%s speech_act=%s directness=%s topic=%q conf=%.2f relations=%d meaning=%q",
				ic.Intent, ic.SpeechAct, ic.Directness, ic.Topic, ic.Confidence, len(ic.Relations), ic.Meaning)
		} else {
			cerrado = fmt.Sprintf("ERROR: %v", err)
		}
		dClosed := time.Since(start)

		t.Logf("VARIANTE %-36s | %s", v.facultad, v.phrase)
		t.Logf("   ABIERTO (%4.1fs): %s", dOpen.Seconds(), abierto)
		t.Logf("   CERRADO (%4.1fs): %s", dClosed.Seconds(), cerrado)
	}
}
