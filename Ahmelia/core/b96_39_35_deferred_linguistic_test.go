package core

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"ahmelia/interaction"
	"ahmelia/linguistics"
)

type b963935SlowLanguageClient struct {
	mu       sync.Mutex
	calls    int
	response string
}

func (c *b963935SlowLanguageClient) Available() bool { return true }
func (c *b963935SlowLanguageClient) Generate(prompt string) (string, error) {
	return c.GenerateContext(context.Background(), prompt)
}
func (c *b963935SlowLanguageClient) GenerateContext(ctx context.Context, _ string) (string, error) {
	c.mu.Lock()
	c.calls++
	c.mu.Unlock()
	if ctx == nil {
		ctx = context.Background()
	}
	if deadline, ok := ctx.Deadline(); ok && time.Until(deadline) < 500*time.Millisecond {
		<-ctx.Done()
		return "", ctx.Err()
	}
	time.Sleep(10 * time.Millisecond)
	return c.response, nil
}
func (c *b963935SlowLanguageClient) Calls() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.calls
}

func b963935QuestionJSON() string {
	return `{"intent":"question","entities":[],"references":[],"constraints":[],"meaning":"Pregunta sobre una capacidad futura","speech_act":"question","topic":"capacidad futura","question_type":"general","directness":"question","user_state_signals":[],"correction_target":"","relations":[{"source":"user","predicate":"asks_about","target":"capacidad futura","value":"","confidence":0.95}],"confidence":0.95}`
}

func TestB963935LiveBudgetFallsBackToDeferredWithoutBlocking(t *testing.T) {
	t.Setenv("AHMELIA_LINGUISTIC_SYNC_BUDGET_MS", "50")
	client := &b963935SlowLanguageClient{response: b963935QuestionJSON()}
	interpreter := linguistics.NewInterpreter(client)
	resolver := linguistics.NewDeferredResolver(interpreter, 4)
	resolver.Start(context.Background())
	defer resolver.Stop()

	e := &Engine{
		interactionManager:    interaction.NewManager(),
		linguisticInterpreter: interpreter,
		linguisticPipeline:    linguistics.NewPipeline(linguistics.NewRouter(), interpreter, linguistics.NewValidator()),
		deferredLinguistic:    resolver,
	}
	request := "¿Podrías interpretar esta situación de manera diferente?"
	e.flowTrace = NewTurnFlowTrace(request)

	started := time.Now()
	_ = e.analyzeSemanticBoundary(request, interaction.Context{Language: "es"}, false, nil)
	elapsed := time.Since(started)
	if elapsed > 500*time.Millisecond {
		t.Fatalf("live semantic boundary exceeded bounded budget: %v", elapsed)
	}

	lines := e.flowTrace.Snapshot()
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "deferred=true job=") {
		t.Fatalf("expected timeout to enqueue deferred work; trace=%s", joined)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if got := resolver.Ready(); len(got) == 1 {
			if got[0].Interpretation.Confidence <= 0 {
				t.Fatalf("deferred result lost interpretation confidence: %+v", got[0])
			}
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatal("timeout esperando el resultado diferido")
}

func TestB963935DeferredEvidenceUsesExistingInitiativePath(t *testing.T) {
	// The initiative package test proves the adapter contracts. This core-level
	// test verifies that Engine can consume a completed deferred result and map
	// it back through its original SemanticRequest without a response callback.
	client := &b963935SlowLanguageClient{response: b963935QuestionJSON()}
	interpreter := linguistics.NewInterpreter(client)
	resolver := linguistics.NewDeferredResolver(interpreter, 2)
	resolver.Start(context.Background())
	defer resolver.Stop()

	req := interaction.Analyze("¿Tienes presente mi capacidad futura?", interaction.Context{Language: "es"})
	jobID, err := resolver.Submit(req, interaction.Context{Language: "es"})
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if result, ok := resolver.Get(jobID); ok {
			enriched := mergeLinguisticInterpretation(result.Job.Request, result.Interpretation)
			if len(enriched.Relations) == 0 {
				t.Fatalf("deferred evidence did not re-enter semantic contract: %+v", enriched)
			}
			if enriched.Raw != result.Job.Request.Raw {
				t.Fatalf("deferred resolution changed the original turn identity")
			}
			if resolver.Ready()[0].Delivered {
				t.Fatal("worker must not mark its own result as delivered")
			}
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatal("timeout esperando resultado diferido")
}
