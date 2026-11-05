package resource

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Prober define el contrato para medir una configuración de modelo.
type Prober interface {
	Measure(ctx context.Context, config ModelConfig, prompt string) (Measurement, error)
}

// OllamaProber implementa Prober contra la API HTTP de Ollama.
type OllamaProber struct {
	BaseURL      string
	VRAMBudgetMB float64
	HTTPClient   *http.Client
}

// NewOllamaProber crea un prober con valores por defecto sensatos.
func NewOllamaProber() *OllamaProber {
	baseURL := strings.TrimSpace(getEnv("AHMELIA_OLLAMA_URL", "http://127.0.0.1:11434"))
	vramBudget := parseFloat(getEnv("AHMELIA_VRAM_BUDGET_MB", "1800"))
	return &OllamaProber{
		BaseURL:      strings.TrimRight(baseURL, "/"),
		VRAMBudgetMB: vramBudget,
		HTTPClient:   &http.Client{Timeout: 5 * time.Minute},
	}
}

func getEnv(key, def string) string {
	if v := strings.TrimSpace(osGetenv(key)); v != "" {
		return v
	}
	return def
}

func parseFloat(s string) float64 {
	var f float64
	fmt.Sscanf(s, "%f", &f)
	return f
}

// osGetenv es un wrapper para permitir tests que lo reemplacen.
var osGetenv = func(key string) string {
	return ""
}

// Measure ejecuta una única medición contra Ollama.
func (p *OllamaProber) Measure(ctx context.Context, config ModelConfig, prompt string) (Measurement, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	config = config.Normalize()

	// Construir petición
	reqBody := GenerateRequest{
		Model:  config.Model,
		Prompt: prompt,
		Stream: false,
		Options: GenerateOptions{
			Temperature: config.Temperature,
			NumPredict:  256,
		},
		KeepAlive: getEnv("AHMELIA_OLLAMA_KEEP_ALIVE", "10m"),
	}
	if config.NumThread > 0 {
		reqBody.Options.NumThread = config.NumThread
	}
	// num_gpu se envía siempre, incluido 0: GPULayers=0 significa "solo CPU"
	// y num_gpu:0 fuerza a Ollama a no hacer offload, en vez de omitir el
	// parámetro y dejar que Ollama autodecida.
	reqBody.Options.NumGPU = config.GPULayers

	data, err := json.Marshal(reqBody)
	if err != nil {
		return Measurement{}, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.BaseURL+"/api/generate", strings.NewReader(string(data)))
	if err != nil {
		return Measurement{}, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	start := time.Now()
	resp, err := p.HTTPClient.Do(req)
	elapsed := time.Since(start)
	if err != nil {
		if ctx.Err() == context.Canceled {
			return Measurement{}, ctx.Err()
		}
		if ctx.Err() == context.DeadlineExceeded {
			return Measurement{}, ctx.Err()
		}
		return Measurement{}, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return Measurement{}, fmt.Errorf("read body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Measurement{}, fmt.Errorf("ollama HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var genResp GenerateResponse
	if err := json.Unmarshal(body, &genResp); err != nil {
		return Measurement{}, fmt.Errorf("unmarshal response: %w", err)
	}

	// Obtener info de modelos cargados para RAM/VRAM
	ramMB, vramMB, err := p.fetchModelMemory(ctx, config.Model)
	if err != nil {
		// No fallar la medición por esto, solo dejar 0
		ramMB, vramMB = 0, 0
	}

	// Tokens por segundo: Ollama no devuelve token count directo en /api/generate
	// Aproximamos: longitud de respuesta en tokens / tiempo de evaluación
	// Nota: esta es una aproximación; la medición real usa /api/generate con stream=true
	// para contar tokens, pero la spec pide usar la respuesta completa.
	tokenCount := estimateTokens(genResp.Response)
	tokensPerSec := 0.0
	if elapsed.Seconds() > 0 {
		tokensPerSec = float64(tokenCount) / elapsed.Seconds()
	}

	// Primera respuesta = tiempo total (carga + evaluación) en modo no-stream
	firstResponseMs := elapsed.Milliseconds()

	overBudget := p.VRAMBudgetMB > 0 && vramMB > p.VRAMBudgetMB

	return Measurement{
		TokensPerSecond: tokensPerSec,
		FirstResponseMs: float64(firstResponseMs),
		RAMMB:           ramMB,
		VRAMMB:          vramMB,
		Repetition:      1, // el caller incrementa
		OverBudget:      overBudget,
		Timestamp:       time.Now().UTC().Format(time.RFC3339),
	}, nil
}

// fetchModelMemory consulta /api/ps para obtener memoria del modelo cargado.
func (p *OllamaProber) fetchModelMemory(ctx context.Context, model string) (float64, float64, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.BaseURL+"/api/ps", nil)
	if err != nil {
		return 0, 0, err
	}
	resp, err := p.HTTPClient.Do(req)
	if err != nil {
		return 0, 0, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, 0, err
	}

	var psResp PSResponse
	if err := json.Unmarshal(body, &psResp); err != nil {
		return 0, 0, err
	}

	for _, m := range psResp.Models {
		if strings.HasPrefix(m.Name, model) || strings.HasPrefix(model, m.Name) {
			// size_vram y size son bytes
			vramMB := float64(m.SizeVRAM) / (1024 * 1024)
			ramMB := float64(m.Size) / (1024 * 1024)
			return ramMB, vramMB, nil
		}
	}
	return 0, 0, fmt.Errorf("model %s not found in /api/ps", model)
}

type GenerateRequest struct {
	Model     string          `json:"model"`
	Prompt    string          `json:"prompt"`
	Stream    bool            `json:"stream"`
	Format    json.RawMessage `json:"format,omitempty"`
	Options   GenerateOptions `json:"options"`
	KeepAlive string          `json:"keep_alive,omitempty"`
}

type GenerateOptions struct {
	Temperature float64 `json:"temperature"`
	NumPredict  int     `json:"num_predict"`
	NumThread   int     `json:"num_thread,omitempty"`
	NumGPU      int     `json:"num_gpu"` // siempre presente: 0 = CPU forzado
}

type GenerateResponse struct {
	Response string `json:"response"`
	Done     bool   `json:"done"`
	// Campos adicionales que Ollama puede devolver
	EvalCount    int   `json:"eval_count"`
	EvalDuration int64 `json:"eval_duration"` // nanosegundos
}

// PSResponse para /api/ps
type PSResponse struct {
	Models []PSModel `json:"models"`
}

type PSModel struct {
	Name     string    `json:"name"`
	Size     int64     `json:"size"`      // RAM bytes
	SizeVRAM int64     `json:"size_vram"` // VRAM bytes
	Digest   string    `json:"digest"`
	Details  PSDetails `json:"details"`
}

type PSDetails struct {
	Format            string   `json:"format"`
	Family            string   `json:"family"`
	Families          []string `json:"families"`
	ParameterSize     string   `json:"parameter_size"`
	QuantizationLevel string   `json:"quantization_level"`
}

// estimateTokens aproxima tokens (Ollama usa ~4 chars/token para español)
func estimateTokens(text string) int {
	if text == "" {
		return 0
	}
	// Aproximación grosera: 4 caracteres por token
	return len(text) / 4
}

// SetOSGetenv permite tests reemplazar la lectura de variables de entorno.
func SetOSGetenv(f func(string) string) {
	osGetenv = f
}
