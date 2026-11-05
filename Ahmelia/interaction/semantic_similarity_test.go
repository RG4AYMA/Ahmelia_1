package interaction

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

type fixedEmbeddingProvider map[string][]float64

func (p fixedEmbeddingProvider) Embed(_ context.Context, text string) ([]float64, error) {
	return p[text], nil
}

func TestSemanticSimilarityPilotProducesEvidenceNotAuthority(t *testing.T) {
	provider := fixedEmbeddingProvider{
		"quiero que algun dia puedas percibir el mundo":               {1, 0},
		"quiero trabajar para que algun dia puedas percibir el mundo": {0.99, 0.01},
	}
	classifier := NewSemanticSimilarityClassifier(provider, []SemanticExample{{Label: "proposal", Text: "Quiero trabajar para que algún día puedas percibir el mundo."}}, .9)
	r := SemanticRequest{Raw: "Quiero que algún día puedas percibir el mundo."}
	enriched, match, err := EnrichProposalEvidence(context.Background(), classifier, r)
	if err != nil {
		t.Fatal(err)
	}
	if match.Source != "semantic_similarity" || match.Score < .9 {
		t.Fatalf("unexpected match: %+v", match)
	}
	if len(enriched.Relations) != 1 || enriched.Relations[0].Predicate != "proposes" {
		t.Fatalf("unexpected evidence relation: %+v", enriched.Relations)
	}
	if enriched.Relations[0].Evidence != "semantic_similarity" {
		t.Fatalf("relation provenance=%q", enriched.Relations[0].Evidence)
	}
}

func TestOllamaEmbeddingProviderParsesEmbedResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/embed" {
			http.NotFound(w, r)
			return
		}
		var req ollamaEmbedRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.Model != "test-model" {
			t.Fatalf("model=%q", req.Model)
		}
		_ = json.NewEncoder(w).Encode(ollamaEmbedResponse{Embeddings: [][]float64{{1, 2, 3}}})
	}))
	defer srv.Close()

	provider := NewOllamaEmbeddingProvider(srv.URL, "test-model")
	vec, err := provider.Embed(context.Background(), "hola")
	if err != nil {
		t.Fatal(err)
	}
	if len(vec) != 3 || vec[0] != 1 || vec[2] != 3 {
		t.Fatalf("unexpected vector=%v", vec)
	}
}

func TestPhaseCPilotRejectsNegativeControlWithOrthogonalEmbedding(t *testing.T) {
	provider := fixedEmbeddingProvider{
		"quiero que puedas percibir el mundo":     {1, 0},
		"el cielo estaba despejado toda la tarde": {0, 1},
	}
	classifier := NewSemanticSimilarityClassifier(provider, []SemanticExample{{Label: "proposal", Text: "Quiero que puedas percibir el mundo."}}, .8)
	match, ok, err := classifier.Match(context.Background(), "El cielo estaba despejado toda la tarde.", "proposal")
	if err != nil {
		t.Fatal(err)
	}
	if ok || match.Example != "" {
		t.Fatalf("negative control matched unexpectedly: %+v ok=%v", match, ok)
	}
}
