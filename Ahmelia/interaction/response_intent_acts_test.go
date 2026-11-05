package interaction

import "testing"

func TestResponseIntentConsumesCommunicativeActs(t *testing.T) {
	cases := []struct {
		input string
		want  ResponseIntentKind
	}{
		{"Mi color favorito es negro", ResponseIntentAcknowledge},
		{"Estoy pensando en usar el cielo o la visión del jugador", ResponseIntentPropose},
		{"¿Qué opinas de esta idea?", ResponseIntentAsk},
		{"Perfecto, eso está bien", ResponseIntentAcknowledge},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			r := Analyze(tc.input, Context{})
			got := DetermineResponseIntent(r, ConversationSnapshot{Language: "es"})
			if got.Kind != tc.want {
				t.Fatalf("response intent = %q, want %q; acts=%+v kind=%q", got.Kind, tc.want, r.CommunicativeActs, r.Kind)
			}
		})
	}
}

func TestDecideResponseFeedbackHasDeterministicFallback(t *testing.T) {
	r := Analyze("Has mejorado mucho los últimos meses pero aún falta más por mejorar", Context{Language: "es"})
	d := DecideResponse(r, Context{Language: "es"})
	if !d.Handle || d.Kind != ResponseAcknowledge || d.Reply == "" {
		t.Fatalf("feedback decision=%+v", d)
	}
}
