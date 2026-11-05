package core

import (
	"context"
	"errors"
	"testing"

	"ahmelia/interaction"
)

type b96FakeEmbeddingProvider struct {
	vectors map[string][]float64
	err     error
}

func (f *b96FakeEmbeddingProvider) Embed(_ context.Context, text string) ([]float64, error) {
	if f.err != nil {
		return nil, f.err
	}
	if v, ok := f.vectors[text]; ok {
		return v, nil
	}
	return []float64{1, 0}, nil
}

func b96SnapshotWithAssistant(text string) interaction.ConversationSnapshot {
	return interaction.ConversationSnapshot{Turns: []interaction.TurnRecord{{Assistant: text}}}
}

func TestB96InitiativeSemanticDuplicateSuppressesEquivalent(t *testing.T) {
	provider := &b96FakeEmbeddingProvider{vectors: map[string][]float64{
		"entiendo tu entusiasmo":        {1, 0},
		"estoy emocionado por ayudarte": {0.95, 0.3122498999},
	}}
	guard := interaction.NewInitiativeSimilarityGuard(provider)
	dup, err := guard.IsSemanticDuplicate(context.Background(), "Estoy emocionado por ayudarte", b96SnapshotWithAssistant("Entiendo tu entusiasmo"))
	if err != nil {
		t.Fatal(err)
	}
	if !dup {
		t.Fatal("expected semantic duplicate")
	}
}

func TestB96InitiativeSemanticDuplicateDoesNotSuppressDistinct(t *testing.T) {
	provider := &b96FakeEmbeddingProvider{vectors: map[string][]float64{
		"respuesta nueva":    {1, 0},
		"respuesta anterior": {0, 1},
	}}
	guard := interaction.NewInitiativeSimilarityGuard(provider)
	dup, err := guard.IsSemanticDuplicate(context.Background(), "respuesta nueva", b96SnapshotWithAssistant("respuesta anterior"))
	if err != nil {
		t.Fatal(err)
	}
	if dup {
		t.Fatal("did not expect semantic duplicate")
	}
}

func TestB96InitiativeSemanticDuplicateFailsOpenOnProviderError(t *testing.T) {
	guard := interaction.NewInitiativeSimilarityGuard(&b96FakeEmbeddingProvider{err: errors.New("provider down")})
	dup, err := guard.IsSemanticDuplicate(context.Background(), "respuesta nueva", b96SnapshotWithAssistant("respuesta anterior"))
	if err == nil {
		t.Fatal("expected provider error")
	}
	if dup {
		t.Fatal("provider failure must not report duplicate")
	}
}

func TestB96InitiativeSemanticDuplicateNilGuardKeepsLiteralBehavior(t *testing.T) {
	var guard *interaction.InitiativeSimilarityGuard
	if guard != nil {
		t.Fatal("expected nil guard when optional embedding configuration is absent")
	}
	if !isDuplicateInitiativeResponse("misma respuesta", "misma respuesta") {
		t.Fatal("expected existing literal duplicate check to remain true")
	}
	if isDuplicateInitiativeResponse("respuesta distinta", "misma respuesta") {
		t.Fatal("expected existing literal duplicate check to remain false")
	}
}
