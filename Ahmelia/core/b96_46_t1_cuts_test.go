package core

import (
	"context"
	"strings"
	"sync"
	"testing"

	"ahmelia/interaction"
	"ahmelia/language"
)

// recordingLanguageClient guarda cada prompt que Ahmelia le manda al modelo y
// devuelve siempre una respuesta válida. Permite comprobar QUÉ evidencia viaja
// al lenguaje sin depender de la redacción de un modelo real.
type recordingLanguageClient struct {
	mu      sync.Mutex
	prompts []string
}

func (r *recordingLanguageClient) Available() bool { return true }

func (r *recordingLanguageClient) Generate(prompt string) (string, error) {
	r.mu.Lock()
	r.prompts = append(r.prompts, prompt)
	r.mu.Unlock()
	return `{"response":"ok"}`, nil
}

func (r *recordingLanguageClient) GenerateContext(_ context.Context, prompt string) (string, error) {
	return r.Generate(prompt)
}

func (r *recordingLanguageClient) all() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return strings.Join(r.prompts, "\n---\n")
}

// SetStartupChannel solo debe llenar el canal cuando todavía no hay uno real:
// un canal ya observado manda, y un canal vacío o desconocido no cuenta.
func TestSetStartupChannelFillsOnlyWhenChannelUnknown(t *testing.T) {
	isolateTestWorkspace(t)
	t.Setenv("AHMELIA_DISABLE_TTS", "1")
	e := NewEngine(nil, nil, nil, nil, nil)

	e.lastInputChannel = ""
	e.SetStartupChannel(interaction.ChannelUnknown)
	e.SetStartupChannel("")
	if e.lastInputChannel != "" {
		t.Fatalf("un canal desconocido o vacío no debe llenar el canal de arranque, got %q", e.lastInputChannel)
	}

	e.SetStartupChannel(interaction.ChannelVoice)
	if e.lastInputChannel != interaction.ChannelVoice {
		t.Fatalf("el canal de arranque no se llenó: %q", e.lastInputChannel)
	}

	e.SetStartupChannel(interaction.ChannelText)
	if e.lastInputChannel != interaction.ChannelVoice {
		t.Fatalf("el canal de arranque no debe pisar un canal ya observado: %q", e.lastInputChannel)
	}

	var nilEngine *Engine
	nilEngine.SetStartupChannel(interaction.ChannelVoice) // no debe entrar en pánico
}

// Regresión del 13 de octubre de 2026: la frase "...las cosas empezaran a salir a
// flote, que sabes sobre ti misma?" se tomó como orden de salida. Ahmelia se
// despidió y ejecutó Shutdown en medio de una conversación. Ahora el turno debe
// seguir como pregunta y la despedida no puede llegar al lenguaje.
func TestLongSentenceContainingSalirDoesNotSayGoodbyeOrShutDown(t *testing.T) {
	e := buildTestEngine(t)
	e.conversationState = interaction.NewConversationState()
	rec := &recordingLanguageClient{}
	e.conversationGenerator = language.NewConversationalGenerator(rec)
	e.SetResponseCallback(func(string) {})

	e.Process("que piensas de lo que hemos construido es dificil saber cuando las cosas empezaran a salir a flote, que sabes sobre ti misma?")

	if strings.Contains(rec.all(), "Hasta luego") {
		t.Fatalf("la frase se interpretó como una orden de salida; prompts=%q", rec.all())
	}
}
