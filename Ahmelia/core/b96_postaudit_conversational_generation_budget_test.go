package core

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"ahmelia/cognition"
	"ahmelia/interaction"
	"ahmelia/language"
	"ahmelia/socialinteraction"
)

type postAuditTimeoutConversationalClient struct {
	calls int
}

func (c *postAuditTimeoutConversationalClient) Available() bool { return true }

func (c *postAuditTimeoutConversationalClient) GenerateContext(ctx context.Context, _ string) (string, error) {
	c.calls++
	<-ctx.Done()
	return "", ctx.Err()
}

func TestPostAuditConversationalGeneratorBudgetBoundsBothPublicPaths(t *testing.T) {
	t.Setenv("AHMELIA_CONVERSATIONAL_GENERATION_BUDGET_MS", "40")
	client := &postAuditTimeoutConversationalClient{}
	generator := language.NewConversationalGenerator(client)
	req := interaction.SemanticRequest{Raw: "hola", Normalized: "hola", Kind: interaction.KindConversation, Confidence: 1}
	intent := interaction.ResponseIntent{Kind: interaction.ResponseIntentAcknowledge, ShouldSpeak: true, Confidence: 1}
	start := time.Now()
	_, err := generator.Generate(req, interaction.ConversationSnapshot{}, intent)
	elapsedGenerate := time.Since(start)
	if !errors.Is(err, language.ErrConversationalGenerationTimeout) {
		t.Fatalf("Generate() debe devolver el sentinel de timeout, obtuvo %v", err)
	}
	if elapsedGenerate > 500*time.Millisecond {
		t.Fatalf("Generate() superó ampliamente el presupuesto: %v", elapsedGenerate)
	}

	start = time.Now()
	_, err = generator.GenerateWithContext(context.Background(), req, interaction.ConversationSnapshot{}, intent,
		// strategy is intentionally empty; the generator must own the time budget.
		// Use the concrete zero-value without introducing another decision source.
		cognition.ConversationalResponseStrategy{}, socialinteraction.ResponsePlan{})
	elapsedContext := time.Since(start)
	if !errors.Is(err, language.ErrConversationalGenerationTimeout) {
		t.Fatalf("GenerateWithContext() debe devolver el sentinel de timeout, obtuvo %v", err)
	}
	if elapsedContext > 500*time.Millisecond {
		t.Fatalf("GenerateWithContext() superó ampliamente el presupuesto: %v", elapsedContext)
	}
	if client.calls != 2 {
		t.Fatalf("esperaba dos llamadas cancelables al cliente, obtuve %d", client.calls)
	}
}

func TestPostAuditConversationalTimeoutFallsBackAndLeavesTrace(t *testing.T) {
	t.Setenv("AHMELIA_CONVERSATIONAL_GENERATION_BUDGET_MS", "40")
	e := newB963334AuditEngine(t)
	e.flowTrace = NewTurnFlowTrace("post-audit-language-timeout")
	client := &postAuditTimeoutConversationalClient{}
	e.conversationGenerator = language.NewConversationalGenerator(client)

	req := interaction.SemanticRequest{
		Raw:        "esto es un caso no visto",
		Normalized: "esto es un caso no visto",
		Kind:       interaction.KindConversation,
		Confidence: 1,
	}
	intent := interaction.ResponseIntent{
		Kind:        interaction.ResponseIntentAcknowledge,
		ShouldSpeak: true,
		Confidence:  1,
	}
	contract := interaction.NewResponseContract(req, intent, interaction.MoveAcknowledge)

	start := time.Now()
	got, ok := e.generateResponseContractWithStrategy(req, contract, language.RuntimeFacts{}, e.lastConversationalCognition.Strategy)
	elapsed := time.Since(start)
	if !ok {
		t.Fatal("el timeout del generador no debe perder el turno: debe existir fallback")
	}
	if got != "Entendido." {
		t.Fatalf("se esperaba el fallback conversacional existente, obtuve %q", got)
	}
	if elapsed > 500*time.Millisecond {
		t.Fatalf("el turno siguió bloqueado tras el presupuesto: %v", elapsed)
	}
	trace := strings.Join(e.flowTrace.Snapshot(), "\n")
	if !strings.Contains(trace, "LANGUAGE DEGRADATION") {
		t.Fatalf("la degradación del generador no quedó observable en flowTrace: %s", trace)
	}
	if client.calls != 1 {
		t.Fatalf("un timeout inicial debe terminar el rendering sin reintento de guardrail: calls=%d", client.calls)
	}
}

type postAuditGuardrailTimeoutClient struct {
	calls int
}

func (c *postAuditGuardrailTimeoutClient) Available() bool { return true }

func (c *postAuditGuardrailTimeoutClient) GenerateContext(ctx context.Context, _ string) (string, error) {
	c.calls++
	if c.calls == 1 {
		return `{"response":"Voy a mejorar eso"}`, nil
	}
	<-ctx.Done()
	return "", ctx.Err()
}

func TestPostAuditConversationalBudgetAlsoBoundsGuardrailRetry(t *testing.T) {
	t.Setenv("AHMELIA_CONVERSATIONAL_GENERATION_BUDGET_MS", "40")
	client := &postAuditGuardrailTimeoutClient{}
	generator := language.NewConversationalGenerator(client)
	req := interaction.SemanticRequest{
		Raw: "eso no quedó bien",
		CommunicativeActs: interaction.CommunicativeActs{
			Primary: interaction.ActFeedback,
		},
	}

	start := time.Now()
	_, err := generator.GenerateGrounded(req, interaction.ConversationSnapshot{}, interaction.ResponseIntent{}, cognition.ConversationalResponseStrategy{}, socialinteraction.ResponsePlan{}, language.RuntimeFacts{}, nil)
	elapsed := time.Since(start)
	if !errors.Is(err, language.ErrConversationalGenerationTimeout) {
		t.Fatalf("el retry del guardrail debe respetar el mismo presupuesto: %v", err)
	}
	if elapsed > 500*time.Millisecond {
		t.Fatalf("el retry siguió bloqueado después del presupuesto: %v", elapsed)
	}
	if client.calls != 2 {
		t.Fatalf("esperaba llamada inicial + retry cancelable, obtuve %d", client.calls)
	}
}
