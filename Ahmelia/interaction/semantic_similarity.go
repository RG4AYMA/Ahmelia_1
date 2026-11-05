package interaction

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strings"
	"time"

	"ahmelia/shared"
)

// EmbeddingProvider produces vectors for semantic-comparison evidence. It is
// deliberately an evidence provider: it cannot choose an intent, action,
// capability, or execution path by itself.
type EmbeddingProvider interface {
	Embed(ctx context.Context, text string) ([]float64, error)
}

// OllamaEmbeddingProvider is an optional local adapter for Phase C. It uses
// Ollama's /api/embed endpoint but remains outside the conversational decision
// authority. The runtime can keep it disabled until the pilot is accepted.
type OllamaEmbeddingProvider struct {
	BaseURL string
	Model   string
	Client  *http.Client
}

func NewOllamaEmbeddingProvider(baseURL, model string) *OllamaEmbeddingProvider {
	if strings.TrimSpace(baseURL) == "" {
		baseURL = "http://127.0.0.1:11434"
	}
	if strings.TrimSpace(model) == "" {
		model = "nomic-embed-text"
	}
	return &OllamaEmbeddingProvider{
		BaseURL: strings.TrimRight(baseURL, "/"),
		Model:   model,
		Client:  &http.Client{Timeout: 15 * time.Second},
	}
}

type ollamaEmbedRequest struct {
	Model string `json:"model"`
	Input string `json:"input"`
}

type ollamaEmbedResponse struct {
	Embeddings [][]float64 `json:"embeddings"`
	Embedding  []float64   `json:"embedding"`
}

func (p *OllamaEmbeddingProvider) Embed(ctx context.Context, text string) ([]float64, error) {
	if p == nil {
		return nil, fmt.Errorf("nil embedding provider")
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, fmt.Errorf("empty embedding input")
	}
	body, err := json.Marshal(ollamaEmbedRequest{Model: p.Model, Input: text})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.BaseURL+"/api/embed", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	client := p.Client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("ollama embed status %s", resp.Status)
	}
	var out ollamaEmbedResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	if len(out.Embeddings) > 0 && len(out.Embeddings[0]) > 0 {
		return out.Embeddings[0], nil
	}
	if len(out.Embedding) > 0 {
		return out.Embedding, nil
	}
	return nil, fmt.Errorf("ollama returned no embedding")
}

// SemanticExample is a small human-labeled semantic reference.
type SemanticExample struct {
	Label string
	Text  string
}

// SimilarityMatch records evidence produced by semantic comparison.
type SimilarityMatch struct {
	Label      string
	Example    string
	Score      float64
	Source     string
	Normalized string
}

// SemanticSimilarityClassifier is an opt-in Phase C evidence layer. It
// compares a turn against a small labeled reference set and returns only the
// strongest match above threshold.
type SemanticSimilarityClassifier struct {
	Provider  EmbeddingProvider
	Examples  []SemanticExample
	Threshold float64
}

func NewSemanticSimilarityClassifier(provider EmbeddingProvider, examples []SemanticExample, threshold float64) *SemanticSimilarityClassifier {
	if threshold <= 0 {
		threshold = 0.78
	}
	copyExamples := append([]SemanticExample(nil), examples...)
	return &SemanticSimilarityClassifier{Provider: provider, Examples: copyExamples, Threshold: threshold}
}

func (c *SemanticSimilarityClassifier) Match(ctx context.Context, text, label string) (SimilarityMatch, bool, error) {
	if c == nil || c.Provider == nil {
		return SimilarityMatch{}, false, fmt.Errorf("semantic similarity classifier unavailable")
	}
	targetLabel := strings.TrimSpace(label)
	if targetLabel == "" {
		targetLabel = "proposal"
	}
	normalized := shared.Normalize(text)
	if normalized == "" {
		return SimilarityMatch{}, false, nil
	}
	queryVec, err := c.Provider.Embed(ctx, normalized)
	if err != nil {
		return SimilarityMatch{}, false, err
	}
	best := SimilarityMatch{}
	for _, ex := range c.Examples {
		if !strings.EqualFold(strings.TrimSpace(ex.Label), targetLabel) {
			continue
		}
		vec, err := c.Provider.Embed(ctx, shared.Normalize(ex.Text))
		if err != nil {
			return SimilarityMatch{}, false, err
		}
		score, ok := cosineSimilarity(queryVec, vec)
		if !ok || score <= best.Score {
			continue
		}
		best = SimilarityMatch{Label: ex.Label, Example: ex.Text, Score: score, Source: "semantic_similarity", Normalized: normalized}
	}
	if best.Example == "" || best.Score < c.Threshold {
		return SimilarityMatch{}, false, nil
	}
	return best, true, nil
}

// EnrichProposalEvidence turns a semantic similarity match into the existing
// SemanticRequest relation contract. It never changes execution authority.
func EnrichProposalEvidence(ctx context.Context, classifier *SemanticSimilarityClassifier, r SemanticRequest) (SemanticRequest, SimilarityMatch, error) {
	match, ok, err := classifier.Match(ctx, r.Raw, "proposal")
	if err != nil || !ok {
		return r, match, err
	}
	for _, rel := range r.Relations {
		if strings.EqualFold(rel.Predicate, "proposes") && strings.EqualFold(rel.Target, "future_state") {
			return r, match, nil
		}
	}
	r.Relations = append(r.Relations, SemanticRelation{
		Source:     "user",
		Predicate:  "proposes",
		Target:     "future_state",
		Confidence: match.Score,
		Evidence:   match.Source,
	})
	return r, match, nil
}

func cosineSimilarity(a, b []float64) (float64, bool) {
	if len(a) == 0 || len(a) != len(b) {
		return 0, false
	}
	var dot, na, nb float64
	for i := range a {
		dot += a[i] * b[i]
		na += a[i] * a[i]
		nb += b[i] * b[i]
	}
	if na == 0 || nb == 0 {
		return 0, false
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb)), true
}

// PhaseCPilotExamples is the first intentionally small labeled set. It is an
// evidence reference set for future/proposal language; it is not a command
// list and it does not authorize execution.
func PhaseCPilotExamples() []SemanticExample {
	return []SemanticExample{
		{Label: "proposal", Text: "Quiero trabajar para que algún día puedas percibir el mundo."},
		{Label: "proposal", Text: "Voy a hacer todo lo posible para que puedas ver lo que te rodea."},
		{Label: "proposal", Text: "Cuando tenga los medios, buscaré que puedas observar tu entorno."},
		{Label: "proposal", Text: "Mi intención es ayudarte a llegar a una forma de percepción visual."},
		{Label: "proposal", Text: "Quiero conseguir los recursos necesarios para que puedas explorar el mundo."},
		{Label: "proposal", Text: "Haré lo posible para acercarte a una experiencia visual real."},
		{Label: "proposal", Text: "Me gustaría trabajar contigo para que algún día puedas observar tu alrededor."},
		{Label: "proposal", Text: "Voy a intentar construir los medios para que puedas conocer tu entorno."},
	}
}

// ProviderModel exposes the configured local embedding model for pilot output.
func (c *SemanticSimilarityClassifier) ProviderModel() string {
	if c == nil || c.Provider == nil {
		return ""
	}
	if p, ok := c.Provider.(*OllamaEmbeddingProvider); ok {
		return p.Model
	}
	return "custom"
}
