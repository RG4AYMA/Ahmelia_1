package core

import (
	"context"

	"ahmelia/interaction"
	"ahmelia/memory_intelligence/semindex"
)

// ollamaEmbedder adapta el proveedor existente (float64) al contrato de
// semindex (float32). ID incluye el modelo: cambiar de modelo = otro espacio.
type ollamaEmbedder struct {
	p     *interaction.OllamaEmbeddingProvider
	model string
}

func (o ollamaEmbedder) ID() string { return "ollama:" + o.model }

func (o ollamaEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	v, err := o.p.Embed(ctx, text)
	if err != nil {
		return nil, err
	}
	out := make([]float32, len(v))
	for i, x := range v {
		out[i] = float32(x)
	}
	return out, nil
}

var _ semindex.Embedder = ollamaEmbedder{}
