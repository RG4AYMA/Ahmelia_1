package interaction

import "testing"

func TestShouldFinalizeObjectiveDraftRequiresExplicitHandoff(t *testing.T) {
	cases := []struct {
		text string
		want bool
	}{
		{"Crea una función Go llamada IsEven y añade tests.", false},
		{"Condiciones: trabaja dentro del workspace y verifica el resultado.", false},
		{"No me informes de éxito hasta haber verificado el resultado.", false},
		{"Adelante, hazlo.", true},
		{"Puedes hacerlo.", true},
		{"Ejecuta ahora.", true},
	}
	for _, tc := range cases {
		r := Parse(tc.text, ChannelVoice)
		if got := ShouldFinalizeObjectiveDraft(r); got != tc.want {
			t.Fatalf("%q: got %v want %v", tc.text, got, tc.want)
		}
	}
}
