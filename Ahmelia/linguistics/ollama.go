package linguistics

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptrace"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// OllamaClient se encarga únicamente de la comunicación técnica
// entre Ahmelia y el servidor local de Ollama.
//
// No contiene lógica cognitiva.
// No interpreta decisiones.
// No ejecuta acciones.
//
// Su responsabilidad es:
//
//	Prompt -> Ollama -> Respuesta.
type StructuredPromptObserver func(prompt string, schema json.RawMessage, numPredict int)

// GenerationTraceObserver is an opt-in diagnostic seam for observing the real
// Ollama HTTP boundary. Timings are measured from request preparation and do
// not affect the request, deadline, or response handling.
type GenerationTrace struct {
	Endpoint         string
	Model            string
	Prompt           string
	PromptBytes      int
	Stream           bool
	StartedAt        time.Time
	ConnectStartedAt time.Time
	ConnectedAt      time.Time
	GotConnAt        time.Time
	ConnReused       bool
	RequestWrittenAt time.Time
	FirstResponseAt  time.Time
	HeadersAt        time.Time
	FirstBodyAt      time.Time
	FirstContentAt   time.Time
	CompletedAt      time.Time
	ResponseBytes    int64
	Schema           string
	NumPredict       int
	Error            string
}

type GenerationTraceObserver func(GenerationTrace)

// Gate es la interfaz opcional para controlar el acceso al modelo.
// Si es nil, el comportamiento es el actual (sin control).
// Si no es nil, cada llamada al modelo pasa por Enter/Exit.
type Gate interface {
	// Enter solicita acceso al modelo para una clase de tarea y prioridad.
	// Devuelve un contexto que se cancela cuando se debe liberar, y una función Exit idempotente.
	// Si el contexto de entrada se cancela antes de conceder, devuelve error.
	Enter(ctx context.Context, class string, priority int) (context.Context, func(), error)
}

// ModelClass identifica la clase de tarea para el gate.
type ModelClass string

const (
	ModelClassInterpretation ModelClass = "interpretation"
	ModelClassGeneration     ModelClass = "generation"
	ModelClassProposal       ModelClass = "proposal"
)

// GatePriority define prioridades numéricas (menor = mayor prioridad).
type GatePriority int

const (
	GatePriorityOwn      GatePriority = 0
	GatePriorityLearning GatePriority = 1
	GatePriorityUserGoal GatePriority = 2
	GatePriorityUrgent   GatePriority = 3
	GatePriorityUserTurn GatePriority = 4
)

type OllamaClient struct {
	BaseURL string
	Model   string
	client  *http.Client

	readyMu        sync.Mutex
	ready          bool
	warming        bool
	readyCh        chan struct{}
	readyErr       error
	stateMu        sync.RWMutex
	state          string
	lastEvent      string
	promptMu       sync.RWMutex
	promptObserver StructuredPromptObserver
	traceObserver  GenerationTraceObserver

	// Gate opcional para control de acceso al modelo.
	// Si es nil, no hay control de acceso.
	Gate Gate
}

// NewOllamaClient crea un cliente configurado para usar Ollama local
// y el modelo qwen3:4b-instruct-2507-q4_K_M, salvo que AHMELIA_OLLAMA_MODEL
// lo sobrescriba.
func NewOllamaClient() *OllamaClient {
	baseURL := strings.TrimSpace(os.Getenv("AHMELIA_OLLAMA_URL"))
	if baseURL == "" {
		// Ollama commonly binds IPv4 locally. Using 127.0.0.1 avoids a
		// localhost -> ::1 mismatch on hosts where Ollama is not listening
		// on IPv6. The URL remains configurable through the environment.
		baseURL = "http://127.0.0.1:11434"
	}
	model := strings.TrimSpace(os.Getenv("AHMELIA_OLLAMA_MODEL"))
	if model == "" {
		model = "qwen3:4b-instruct-2507-q4_K_M"
	}
	return &OllamaClient{
		BaseURL: baseURL,
		Model:   model,
		client: &http.Client{
			Timeout: ollamaHTTPTimeout(),
		},
		state: "MODEL_UNAVAILABLE",
	}
}

