package classifier

import "testing"

func TestB96223EnglishProgramTargetExtraction(t *testing.T) {
	cases := []struct {
		input  string
		target string
	}{
		{"open unity", "unity"},
		{"Open Unity.", "unity"},
		{"launch unity", "unity"},
		{"run unity", "unity"},
		{"start chrome", "chrome"},
	}
	for _, tc := range cases {
		got := ClassifyCommand(tc.input)
		if got.Action != "abrir_programa" {
			t.Errorf("%q: action=%q, want abrir_programa", tc.input, got.Action)
		}
		if got.Target != tc.target {
			t.Errorf("%q: target=%q, want %q", tc.input, got.Target, tc.target)
		}
	}
}

func TestB96223TargetExtractionDoesNotDependOnLanguage(t *testing.T) {
	cases := map[string]string{
		"abre unity": "unity",
		"abrir unity": "unity",
		"open unity": "unity",
		"launch unity": "unity",
		"run unity": "unity",
	}
	for input, want := range cases {
		got := ClassifyCommand(input)
		if got.Target != want {
			t.Errorf("%q: target=%q, want %q", input, got.Target, want)
		}
	}
}
