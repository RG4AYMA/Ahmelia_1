package interaction

import "testing"

func TestObjectiveDraftStaysOutsideOperationalObjectiveUntilComplete(t *testing.T) {
	s := NewConversationState()
	draft := s.BeginObjectiveDraft("Quiero que implementes una utilidad de programación.", 1)
	if draft == nil || draft.Text == "" {
		t.Fatalf("expected draft, got %+v", draft)
	}
	if s.Snapshot().ActiveObjective != "" {
		t.Fatalf("incomplete delegation leaked into ActiveObjective: %q", s.Snapshot().ActiveObjective)
	}
	if got, ok := s.PendingObjectiveDraft(); !ok || got.Text != draft.Text {
		t.Fatalf("pending draft=%+v ok=%t", got, ok)
	}
}

func TestObjectiveDraftAssemblyIgnoresStructuralLabels(t *testing.T) {
	s := NewConversationState()
	s.BeginObjectiveDraft("Quiero que implementes una utilidad de programación.", 1)
	s.AppendObjectiveDraft("Objetivo:")
	draft, _ := s.PendingObjectiveDraft()
	if draft.Text != "Quiero que implementes una utilidad de programación." {
		t.Fatalf("label changed draft: %q", draft.Text)
	}
	s.AppendObjectiveDraft("Crea una función Go llamada IsEven.")
	draft, _ = s.PendingObjectiveDraft()
	if draft.Text != "Quiero que implementes una utilidad de programación.\nCrea una función Go llamada IsEven." {
		t.Fatalf("unexpected assembled draft: %q", draft.Text)
	}
}

func TestShouldAppendObjectiveDraftIsDomainNeutral(t *testing.T) {
	for _, input := range []string{
		"Objetivo:",
		"Crea una función Go llamada IsEven.",
		"Ejecuta los tests.",
		"Verifica el resultado.",
		"No me informes de éxito hasta verificarlo.",
	} {
		if !ShouldAppendObjectiveDraft(Analyze(input, Context{})) {
			t.Fatalf("expected objective fragment for %q", input)
		}
	}
	for _, input := range []string{"¿qué llevamos?", "busca información sobre Go", "cancela"} {
		if ShouldAppendObjectiveDraft(Analyze(input, Context{})) {
			t.Fatalf("unexpected objective fragment for independent turn %q", input)
		}
	}
}
