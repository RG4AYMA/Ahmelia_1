package linguistics

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestOllamaAvailableUsesShortHealthTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(750 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewOllamaClient()
	client.BaseURL = server.URL

	start := time.Now()
	available := client.Available()
	elapsed := time.Since(start)

	if available {
		t.Fatal("Available() debería devolver false cuando el health check supera su timeout")
	}
	if elapsed > time.Second {
		t.Fatalf("Available() tardó demasiado: %v", elapsed)
	}
}
