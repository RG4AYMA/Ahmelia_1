package core

// fase2_render_integration_test.go — Integración de la superficie determinista
// de Fase 2 (linguistics/render.go) en el camino real de respuesta.
//
// Lo que se fija aquí:
//  1. La escena del contrato se traduce UNA vez al renderizador; el sujeto no
//     puede invertirse y el modelo queda fuera de esa redacción.
//  2. Un renderizador que rechaza (slots incompletos, escena fuera del conjunto
//     o sujeto no mapeable) NO produce superficie.
//  3. El camino vivo del ciclo personal (sync) lleva escena: la traza del turno
//     lo demuestra con renderer=code:fase2.
//  4. El acuse de feedback usa la superficie canónica sin eco de tema.

import (
	"os"
	"strings"
	"testing"

	"ahmelia/cognition"
	"ahmelia/interaction"
	"ahmelia/language"
	"ahmelia/linguistics"
)

func fase2TestContract(scene *interaction.RenderScene) (interaction.SemanticRequest, interaction.ResponseContract) {
	req := interaction.Analyze("¿cuál es mi color favorito?", interaction.Context{})
	intent := interaction.ResponseIntent{Kind: interaction.ResponseIntentAnswer, ShouldSpeak: true, Confidence: .95}
	contract := interaction.NewResponseContract(req, intent, interaction.MoveAnswer)
	contract.Source = "user_model"
	contract.RenderScene = scene
	return req, contract
}

func fase2TraceLines(e *Engine) string {
	if e == nil || e.lastFlowTrace == nil {
		return ""
	}
	return strings.Join(e.lastFlowTrace.Lines(), "\n")
}

func TestFase2SceneRendersValueSurfacesFromContract(t *testing.T) {
	e := newInteractionE2EEngine(t)

	req, contract := fase2TestContract(&interaction.RenderScene{
		Scene:     string(linguistics.MoveResponderValor),
		Subject:   interaction.UserObservationSubjectUser,
		Attribute: "color favorito",
		Value:     "negro",
	})
	text, ok := e.generateResponseContractWithStrategy(req, contract, language.RuntimeFacts{}, cognition.ConversationalResponseStrategy{})
	if !ok || text != "Tu color favorito es negro." {
		t.Fatalf("escena responder_valor user no rindió superficie determinista: ok=%v text=%q", ok, text)
	}

	// Sujeto Ahmelia: la única forma autorizada es "Mi ..." — la inversión es
	// imposible por construcción (el sujeto viaja en el contrato).
	req2, contract2 := fase2TestContract(&interaction.RenderScene{
		Scene:     string(linguistics.MoveResponderValor),
		Subject:   interaction.UserObservationSubjectSelf,
		Attribute: "color favorito",
		Value:     "azul",
	})
	text2, ok2 := e.generateResponseContractWithStrategy(req2, contract2, language.RuntimeFacts{}, cognition.ConversationalResponseStrategy{})
	if !ok2 || text2 != "Mi color favorito es azul." {
		t.Fatalf("escena responder_valor self no rindió superficie determinista: ok=%v text=%q", ok2, text2)
	}
}

func TestFase2SceneRefusalFallsBackToContractEvidence(t *testing.T) {
	e := newInteractionE2EEngine(t)
	req, contract := fase2TestContract(&interaction.RenderScene{
		Scene:   string(linguistics.MoveResponderValor),
		Subject: interaction.UserObservationSubjectUser,
		// Sin atributo ni valor: el renderizador debe rechazar (slots incompletos).
	})
	contract.AddEvidence("Respaldo exacto del contrato.", "user_model", true, .9)
	text, ok := e.generateResponseContractWithStrategy(req, contract, language.RuntimeFacts{}, cognition.ConversationalResponseStrategy{})
	if !ok || text != "Respaldo exacto del contrato." {
		t.Fatalf("rechazo del renderizador no cayó al respaldo del contrato: ok=%v text=%q", ok, text)
	}
}

