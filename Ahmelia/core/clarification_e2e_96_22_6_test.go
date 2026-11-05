package core

import (
	"os"
	"strings"
	"testing"

	"ahmelia/interaction"
	"ahmelia/memory"
	"ahmelia/selfknowledge"
)

// TestClarificationSurvivesInterruptionAndResumes — B96.22.6. Reproduce el
// ejemplo exacto del usuario:
//
//	"Quiero avanzar esta escena."
//	Ahmelia: "¿Priorizamos la iluminación o la interacción del personaje?"
//	(el usuario pide otra cosa en el medio: "abre chrome")
//	Ahmelia: hace lo nuevo, Y retoma la pregunta original en el mismo cierre.
//	"la interacción."
//	Ahmelia: ya no pierde el objetivo original ("avanzar escena").
func TestClarificationSurvivesInterruptionAndResumes(t *testing.T) {
	isolateTestWorkspace(t)
	t.Setenv("AHMELIA_SKIP_PROGRAM_INDEX_SCAN", "1")
	longMem := memory.LoadLongMemory()
	shortMem := LoadShortMemory()
	sess := memory.NewSession()
	history := selfknowledge.NewActionHistory(4)
	registry := selfknowledge.NewModuleRegistry()
	selfQuery := selfknowledge.NewSelfQuery(history, registry, "test")
	e := NewEngine(sess, longMem, &shortMem, history, selfQuery)
	installF9LanguageStubs(t, e)

	var responses []string
	e.SetResponseCallback(func(msg string) { responses = append(responses, msg) })

	// 1) Ahmelia queda con una pregunta pendiente (simula lo que un futuro
	// consumidor — ProductionRuntime, una capacidad de escenas — haría vía
	// AskClarification en vez de responder directo).
	question := e.AskClarification(
		"avanzar escena",
		"¿Priorizamos la iluminación o la interacción del personaje?",
		"iluminación", "interacción",
	)
	if question == "" {
		t.Fatal("se esperaba el texto de la pregunta")
	}
	if !e.clarificationQueue.HasOpen() {
		t.Fatal("la aclaración debe quedar encolada")
	}

	// 2) El usuario se desvía con algo completamente distinto. La pregunta
	// pendiente NO debe perderse, y Ahmelia debe retomarla sola al cerrar
	// este turno, según la decisión de producto tomada explícitamente.
	responses = nil
	e.Process("abre chrome")

	if !e.clarificationQueue.HasOpen() {
		t.Fatal("la aclaración original no debía resolverse con una interrupción")
	}
	found := false
	for _, r := range responses {
		if r == "Volviendo a lo anterior: ¿Priorizamos la iluminación o la interacción del personaje?" {
			found = true
		}
	}
	if !found {
		t.Fatalf("se esperaba que el cierre del turno retomara la pregunta pendiente; respuestas=%v", responses)
	}

	// 3) El usuario ahora sí contesta la pregunta. El objetivo original debe
	// conservarse (fusionado con la respuesta), y la cola debe quedar vacía.
	responses = nil
	e.Process("la interacción")

	if e.clarificationQueue.HasOpen() {
		t.Fatal("la aclaración debía quedar resuelta tras la respuesta del usuario")
	}
}

// TestAskClarificationReturnsQuestionEvenWithoutQueue — defensivo: si por
// algún motivo el Engine no tuviera cola (nil), AskClarification no debe
// entrar en pánico y debe seguir devolviendo la pregunta para que el
// llamador pueda usarla como respuesta.
func TestAskClarificationReturnsQuestionEvenWithoutQueue(t *testing.T) {
	var e *Engine
	got := e.AskClarification("meta", "¿A o B?", "a", "b")
	if got != "¿A o B?" {
		t.Fatalf("got=%q", got)
	}
}

func TestConversationQuestionDoesNotFallIntoLowConfidenceResponse(t *testing.T) {
	isolateTestWorkspace(t)
	defer os.Remove("memory/conversation_state.json")
	_ = os.Remove("memory/conversation_state.json")
	t.Setenv("AHMELIA_SKIP_PROGRAM_INDEX_SCAN", "1")
	longMem := memory.LoadLongMemory()
	shortMem := LoadShortMemory()
	sess := memory.NewSession()
	history := selfknowledge.NewActionHistory(4)
	registry := selfknowledge.NewModuleRegistry()
	selfQuery := selfknowledge.NewSelfQuery(history, registry, "test")
	e := NewEngine(sess, longMem, &shortMem, history, selfQuery)
	installF9LanguageStubs(t, e)

	var responses []string
	e.SetResponseCallback(func(msg string) { responses = append(responses, msg) })
	e.Process("que haces?")

	if len(responses) == 0 {
		t.Fatal("se esperaba una respuesta")
	}
	last := responses[len(responses)-1]
	if strings.Contains(last, "30%") || strings.Contains(last, "poca confianza") {
		t.Fatalf("una pregunta sobre el estado conversacional no debe producir incertidumbre artificial: %q", last)
	}
	if !strings.Contains(strings.ToLower(last), "aquí contigo") {
		t.Fatalf("respuesta inesperada: %q", last)
	}
}

func TestConversationAnswerToPersonalQuestionIsCaptured(t *testing.T) {
	isolateTestWorkspace(t)
	defer os.Remove("memory/user_profile.json")
	defer os.Remove("memory/conversation_state.json")
	_ = os.Remove("memory/conversation_state.json")
	t.Setenv("AHMELIA_SKIP_PROGRAM_INDEX_SCAN", "1")
	longMem := memory.LoadLongMemory()
	shortMem := LoadShortMemory()
	sess := memory.NewSession()
	history := selfknowledge.NewActionHistory(4)
	registry := selfknowledge.NewModuleRegistry()
	selfQuery := selfknowledge.NewSelfQuery(history, registry, "test")
	e := NewEngine(sess, longMem, &shortMem, history, selfQuery)
	installF9LanguageStubs(t, e)

	e.Process("¿cuál es mi bebida favorita?")
	e.Process("café")

	store, err := interaction.LoadUserModelStore(interaction.UserModelPath, "ayma240")
	if err != nil {
		t.Fatalf("no se pudo cargar el UserModel formal: %v", err)
	}
	prefs, err := interaction.GetRelevantPreferences(store, interaction.UserModelActorCognition, interaction.UserModelQueryContext{Topics: []string{"bebida favorita"}})
	if err != nil || len(prefs) != 1 || prefs[0].Value != "café" {
		t.Fatalf("la respuesta personal no quedó en B96.34: err=%v prefs=%v", err, prefs)
	}
}
