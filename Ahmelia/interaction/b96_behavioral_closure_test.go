package interaction

import "testing"

// B96.32-B96.39 closure invariant: legacy RequestKind/SpeechAct projections
// may be present for persistence/compatibility, but changing them after a
// live turn has been semantically classified must not change the canonical
// communicative act or response intent.
func TestBehavioralClosure_LegacyProjectionCannotOverrideCanonicalActs(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  CommunicativeAct
	}{
		{"question", "¿cuál es mi color favorito?", ActQuestion},
		{"declaration", "mi color favorito es negro", ActDeclaration},
		{"feedback", "perfecto, eso quedó bien", ActFeedback},
		{"correction", "no, me refería al segundo archivo", ActCorrection},
		{"proposal", "podríamos mejorar este proyecto", ActProposal},
		{"idea", "tengo una idea para el proyecto", ActIdea},
		{"exploration", "estoy pensando en cambiar el sistema", ActExploration},
		{"confirmation", "sí, exactamente", ActConfirmation},
		{"rejection", "no", ActRejection},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := Analyze(tc.input, Context{})
			before := r.CommunicativeActs
			r.Kind = KindAction
			r.SpeechAct = SpeechCommand
			if got := CanonicalActs(r); got.Primary != before.Primary || !got.Has(tc.want) {
				t.Fatalf("legacy projection changed canonical act: before=%+v after=%+v", before, got)
			}
		})
	}
}

func TestBehavioralClosure_CompoundActIsExplicit(t *testing.T) {
	r := Analyze("sí, hazlo, pero primero cambia el nombre", Context{})
	if !r.CommunicativeActs.Has(ActConfirmation) || !r.CommunicativeActs.Has(ActCommand) {
		t.Fatalf("expected confirmation + command, got %+v", r.CommunicativeActs)
	}
	if !r.CommunicativeActs.Has(ActCompound) {
		t.Fatalf("expected explicit compound act, got %+v", r.CommunicativeActs)
	}
}

func TestBehavioralClosure_IdeaNeverBecomesCommand(t *testing.T) {
	for _, input := range []string{
		"estoy pensando en modificar el sistema",
		"tengo una idea para cambiar el proyecto",
		"podríamos mejorar los enemigos",
	} {
		r := Analyze(input, Context{})
		if r.CommunicativeActs.Has(ActCommand) {
			t.Fatalf("idea/exploration became command: input=%q acts=%+v", input, r.CommunicativeActs)
		}
	}
}
