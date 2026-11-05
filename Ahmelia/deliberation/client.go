package deliberation

// client.go — FRONTERA ESTRECHA CON EL MODELO.

import (
	"context"
	"encoding/json"
)

// Client is the only model boundary deliberation needs. Implementations may
// be a local LLM, a test double or a future service. The harness owns the
// workspace, the schema, validation and the record; the client only generates.
type Client interface {
	GenerateShadowContext(ctx context.Context, prompt string, schema json.RawMessage, numPredict int) (string, error)
}
