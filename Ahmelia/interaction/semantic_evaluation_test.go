package interaction

import "testing"

func TestEvaluateSemanticAcceptsCoherentQuestion(t *testing.T) {
	r := Analyze("¿por qué el cielo es azul?", Context{})
	e := EvaluateSemantic(r, Context{})
	if e.Status != SemanticAccepted {
		t.Fatalf("status=%q score=%.2f issues=%v", e.Status, e.Score, e.Issues)
	}
}

func TestEvaluateSemanticReviewsUnresolvedReference(t *testing.T) {
	r := Analyze("¿qué hacemos con eso?", Context{})
	e := EvaluateSemantic(r, Context{})
	if e.Status != SemanticReview {
		t.Fatalf("status=%q score=%.2f issues=%v", e.Status, e.Score, e.Issues)
	}
	if e.OpenRefs == 0 {
		t.Fatal("expected an unresolved reference")
	}
}

func TestEvaluateSemanticAcceptsContextualContinuation(t *testing.T) {
	ctx := Context{ActiveObjective: "avanzar la escena"}
	r := Analyze("también cambia la función", ctx)
	e := EvaluateSemantic(r, ctx)
	if e.Status != SemanticAccepted {
		t.Fatalf("status=%q score=%.2f issues=%v", e.Status, e.Score, e.Issues)
	}
}

func TestEvaluateSemanticIgnoresLegacyKindSpeechActMutation(t *testing.T) {
	r := Analyze("detente", Context{})
	if !r.CommunicativeActs.Has(ActControl) {
		t.Fatalf("expected canonical control act, got %+v", r.CommunicativeActs)
	}
	// Deliberately corrupt the compatibility projections. The canonical
	// contract must remain unchanged and semantic evaluation must not reopen
	// the retired Kind/SpeechAct authority.
	r.Kind = KindQuestion
	r.SpeechAct = SpeechCommand
	e := EvaluateSemantic(r, Context{})
	if e.Status == SemanticRejected || len(e.Issues) > 0 {
		t.Fatalf("legacy projections must not create semantic contradictions: status=%q issues=%v", e.Status, e.Issues)
	}
}

func TestEvaluateSemanticDoesNotClaimKnowledge(t *testing.T) {
	r := Analyze("¿cuál es la capital de Japón?", Context{})
	e := EvaluateSemantic(r, Context{})
	if e.Status != SemanticAccepted {
		t.Fatalf("status=%q score=%.2f issues=%v", e.Status, e.Score, e.Issues)
	}
	// Evaluation only judges the interpretation; it must not assert that the
	// answer is known or select a knowledge provider.
	if e.Reason == "" {
		t.Fatal("evaluation reason should be explicit")
	}
}
