package interaction

import "testing"

func TestSemanticRequestCarriesCommunicationMeaning(t *testing.T) {
	r := Analyze("No, me refería al segundo diseño.", Context{
		ActiveObjective: "comparar diseños",
	})
	if r.Kind != KindCorrection {
		t.Fatalf("kind=%q, want correction", r.Kind)
	}
	if r.SpeechAct != SpeechCorrect {
		t.Fatalf("speech act=%q, want correct", r.SpeechAct)
	}
	if r.Directness == "" {
		t.Fatal("directness should be populated")
	}
}
