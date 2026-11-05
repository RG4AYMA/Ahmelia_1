package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"ahmelia/interaction"
)

func main() {
	baseURL := os.Getenv("AHMELIA_OLLAMA_URL")
	model := os.Getenv("AHMELIA_EMBEDDING_MODEL")
	threshold := 0.78
	provider := interaction.NewOllamaEmbeddingProvider(baseURL, model)
	classifier := interaction.NewSemanticSimilarityClassifier(provider, interaction.PhaseCPilotExamples(), threshold)

	ctx := context.Background()
	positives := []string{
		"Quiero que algún día puedas mirar lo que hay a tu alrededor.",
		"Voy a esforzarme para que tengas una forma de percibir tu entorno.",
		"Cuando tenga la tecnología necesaria, haré posible que puedas observar el mundo.",
	}
	negatives := []string{
		"Ayer llovió toda la tarde.",
		"El negro sigue siendo mi color favorito.",
		"La ventana está abierta desde esta mañana.",
	}

	fmt.Printf("Phase C proposal semantic pilot | threshold=%.2f model=%q\n", threshold, classifier.ProviderModel())
	for _, sentence := range positives {
		match, ok, err := classifier.Match(ctx, sentence, "proposal")
		printResult("POS", sentence, match, ok, err)
	}
	for _, sentence := range negatives {
		match, ok, err := classifier.Match(ctx, sentence, "proposal")
		printResult("NEG", sentence, match, ok, err)
	}
}

func printResult(kind, text string, match interaction.SimilarityMatch, ok bool, err error) {
	if err != nil {
		fmt.Printf("[%s] %s\n  ERROR: %v\n", kind, text, err)
		return
	}
	fmt.Printf("[%s] %s\n  matched=%t score=%.4f source=%s example=%q\n", kind, text, ok, match.Score, match.Source, match.Example)
	if ok {
		fmt.Printf("  relation=user --proposes--> future_state (confidence=%.4f)\n", match.Score)
	} else if strings.TrimSpace(text) == "" {
		fmt.Println("  empty input")
	}
}
