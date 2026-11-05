package linguistics

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"ahmelia/interaction"
)

// awaitStubClient sirve la puerta abierta con retardo controlado, sin red.
type awaitStubClient struct {
	delay time.Duration
	fail  bool
	raw   string
}

func (c *awaitStubClient) GenerateStructuredContext(ctx context.Context, prompt string, schema json.RawMessage, numPredict int) (string, error) {
	if c.delay > 0 {
		select {
		case <-time.After(c.delay):
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	if c.fail {
		return "", errors.New("modelo no disponible")
	}
	return c.raw, nil
}
func (c *awaitStubClient) Generate(string) (string, error) { return c.raw, nil }
func (c *awaitStubClient) Available() bool                 { return true }

func newAwaitResolver(t *testing.T, client *awaitStubClient) *DeferredResolver {
	t.Helper()
	resolver := NewDeferredResolver(NewInterpreter(client), 4)
	resolver.SetOpenComprehension(true)
	resolver.Start(context.Background())
	t.Cleanup(resolver.Stop)
	return resolver
}

func submitAwaitProbe(t *testing.T, resolver *DeferredResolver) string {
	t.Helper()
	req := interaction.SemanticRequest{
		Raw:        "la verdad es que hiciste un trabajo enorme hoy",
		Normalized: "la verdad es que hiciste un trabajo enorme hoy",
		Kind:       interaction.KindUnknown,
	}
	jobID, err := resolver.Submit(req, interaction.Context{Language: "es"})
	if err != nil {
		t.Fatalf("Submit falló: %v", err)
	}
	return jobID
}

func TestAwaitJobReturnsCompletedOpenReading(t *testing.T) {
	client := &awaitStubClient{
		delay: 250 * time.Millisecond,
		raw:   `{"purpose":"feedback","owner_of_data":"user","topic":"el esfuerzo","meaning":"Elogio sobre el trabajo","mentions":[],"confidence":0.9}`,
	}
	resolver := newAwaitResolver(t, client)
	jobID := submitAwaitProbe(t, resolver)

	waitCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	result, ok := resolver.AwaitJob(waitCtx, jobID)
	if !ok {
		t.Fatal("AwaitJob no entregó el resultado completado")
	}
	if result.Open == nil || result.Open.Purpose != OpenPurposeFeedback {
		t.Fatalf("lectura abierta incompleta: %+v", result.Open)
	}
}

func TestAwaitJobHonoursBudget(t *testing.T) {
	client := &awaitStubClient{
		delay: 3 * time.Second,
		raw:   `{"purpose":"feedback","owner_of_data":"user","topic":"x","meaning":"y","mentions":[],"confidence":0.9}`,
	}
	resolver := newAwaitResolver(t, client)
	jobID := submitAwaitProbe(t, resolver)

	started := time.Now()
	waitCtx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	if _, ok := resolver.AwaitJob(waitCtx, jobID); ok {
		t.Fatal("AwaitJob no debe entregar un resultado que aún no existe")
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("AwaitJob ignoró su presupuesto: %v", elapsed)
	}
}

func TestAwaitJobDetectsFailedReading(t *testing.T) {
	client := &awaitStubClient{delay: 50 * time.Millisecond, fail: true}
	resolver := newAwaitResolver(t, client)
	jobID := submitAwaitProbe(t, resolver)

	started := time.Now()
	waitCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if _, ok := resolver.AwaitJob(waitCtx, jobID); ok {
		t.Fatal("una lectura fallida no debe entregarse como resultado")
	}
	if elapsed := time.Since(started); elapsed > 2*time.Second {
		t.Fatalf("la detección de fallo demoró demasiado: %v", elapsed)
	}
}
