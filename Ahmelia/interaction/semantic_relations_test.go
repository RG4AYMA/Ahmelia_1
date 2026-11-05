package interaction

import "testing"

func TestInferSemanticRelationsFutureCommitment(t *testing.T) {
	r := SemanticRequest{Raw: "haré todo lo posible para que puedas ver por ti misma", Normalized: "haré todo lo posible para que puedas ver por ti misma"}
	r = InferSemanticRelations(r)
	if len(r.Relations) < 2 {
		t.Fatalf("expected future commitment relations, got %#v", r.Relations)
	}
	acts := ClassifyCommunicativeActs(r)
	if acts.Primary != ActProposal {
		t.Fatalf("expected proposal primary act from future relation, got %q", acts.Primary)
	}
}

func TestIsQuestionRecognizesBareComoEstas(t *testing.T) {
	r := SemanticRequest{Raw: "hola como estas", Normalized: "hola como estas"}
	acts := ClassifyCommunicativeActs(r)
	if acts.Primary != ActQuestion {
		t.Fatalf("expected question primary act, got %q", acts.Primary)
	}
	if !acts.Has(ActSocial) {
		t.Fatalf("expected social secondary act, got %#v", acts)
	}
}

func TestInferSemanticRelationsAccentVariantsConverge(t *testing.T) {
	cases := []string{
		"hare todo lo posible para que puedas ver por ti misma",
		"haré todo lo posible para que podrás ver por ti misma",
		"podras saber cual te gusta más",
	}
	for _, input := range cases {
		r := SemanticRequest{Raw: input, Normalized: input}
		r = InferSemanticRelations(r)
		if len(r.Relations) == 0 {
			t.Fatalf("%q produced no semantic relations", input)
		}
	}
}