// ModelName reports the configured local model for provenance traces.
func (c *OllamaClient) ModelName() string {
	if c == nil {
		return ""
	}
	return strings.TrimSpace(c.Model)
}

// GenerationSource identifies the local model service in production and
// experience records.
func (c *OllamaClient) GenerationSource() string {
	if c == nil || c.ModelName() == "" {
		return "ollama"
	}
	return "ollama:" + c.ModelName()
}

const (
	OllamaServerUnreachable = "SERVER_UNREACHABLE"
	OllamaModelWarming      = "MODEL_WARMING"
	OllamaModelReady        = "MODEL_READY"
	OllamaModelUnavailable  = "MODEL_UNAVAILABLE"
	OllamaUnknown           = "UNKNOWN"

	OllamaRequestTimeout   = "REQUEST_TIMEOUT"
	OllamaContextCancelled = "CONTEXT_CANCELLED"
	OllamaHTTPError        = "HTTP_ERROR"
	OllamaInvalidResponse  = "INVALID_RESPONSE"
	OllamaSuccess          = "SUCCESS"
)

func (c *OllamaClient) setState(state, event string) {
	if c == nil {
		return
	}
	c.stateMu.Lock()
	c.state, c.lastEvent = state, event
	c.stateMu.Unlock()
}

// State returns the current dynamic infrastructure state. It is deliberately
// not sticky: request failures move the client out of READY and later calls
// are allowed to recover it.
func (c *OllamaClient) State() string {
	if c == nil {
		return OllamaUnknown
	}
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()
	if c.state == "" {
		return OllamaUnknown
	}
	return c.state
}

// SetStructuredPromptObserver installs an optional test/diagnostic seam for
// observing the exact structured-generation prompt sent through this client.
// It has no effect when unset and does not alter the request or response.
func (c *OllamaClient) SetStructuredPromptObserver(observer StructuredPromptObserver) {
	if c == nil {
		return
	}
	c.promptMu.Lock()
	c.promptObserver = observer
	c.promptMu.Unlock()
}

// SetGenerationTraceObserver installs an optional observer for real Ollama
// requests. It is silent when unset and never changes generation behavior.
func (c *OllamaClient) SetGenerationTraceObserver(observer GenerationTraceObserver) {
	if c == nil {
		return
	}
	c.promptMu.Lock()
	c.traceObserver = observer
	c.promptMu.Unlock()
}

func (c *OllamaClient) observeGenerationTrace(trace GenerationTrace) {
	c.promptMu.RLock()
	observer := c.traceObserver
	c.promptMu.RUnlock()
	if observer != nil {
		observer(trace)
	}
}

func (c *OllamaClient) observeStructuredPrompt(prompt string, schema json.RawMessage, numPredict int) {
	if c == nil {
		return
	}
	c.promptMu.RLock()
	observer := c.promptObserver
	c.promptMu.RUnlock()
	if observer != nil {
		observer(prompt, append(json.RawMessage(nil), schema...), numPredict)
	}
}

func (c *OllamaClient) LastEvent() string {
	if c == nil {
		return OllamaUnknown
	}
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()
	if c.lastEvent == "" {
		return OllamaUnknown
	}
	return c.lastEvent
}

// generateRequest representa la petición enviada a:
//
//	POST /api/generate
type generateRequest struct {
	Model     string          `json:"model"`
	Prompt    string          `json:"prompt"`
	Stream    bool            `json:"stream"`
	Format    json.RawMessage `json:"format,omitempty"`
	Options   generateOptions `json:"options"`
	KeepAlive string          `json:"keep_alive,omitempty"`
}

// generateOptions contiene únicamente parámetros técnicos de generación.
//
// No modifica el razonamiento de Ahmelia.
// Solamente controla la forma en que Ollama genera la respuesta.
type generateOptions struct {
	Temperature float64 `json:"temperature"`
	NumPredict  int     `json:"num_predict"`
}

