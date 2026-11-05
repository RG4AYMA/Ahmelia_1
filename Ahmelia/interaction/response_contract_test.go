package interaction

import "testing"

func TestResponseContractCarriesSubjectAndMove(t *testing.T) {
	r := Analyze("¿Tú tienes color favorito?", Context{Language: "es"})
	intent := DetermineResponseIntent(r, ConversationSnapshot{Language: "es"})
	c := NewResponseContract(r, intent, MoveAnswer)
	if !c.Valid() {
		t.Fatalf("expected valid response contract: %+v", c)
	}
	if c.Subject != UserObservationSubjectSelf {
		t.Fatalf("subject=%q, want self", c.Subject)
	}
	if c.ShouldAct {
		t.Fatal("self question must not become an action")
	}
}

func TestResponseContractRejectsUnrelatedEvidence(t *testing.T) {
	r := Analyze("Bien hecho", Context{Language: "es"})
	intent := DetermineResponseIntent(r, ConversationSnapshot{Language: "es"})
	c := NewResponseContract(r, intent, MoveAcknowledge)
	c.AddEvidence("Tu color favorito es negro.", "user_model", false, 1)
	if len(c.Evidence) != 0 {
		t.Fatalf("unrelated evidence leaked into response contract: %+v", c.Evidence)
	}
	c.AddEvidence("La evaluación del usuario fue positiva.", "conversation", true, .9)
	if len(c.Evidence) != 1 {
		t.Fatalf("relevant evidence was rejected: %+v", c.Evidence)
	}
}
