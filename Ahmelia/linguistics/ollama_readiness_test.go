package linguistics

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestOllamaWarmupIsSingleSharedModelLifecycle(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/generate" {
			http.NotFound(w, r)
			return
		}
		calls.Add(1)
		body, _ := io.ReadAll(r.Body)
		var req generateRequest
		if err := json.Unmarshal(body, &req); err != nil {
			t.Fatal(err)
		}
		if req.Model != "test-model" {
			t.Fatalf("model=%q", req.Model)
		}
		if req.KeepAlive == "" {
			t.Fatal("warmup/generation must keep the local model resident")
		}
		_, _ = w.Write([]byte(`{"response":"OK"}`))
	}))
	defer server.Close()

	client := NewOllamaClient()
	client.BaseURL = server.URL
	client.Model = "test-model"
	client.client = server.Client()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	client.Warmup(ctx)
	client.Warmup(ctx)
	if err := client.WaitReady(ctx); err != nil {
		t.Fatal(err)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("expected exactly one warmup request, got %d", got)
	}
}
