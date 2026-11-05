package linguistics

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestD35StructuredStreamingAccumulatesNDJSONAndReportsChunks(t *testing.T) {
	var generateCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/generate" {
			http.NotFound(w, r)
			return
		}
		var req generateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatal(err)
		}
		if !req.Stream {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"response":"OK"}`))
			return
		}
		generateCalls.Add(1)
		w.Header().Set("Content-Type", "application/x-ndjson")
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("test server has no flusher")
		}
		for _, piece := range []string{`{"files":[`, `{"path":"main.go"},`, `"content":"ok"}]}`} {
			line, _ := json.Marshal(map[string]any{"response": piece, "done": false})
			_, _ = w.Write(append(line, '\n'))
			flusher.Flush()
		}
		done, _ := json.Marshal(map[string]any{"response": "", "done": true})
		_, _ = w.Write(append(done, '\n'))
		flusher.Flush()
	}))
	defer server.Close()

	c := NewOllamaClient()
	c.BaseURL = server.URL
	c.Model = "test-model"
	var chunks []string
	got, err := c.GenerateStructuredStreamContext(context.Background(), "produce", json.RawMessage(`{"type":"object"}`), 32, func(s string) { chunks = append(chunks, s) })
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, `"files"`) || !strings.Contains(got, `main.go`) {
		t.Fatalf("unexpected accumulated response: %s", got)
	}
	if len(chunks) != 3 {
		t.Fatalf("expected 3 content chunks, got %d", len(chunks))
	}
	if c.State() != OllamaModelReady || c.LastEvent() != OllamaSuccess {
		t.Fatalf("unexpected telemetry state=%s event=%s", c.State(), c.LastEvent())
	}
	if generateCalls.Load() != 1 {
		t.Fatalf("expected one streaming generation, got %d", generateCalls.Load())
	}
}

func TestD35StreamingCancellationDoesNotPoisonClient(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/generate" {
			http.NotFound(w, r)
			return
		}
		var req generateRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		w.Header().Set("Content-Type", "application/json")
		if !req.Stream {
			_, _ = w.Write([]byte(`{"response":"OK"}`))
			return
		}
		flusher := w.(http.Flusher)
		_, _ = w.Write([]byte(`{"response":"partial","done":false}` + "\n"))
		flusher.Flush()
		<-r.Context().Done()
	}))
	defer server.Close()

	c := NewOllamaClient()
	c.BaseURL = server.URL
	c.Model = "test-model"
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	_, err := c.GenerateStructuredStreamContext(ctx, "produce", json.RawMessage(`{"type":"object"}`), 32, nil)
	if err == nil {
		t.Fatal("expected cancellation error")
	}
	if c.LastEvent() != OllamaRequestTimeout && c.LastEvent() != OllamaContextCancelled {
		t.Fatalf("expected cancellation telemetry, got %s", c.LastEvent())
	}
}

func TestD35InfrastructureRecoversAfterServerOutage(t *testing.T) {
	var unavailable atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if unavailable.Load() {
			http.Error(w, "offline", http.StatusServiceUnavailable)
			return
		}
		if r.URL.Path == "/api/generate" {
			var req generateRequest
			_ = json.NewDecoder(r.Body).Decode(&req)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"response":"OK"}`))
			return
		}
		if r.URL.Path == "/api/tags" {
			_, _ = w.Write([]byte(`{"models":[]}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	c := NewOllamaClient()
	c.BaseURL = server.URL
	c.Model = "test-model"
	unavailable.Store(true)
	if _, err := c.GenerateContext(context.Background(), "first"); err == nil {
		t.Fatal("expected outage")
	}
	if c.State() == OllamaModelReady {
		t.Fatal("client must leave READY after outage")
	}

	unavailable.Store(false)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	got, err := c.GenerateContext(ctx, "second")
	if err != nil {
		t.Fatal(err)
	}
	if got != "OK" {
		t.Fatalf("unexpected response %q", got)
	}
	if c.State() != OllamaModelReady || c.LastEvent() != OllamaSuccess {
		t.Fatalf("recovery did not converge: state=%s event=%s", c.State(), c.LastEvent())
	}
}
