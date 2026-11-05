package core

// late_personal_query_test.go — puerta DIFERIDA de consulta personal.
//
// Gap de la demo por voz del propietario (2026-10-17 19:10-19:29): con la
// comprensión abierta el turno vivo ya no espera la lectura (presupuesto
// sync ~900 ms) y la lectura llega después. Cuando esa lectura tardía muestra
// una consulta sobre un dato propio sin valor, la deuda honesta sigue siendo
// la misma: reconfirmar y abrir la única puerta de admisión (Eje Compañera,
// Paso 1). Esta batería fija el carril diferido.
//
// Frases exactas del propietario (R11): solo aquí y en la evidencia.

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"ahmelia/interaction"
	"ahmelia/linguistics"
)

func latePersonalDeferred(raw string, om linguistics.OpenMeaning) linguistics.DeferredResult {
	return linguistics.DeferredResult{
		Job: linguistics.DeferredJob{
			ID:      "ling-deferred-personal-test",
			Request: interaction.Analyze(raw, interaction.Context{}),
		},
		Open: &om,
	}
}

func latePersonalOpenQuestion(t *testing.T, e *Engine) *interaction.OpenQuestion {
	t.Helper()
	snap := e.conversationState.Snapshot()
	for i := range snap.OpenQuestions {
		if snap.OpenQuestions[i].Status == interaction.OpenQuestionOpen {
			return &snap.OpenQuestions[i]
		}
	}
	return nil
}

func TestLatePersonalQuery_OpensAskBackOnceAndAnchorsTurn(t *testing.T) {
	e := newInteractionE2EEngine(t)
	var responses []string
	e.SetResponseCallback(func(msg string) { responses = append(responses, msg) })

	// Un turno real primero: la pregunta diferida no puede anclarse en 0.
	e.Process("hola")
	before := len(responses)

	// Forma real de la demo: "sabes ..." llegó como declaración al contrato
	// abierto y, aun así, es consulta por atributo propio sin valor.
	result := latePersonalDeferred("sabes mi bebida favorita", linguistics.OpenMeaning{
		Purpose:     linguistics.OpenPurposeDeclaration,
		OwnerOfData: linguistics.OpenOwnerUser,
		Topic:       "mi bebida favorita",
		Meaning:     "El usuario pregunta si conozco su bebida favorita.",
		Confidence:  .98,
	})
	if !e.deliverLatePersonalQuery(result) {
		t.Fatal("la consulta personal tardía no fue asumida por la puerta diferida")
	}
	spoken := responses[before:]
	if len(spoken) != 1 ||
		!strings.Contains(spoken[0], "Todavía no tengo ese dato guardado") ||
		!strings.Contains(spoken[0], "¿Me lo dices?") {
		t.Fatalf("ask-back diferido no hablado exactamente una vez: %+v", spoken)
	}
	open := latePersonalOpenQuestion(t, e)
	if open == nil || open.Key != "bebida favorita" || open.Category != "preference" {
		t.Fatalf("pregunta abierta diferida inesperada: %+v", open)
	}
	if open.AskedTurn == 0 {
		t.Fatalf("pregunta diferida sin ancla de turno: %+v", open)
	}

	// Entrega repetida (jobs repetidos de la demo): ni habla nueva ni pregunta nueva.
	before2 := len(responses)
	if !e.deliverLatePersonalQuery(result) {
		t.Fatal("la repetición debía quedar asumida (en silencio)")
	}
	if len(responses) != before2 {
		t.Fatalf("el ask-back se repitió: %+v", responses[before2:])
	}
	if snap := e.conversationState.Snapshot(); len(snap.OpenQuestions) != 1 {
		t.Fatalf("preguntas abiertas=%d, esperaba 1: %+v", len(snap.OpenQuestions), snap.OpenQuestions)
	}
}

