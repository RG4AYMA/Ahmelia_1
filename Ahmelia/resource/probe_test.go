package resource

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestOllamaProber_Measure_Success(t *testing.T) {
	// Servidor falso que simula Ollama
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/generate":
			w.Header().Set("Content-Type", "application/json")
			// Respuesta con eval_count y eval_duration para cálculo real
			resp := GenerateResponse{
				Response:     "Esta es una respuesta de prueba del modelo.",
				Done:         true,
				EvalCount:    50,
				EvalDuration: 2_000_000_000, // 2 segundos en nanosegundos
			}
			json.NewEncoder(w).Encode(resp)
		case "/api/ps":
			w.Header().Set("Content-Type", "application/json")
			// Modelo cargado con VRAM y RAM
			resp := PSResponse{
				Models: []PSModel{
					{
						Name:     "gemma3:4b",
						Size:     4 * 1024 * 1024 * 1024, // 4 GB RAM
						SizeVRAM: 2 * 1024 * 1024 * 1024, // 2 GB VRAM
						Digest:   "abc123",
						Details:  PSDetails{ParameterSize: "4B"},
					},
				},
			}
			json.NewEncoder(w).Encode(resp)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	// Configurar prober con URL del servidor falso
	prober := &OllamaProber{
		BaseURL:      server.URL,
		VRAMBudgetMB: 1800,
		HTTPClient:   &http.Client{Timeout: 10 * time.Second},
	}

	ctx := context.Background()
	config := ModelConfig{Model: "gemma3:4b", GPULayers: 0, NumThread: 4}.Normalize()
	prompt := "Hola, ¿cómo estás?"

	m, err := prober.Measure(ctx, config, prompt)
	if err != nil {
		t.Fatalf("Measure error: %v", err)
	}

	// Verificaciones
	if m.TokensPerSecond <= 0 {
		t.Fatalf("TokensPerSecond should be > 0, got %v", m.TokensPerSecond)
	}
	if m.FirstResponseMs <= 0 {
		t.Fatalf("FirstResponseMs should be > 0, got %v", m.FirstResponseMs)
	}
	if m.RAMMB != 4096 {
		t.Fatalf("RAMMB = %v, want 4096", m.RAMMB)
	}
	if m.VRAMMB != 2048 {
		t.Fatalf("VRAMMB = %v, want 2048", m.VRAMMB)
	}
	if !m.OverBudget {
		t.Fatal("OverBudget should be true (2048 MB > 1800 MB budget)")
	}
	if m.Repetition != 1 {
		t.Fatalf("Repetition = %d, want 1", m.Repetition)
	}
	if m.Error != "" {
		t.Fatalf("Error should be empty, got %q", m.Error)
	}
	if m.Timestamp == "" {
		t.Fatal("Timestamp should be set")
	}
}

func TestOllamaProber_Measure_OverBudget(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/generate":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(GenerateResponse{
				Response:  "Respuesta corta",
				Done:      true,
				EvalCount: 10,
			})
		case "/api/ps":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(PSResponse{
				Models: []PSModel{
					{Name: "gemma3:4b", Size: 4 * 1024 * 1024 * 1024, SizeVRAM: 3 * 1024 * 1024 * 1024}, // 3 GB VRAM
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	prober := &OllamaProber{
		BaseURL:      server.URL,
		VRAMBudgetMB: 1800, // 3 GB = 3072 MB > 1800
		HTTPClient:   &http.Client{Timeout: 10 * time.Second},
	}

	m, err := prober.Measure(context.Background(), ModelConfig{Model: "gemma3:4b"}.Normalize(), "test")
	if err != nil {
		t.Fatalf("Measure error: %v", err)
	}

	if !m.OverBudget {
		t.Fatal("OverBudget should be true (VRAM 3072 > budget 1800)")
	}
	if m.VRAMMB != 3072 {
		t.Fatalf("VRAMMB = %v, want 3072", m.VRAMMB)
	}
}

func TestOllamaProber_Measure_ContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simular latencia
		time.Sleep(100 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(GenerateResponse{Response: "slow", Done: true})
	}))
	defer server.Close()

	prober := &OllamaProber{
		BaseURL:    server.URL,
		HTTPClient: &http.Client{Timeout: 10 * time.Second},
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancelar inmediatamente

	_, err := prober.Measure(ctx, ModelConfig{Model: "test"}.Normalize(), "test")
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}
	if err != context.Canceled {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestOllamaProber_Measure_ContextTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond) // Más lento que el timeout
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(GenerateResponse{Response: "slow", Done: true})
	}))
	defer server.Close()

	prober := &OllamaProber{
		BaseURL:    server.URL,
		HTTPClient: &http.Client{Timeout: 10 * time.Second},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := prober.Measure(ctx, ModelConfig{Model: "test"}.Normalize(), "test")
	if err == nil {
		t.Fatal("expected error from deadline exceeded")
	}
	if err != context.DeadlineExceeded {
		t.Fatalf("expected context.DeadlineExceeded, got %v", err)
	}
}

func TestOllamaProber_Measure_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}))
	defer server.Close()

	prober := &OllamaProber{
		BaseURL:    server.URL,
		HTTPClient: &http.Client{Timeout: 10 * time.Second},
	}

	_, err := prober.Measure(context.Background(), ModelConfig{Model: "test"}.Normalize(), "test")
	if err == nil {
		t.Fatal("expected error from HTTP 500")
	}
	if !strings.Contains(err.Error(), "HTTP 500") {
		t.Fatalf("error should mention HTTP 500: %v", err)
	}
}

