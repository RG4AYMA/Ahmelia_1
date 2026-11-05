package interaction

import "testing"

func TestAnalyzeWindowSwitchIsExecutableAction(t *testing.T) {
	r := Analyze("cambia a chrome", Context{})
	if r.Kind != KindAction {
		t.Fatalf("kind=%q, want action", r.Kind)
	}
	if r.Action.Name != "cambiar_ventana" {
		t.Fatalf("action=%q, want cambiar_ventana", r.Action.Name)
	}
	if r.Action.Target != "chrome" {
		t.Fatalf("target=%q, want chrome", r.Action.Target)
	}
}

func TestAnalyzeWindowSwitchWithoutTargetIsExecutableAction(t *testing.T) {
	r := Analyze("cambia de ventana", Context{})
	if r.Kind != KindAction {
		t.Fatalf("kind=%q, want action", r.Kind)
	}
	if r.Action.Name != "cambiar_ventana" {
		t.Fatalf("action=%q, want cambiar_ventana", r.Action.Name)
	}
}
