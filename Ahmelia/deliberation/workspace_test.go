package deliberation

import (
	"strings"
	"testing"
)

func TestWorkspaceAssignsStableIDsAndRenders(t *testing.T) {
	var ws Workspace
	ws.Add("turno_actual", "abre eso", "engine", 1)
	ws.Add("interpretacion", "el usuario pide continuar el trabajo", "linguistics", 0.8)

	if ws.Empty() {
		t.Fatal("el workspace con elementos no puede estar vacío")
	}
	if !ws.HasID("w1") || !ws.HasID("w2") || ws.HasID("w3") {
		t.Fatalf("IDs inestables: %+v", ws.Items)
	}
	out := ws.Render()
	for _, frag := range []string{"[w1]", "[w2]", "turno_actual", "abre eso"} {
		if !strings.Contains(out, frag) {
			t.Fatalf("el render pierde %q: %q", frag, out)
		}
	}
	if strings.Index(out, "[w1]") > strings.Index(out, "[w2]") {
		t.Fatalf("el render debe ser determinista: %q", out)
	}
}

func TestWorkspaceIsBounded(t *testing.T) {
	var ws Workspace
	for i := 0; i < maxWorkspaceItems+5; i++ {
		ws.Add("hecho", strings.Repeat("x", maxItemRunes+40), "test", 1)
	}
	if len(ws.Items) != maxWorkspaceItems {
		t.Fatalf("el workspace debe respetar el tope: %d", len(ws.Items))
	}
	out := ws.Render()
	if runes := len([]rune(out)); runes > maxWorkspaceRunes+5 {
		t.Fatalf("el render excede el presupuesto: %d runas", runes)
	}
}

func TestWorkspaceEmptyAndNil(t *testing.T) {
	var nilWS *Workspace
	if !nilWS.Empty() || nilWS.Render() != "" || nilWS.HasID("w1") {
		t.Fatal("un workspace nil debe ser inerte")
	}
	var zero Workspace
	if !zero.Empty() {
		t.Fatal("un workspace sin elementos debe estar vacío")
	}
	zero.Add("", "texto sin tipo", "", 1)
	if len(zero.Items) != 1 || zero.Items[0].Kind != "" {
		t.Fatalf("Add debe tolerar tipo vacío: %+v", zero.Items)
	}
	zero.Add("hecho", "   ", "", 1)
	if len(zero.Items) != 1 {
		t.Fatal("Add no debe aceptar texto vacío")
	}
}