func TestOllamaProber_Measure_ModelNotInPS(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/generate":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(GenerateResponse{Response: "ok", Done: true})
		case "/api/ps":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(PSResponse{Models: []PSModel{}}) // Lista vacía
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	prober := &OllamaProber{
		BaseURL:    server.URL,
		HTTPClient: &http.Client{Timeout: 10 * time.Second},
	}

	m, err := prober.Measure(context.Background(), ModelConfig{Model: "unknown-model"}.Normalize(), "test")
	if err != nil {
		t.Fatalf("Measure should not fail if /api/ps doesn't have model: %v", err)
	}
	// RAM/VRAM deben ser 0 cuando no se encuentra el modelo
	if m.RAMMB != 0 || m.VRAMMB != 0 {
		t.Fatalf("RAM/VRAM should be 0 when model not in /api/ps: ram=%v vram=%v", m.RAMMB, m.VRAMMB)
	}
}

func TestOllamaProber_Measure_GPUConfig(t *testing.T) {
	var captured GenerateRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verificar que la petición incluye num_gpu y num_thread
		json.NewDecoder(r.Body).Decode(&captured)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(GenerateResponse{
			Response:     "ok",
			Done:         true,
			EvalCount:    10,
			EvalDuration: 1_000_000_000,
		})
	}))
	defer server.Close()

	prober := &OllamaProber{
		BaseURL:    server.URL,
		HTTPClient: &http.Client{Timeout: 10 * time.Second},
	}

	config := ModelConfig{Model: "gemma3:4b", GPULayers: 20, NumThread: 2}.Normalize()
	m, err := prober.Measure(context.Background(), config, "test")
	if err != nil {
		t.Fatalf("Measure error: %v", err)
	}

	if m.Error != "" {
		t.Fatalf("unexpected error in measurement: %s", m.Error)
	}
	if captured.Options.NumGPU != 20 {
		t.Fatalf("options.num_gpu = %d, want 20", captured.Options.NumGPU)
	}
	if captured.Options.NumThread != 2 {
		t.Fatalf("options.num_thread = %d, want 2", captured.Options.NumThread)
	}
}

func TestOllamaProber_Measure_CPUConfig_SendsNumGPUZero(t *testing.T) {
	// Con GPULayers=0 la petición debe forzar CPU con num_gpu:0 explícito
	// (no omitir el parámetro y dejar que Ollama autodecida el offload).
	var rawBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path != "/api/generate" {
			// /api/ps u otros sondeos: responder vacío (sin capturar body)
			_, _ = w.Write([]byte(`{"models":[]}`))
			return
		}
		body, _ := io.ReadAll(r.Body)
		rawBody = string(body)

		json.NewEncoder(w).Encode(GenerateResponse{
			Response:     "ok",
			Done:         true,
			EvalCount:    10,
			EvalDuration: 1_000_000_000,
		})
	}))
	defer server.Close()

	prober := &OllamaProber{
		BaseURL:    server.URL,
		HTTPClient: &http.Client{Timeout: 10 * time.Second},
	}

	config := ModelConfig{Model: "gemma3:4b", GPULayers: 0, NumThread: 4}.Normalize()
	m, err := prober.Measure(context.Background(), config, "test")
	if err != nil {
		t.Fatalf("Measure error: %v", err)
	}
	if m.Error != "" {
		t.Fatalf("unexpected error in measurement: %s", m.Error)
	}
	if !strings.Contains(rawBody, `"num_gpu":0`) {
		t.Fatalf("la petición CPU-only debe incluir num_gpu:0 explícito; body=%s", rawBody)
	}
}

func TestNewOllamaProber_Defaults(t *testing.T) {
	// Test con variables de entorno por defecto
	oldGetenv := osGetenv
	defer func() { osGetenv = oldGetenv }()

	osGetenv = func(key string) string {
		return ""
	}

	prober := NewOllamaProber()
	if prober.BaseURL != "http://127.0.0.1:11434" {
		t.Fatalf("BaseURL = %q, want default", prober.BaseURL)
	}
	if prober.VRAMBudgetMB != 1800 {
		t.Fatalf("VRAMBudgetMB = %v, want 1800", prober.VRAMBudgetMB)
	}
}

func TestOllamaProber_Measure_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("not valid json"))
	}))
	defer server.Close()

	prober := &OllamaProber{
		BaseURL:    server.URL,
		HTTPClient: &http.Client{Timeout: 10 * time.Second},
	}

	_, err := prober.Measure(context.Background(), ModelConfig{Model: "test"}.Normalize(), "test")
	if err == nil {
		t.Fatal("expected error from invalid JSON")
	}
	if !strings.Contains(err.Error(), "unmarshal") && !strings.Contains(err.Error(), "marshal") {
		t.Fatalf("error should mention unmarshal/marshal: %v", err)
	}
}

func TestEstimateTokens(t *testing.T) {
	cases := []struct {
		input    string
		expected int
	}{
		{"", 0},
		{"hola", 1},       // 4 chars = 1 token
		{"hola mundo", 2}, // 10 chars = 2 tokens
		{"a", 0},          // < 4 chars = 0
		{"abcd", 1},       // exactly 4 = 1
		{"abcde", 1},      // 5 chars = 1
	}
	for _, tc := range cases {
		got := estimateTokens(tc.input)
		if got != tc.expected {
			t.Fatalf("estimateTokens(%q) = %d, want %d", tc.input, got, tc.expected)
		}
	}
}