func TestLatePersonalQuery_AnswerEntersAndRepeatAnswersWithValue(t *testing.T) {
	e := newInteractionE2EEngine(t)
	var responses []string
	e.SetResponseCallback(func(msg string) { responses = append(responses, msg) })

	e.Process("hola")
	// Forma "cuál es mi ...": pregunta en el contrato abierto (jobs 6/7 de la demo).
	result := latePersonalDeferred("cuál es mi bebida favorita", linguistics.OpenMeaning{
		Purpose:     linguistics.OpenPurposeQuestion,
		OwnerOfData: linguistics.OpenOwnerUser,
		Topic:       "bebida favorita",
		Meaning:     "El usuario pregunta por su bebida favorita.",
		Confidence:  1,
	})
	if !e.deliverLatePersonalQuery(result) {
		t.Fatal("consulta personal tardía no asumida")
	}
	open := latePersonalOpenQuestion(t, e)
	if open == nil {
		t.Fatal("sin pregunta abierta tras el ask-back diferido")
	}

	// La respuesta del propietario entra por la misma puerta (Eje Compañera).
	e.Process("me gusta la gaseosa de piña")
	prefs := scenePrefs(t)
	if len(prefs) != 1 || prefs[0].Key != "bebida favorita" || !strings.Contains(prefs[0].Value, "gaseosa de piña") {
		t.Fatalf("la respuesta no entró por la puerta diferida: %+v", prefs)
	}
	snap := e.conversationState.Snapshot()
	answered := false
	for _, q := range snap.OpenQuestions {
		if q.Key == "bebida favorita" && q.Status == interaction.OpenQuestionAnswered && q.AnsweredTurn == open.AskedTurn+1 {
			answered = true
		}
	}
	if !answered {
		t.Fatalf("la pregunta diferida no quedó respondida en el turno siguiente: %+v", snap.OpenQuestions)
	}

	// Repetición de la consulta con el dato ya capturado: responde el valor
	// por el carril diferido (misma voz y contrato que sync), no vuelve a
	// preguntar ni abre otra puerta.
	before := len(responses)
	if !e.deliverLatePersonalQuery(result) {
		t.Fatal("la entrega posterior debía quedar asumida")
	}
	spoken := responses[before:]
	if len(spoken) != 1 ||
		!strings.Contains(spoken[0], "Tu bebida favorita es") ||
		!strings.Contains(spoken[0], "gaseosa de piña") {
		t.Fatalf("la repetición no respondió el valor real: %+v", spoken)
	}
	if latePersonalOpenQuestion(t, e) != nil {
		t.Fatal("la repetición con dato no debe abrir otra pregunta")
	}
}

func TestLatePersonalQuery_RestrictiveMarksSilence(t *testing.T) {
	e := newInteractionE2EEngine(t)
	var responses []string
	e.SetResponseCallback(func(msg string) { responses = append(responses, msg) })

	e.Process("hola")
	before := len(responses)
	result := latePersonalDeferred("sabes mi bebida favorita", linguistics.OpenMeaning{
		Purpose:     linguistics.OpenPurposeQuestion,
		OwnerOfData: linguistics.OpenOwnerUser,
		Topic:       "mi bebida favorita",
		Confidence:  .9,
		Marks:       linguistics.OpenMarks{Negation: &linguistics.OpenNegation{Scope: "estado", Target: "gaseosa de piña"}},
	})
	if !e.deliverLatePersonalQuery(result) {
		t.Fatal("la entrega con marcas debe quedar asumida (en silencio), no caer al camino genérico")
	}
	if len(responses) != before {
		t.Fatalf("habló una lectura con marcas restrictivas: %+v", responses[before:])
	}
	if latePersonalOpenQuestion(t, e) != nil {
		t.Fatal("las marcas restrictivas no deben abrir pregunta personal")
	}
}

// Guarda de cableado (patrón H7): la puerta diferida debe seguir invocada
// desde la entrega diferida del ciclo de iniciativa.
func TestLatePersonalQuery_WiringFromDeferredDelivery(t *testing.T) {
	body, err := os.ReadFile("engine.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "e.deliverLatePersonalQuery(result)") {
		t.Fatal("engine.go no invoca deliverLatePersonalQuery desde la entrega diferida")
	}
	if !strings.Contains(string(body), "e.payReadyPersonalQueries()") {
		t.Fatal("engine.go no paga la deuda personal diferida desde la evaluación idle")
	}
}

// ---------------------------------------------------------------------------
// Camino idle completo: la evaluación de iniciativa debe pagar la deuda
// personal ANTES de la guarda de rendición y del arbitraje. El resolver corre
// en modo abierto con un cliente stub local (sin red): la lectura es evidencia,
// la puerta de entrega es la misma de la corrida viva.

