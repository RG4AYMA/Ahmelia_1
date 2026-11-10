package core

import (
	"context"
	"strings"
	"testing"
	"time"

	"ahmelia/interaction"
	"ahmelia/language"
	"ahmelia/production"
)

// blockingConversationalClient models the real single-slot language client
// while a background job owns it: any attempt to render through it waits until
// the context expires or the test releases it.
type blockingConversationalClient struct {
	released chan struct{}
}

func (c *blockingConversationalClient) Available() bool { return true }

func (c *blockingConversationalClient) GenerateContext(ctx context.Context, _ string) (string, error) {
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case <-c.released:
		return `{"response":"modelo"}`, nil
	}
}

func TestOperationalStatusDoesNotWaitForLanguage(t *testing.T) {
	e := buildTestEngine(t)
	client := &blockingConversationalClient{released: make(chan struct{})}
	defer close(client.released)
	e.conversationGenerator = language.NewConversationalGenerator(client)
	var responses []string
	e.SetResponseCallback(func(text string) { responses = append(responses, text) })

	req := interaction.Analyze("estado del trabajo", interaction.Context{})
	asyncText := "Lo estoy trabajando en segundo plano. Te aviso cuando termine."
	start := time.Now()
	if !e.renderVerbatimSystemResponse(req, "production_async_started", asyncText, interaction.MoveAcknowledge, interaction.ResponseIntentAcknowledge, false) {
		t.Fatal("el estado async no se entregó")
	}
	if elapsed := time.Since(start); elapsed > 500*time.Millisecond {
		t.Fatalf("el estado async esperó al lenguaje: %s", elapsed)
	}

	authText := "La tarea quedó preparada, pero necesito tu autorización explícita para ejecutar programming.create_file."
	start = time.Now()
	if !e.renderVerbatimSystemResponse(req, "execution_authorization_required", authText, interaction.MoveAsk, interaction.ResponseIntentClarify, true) {
		t.Fatal("la autorización no se entregó")
	}
	if elapsed := time.Since(start); elapsed > 500*time.Millisecond {
		t.Fatalf("la autorización esperó al lenguaje: %s", elapsed)
	}

	if len(responses) != 2 || responses[0] != asyncText || responses[1] != authText {
		t.Fatalf("respuestas = %#v", responses)
	}
}

func TestCancelDuringBackgroundWorkDoesNotWaitForLanguage(t *testing.T) {
	e := buildTestEngine(t)
	client := &blockingConversationalClient{released: make(chan struct{})}
	defer close(client.released)
	e.conversationGenerator = language.NewConversationalGenerator(client)

	started := make(chan struct{})
	release := make(chan struct{})
	id := e.StartBackgroundJob(context.Background(), "slow-shadow", func(ctx context.Context) (production.ActionResult, error) {
		close(started)
		select {
		case <-release:
			return production.ActionResult{}, nil
		case <-ctx.Done():
			return production.ActionResult{}, ctx.Err()
		}
	})
	if id == "" {
		t.Fatal("no se creó el job de fondo")
	}
	<-started
	defer func() {
		close(release)
		waitForBackgroundJob(t, e, id, BackgroundJobCanceled)
	}()

	var responses []string
	e.SetResponseCallback(func(text string) { responses = append(responses, text) })
	start := time.Now()
	e.Process("cancela el trabajo")
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("la cancelación esperó al lenguaje: %s", elapsed)
	}
	if joined := strings.Join(responses, " "); !strings.Contains(joined, "Cancelé el trabajo") {
		t.Fatalf("no se anunció la cancelación: %q", joined)
	}
	waitForBackgroundJob(t, e, id, BackgroundJobCanceled)
}
