package interaction

import "testing"

func TestSemanticClassifier_FeedbackDoesNotTriggerOnGenericWord(t *testing.T) {
	cases := []struct {
		input string
		want  bool
	}{
		{"no redacté bien", false},
		{"no redacte bien", false},
		{"el documento quedó bien estructurado", false},
		{"esta mejorando pero aun, faltan pequeñas mejoras", true},
		{"está mejorando pero aún faltan pequeñas mejoras", true},
		{"perfecto, eso quedó bien", true},
		{"eso está mal", true},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			if got := isFeedback(normalize(tc.input)); got != tc.want {
				t.Fatalf("isFeedback(%q) = %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}

func TestSemanticClassifier_LearningRequiresExplicitMemoryIntent(t *testing.T) {
	cases := []struct {
		input string
		want  bool
	}{
		{"esto es importante", false},
		{"tienes que revisar esto", false},
		{"deberías mejorar la respuesta", false},
		{"recuerda que mi color favorito es negro", true},
		{"a partir de ahora usa este formato", true},
		{"ten en cuenta esta preferencia", true},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			if got := isLearning(normalize(tc.input)); got != tc.want {
				t.Fatalf("isLearning(%q) = %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}

func TestSemanticClassifier_WorkContinuationRequiresWorkEvidence(t *testing.T) {
	ctx := Context{ActiveObjective: "mejorar el proyecto"}
	cases := []struct {
		input string
		want  bool
	}{
		{"también estoy cansado", false},
		{"además me gusta este enfoque", false},
		{"también quiero que la función acepte negativos", true},
		{"además modifica el proyecto", true},
		{"quiero que la función sea más rápida", true},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			r := Analyze(tc.input, ctx)
			if got := r.AddressesCurrent; got != tc.want {
				t.Fatalf("Analyze(%q).AddressesCurrent = %v, want %v; kind=%q", tc.input, got, tc.want, r.Kind)
			}
		})
	}
}

func TestSemanticClassifier_ComoIsNotGenericQuestionTrigger(t *testing.T) {
	cases := []struct {
		input string
		want  bool
	}{
		{"hazlo como te dije", false},
		{"trabaja como siempre", false},
		{"como funciona esto", true},
		{"como puedo arreglarlo", true},
		{"¿como funciona esto?", true},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			if got := isQuestion(normalize(tc.input)); got != tc.want {
				t.Fatalf("isQuestion(%q) = %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}
