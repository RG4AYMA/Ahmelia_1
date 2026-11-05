package linguistics

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"ahmelia/interaction"
)

type deferredTestClient struct {
	mu       sync.Mutex
	calls    int
	response string
	wait     time.Duration
}

func (c *deferredTestClient) Available() bool { return true }
func (c *deferredTestClient) Generate(prompt string) (string, error) {
	return c.GenerateContext(context.Background(), prompt)
}
func (c *deferredTestClient) GenerateContext(ctx context.Context, _ string) (string, error) {
	c.mu.Lock()
	c.calls++
	call := c.calls
	c.mu.Unlock()
	if call == 1 && c.wait > 0 {
		timer := time.NewTimer(c.wait)
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	return c.response, nil
}
func (c *deferredTestClient) Calls() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.calls
}

type blockingLanguageClient struct {
	started   chan struct{}
	cancelled chan struct{}
	onceStart sync.Once
	onceStop  sync.Once
}

func (c *blockingLanguageClient) Available() bool { return true }
func (c *blockingLanguageClient) Generate(prompt string) (string, error) {
	return c.GenerateContext(context.Background(), prompt)
}
func (c *blockingLanguageClient) GenerateContext(ctx context.Context, _ string) (string, error) {
	c.onceStart.Do(func() { close(c.started) })
	<-ctx.Done()
	c.onceStop.Do(func() { close(c.cancelled) })
	return "", ctx.Err()
}

func deferredInterpretationJSON(topic string) string {
	return `{"intent":"question","entities":[],"references":[],"constraints":[],"meaning":"Pregunta sobre ` + topic + `","speech_act":"question","topic":"` + topic + `","question_type":"general","directness":"question","user_state_signals":[],"correction_target":"","relations":[{"source":"user","predicate":"asks_about_attribute","target":"` + topic + `","value":"","confidence":0.95}],"confidence":0.95}`
}

