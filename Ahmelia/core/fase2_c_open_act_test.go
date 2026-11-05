package core

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"ahmelia/interaction"
	"ahmelia/linguistics"
)

// fase2CAwaitClient simula el modelo local sin red: con presupuesto corto
// (sync) se queda esperando el deadline; con presupuesto real devuelve la
// lectura abierta. La misma instancia sirve el sync (contrato cerrado) y el
// job diferido (comprensión abierta).
type fase2CAwaitClient struct {
	openJSON string
}

func (c *fase2CAwaitClient) Available() bool { return true }
func (c *fase2CAwaitClient) Generate(prompt string) (string, error) {
	return c.GenerateContext(context.Background(), prompt)
}
func (c *fase2CAwaitClient) GenerateContext(ctx context.Context, prompt string) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if deadline, ok := ctx.Deadline(); ok && time.Until(deadline) < 500*time.Millisecond {
		<-ctx.Done()
		return "", ctx.Err()
	}
	time.Sleep(5 * time.Millisecond)
	return `{}`, nil
}
func (c *fase2CAwaitClient) GenerateStructuredContext(ctx context.Context, prompt string, schema json.RawMessage, numPredict int) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if deadline, ok := ctx.Deadline(); ok && time.Until(deadline) < 500*time.Millisecond {
		<-ctx.Done()
		return "", ctx.Err()
	}
	time.Sleep(5 * time.Millisecond)
	return c.openJSON, nil
}

const fase2CElogio = "la verdad es que hiciste un trabajo enorme hoy, se nota todo el esfuerzo"
const fase2COpenFeedbackJSON = `{"purpose":"feedback","owner_of_data":"user","topic":"el esfuerzo de Ahmelia","meaning":"Elogio sobre el trabajo de Ahmelia","mentions":[],"confidence":0.95}`

func newFase2CEngine(t *testing.T) (*Engine, *linguistics.DeferredResolver) {
	t.Helper()
	t.Setenv("AHMELIA_LINGUISTIC_SYNC_BUDGET_MS", "50")
	client := &fase2CAwaitClient{openJSON: fase2COpenFeedbackJSON}
	interpreter := linguistics.NewInterpreter(client)
	resolver := linguistics.NewDeferredResolver(interpreter, 4)
	resolver.SetOpenComprehension(true)
	resolver.Start(context.Background())
	t.Cleanup(resolver.Stop)
	e := &Engine{
		interactionManager:    interaction.NewManager(),
		linguisticInterpreter: interpreter,
		linguisticPipeline:    linguistics.NewPipeline(linguistics.NewRouter(), interpreter, linguistics.NewValidator()),
		deferredLinguistic:    resolver,
	}
	return e, resolver
}

// Fase 2 C: el turno sin acto tipado espera acotado su propia lectura abierta;
// cuando la lectura alcanza el acto de feedback, el acto llega al turno vivo.
func TestFase2COpenReadingAwaitSuppliesFeedbackAct(t *testing.T) {
	e, _ := newFase2CEngine(t)
	e.flowTrace = NewTurnFlowTrace(fase2CElogio)
	req := e.analyzeSemanticBoundary(fase2CElogio, interaction.Context{Language: "es"}, false, nil)
	if !req.CommunicativeActs.Has(interaction.ActFeedback) {
		t.Fatalf("el acto de feedback de la lectura no llegó al turno: kind=%s acts=%+v", req.Kind, req.CommunicativeActs)
	}
	if strings.TrimSpace(req.Topic) != "el esfuerzo de Ahmelia" {
		t.Fatalf("la lectura no dejó su tema en el turno: topic=%q", req.Topic)
	}
	joined := strings.Join(e.flowTrace.Snapshot(), "\n")
	if !strings.Contains(joined, "await=true") || !strings.Contains(joined, "act=feedback") {
		t.Fatalf("la traza no muestra la espera acotada con acto tipado:\n%s", joined)
	}
}

