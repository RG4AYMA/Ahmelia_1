package interaction

import "testing"

func TestSolveQuestionArithmetic(t *testing.T) {
	cases := map[string]string{
		"2 + 2":               "4",
		"¿cuánto es 2 + 2?":   "4",
		"calcula (2 + 3) * 4": "20",
		"10 / 2 + 7":          "12",
	}
	for input, want := range cases {
		r := Analyze(input, Context{})
		got, ok := SolveQuestion(r)
		if !ok || got != want {
			t.Fatalf("%q => %q/%v, want %q", input, got, ok, want)
		}
	}
}

func TestSolveQuestionRejectsNaturalLanguageAsArithmetic(t *testing.T) {
	r := Analyze("¿por qué el cielo es azul?", Context{})
	if _, ok := SolveQuestion(r); ok {
		t.Fatal("natural-language knowledge question must not be evaluated as arithmetic")
	}
}
