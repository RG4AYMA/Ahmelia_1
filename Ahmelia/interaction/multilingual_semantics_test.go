package interaction

import "testing"

func TestBuildSemanticMeaningConvergesAcrossLanguages(t *testing.T) {
	es := Analyze("¿Qué es lo que hicimos?", Context{})
	en := Analyze("What did we do?", Context{})

	esMeaning := BuildSemanticMeaning(es)
	enMeaning := BuildSemanticMeaning(en)

	if esMeaning.Primary != ConceptQuestion {
		t.Fatalf("Spanish primary concept = %q, want %q", esMeaning.Primary, ConceptQuestion)
	}
	if enMeaning.Primary != ConceptQuestion {
		t.Fatalf("English primary concept = %q, want %q", enMeaning.Primary, ConceptQuestion)
	}
	if !SemanticallyEquivalent(esMeaning, enMeaning) {
		t.Fatalf("Spanish and English did not converge: %#v vs %#v", esMeaning, enMeaning)
	}
}

func TestMultilingualSemanticsKeepsCorrectionAsMeaning(t *testing.T) {
	es := BuildSemanticMeaning(Analyze("No, quería más contraste.", Context{}))
	en := BuildSemanticMeaning(Analyze("No, I meant more contrast.", Context{}))

	if es.Primary != ConceptCorrection || en.Primary != ConceptCorrection {
		t.Fatalf("correction lost: es=%q en=%q", es.Primary, en.Primary)
	}
}

func TestMultilingualSemanticsPreservesReferencesAndContinuation(t *testing.T) {
	ctx := Context{ActiveObjective: "avanzar la escena", ActiveWorkID: "scene-1"}
	r := Analyze("Continúa con eso.", ctx)
	m := BuildSemanticMeaning(r)

	if !containsSemanticConcept(m.Concepts, ConceptContinuation) {
		t.Fatal("continuation concept missing")
	}
	if !containsSemanticConcept(m.Concepts, ConceptReference) {
		t.Fatal("reference concept missing")
	}
}

func TestMultilingualSemanticsDoesNotBecomeDomainSpecific(t *testing.T) {
	r := Analyze("Create a Go project and test it.", Context{})
	m := BuildSemanticMeaning(r)

	for _, c := range m.Concepts {
		if c == SemanticConcept("programming") || c == SemanticConcept("go") {
			t.Fatalf("domain leaked into language semantics: %q", c)
		}
	}
}

func containsSemanticConcept(cs []SemanticConcept, want SemanticConcept) bool {
	for _, c := range cs {
		if c == want {
			return true
		}
	}
	return false
}