// generateResponse representa la parte de la respuesta de Ollama
// que necesitamos para esta integración.
type generateResponse struct {
	Response string `json:"response"`
}

// interpretationJSONSchema devuelve el contrato estructural que Ollama debe
// respetar durante la generación. La semántica sigue perteneciendo al
// Interpreter/Validator y posteriormente a la arquitectura cognitiva; el
// schema solo impide que el modelo produzca una forma incompatible.
func interpretationJSONSchema() json.RawMessage {
	return json.RawMessage(`{
		"type":"object",
		"properties":{
			"intent":{"type":"string","enum":["command","question","learning","feedback","correction","unknown"]},
			"entities":{"type":"array","items":{"type":"string"}},
			"relations":{"type":"array","items":{
				"type":"object",
				"properties":{
					"source":{"type":"string"},
					"predicate":{"type":"string","enum":["addresses","asks_about","asks_about_state","asks_about_opinion","proposes","anticipates","gives_feedback_to","evaluates","refers_to","affects","depends_on","supports","commits_to","enables","declares_attribute","declares_preference","asks_about_attribute","asks_about_preference"]},
					"target":{"type":"string"},
					"value":{"type":"string"},
					"confidence":{"type":"number","minimum":0,"maximum":1}
				},
				"required":["source","predicate","target","confidence"]
			}},
			"references":{"type":"array","items":{"type":"string"}},
			"constraints":{"type":"array","items":{"type":"string"}},
			"meaning":{"type":"string"},
			"confidence":{"type":"number","minimum":0,"maximum":1},
			"speech_act":{"type":"string","enum":["question","command","inform","propose","correct","confirm","reject","continue","stop","pause","social","feedback","unknown"]},
			"topic":{"type":"string"},
			"question_type":{"type":"string","enum":["quantity","person","location","time","explanation","procedure_or_explanation","general",""]},
			"directness":{"type":"string","enum":["direct_command","polite_request","question","statement",""]},
			"user_state_signals":{"type":"array","items":{"type":"string"}},
			"correction_target":{"type":"string"}
		},
		"required":["intent","entities","relations","references","constraints","meaning","confidence","speech_act","topic","question_type","directness","user_state_signals","correction_target"]
	}`)
}

// Generate envía un prompt al modelo configurado en Ollama y devuelve
// la respuesta completa.
//
// stream=false es importante porque Interpreter necesita recibir
// una única respuesta JSON completa.
func ollamaHTTPTimeout() time.Duration {
	ms := 0
	if raw := strings.TrimSpace(os.Getenv("AHMELIA_OLLAMA_HTTP_TIMEOUT_MS")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			ms = parsed
		}
	}
	if ms < 1000 {
		ms = 180000
	}
	if ms > 600000 {
		ms = 600000
	}
	return time.Duration(ms) * time.Millisecond
}

func ollamaWarmupTimeout() time.Duration {
	ms := 0
	if raw := strings.TrimSpace(os.Getenv("AHMELIA_OLLAMA_WARMUP_TIMEOUT_MS")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			ms = parsed
		}
	}
	if ms < 1000 {
		ms = 180000
	}
	if ms > 600000 {
		ms = 600000
	}
	return time.Duration(ms) * time.Millisecond
}

func ollamaKeepAlive() string {
	value := strings.TrimSpace(os.Getenv("AHMELIA_OLLAMA_KEEP_ALIVE"))
	if value == "" {
		return "10m"
	}
	return value
}

