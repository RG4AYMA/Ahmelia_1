package deliberation

import (
	"errors"
	"testing"
)

func TestEvaluateShadowOutcomes(t *testing.T) {
	harness := HarnessDecision{Action: "respond", Strategy: StrategyAnswer, Intent: "answer", Speak: true}
	ws := testWorkspace()

	tests := []struct {
		name string
		raw  string
		want Outcome
	}{
		{
			name: "acuerdo",
			raw:  `{"reading":"pregunta directa","strategy":"answer","critique":"","expectation":"la respuesta menciona la hora","evidence_ids":["w1"],"confidence":0.8,"abstain":false}`,
			want: OutcomeAgree,
		},
		{
			name: "refinamiento",
			raw:  `{"reading":"pregunta directa","strategy":"answer","critique":"falta citar el trabajo activo","expectation":"la respuesta menciona Unity","evidence_ids":["w1"],"confidence":0.8,"abstain":false}`,
			want: OutcomeRefine,
		},
		{
			name: "desacuerdo",
			raw:  `{"reading":"la referencia no está resuelta","strategy":"clarify","critique":"hay dos proyectos","expectation":"el usuario aclara cuál","evidence_ids":["w1","w2"],"confidence":0.7,"abstain":false}`,
			want: OutcomeDisagree,
		},
		{
			name: "observacion sin postura",
			raw:  `{"reading":"el turno retoma trabajo previo","strategy":"unknown","critique":"","expectation":"","evidence_ids":["w2"],"confidence":0.6,"abstain":false}`,
			want: OutcomeObservation,
		},
		{
			name: "abstencion",
			raw:  `{"reading":"","strategy":"unknown","critique":"","expectation":"","evidence_ids":[],"confidence":0,"abstain":true}`,
			want: OutcomeAbstain,
		},
		{
			name: "sin fundamento",
			raw:  `{"reading":"algo inventado","strategy":"answer","critique":"","expectation":"","evidence_ids":["w77"],"confidence":0.9,"abstain":false}`,
			want: OutcomeUngrounded,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := ParseProposal(tt.raw)
			if err != nil {
				t.Fatalf("ParseProposal: %v", err)
			}
			p.Bind(ws)
			rec := Evaluate(harness, p, "abre eso", "turn-1")
			if rec.Outcome != tt.want {
				t.Fatalf("outcome=%s, esperado %s (reasons=%v)", rec.Outcome, tt.want, rec.Reasons)
			}
			if rec.Harness.Strategy != StrategyAnswer {
				t.Fatalf("la decisión del arnés no puede alterarse: %+v", rec.Harness)
			}
		})
	}
}

func TestInvalidRecordKeepsFailureVisible(t *testing.T) {
	rec := InvalidRecord(HarnessDecision{Strategy: StrategyAnswer}, "hola", "turn-9", errors.New("json roto"))
	if rec.Outcome != OutcomeInvalid || rec.Error == "" {
		t.Fatalf("un fallo de contrato debe quedar registrado: %+v", rec)
	}
	if rec.Harness.Strategy != StrategyAnswer {
		t.Fatal("el registro inválido no debe perder la decisión del arnés")
	}
}
