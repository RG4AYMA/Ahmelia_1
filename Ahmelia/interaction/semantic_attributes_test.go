package interaction

import (
	"testing"
	"time"
)

func TestSemanticAttributeReferenceSupportsUnseenUserAttributes(t *testing.T) {
	cases := []struct {
		name  string
		text  string
		key   string
		value string
	}{
		{"name", "Mi nombre es Adrian Yesid Marin Angulo", "nombre", "Adrian Yesid Marin Angulo"},
		{"city", "Mi ciudad es Barranquilla", "ciudad", "Barranquilla"},
		{"profession", "Mi profesión es desarrollador", "profesion", "desarrollador"},
		{"age", "Mi edad es 17", "edad", "17"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := Analyze(tc.text, Context{Language: "es"})
			req = InferSemanticRelations(req)
			ref, ok := ResolveSemanticAttribute(req)
			if !ok || !ref.HasValue || ref.Subject != UserObservationSubjectUser {
				t.Fatalf("expected generic user declaration, got %+v ok=%v", ref, ok)
			}
			if ref.Key != tc.key || ref.Value != tc.value {
				t.Fatalf("unexpected reference: %+v", ref)
			}
			obs := DetectUserObservationsFromSemantic(req, time.Now(), "test")
			if len(obs) != 1 || obs[0].Key != tc.key || obs[0].Value != tc.value || obs[0].Domain != UserModelDomainAttribute {
				t.Fatalf("unexpected observation: %+v", obs)
			}
		})
	}
}

func TestSemanticAttributeQueryUsesSameKeyForUserAndSelf(t *testing.T) {
	cases := []struct {
		name    string
		text    string
		subject UserObservationSubject
		key     string
	}{
		{"user-name", "¿Cuál es mi nombre?", UserObservationSubjectUser, "nombre"},
		{"user-city", "¿Cuál es mi ciudad?", UserObservationSubjectUser, "ciudad"},
		{"self-drink", "¿Cuál es tu bebida favorita?", UserObservationSubjectSelf, "bebida favorita"},
		{"self-color", "¿Tú tienes color favorito?", UserObservationSubjectSelf, "color favorito"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := Analyze(tc.text, Context{Language: "es"})
			req = InferSemanticRelations(req)
			ref, ok := ResolveSemanticAttribute(req)
			if !ok || ref.HasValue {
				t.Fatalf("expected attribute query, got %+v ok=%v", ref, ok)
			}
			if ref.Subject != tc.subject || ref.Key != tc.key {
				t.Fatalf("unexpected ownership/key: %+v", ref)
			}
		})
	}
}

func TestSemanticAttributeQuerySupportsKnowledgeQuestionsWithInterrogativeComplement(t *testing.T) {
	cases := []struct {
		name    string
		text    string
		subject UserObservationSubject
		key     string
	}{
		{"user-color", "¿Sabes cuál es mi color favorito?", UserObservationSubjectUser, "color favorito"},
		{"user-drink", "¿Sabes cuál es mi bebida favorita?", UserObservationSubjectUser, "bebida favorita"},
		{"user-name", "¿Sabes cuál es mi nombre?", UserObservationSubjectUser, "nombre"},
		{"self-color", "¿Sabes cuál es tu color favorito?", UserObservationSubjectSelf, "color favorito"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := Analyze(tc.text, Context{Language: "es"})
			ref, ok := ResolveSemanticAttribute(req)
			if !ok || ref.HasValue {
				t.Fatalf("expected generic attribute query, got %+v ok=%v", ref, ok)
			}
			if ref.Subject != tc.subject || ref.Key != tc.key {
				t.Fatalf("unexpected ownership/key: %+v", ref)
			}
			if query, ok := DetectUserModelQuery(req); tc.subject == UserObservationSubjectUser {
				if !ok || query.Topics[0] != tc.key {
					t.Fatalf("expected User Model query for %q, got %+v ok=%v", tc.text, query, ok)
				}
			} else if ok {
				t.Fatalf("self query must not enter User Model: %+v", query)
			}
		})
	}
}

func TestSemanticAttributeDoesNotPromoteMixedOwnershipToSelf(t *testing.T) {
	for _, text := range []string{
		"¿Tú tienes alguna noción de mi bebida favorita?",
		"¿Tú tienes idea de mi color favorito?",
	} {
		req := Analyze(text, Context{Language: "es"})
		if ref, ok := ResolveSemanticAttribute(req); ok {
			t.Fatalf("mixed ownership question must not be forced into a SELF attribute by fast grammar: text=%q ref=%+v", text, ref)
		}
	}
}

func TestSemanticPreferenceDomainIsGeneric(t *testing.T) {
	for _, key := range []string{"color favorito", "bebida favorita", "dulce favorito", "libro favorito", "estilo preferido"} {
		if got := SemanticAttributeDomainForKey(key); got != SemanticAttributeDomainPreference {
			t.Fatalf("%q should be a preference domain, got %q", key, got)
		}
	}
	if got := SemanticAttributeDomainForKey("ciudad"); got != SemanticAttributeDomainAttribute {
		t.Fatalf("city must remain generic attribute, got %q", got)
	}
}

func TestSelfQuestionCannotBecomeUserObservation(t *testing.T) {
	req := Analyze("¿Tú tienes color favorito?", Context{Language: "es"})
	req = InferSemanticRelations(req)
	if got := DetectUserObservationsFromSemantic(req, time.Now(), "test"); got != nil {
		t.Fatalf("self-owned question must not become a user observation: %+v", got)
	}
	if _, ok := DetectUserModelQuery(req); ok {
		t.Fatal("self-owned question must not enter UserModel query path")
	}
}