// Warmup starts one model-load operation for this shared Ollama client.
// The warmup is infrastructure lifecycle, not cognition: it only ensures the
// configured model is resident so the first real linguistic request does not
// consume the live interaction budget waiting for a cold model load.
func (c *OllamaClient) Warmup(ctx context.Context) {
	if c == nil || strings.TrimSpace(c.BaseURL) == "" || strings.TrimSpace(c.Model) == "" {
		return
	}
	c.readyMu.Lock()
	if c.ready || c.warming {
		c.readyMu.Unlock()
		return
	}
	c.warming = true
	c.setState(OllamaModelWarming, OllamaModelWarming)
	c.readyCh = make(chan struct{})
	readyCh := c.readyCh
	c.readyMu.Unlock()

	go func() {
		warmCtx := ctx
		if warmCtx == nil {
			warmCtx = context.Background()
		}
		var cancel context.CancelFunc
		warmCtx, cancel = context.WithTimeout(warmCtx, ollamaWarmupTimeout())
		defer cancel()
		err := c.warmupContext(warmCtx)
		c.readyMu.Lock()
		c.warming = false
		c.readyErr = err
		c.ready = err == nil
		if err == nil {
			c.setState(OllamaModelReady, OllamaModelReady)
		} else {
			c.setState(OllamaModelUnavailable, OllamaServerUnreachable)
		}
		close(readyCh)
		c.readyMu.Unlock()
	}()
}

// WaitReady exposes the infrastructure readiness boundary to startup
// orchestration and diagnostics. It never performs cognition or execution.
func (c *OllamaClient) WaitReady(ctx context.Context) error {
	if c == nil {
		return fmt.Errorf("ollama: cliente no inicializado")
	}
	c.Warmup(ctx)
	return c.waitReady(ctx)
}

