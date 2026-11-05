package ownership

import "testing"

// B (2026-10-18) — respaldo por raíz+persona cuando el SEMANTIC de reglas
// vacía: construcciones de afección resueltas por el clítico de dativo, y
// formas verbales por raíz + persona que las listas completas no enumeran.
// Control negativo: "me gusta tu color" conserva su abstención.

func TestResolveAffectionAddressingKeepsQuestionOwnerResolvable(t *testing.T) {
	cases := []string{
		"te gustaría ver mi proyecto",
		"te gustaría que te cuente mis ideas",
		"te importaría que use mi cuaderno",
	}
	for _, input := range cases {
		got := Resolve(input)
		if got.Subject != SubjectSelf || got.Ambiguous {
			t.Fatalf("%q subject=%q ambiguous=%t want self; resolution=%+v", input, got.Subject, got.Ambiguous, got)
		}
		if got.Confidence <= 0 || len(got.Evidence) == 0 {
			t.Fatalf("%q missing graded evidence: %+v", input, got)
		}
	}
}

func TestResolveAffectionAddressingPreservesFirstPersonAbstention(t *testing.T) {
	got := Resolve("me gusta tu color")
	if got.Subject != SubjectUnknown || !got.Ambiguous {
		t.Fatalf("la abstención de primera persona se rompió: %+v", got)
	}
}

func TestResolveRootPersonVerbForms(t *testing.T) {
	self := []string{
		"podrias acompanarme manana",
		"tendrias un momento hoy",
		"que haras despues",
	}
	for _, input := range self {
		if got := Resolve(input); got.Subject != SubjectSelf {
			t.Fatalf("%q subject=%q want self; resolution=%+v", input, got.Subject, got)
		}
	}
	user := []string{
		"podre terminar el informe",
		"hare todo lo posible",
	}
	for _, input := range user {
		if got := Resolve(input); got.Subject != SubjectUser {
			t.Fatalf("%q subject=%q want user; resolution=%+v", input, got.Subject, got)
		}
	}
}
