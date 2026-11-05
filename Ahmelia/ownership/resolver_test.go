package ownership

import "testing"

func TestResolveOwnershipGeneralizesAcrossVocabularyWithoutQuestionMark(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  Subject
	}{
		{"color question", "tu color favorito", SubjectSelf},
		{"color declaration", "tranquila pronto sabras tu color favorito", SubjectSelf},
		{"animal question", "tu animal favorito", SubjectSelf},
		{"animal future", "algun dia tendras tu propio animal favorito", SubjectSelf},
		{"name", "tu nombre", SubjectSelf},
		{"purpose", "tu proposito", SubjectSelf},
		{"user color", "mi color favorito", SubjectUser},
		{"user animal", "mi animal favorito", SubjectUser},
		{"user name", "mi nombre", SubjectUser},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Resolve(tc.input)
			if got.Subject != tc.want {
				t.Fatalf("%q subject=%q want=%q resolution=%+v", tc.input, got.Subject, tc.want, got)
			}
			if got.Confidence <= 0 {
				t.Fatalf("%q confidence=%v want > 0", tc.input, got.Confidence)
			}
			if len(got.Evidence) == 0 {
				t.Fatalf("%q produced no ownership evidence", tc.input)
			}
		})
	}
}

func TestResolveOwnershipUsesPossessiveReferentInsideInterrogativeComplement(t *testing.T) {
	cases := []struct {
		input string
		want  Subject
	}{
		{"sabes cual es mi color favorito", SubjectUser},
		{"tu sabes cual es mi bebida favorita", SubjectUser},
		{"sabes cual es tu color favorito", SubjectSelf},
		{"do you know what is my name", SubjectUser},
		{"do you know which is your favorite color", SubjectSelf},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			got := Resolve(tc.input)
			if got.Subject != tc.want {
				t.Fatalf("subject=%q want=%q resolution=%+v", got.Subject, tc.want, got)
			}
			if got.Ambiguous {
				t.Fatalf("query referent should not remain ambiguous: %+v", got)
			}
		})
	}
}

func TestResolveOwnershipNegativeControlsAndConflict(t *testing.T) {
	cases := []struct {
		name      string
		input     string
		ambiguous bool
	}{
		{"ownerless statement", "el sistema esta listo", false},
		{"unrelated question", "que esta pasando", false},
		{"mixed ownership", "me gusta tu color", true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Resolve(tc.input)
			if got.Subject != SubjectUnknown {
				t.Fatalf("%q subject=%q want unknown; resolution=%+v", tc.input, got.Subject, got)
			}
			if got.Ambiguous != tc.ambiguous {
				t.Fatalf("%q ambiguous=%t want=%t; resolution=%+v", tc.input, got.Ambiguous, tc.ambiguous, got)
			}
		})
	}
}

func TestResolveOwnershipDoesNotDependOnQuestionMark(t *testing.T) {
	withoutQuestion := Resolve("tu proposito")
	withQuestion := Resolve("tu proposito?")
	if withoutQuestion.Subject != SubjectSelf || withQuestion.Subject != SubjectSelf {
		t.Fatalf("question gate regressed: without=%+v with=%+v", withoutQuestion, withQuestion)
	}
}

func TestResolveOwnershipPreservesFirstPersonQueriesWithoutDomainVocabulary(t *testing.T) {
	cases := []string{
		"que nivel tengo en blender",
		"que se sobre blender",
		"que nivel tengo en una capacidad distinta",
	}
	for _, input := range cases {
		got := Resolve(input)
		if got.Subject != SubjectUser {
			t.Fatalf("%q subject=%q want user; resolution=%+v", input, got.Subject, got)
		}
	}
}
