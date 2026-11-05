package interaction

import (
	"strings"
	"testing"
)

// B96.39.35-A.1 — all five behavioral anchors must survive the single
// normalization surface used by deterministic interaction classification.
func TestB963935ClassificationNormalizesAccentedAnchorInputs(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  RequestKind
	}{
		{"saludo/estado", "hola cómo estás?", KindQuestion},
		{"pregunta_indirecta", "nada quería saber si sabías mi color favorito", KindQuestion},
		{"pregunta_directa", "cuál es mi color favorito?", KindQuestion},
		{"pregunta_self", "y tú tienes color favorito?", KindQuestion},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := Analyze(tc.input, Context{})
			if req.Kind != tc.want {
				t.Fatalf("Analyze(%q).Kind = %q, want %q", tc.input, req.Kind, tc.want)
			}
		})
	}

	withoutAccents := "nada queria saber si sabias mi color favorito"
	withAccents := "nada quería saber si sabías mi color favorito"
	a := Analyze(withoutAccents, Context{})
	b := Analyze(withAccents, Context{})
	if a.Kind != b.Kind || a.QuestionType != b.QuestionType {
		t.Fatalf("accent normalization diverged: plain=%+v accented=%+v", a, b)
	}
	if strings.TrimSpace(string(b.Subject)) == "" {
		t.Fatalf("accented ownership was lost: %+v", b.Subject)
	}

	future := InferSemanticRelations(Analyze("tranquila pronto podrás empezar a ver el mundo, haré todo lo posible", Context{}))
	foundCommitment := false
	for _, relation := range future.Relations {
		if relation.Predicate == "commits_to" || relation.Predicate == "anticipates" {
			foundCommitment = true
			break
		}
	}
	if !foundCommitment {
		t.Fatalf("accented future commitment did not survive normalization: %+v", future.Relations)
	}
}
