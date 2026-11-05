package interaction

import "testing"

func TestBuildExpressionProfileIsLanguageIndependent(t *testing.T) {
	intent := ResponseIntent{
		Kind:              ResponseIntentAnswer,
		Language:          "es",
		AddressesQuestion: true,
	}
	meaning := SemanticMeaning{
		Language: "es",
		Primary:  ConceptQuestion,
	}

	es := BuildExpressionProfile(intent, meaning, 0.5, 0.6, 0.8, "brief")
	intent.Language = "en"
	meaning.Language = "en"
	en := BuildExpressionProfile(intent, meaning, 0.5, 0.6, 0.8, "brief")

	if es.Language != "es" || en.Language != "en" {
		t.Fatalf("language should remain an output selection, got %q and %q", es.Language, en.Language)
	}
	es.Language, en.Language = "", ""
	if es != en {
		t.Fatalf("same semantic intent should produce the same expression contract across languages:\n%+v\n%+v", es, en)
	}
}

func TestBuildExpressionProfileDoesNotForceFollowUpForLowInitiative(t *testing.T) {
	intent := ResponseIntent{
		Kind:           ResponseIntentInvite,
		Language:       "es",
		NeedsUserInput: true,
	}
	p := BuildExpressionProfile(intent, SemanticMeaning{}, 0.5, 0.5, 0.2, "brief")
	if p.AskFollowUp {
		t.Fatal("low initiative should suppress optional follow-up")
	}
}

func TestBuildExpressionProfilePreservesQuestionRelationship(t *testing.T) {
	intent := ResponseIntent{
		Kind:              ResponseIntentAnswer,
		AddressesQuestion: true,
	}
	p := BuildExpressionProfile(intent, SemanticMeaning{}, 0.5, 0.5, 0.8, "brief")
	if !p.PreserveQuestion {
		t.Fatal("answer should preserve its relation to the question")
	}
}
