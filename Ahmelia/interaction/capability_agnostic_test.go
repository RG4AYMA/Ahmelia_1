package interaction

import "testing"

func TestResolveCapabilityInteractionIsProviderNeutral(t *testing.T) {
	r := Analyze("abre Chrome", Context{Language: "es"})
	got := ResolveCapabilityInteraction(r)
	if !got.Executable {
		t.Fatalf("expected executable request: %+v", got)
	}
	if got.Request.Operation == "" || got.Request.Target != "chrome" {
		t.Fatalf("unexpected request: %+v", got.Request)
	}
	if got.Request.Source != "interaction" {
		t.Fatalf("unexpected source: %+v", got.Request)
	}
}

func TestResolveCapabilityInteractionDoesNotTurnConversationIntoCapability(t *testing.T) {
	r := Analyze("hoy quiero hablar de otra cosa", Context{Language: "es"})
	got := ResolveCapabilityInteraction(r)
	if got.Executable {
		t.Fatalf("conversation became executable: %+v", got)
	}
	if got.Request.HasOperation() {
		t.Fatalf("unexpected operation: %+v", got.Request)
	}
}

func TestToCapabilityRequestDoesNotSelectProvider(t *testing.T) {
	a := ActionIntent{Name: "future.operation", Target: "thing", Confidence: 0.81}
	got := ToCapabilityRequest(a, "do the thing")
	if got.Operation != "future.operation" || got.Target != "thing" || got.Confidence != 0.81 {
		t.Fatalf("unexpected request: %+v", got)
	}
	if got.Parameters["raw"] != "do the thing" {
		t.Fatalf("raw parameter missing: %+v", got.Parameters)
	}
}
