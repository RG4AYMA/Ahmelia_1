package interaction

import "testing"

// B (2026-10-18): el análisis semántico hereda el respaldo raíz+persona del
// Ownership Resolver — la frase del propietario deja de quedar con sujeto
// vacío, y el respaldo no depende de que la oración sea pregunta.
func TestAnalyzeResolvesAddressedOwnerWhenRulesWouldVacate(t *testing.T) {
	cases := []struct {
		input string
		want  UserObservationSubject
	}{
		{"te gustaría ver mi proyecto", UserObservationSubjectSelf},
		{"¿te gustaría ver mi proyecto?", UserObservationSubjectSelf},
		{"¿podrías venir mañana?", UserObservationSubjectSelf},
		{"podré ordenar todo mañana", UserObservationSubjectUser},
	}
	for _, tc := range cases {
		r := Analyze(tc.input, Context{Language: "es"})
		if r.Subject != tc.want {
			t.Fatalf("%q subject=%q want=%q confidence=%.2f evidence=%+v", tc.input, r.Subject, tc.want, r.SubjectConfidence, r.SubjectEvidence)
		}
		if r.SubjectConfidence <= 0 || len(r.SubjectEvidence) == 0 {
			t.Fatalf("%q missing graded ownership evidence: confidence=%.2f evidence=%+v", tc.input, r.SubjectConfidence, r.SubjectEvidence)
		}
	}
}