func latePersonalStubOpenRaw(purpose, topic string) string {
	return `{"purpose":"` + purpose + `","owner_of_data":"user","topic":"` + topic + `",` +
		`"meaning":"El usuario consulta por un dato propio.","mentions":[],"confidence":0.98}`
}

func newLatePersonalIdleResolver(t *testing.T, raw string) *linguistics.DeferredResolver {
	t.Helper()
	resolver := linguistics.NewDeferredResolver(linguistics.NewInterpreter(stubOpenLLM{raw: raw}), 8)
	resolver.SetOpenComprehension(true)
	resolver.Start(context.Background())
	t.Cleanup(resolver.Stop)
	return resolver
}

func waitLatePersonalReady(t *testing.T, resolver *linguistics.DeferredResolver, want int) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if len(resolver.Ready()) >= want {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("timeout esperando %d lecturas diferidas listas (hay %d)", want, len(resolver.Ready()))
}

// latePersonalAgeTurn mueve el último turno al pasado (silencio genuino): la
// puerta de 8 s de la evaluación idle queda abierta.
func latePersonalAgeTurn(e *Engine) {
	past := time.Now().Add(-2 * time.Minute)
	e.conversationState.LastTurnAt = past
	if n := len(e.conversationState.Turns); n > 0 {
		e.conversationState.Turns[n-1].At = past
	}
}

func TestLatePersonalQuery_IdlePaysValueBeforeArbitration(t *testing.T) {
	e := newInteractionE2EEngine(t)
	var responses []string
	e.SetResponseCallback(func(msg string) { responses = append(responses, msg) })

	e.Process("hola")
	// Ciclo real (re-demo): ask-back diferido → captura → la repetición debe
	// responder el valor aunque la lectura llegue marcada «continuación», que
	// es el caso exacto que la guarda de rendición suprimía.
	ask := latePersonalDeferred("cuál es mi bebida favorita", linguistics.OpenMeaning{
		Purpose:     linguistics.OpenPurposeQuestion,
		OwnerOfData: linguistics.OpenOwnerUser,
		Topic:       "bebida favorita",
		Meaning:     "El usuario pregunta por su bebida favorita.",
		Confidence:  1,
	})
	if !e.deliverLatePersonalQuery(ask) {
		t.Fatal("consulta personal tardía no asumida")
	}
	e.Process("me gusta la gaseosa de piña")
	if prefs := scenePrefs(t); len(prefs) != 1 {
		t.Fatalf("la captura previa no entró: %+v", prefs)
	}

	repeat := interaction.Analyze("sabes mi bebida favorita", interaction.Context{})
	repeat.Continues = true
	resolver := newLatePersonalIdleResolver(t, latePersonalStubOpenRaw("declaracion", "bebida favorita"))
	if _, err := resolver.Submit(repeat, interaction.Context{Language: "es"}); err != nil {
		t.Fatal(err)
	}
	waitLatePersonalReady(t, resolver, 1)
	e.deferredLinguistic = resolver
	latePersonalAgeTurn(e)

	before := len(responses)
	e.evaluateIdleConversationalInitiative()

	spoken := responses[before:]
	if len(spoken) != 1 ||
		!strings.Contains(spoken[0], "Tu bebida favorita es") ||
		!strings.Contains(spoken[0], "gaseosa de piña") {
		t.Fatalf("el camino idle no respondió el valor antes del arbitraje: %+v", spoken)
	}
	if e.lastInitiativeProposal != nil {
		t.Fatal("la deuda personal no debe pasar por el arbitraje de iniciativa")
	}
	if got := len(resolver.Ready()); got != 0 {
		t.Fatalf("el job no quedó consumido: %d", got)
	}
	if e.payReadyPersonalQueries() {
		t.Fatal("sin jobs listos, la deuda no puede reclamar el tick")
	}
	if len(responses) != before+1 {
		t.Fatalf("habla extra tras consumir el job: %+v", responses[before:])
	}
}