// La respuesta a una pregunta abierta no espera la lectura: el carril de
// admisión es dueño de ese turno y la lectura solo queda encolada para
// después del turno (el render del propio turno usa el modelo primero).
func TestFase2CAwaitSkipsAnswerToOpenQuestion(t *testing.T) {
	e, _ := newFase2CEngine(t)
	state := interaction.NewConversationState()
	// Fase de entrada real: el estado ya registró el turno que preguntó (1) y
	// activeConversationTurnID todavía no está fijado (0) para el turno nuevo.
	state.RecordTurn(interaction.SemanticRequest{Raw: "¿cuál es tu bebida favorita?", Normalized: "¿cuál es tu bebida favorita?"}, "Todavía no tengo ese dato guardado. ¿Me lo dices?")
	state.RegisterOpenQuestion(interaction.OpenQuestionKindPersonalValue, "preference", "bebida favorita", "Todavía no tengo ese dato guardado. ¿Me lo dices?", 1)
	e.conversationState = state
	e.flowTrace = NewTurnFlowTrace("la gaseosa de piña")
	pending := []pendingDeferredReading{}
	_ = e.analyzeSemanticBoundary("la gaseosa de piña", interaction.Context{Language: "es"}, false, &pending)
	joined := strings.Join(e.flowTrace.Snapshot(), "\n")
	if strings.Contains(joined, "await=true") {
		t.Fatalf("la respuesta a una pregunta abierta no debe esperar la lectura:\n%s", joined)
	}
	if len(pending) != 1 {
		t.Fatalf("la lectura del turno debe quedar encolada para después del turno, pending=%d", len(pending))
	}
}

// Cierre diferido de una lectura de feedback que el turno vivo no alcanzó a
// esperar: la escena determinista se dice igual; si el turno vivo ya la dijo,
// la entrega cierra el job en silencio.
func TestFase2CLateTypedActSpeaksFeedbackScene(t *testing.T) {
	t.Setenv("AHMELIA_DISABLE_TTS", "1")
	state := interaction.NewConversationState()
	e := &Engine{conversationState: state}
	var spoken []string
	e.onResponse = func(s string) { spoken = append(spoken, s) }
	req := interaction.Analyze(fase2CElogio, interaction.Context{Language: "es"})
	result := linguistics.DeferredResult{
		Job: linguistics.DeferredJob{ID: "fase2c-late-1", Request: req},
		Open: &linguistics.OpenMeaning{
			Purpose:    linguistics.OpenPurposeFeedback,
			Topic:      "el esfuerzo de Ahmelia",
			Meaning:    "Elogio sobre el trabajo de Ahmelia",
			Confidence: .95,
		},
		Ready: true,
	}
	if !e.deliverLateTypedAct(result) {
		t.Fatal("la lectura de feedback debe quedar bajo la autoridad de entrega tardía")
	}
	if len(spoken) != 1 || !strings.Contains(spoken[0], "Gracias por decírmelo") {
		t.Fatalf("se esperaba la escena determinista de feedback, hablado=%q", spoken)
	}
	// La escena ya dicha por el turno vivo no se repite.
	spoken = nil
	state.SetAssistantResponse("Entendido. Gracias por decírmelo; lo tengo en cuenta.")
	if !e.deliverLateTypedAct(result) {
		t.Fatal("la entrega tardía debe cerrar el job aunque no hable")
	}
	if len(spoken) != 0 {
		t.Fatalf("la escena ya dicha no debe repetirse, hablado=%q", spoken)
	}
}

// Una lectura que no produjo acto tipado no es una entrega: la escena tardía
// no habla (la supresión genérica se prueba en la corrida viva).
func TestFase2CLateTypedActIgnoresNonFeedbackReadings(t *testing.T) {
	state := interaction.NewConversationState()
	e := &Engine{conversationState: state}
	req := interaction.Analyze(fase2CElogio, interaction.Context{Language: "es"})
	result := linguistics.DeferredResult{
		Job: linguistics.DeferredJob{ID: "fase2c-late-2", Request: req},
		Open: &linguistics.OpenMeaning{
			Purpose:    linguistics.OpenPurposeSocial,
			Topic:      "saludo",
			Meaning:    "Saludo social",
			Confidence: .9,
		},
		Ready: true,
	}
	if e.deliverLateTypedAct(result) {
		t.Fatal("una lectura sin acto tipado no debe quedar bajo la autoridad de esta entrega")
	}
}
