package actioncatalog

import "testing"

func TestBuiltinActionsAreInternallyCoherent(t *testing.T) {
	defs := BuiltinActions()
	seen := make(map[string]struct{}, len(defs))
	for _, def := range defs {
		if def.Action == "" {
			t.Fatal("catalog contains an empty action")
		}
		if _, exists := seen[def.Action]; exists {
			t.Fatalf("duplicate canonical action %q", def.Action)
		}
		seen[def.Action] = struct{}{}
		if !def.Permission.IsValid() {
			t.Fatalf("action %q has invalid permission %q", def.Action, def.Permission)
		}
		switch def.Safety {
		case SafetyTierSafe, SafetyTierReversible, SafetyTierDestructive, SafetyTierSensitive:
		default:
			t.Fatalf("action %q has invalid safety tier %q", def.Action, def.Safety)
		}
		if def.StandaloneExecutable && !def.EngineWired {
			t.Fatalf("action %q is standalone executable without an engine binding", def.Action)
		}
		if def.VisibleCapability && def.Label == "" {
			t.Fatalf("visible action %q has no capability label", def.Action)
		}
	}
}

func TestDerivedViewsDoNotInventActions(t *testing.T) {
	defs := BuiltinActions()
	known := make(map[string]bool, len(defs))
	for _, def := range defs {
		known[def.Action] = true
	}
	for action := range ActionTriggers() {
		if !known[action] {
			t.Fatalf("trigger view invented unknown action %q", action)
		}
	}
	for action := range ActionLabels() {
		if !known[action] {
			t.Fatalf("label view invented unknown action %q", action)
		}
	}
	for action := range WiredActions() {
		if !known[action] {
			t.Fatalf("wired view invented unknown action %q", action)
		}
	}
	for action := range ActionSafety() {
		if !known[action] {
			t.Fatalf("safety view invented unknown action %q", action)
		}
	}
	for action := range StandaloneExecutableActions() {
		if !known[action] {
			t.Fatalf("standalone view invented unknown action %q", action)
		}
	}
}

func TestRenombrarArchivoIsOneCanonicalExecutableDefinition(t *testing.T) {
	def, ok := ActionDefinitionFor("renombrar_archivo")
	if !ok {
		t.Fatal("renombrar_archivo missing from canonical catalog")
	}
	if !def.EngineWired || !def.StandaloneExecutable {
		t.Fatalf("renombrar_archivo must expose both execution bindings: %+v", def)
	}
	if def.Safety != SafetyTierReversible {
		t.Fatalf("renombrar_archivo safety = %q, want %q", def.Safety, SafetyTierReversible)
	}
}