func (c *OllamaClient) waitReady(ctx context.Context) error {
	if c == nil {
		return fmt.Errorf("ollama: cliente no inicializado")
	}
	c.readyMu.Lock()
	if c.ready {
		c.readyMu.Unlock()
		return nil
	}
	if !c.warming {
		c.warming = true
		c.readyCh = make(chan struct{})
		readyCh := c.readyCh
		c.readyMu.Unlock()
		go func() {
			warmCtx, cancel := context.WithTimeout(context.Background(), ollamaWarmupTimeout())
			err := c.warmupContext(warmCtx)
			c.readyMu.Lock()
			c.warming = false
			c.readyErr = err
			c.ready = err == nil
			if err == nil {
				c.setState(OllamaModelReady, OllamaModelReady)
			} else {
				c.setState(OllamaModelUnavailable, OllamaServerUnreachable)
			}
			close(readyCh)
			c.readyMu.Unlock()
			cancel()
		}()
		c.readyMu.Lock()
	}
	readyCh := c.readyCh
	readyErr := c.readyErr
	c.readyMu.Unlock()
	if readyErr != nil && readyCh == nil {
		return readyErr
	}
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case <-readyCh:
		c.readyMu.Lock()
		err := c.readyErr
		ready := c.ready
		c.readyMu.Unlock()
		if ready {
			return nil
		}
		if err != nil {
			return err
		}
		return fmt.Errorf("ollama: modelo no quedó listo")
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (c *OllamaClient) warmupContext(ctx context.Context) error {
	requestBody := generateRequest{Model: c.Model, Prompt: "Responde únicamente: OK", Stream: false, Options: generateOptions{Temperature: 0, NumPredict: 1}, KeepAlive: ollamaKeepAlive()}
	data, err := json.Marshal(requestBody)
	if err != nil {
		return fmt.Errorf("ollama warmup: error creando petición JSON: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(c.BaseURL, "/")+"/api/generate", bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("ollama warmup: error creando petición HTTP: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("ollama warmup: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("ollama warmup: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var result generateResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("ollama warmup: respuesta inválida: %w", err)
	}
	return nil
}

// Available performs a short server health check and starts the single model
// warmup lifecycle for the shared client. It does not wait for inference.
func (c *OllamaClient) Available() bool {
	if c == nil || strings.TrimSpace(c.BaseURL) == "" {
		return false
	}
	// Availability is a health check, not model inference. Never reuse the
	// generation client here because it may carry a long inference timeout.
	// A missing/starting Ollama instance must not stall the main interaction
	// pipeline for minutes.
	client := &http.Client{Timeout: 400 * time.Millisecond}
	req, err := http.NewRequest(http.MethodGet, strings.TrimRight(c.BaseURL, "/")+"/api/tags", nil)
	if err != nil {
		return false
	}
	resp, err := client.Do(req)
	if err != nil {
		c.setState(OllamaServerUnreachable, OllamaServerUnreachable)
		return false
	}
	defer resp.Body.Close()
	ok := resp.StatusCode >= 200 && resp.StatusCode < 300
	if ok {
		c.setState(OllamaModelWarming, OllamaModelWarming)
		c.Warmup(context.Background())
	} else {
		c.setState(OllamaModelUnavailable, OllamaHTTPError)
	}
	return ok
}

func (c *OllamaClient) Generate(prompt string) (string, error) {
	return c.GenerateContext(context.Background(), prompt)
}

// GenerateContext is the context-aware HTTP boundary used by live and
// deferred linguistic interpretation. A caller can stop waiting without
// creating a second Ollama client or a second interpreter. The existing
// 120-second client timeout remains the upper bound for background work.
func (c *OllamaClient) GenerateContext(ctx context.Context, prompt string) (string, error) {
	return c.generateStructuredWithGate(ctx, prompt, interpretationJSONSchema(), 256, false, nil, ModelClassInterpretation, int(GatePriorityLearning))
}

// GenerateConversationalContext uses the language renderer's JSON contract,
// which is distinct from the linguistic interpreter's semantic schema.
func (c *OllamaClient) GenerateConversationalContext(ctx context.Context, prompt string) (string, error) {
	schema := json.RawMessage(`{"type":"object","properties":{"response":{"type":"string"}},"required":["response"]}`)
	c.observeStructuredPrompt(prompt, schema, 256)
	return c.generateStructuredWithGate(ctx, prompt, schema, 256, false, nil, ModelClassGeneration, int(GatePriorityUserGoal))
}

// GenerateStructuredContext is the generic bounded structured-output boundary
// for capabilities that need a machine-readable model response. It does not
// interpret the content or grant any authority; callers own the schema and
// semantics. GenerateContext remains the canonical linguistic interpretation
// entry point and keeps its existing schema.
func (c *OllamaClient) GenerateStructuredContext(ctx context.Context, prompt string, schema json.RawMessage, numPredict int) (string, error) {
	c.observeStructuredPrompt(prompt, schema, numPredict)
	return c.generateStructuredWithGate(ctx, prompt, schema, numPredict, false, nil, ModelClassProposal, int(GatePriorityUserGoal))
}

// GenerateShadowContext is the lowest-priority structured boundary. It is used
// by background deliberation: the model may opine after the turn is closed,
// but it must never displace a user turn, a learning pass or an own-goal query.
func (c *OllamaClient) GenerateShadowContext(ctx context.Context, prompt string, schema json.RawMessage, numPredict int) (string, error) {
	c.observeStructuredPrompt(prompt, schema, numPredict)
	return c.generateStructuredWithGate(ctx, prompt, schema, numPredict, false, nil, ModelClassProposal, int(GatePriorityOwn))
}

// GenerateStructuredStreamContext is the shared streaming boundary for any
// capability that needs structured model output. Ollama emits NDJSON chunks;
// the infrastructure accumulates them until done=true, so callers receive one
// complete structured document while optionally observing incremental chunks.
func (c *OllamaClient) GenerateStructuredStreamContext(ctx context.Context, prompt string, schema json.RawMessage, numPredict int, onChunk func(string)) (string, error) {
	c.observeStructuredPrompt(prompt, schema, numPredict)
	return c.generateStructuredWithGate(ctx, prompt, schema, numPredict, true, onChunk, ModelClassProposal, int(GatePriorityUserGoal))
}

// GenerateStreamContext is the generic streaming text boundary. It is not a
// programming-specific API: future capabilities can reuse the same lifecycle,
// cancellation and telemetry semantics.
func (c *OllamaClient) GenerateStreamContext(ctx context.Context, prompt string, onChunk func(string)) (string, error) {
	return c.generateStructuredWithGate(ctx, prompt, nil, 256, true, onChunk, ModelClassGeneration, int(GatePriorityUserGoal))
}

type streamGenerateResponse struct {
	Response string `json:"response"`
	Done     bool   `json:"done"`
}

// generateStructuredWithGate wraps generateStructured with optional gate control.
// class/priority determine the gate behavior; if c.Gate is nil, behaves exactly as before.
func (c *OllamaClient) generateStructuredWithGate(ctx context.Context, prompt string, schema json.RawMessage, numPredict int, stream bool, onChunk func(string), class ModelClass, priority int) (string, error) {
	if c.Gate == nil {
		return c.generateStructured(ctx, prompt, schema, numPredict, stream, onChunk)
	}

	// Enter gate
	gateCtx, exit, err := c.Gate.Enter(ctx, string(class), priority)
	if err != nil {
		return "", fmt.Errorf("gate enter: %w", err)
	}
	defer exit()

	return c.generateStructured(gateCtx, prompt, schema, numPredict, stream, onChunk)
}

func (c *OllamaClient) generateStructured(ctx context.Context, prompt string, schema json.RawMessage, numPredict int, stream bool, onChunk func(string)) (string, error) {
	if c == nil {
		return "", fmt.Errorf("ollama: cliente no inicializado")
	}
	if strings.TrimSpace(c.BaseURL) == "" {
		return "", fmt.Errorf("ollama: BaseURL no configurada")
	}
	if strings.TrimSpace(c.Model) == "" {
		return "", fmt.Errorf("ollama: modelo no configurado")
	}
	if c.client == nil {
		return "", fmt.Errorf("ollama: cliente HTTP no inicializado")
	}
	if strings.TrimSpace(prompt) == "" {
		return "", fmt.Errorf("ollama: prompt vacío")
	}
	if schema != nil && len(schema) == 0 {
		return "", fmt.Errorf("ollama: schema vacío")
	}
	if numPredict < 1 {
		numPredict = 2048
	}
	trace := GenerationTrace{Endpoint: strings.TrimRight(c.BaseURL, "/") + "/api/generate", Model: c.Model, Prompt: prompt, PromptBytes: len([]byte(prompt)), Stream: stream, Schema: string(schema), NumPredict: numPredict, StartedAt: time.Now()}
	defer func() {
		if trace.CompletedAt.IsZero() {
			trace.CompletedAt = time.Now()
		}
		c.observeGenerationTrace(trace)
	}()
	if ctx == nil {
		ctx = context.Background()
	}
	if err := c.waitReady(ctx); err != nil {
		if errors.Is(err, context.Canceled) {
			c.setState(OllamaModelUnavailable, OllamaContextCancelled)
		} else if errors.Is(err, context.DeadlineExceeded) {
			c.setState(OllamaModelUnavailable, OllamaRequestTimeout)
		} else {
			c.setState(OllamaServerUnreachable, OllamaServerUnreachable)
		}
		return "", fmt.Errorf("ollama: modelo no listo: %w", err)
	}
	requestBody := generateRequest{Model: c.Model, Prompt: prompt, Stream: stream, Format: schema, Options: generateOptions{Temperature: 0, NumPredict: numPredict}, KeepAlive: ollamaKeepAlive()}
	data, err := json.Marshal(requestBody)
	if err != nil {
		return "", fmt.Errorf("ollama: error creando JSON: %w", err)
	}
	traceMu := &sync.Mutex{}
	clientTrace := &httptrace.ClientTrace{
		ConnectStart: func(string, string) {
			traceMu.Lock()
			if trace.ConnectStartedAt.IsZero() {
				trace.ConnectStartedAt = time.Now()
			}
			traceMu.Unlock()
		},
		ConnectDone: func(_, _ string, err error) {
			if err == nil {
				traceMu.Lock()
				if trace.ConnectedAt.IsZero() {
					trace.ConnectedAt = time.Now()
				}
				traceMu.Unlock()
			}
		},
		GotConn: func(info httptrace.GotConnInfo) {
			traceMu.Lock()
			trace.GotConnAt = time.Now()
			trace.ConnReused = info.Reused
			traceMu.Unlock()
		},
		WroteRequest: func(httptrace.WroteRequestInfo) {
			traceMu.Lock()
			if trace.RequestWrittenAt.IsZero() {
				trace.RequestWrittenAt = time.Now()
			}
			traceMu.Unlock()
		},
		GotFirstResponseByte: func() {
			traceMu.Lock()
			if trace.FirstResponseAt.IsZero() {
				trace.FirstResponseAt = time.Now()
			}
			traceMu.Unlock()
		},
	}
	req, err := http.NewRequestWithContext(httptrace.WithClientTrace(ctx, clientTrace), http.MethodPost, trace.Endpoint, bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("ollama: error creando petición: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		trace.Error = err.Error()
		if errors.Is(ctx.Err(), context.Canceled) {
			c.setState(OllamaModelReady, OllamaContextCancelled)
		} else if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			c.setState(OllamaModelReady, OllamaRequestTimeout)
		} else {
			c.setState(OllamaServerUnreachable, OllamaServerUnreachable)
		}
		return "", fmt.Errorf("ollama: no se pudo conectar con Ollama en %s: %w", c.BaseURL, err)
	}
	traceMu.Lock()
	trace.HeadersAt = time.Now()
	traceMu.Unlock()
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		trace.ResponseBytes = int64(len(body))
		trace.Error = fmt.Sprintf("HTTP %d", resp.StatusCode)
		c.setState(OllamaModelUnavailable, OllamaHTTPError)
		return "", fmt.Errorf("ollama: respondió con estado HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var full strings.Builder
	if !stream {
		body, readErr := io.ReadAll(&traceReadCloser{ReadCloser: resp.Body, first: func(at time.Time) {
			traceMu.Lock()
			if trace.FirstBodyAt.IsZero() {
				trace.FirstBodyAt = at
			}
			traceMu.Unlock()
		}})
		trace.ResponseBytes = int64(len(body))
		if readErr != nil {
			c.setState(OllamaModelReady, OllamaInvalidResponse)
			trace.Error = readErr.Error()
			return "", fmt.Errorf("ollama: error leyendo respuesta estructurada: %w", readErr)
		}
		var result generateResponse
		if err := json.Unmarshal(body, &result); err != nil {
			c.setState(OllamaModelReady, OllamaInvalidResponse)
			trace.Error = err.Error()
			return "", fmt.Errorf("ollama: error leyendo respuesta estructurada: %w", err)
		}
		full.WriteString(result.Response)
	} else {
		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
		for scanner.Scan() {
			var chunk streamGenerateResponse
			if err := json.Unmarshal(scanner.Bytes(), &chunk); err != nil {
				c.setState(OllamaModelReady, OllamaInvalidResponse)
				return "", fmt.Errorf("ollama: chunk NDJSON inválido: %w", err)
			}
			if chunk.Response != "" {
				traceMu.Lock()
				if trace.FirstContentAt.IsZero() {
					trace.FirstContentAt = time.Now()
				}
				traceMu.Unlock()
				full.WriteString(chunk.Response)
				if onChunk != nil {
					onChunk(chunk.Response)
				}
			}
			if chunk.Done {
				break
			}
		}
		if err := scanner.Err(); err != nil {
			trace.Error = err.Error()
			if errors.Is(ctx.Err(), context.Canceled) {
				c.setState(OllamaModelReady, OllamaContextCancelled)
			} else if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				c.setState(OllamaModelReady, OllamaRequestTimeout)
			}
			return "", fmt.Errorf("ollama: error leyendo streaming: %w", err)
		}
	}
	response := strings.TrimSpace(full.String())
	if response == "" {
		trace.Error = "empty response"
		c.setState(OllamaModelReady, OllamaInvalidResponse)
		return "", fmt.Errorf("ollama: el modelo devolvió una respuesta vacía")
	}
	c.setState(OllamaModelReady, OllamaSuccess)
	return response, nil
}

type traceReadCloser struct {
	io.ReadCloser
	first func(time.Time)
	seen  bool
}

func (r *traceReadCloser) Read(p []byte) (int, error) {
	n, err := r.ReadCloser.Read(p)
	if n > 0 && !r.seen {
		r.seen = true
		if r.first != nil {
			r.first(time.Now())
		}
	}
	return n, err
}
