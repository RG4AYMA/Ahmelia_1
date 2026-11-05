package interaction

import "testing"

func TestSemanticProductionBeatsGreetingLiteral(t *testing.T) {
	r := Analyze(`Crea un pequeño programa Go llamado prueba_contexto que muestre "Hola contexto" y añade un test.`, Context{Language: "es"})
	if r.Kind != KindProduction {
		t.Fatalf("kind=%q, want %q", r.Kind, KindProduction)
	}
	if r.RequestedAction != "create" || r.ExpectedResponse != "result" {
		t.Fatalf("action=%q response=%q", r.RequestedAction, r.ExpectedResponse)
	}
}

func TestSemanticActionUnderstandsEnglishProgramTarget(t *testing.T) {
	r := Analyze("Open Unity.", Context{Language: "en"})
	if r.Kind != KindAction {
		t.Fatalf("kind=%q, want %q", r.Kind, KindAction)
	}
	if r.Action.Name != "abrir_programa" || r.Action.Target != "unity" {
		t.Fatalf("action=%q target=%q", r.Action.Name, r.Action.Target)
	}
}

func TestSemanticFollowUpKeepsActiveObjective(t *testing.T) {
	r := Analyze("Quiero que la función acepte también números negativos.", Context{
		ActiveObjective: "Crea un programa Go llamado trabajo_a con una función de suma y un test.",
		Language:        "es",
	})
	if !r.AddressesCurrent || !r.Continues {
		t.Fatalf("context lost: current=%v continues=%v", r.AddressesCurrent, r.Continues)
	}
	if r.Objective.Text == "" {
		t.Fatal("active objective was lost")
	}
	if r.RequestedAction != "modify" {
		t.Fatalf("requested action=%q, want modify", r.RequestedAction)
	}
}

func TestSemanticMeaningQuestionIsQuestion(t *testing.T) {
	r := Analyze(`Explícame qué significa "abre Unity".`, Context{Language: "es"})
	if r.Kind != KindQuestion {
		t.Fatalf("kind=%q, want %q", r.Kind, KindQuestion)
	}
	if r.RequestedAction != "answer" {
		t.Fatalf("requested action=%q, want answer", r.RequestedAction)
	}
}

func TestSemanticProhibitionDoesNotBecomeOpenAction(t *testing.T) {
	r := Analyze("No quiero que abras ningún programa.", Context{Language: "es"})
	if r.Kind != KindPause {
		t.Fatalf("kind=%q, want %q", r.Kind, KindPause)
	}
	if r.Action.Name != "" {
		t.Fatalf("unexpected executable action=%q", r.Action.Name)
	}
}
