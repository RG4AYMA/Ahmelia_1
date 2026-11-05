package interaction

import "testing"

func TestCommunicativeActsUniversalFamilies(t *testing.T) {
	cases := []struct {
		name      string
		input     string
		primary   CommunicativeAct
		secondary CommunicativeAct
	}{
		{"question", "¿Cuál es mi color favorito?", ActQuestion, ""},
		{"declaration", "Mi color favorito es negro", ActDeclaration, ActInform},
		{"observation", "Estoy viendo dos opciones y todavía no me decido", ActExploration, ActInform},
		{"idea", "Tengo una idea para este proyecto", ActIdea, ActInform},
		{"proposal", "Te propongo revisar esta parte", ActProposal, ""},
		{"feedback", "Perfecto, eso quedó bien", ActFeedback, ""},
		{"correction", "No, me refería al segundo archivo", ActCorrection, ""},
		{"confirmation", "Sí, exactamente", ActConfirmation, ""},
		{"explicit authorization", "autorizo la ejecucion de la funcion de Go que esta pendiente", ActConfirmation, ""},
		{"authorization with conversational preamble and clitic", "bueno entonces la autorizo explicitamente", ActConfirmation, ""},
		{"affirmative confirmation before authorization", "sí, autorizo la ejecución", ActConfirmation, ""},
		{"rejection", "No", ActRejection, ""},
		{"opinion", "¿Qué opinas de esta idea?", ActOpinionRequest, ActQuestion},
		{"decision", "Decidí seguir con esta opción", ActDecision, ActInform},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := Analyze(tc.input, Context{})
			if r.CommunicativeActs.Primary != tc.primary {
				t.Fatalf("primary act = %q, want %q; kind=%q speech=%q", r.CommunicativeActs.Primary, tc.primary, r.Kind, r.SpeechAct)
			}
			if tc.secondary != "" && !r.CommunicativeActs.Has(tc.secondary) {
				t.Fatalf("secondary act %q missing; acts=%+v", tc.secondary, r.CommunicativeActs)
			}
		})
	}
}

func TestConditionalAuthorizationIsNotConfirmation(t *testing.T) {
	r := Analyze("Si autorizo, la ejecucion", Context{})
	if r.CommunicativeActs.Has(ActConfirmation) {
		t.Fatalf("conditional authorization was treated as a grant: %+v", r.CommunicativeActs)
	}
	if !r.CommunicativeActs.Has(ActAuthorizationInquiry) {
		t.Fatalf("conditional authorization was not retained as an inquiry: %+v", r.CommunicativeActs)
	}
}

func TestInterrogativeOrConditionalAuthorizationNeverGrants(t *testing.T) {
	for _, input := range []string{
		"¿Autorizo la ejecucion?",
		"¿Entonces la autorizo?",
		"Autorizo la ejecucion si se cumplen las condiciones",
		"Bueno, entonces la autorizo si se cumplen las condiciones",
		"No autorizo la ejecución",
		"No sé si autorizo la ejecución",
	} {
		r := Analyze(input, Context{})
		if r.CommunicativeActs.Has(ActConfirmation) {
			t.Errorf("non-performative authorization was treated as a grant: %q acts=%+v", input, r.CommunicativeActs)
		}
	}
}
func TestCommunicativeActsNeverAuthorizesExecution(t *testing.T) {
	r := Analyze("Estoy pensando en modificar el sistema de enemigos", Context{})
	if r.CommunicativeActs.Primary != ActExploration && r.CommunicativeActs.Primary != ActIdea && r.CommunicativeActs.Primary != ActProposal {
		t.Fatalf("exploratory statement classified as %q", r.CommunicativeActs.Primary)
	}
	if r.CommunicativeActs.Has(ActCommand) {
		t.Fatalf("exploration must not acquire command act: %+v", r.CommunicativeActs)
	}
}

func TestCommunicativeActsUnknownIsStillAClassificationResult(t *testing.T) {
	r := Analyze("asdf qwer zxcv", Context{})
	if r.CommunicativeActs.Primary == "" {
		t.Fatal("classification must always expose a primary act, including unknown")
	}
}
