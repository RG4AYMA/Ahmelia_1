package core

import (
	"context"
	"path/filepath"
	"testing"

	"ahmelia/interaction"
	"ahmelia/language"
	"ahmelia/production"
)

type expectationCountingClient struct {
	calls int
}

func (c *expectationCountingClient) Available() bool { return true }

func (c *expectationCountingClient) GenerateContext(ctx context.Context, prompt string) (string, error) {
	c.calls++
	return `{"response":"texto reescrito por el modelo"}`, nil
}

func TestExpectationVerdictBypassesModelRewrite(t *testing.T) {
	e := newExpectationTestEngine(t, filepath.Join(t.TempDir(), "memory", "expectations.jsonl"))
	client := &expectationCountingClient{}
	e.conversationGenerator = language.NewConversationalGenerator(client)
	var spoken string
	e.onResponse = func(msg string) { spoken = msg }

	summary := "Esperaba que «C:\\tmp\\Prueba» existiera; observé que existe. Veredicto: confirmado."
	result := production.ActionResult{
		Executed: true, Success: true, Observed: true, Verified: true,
		OutcomeStatus:      production.OutcomeSucceeded,
		ExpectationID:      "exp-test-1",
		ExpectationStatus:  "confirmed",
		ExpectationSummary: summary,
		Summary:            "Carpeta creada",
	}
	req := interaction.SemanticRequest{Raw: "crea la carpeta Prueba", Kind: interaction.KindAction}
	if ok := e.renderExecutionActionResult(req, result, false); !ok {
		t.Fatal("la respuesta determinista no se entregó")
	}
	if client.calls != 0 {
		t.Fatalf("el modelo no debe reescribir un veredicto; llamadas=%d", client.calls)
	}
	if spoken != summary {
		t.Fatalf("la superficie no es el texto exacto del arnés:\nquiero: %q\nobtuve: %q", summary, spoken)
	}
}

func TestExpectationVerdictResponseRequiresTerminalStatus(t *testing.T) {
	if got := expectationVerdictResponse(production.ActionResult{ExpectationID: "exp-1", ExpectationStatus: "declared"}); got != "" {
		t.Fatalf("una declaración no puede publicarse como veredicto: %q", got)
	}
}