func TestLatePersonalQuery_IdlePaysAskBackAndConsumesFamily(t *testing.T) {
	e := newInteractionE2EEngine(t)
	var responses []string
	e.SetResponseCallback(func(msg string) { responses = append(responses, msg) })
	e.Process("hola")

	resolver := newLatePersonalIdleResolver(t, latePersonalStubOpenRaw("declaracion", "bebida favorita"))
	// Dos formulaciones con firma de ruteo PROPIA (el resolver coalesce firmas
	// idénticas) y misma clave resuelta: la familia debe consumirse con una
	// sola entrega.
	first := interaction.Analyze("sabes mi bebida favorita", interaction.Context{})
	first.Continues = true // la marca que desviaba la repetición en la re-demo
	first.Topic = "bebida favorita"
	second := interaction.Analyze("cuál es mi bebida favorita", interaction.Context{})
	second.Topic = "preferencia de bebida"
	if _, err := resolver.Submit(first, interaction.Context{Language: "es"}); err != nil {
		t.Fatal(err)
	}
	if _, err := resolver.Submit(second, interaction.Context{Language: "es"}); err != nil {
		t.Fatal(err)
	}
	waitLatePersonalReady(t, resolver, 2)
	e.deferredLinguistic = resolver
	latePersonalAgeTurn(e)

	before := len(responses)
	e.evaluateIdleConversationalInitiative()

	spoken := responses[before:]
	if len(spoken) != 1 ||
		!strings.Contains(spoken[0], "Todavía no tengo ese dato guardado") ||
		!strings.Contains(spoken[0], "¿Me lo dices?") {
		t.Fatalf("ask-back diferido por el camino idle: %+v", spoken)
	}
	open := latePersonalOpenQuestion(t, e)
	if open == nil || open.Key != "bebida favorita" {
		t.Fatalf("pregunta abierta inesperada: %+v", open)
	}
	if e.lastInitiativeProposal != nil {
		t.Fatal("la deuda personal no debe pasar por el arbitraje de iniciativa")
	}
	if got := len(resolver.Ready()); got != 0 {
		t.Fatalf("la familia de jobs no quedó consumida: %d", got)
	}
	if e.payReadyPersonalQueries() {
		t.Fatal("sin jobs listos, la deuda no puede reclamar el tick")
	}
	if len(responses) != before+1 {
		t.Fatalf("habla extra tras consumir la familia: %+v", responses[before:])
	}
}

func TestLatePersonalQuery_AlreadySpokenValueStaysSilent(t *testing.T) {
	e := newInteractionE2EEngine(t)
	var responses []string
	e.SetResponseCallback(func(msg string) { responses = append(responses, msg) })

	e.Process("hola")
	ask := latePersonalDeferred("cuál es mi bebida favorita", linguistics.OpenMeaning{
		Purpose:     linguistics.OpenPurposeQuestion,
		OwnerOfData: linguistics.OpenOwnerUser,
		Topic:       "bebida favorita",
		Meaning:     "El usuario pregunta por su bebida favorita.",
		Confidence:  1,
	})
	if !e.deliverLatePersonalQuery(ask) {
		t.Fatal("consulta personal tardía no asumida")
	}
	e.Process("me gusta la gaseosa de piña")
	prefs := scenePrefs(t)
	if len(prefs) != 1 {
		t.Fatalf("la captura previa no entró: %+v", prefs)
	}

	// El camino sync ya respondió la repetición con el dato en mano: la entrega
	// diferida no puede volver a decir lo mismo.
	e.conversationState.RecordTurn(interaction.SemanticRequest{
		Raw: "cuál es mi bebida favorita", Normalized: "cuál es mi bebida favorita",
		Kind: interaction.KindQuestion, Topic: "bebida favorita", Channel: interaction.ChannelText,
	}, "Tu bebida favorita es "+prefs[0].Value+".")

	resolver := newLatePersonalIdleResolver(t, latePersonalStubOpenRaw("pregunta", "bebida favorita"))
	repeat := interaction.Analyze("cuál es mi bebida favorita", interaction.Context{})
	if _, err := resolver.Submit(repeat, interaction.Context{Language: "es"}); err != nil {
		t.Fatal(err)
	}
	waitLatePersonalReady(t, resolver, 1)
	e.deferredLinguistic = resolver
	latePersonalAgeTurn(e)

	before := len(responses)
	if !e.payReadyPersonalQueries() {
		t.Fatal("la entrega ya hablada debía quedar asumida (en silencio)")
	}
	if len(responses) != before {
		t.Fatalf("el valor se habló por segunda vez: %+v", responses[before:])
	}
	if got := len(resolver.Ready()); got != 0 {
		t.Fatalf("el job no quedó consumido: %d", got)
	}
}