func TestFase2ApplyRefusesIncompleteOrForeignScene(t *testing.T) {
	e := newInteractionE2EEngine(t)
	req, contract := fase2TestContract(&interaction.RenderScene{
		Scene:   string(linguistics.MoveResponderValor),
		Subject: interaction.UserObservationSubjectUser,
	})
	_ = req
	if _, ok := e.applyFase2Scene(contract); ok {
		t.Fatal("escena sin slots no puede rendir superficie")
	}
	contract.RenderScene = &interaction.RenderScene{
		Scene:   "escena_inventada",
		Subject: interaction.UserObservationSubjectUser,
	}
	if _, ok := e.applyFase2Scene(contract); ok {
		t.Fatal("escena fuera del conjunto Fase 2 no puede rendir superficie")
	}
	contract.RenderScene = &interaction.RenderScene{
		Scene: string(linguistics.MoveDatoFaltante),
		// Sin sujeto mapeable ("" unknown): el renderizador exige "user".
	}
	if _, ok := e.applyFase2Scene(contract); ok {
		t.Fatal("sujeto no mapeable no puede rendir superficie")
	}
}

// Ciclo personal vivo por el camino sync: el ask-back y la respuesta de valor
// deben salir del renderizador de Fase 2 (la traza lo prueba con
// renderer=code:fase2), no de una redacción del modelo.
func TestFase2PersonalCycleSurfacesComeFromRenderer(t *testing.T) {
	e := newInteractionE2EEngine(t)
	t.Setenv("AHMELIA_FLOW_TRACE", "1")
	var responses []string
	e.SetResponseCallback(func(msg string) { responses = append(responses, msg) })

	e.Process("¿cuál es mi color favorito?")
	trace1 := fase2TraceLines(e)
	if !strings.Contains(trace1, "renderer=code:fase2 scene=dato_faltante") {
		t.Fatalf("el ask-back sync no salió del renderizador de Fase 2:\n%s", trace1)
	}

	e.Process("negro")
	e.Process("¿cuál es mi color favorito?")
	if len(responses) == 0 {
		t.Fatal("el ciclo no habló")
	}
	last := responses[len(responses)-1]
	if !strings.Contains(last, "Tu color favorito es") || !strings.Contains(last, "negro") {
		t.Fatalf("el valor capturado no se respondió: %q (responses=%+v)", last, responses)
	}
	trace3 := fase2TraceLines(e)
	if !strings.Contains(trace3, "renderer=code:fase2 scene=responder_valor") {
		t.Fatalf("la respuesta de valor no salió del renderizador de Fase 2:\n%s", trace3)
	}
}

func TestFase2FeedbackSurfaceIsCanonicalAndTopicFree(t *testing.T) {
	e := newInteractionE2EEngine(t)
	var responses []string
	e.SetResponseCallback(func(msg string) { responses = append(responses, msg) })

	e.Process("mi color favorito es el negro")
	e.Process("perfecto, eso quedó bien")
	if len(responses) == 0 {
		t.Fatal("sin respuesta para el feedback")
	}
	last := responses[len(responses)-1]
	if !strings.Contains(last, "Gracias por decírmelo") {
		t.Fatalf("el acuse de feedback no salió del renderizador canónico: %q (responses=%+v)", last, responses)
	}
	if strings.Contains(strings.ToLower(last), "color") {
		t.Fatalf("eco de tema en el acuse de feedback: %q", last)
	}
}

// Guarda de cableado (patrón H7): los sitios reales deben seguir llevando la
// escena al contrato. Si alguien desconecta la integración, esto falla.
func TestFase2SceneWiringInRealPath(t *testing.T) {
	expected := map[string][]string{
		"engine.go": {
			"fase2AskBackScene(key)",
			"fase2ValueScene(answerAttr, answerValue)",
		},
		"turn_pipeline.go": {
			"fase2AskBackScene(firstQueryTopic(query.Topics))",
			"fase2ValueScene(attributes[0].Key, attributes[0].Value)",
			"fase2ValueScene(prefs[0].Key, prefs[0].Value)",
			"MoveFeedbackRecibido",
			"MoveNoEntendido",
		},
		"response_orchestrator.go": {
			"e.applyFase2Scene(contract)",
		},
	}
	for file, needles := range expected {
		body, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("%s: %v", file, err)
		}
		for _, needle := range needles {
			if !strings.Contains(string(body), needle) {
				t.Errorf("%s no contiene %q: la escena quedó desconectada del camino real", file, needle)
			}
		}
	}
}