func TestB963935DeferredResolverRunsOneWorkerAndStoresEvidence(t *testing.T) {
	client := &deferredTestClient{response: deferredInterpretationJSON("pasatiempo favorito")}
	resolver := NewDeferredResolver(NewInterpreter(client), 4)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	resolver.Start(ctx)
	defer resolver.Stop()

	req := interaction.Analyze("¿Tienes presente mi pasatiempo favorito?", interaction.Context{})
	jobID, err := resolver.Submit(req, interaction.Context{Language: "es"})
	if err != nil {
		t.Fatalf("Submit() error: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if result, ok := resolver.Get(jobID); ok {
			if !result.Ready || len(result.Interpretation.Relations) != 1 {
				t.Fatalf("unexpected deferred result: %+v", result)
			}
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatal("timeout esperando resolución diferida")
}

func TestB963935SemanticCacheGeneralizesAcrossSurfaceForms(t *testing.T) {
	client := &deferredTestClient{response: deferredInterpretationJSON("color favorito")}
	resolver := NewDeferredResolver(NewInterpreter(client), 4)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	resolver.Start(ctx)
	defer resolver.Stop()

	first := interaction.Analyze("¿Tienes presente mi color favorito?", interaction.Context{})
	jobID, err := resolver.Submit(first, interaction.Context{Language: "es"})
	if err != nil {
		t.Fatalf("Submit(first) error: %v", err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, ok := resolver.Get(jobID); ok {
			break
		}
		time.Sleep(2 * time.Millisecond)
	}
	if _, ok := resolver.Get(jobID); !ok {
		t.Fatal("first deferred result was not stored")
	}

	// Adversarial case invented for this block: same semantic target, different
	// interrogative construction, never used in the user-provided logs.
	second := interaction.Analyze("¿Recuerdas mi color favorito?", interaction.Context{})
	cached, ok := resolver.Lookup(second)
	if !ok {
		t.Fatalf("expected semantic cache hit; first=%q second=%q", SemanticSignatureForRequest(first), SemanticSignatureForRequest(second))
	}
	if cached.Job.ID != jobID {
		t.Fatalf("expected cache to resolve to job %s, got %s", jobID, cached.Job.ID)
	}
	if client.Calls() != 1 {
		t.Fatalf("cache lookup must not trigger another LLM call, got %d", client.Calls())
	}

	negative := interaction.Analyze("¿Tienes presente el clima de mañana?", interaction.Context{})
	if _, ok := resolver.Lookup(negative); ok {
		t.Fatalf("negative semantic control unexpectedly hit cache: %q", SemanticSignatureForRequest(negative))
	}
}

func TestB963935TimeoutCanBeAbandonedAndDeferredResolutionCompletes(t *testing.T) {
	client := &deferredTestClient{wait: 300 * time.Millisecond, response: deferredInterpretationJSON("proyecto actual")}
	resolver := NewDeferredResolver(NewInterpreter(client), 4)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	resolver.Start(ctx)
	defer resolver.Stop()

	raw := "¿Qué relación tiene esto con el proyecto actual?"
	req := interaction.Analyze(raw, interaction.Context{})
	liveCtx, liveCancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	_, err := NewInterpreter(client).InterpretContext(liveCtx, raw)
	liveCancel()
	if err == nil || !strings.Contains(err.Error(), "deadline") {
		t.Fatalf("expected bounded live interpretation timeout, got %v", err)
	}

	jobID, err := resolver.Submit(req, interaction.Context{Language: "es"})
	if err != nil {
		t.Fatalf("deferred Submit() error: %v", err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if result, ok := resolver.Get(jobID); ok {
			if len(result.Interpretation.Relations) != 1 {
				t.Fatalf("deferred interpretation missing relation: %+v", result.Interpretation)
			}
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatal("timeout waiting for deferred result")
}

func TestB963935DeferredResolverBoundsBackgroundInference(t *testing.T) {
	client := &deferredTestClient{wait: 500 * time.Millisecond, response: deferredInterpretationJSON("lento")}
	resolver := NewDeferredResolverWithTimeout(NewInterpreter(client), 2, 30*time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	completed := make(chan DeferredResult, 1)
	resolver.SetObserver(func(result DeferredResult) { completed <- result })
	resolver.Start(ctx)
	defer resolver.Stop()

	req := interaction.Analyze("¿Qué sabes del tema?", interaction.Context{})
	jobID, err := resolver.Submit(req, interaction.Context{Language: "es"})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case result := <-completed:
		if result.Job.ID != jobID {
			t.Fatalf("completed job=%s want %s", result.Job.ID, jobID)
		}
		if result.Error == "" {
			t.Fatal("timed-out deferred inference unexpectedly succeeded")
		}
		if !strings.Contains(strings.ToLower(result.Error), "deadline") && !strings.Contains(strings.ToLower(result.Error), "canceled") {
			t.Fatalf("unexpected timeout error: %q", result.Error)
		}
	case <-time.After(300 * time.Millisecond):
		t.Fatal("deferred inference did not terminate within its bounded timeout")
	}
}

func TestB963935CancelOptionalInferencePreemptsActiveJobWithoutStoppingResolver(t *testing.T) {
	client := &blockingLanguageClient{started: make(chan struct{}), cancelled: make(chan struct{})}
	resolver := NewDeferredResolverWithTimeout(NewInterpreter(client), 4, 10*time.Second)
	resolver.Start(context.Background())
	defer resolver.Stop()

	req := interaction.Analyze("¿Podrías ampliar esta explicación?", interaction.Context{Language: "es"})
	if _, err := resolver.Submit(req, interaction.Context{Language: "es"}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-client.started:
	case <-time.After(time.Second):
		t.Fatal("deferred inference did not start")
	}

	resolver.CancelOptionalInference()
	select {
	case <-client.cancelled:
	case <-time.After(time.Second):
		t.Fatal("active deferred inference was not cancelled")
	}

	if _, err := resolver.Submit(req, interaction.Context{Language: "es"}); err != nil {
		t.Fatalf("resolver stopped accepting future jobs after optional cancellation: %v", err)
	}
}
