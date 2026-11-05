package cognition

import (
	"strings"
	"testing"
)

// TestStateManager_ArrancaEsperando confirma el reposo inicial: recién
// creado, sin ningún Process() de por medio, el estado es Esperando
// respuesta, no vacío ni Ocupada.
func TestStateManager_ArrancaEsperando(t *testing.T) {
	m := NewStateManager()
	if m.Current() != StateWaiting {
		t.Fatalf("StateManager recién creado debería estar en %q, obtuve %q", StateWaiting, m.Current())
	}
	if m.Busy() {
		t.Fatal("recién creado, sin nada pendiente, Busy() debería ser false")
	}
}

// TestStateManager_EnterCambiaElEstadoActual confirma que Enter()
// actualiza Current() a la última transición, no acumula ni promedia.
func TestStateManager_EnterCambiaElEstadoActual(t *testing.T) {
	m := NewStateManager()
	m.Enter(StateListening, "abrí chrome")
	m.Enter(StateThinking, "comprendiendo")

	if m.Current() != StateThinking {
		t.Fatalf("Current() = %q, esperaba %q", m.Current(), StateThinking)
	}
}

// TestStateManager_BusyEsTrueEnCualquierEstadoQueNoSeaEsperando confirma
// el atajo booleano: todo menos Esperando respuesta cuenta como ocupada,
// incluida Ocupada misma.
func TestStateManager_BusyEsTrueEnCualquierEstadoQueNoSeaEsperando(t *testing.T) {
	cases := []struct {
		state State
		busy  bool
	}{
		{StateWaiting, false},
		{StateBusy, true},
		{StateListening, true},
		{StateThinking, true},
		{StateExecuting, true},
		{StateLearning, true},
	}

	for _, c := range cases {
		m := NewStateManager()
		m.Enter(c.state, "")
		if got := m.Busy(); got != c.busy {
			t.Errorf("Busy() con estado %q = %v, esperaba %v", c.state, got, c.busy)
		}
	}
}

// TestStateManager_NarrativeIncluyeEstadoYDetalle confirma que la
// respuesta hablada trae el nombre del estado y, si hay, el detalle de
// qué lo disparó.
func TestStateManager_NarrativeIncluyeEstadoYDetalle(t *testing.T) {
	m := NewStateManager()
	m.Enter(StateExecuting, "abrir_programa chrome")

	n := m.Narrative()
	if !strings.Contains(n, string(StateExecuting)) {
		t.Fatalf("Narrative() = %q, debería mencionar el estado %q", n, StateExecuting)
	}
	if !strings.Contains(n, "abrir_programa chrome") {
		t.Fatalf("Narrative() = %q, debería incluir el detalle", n)
	}
}

// TestStateManager_NarrativeSinDetalleNoRompe confirma que un detalle
// vacío (como el que deja el defer de vuelta a Esperando) no deja un
// separador colgado ni rompe el formato.
func TestStateManager_NarrativeSinDetalleNoRompe(t *testing.T) {
	m := NewStateManager()
	m.Enter(StateWaiting, "")

	n := m.Narrative()
	if strings.Contains(n, ": ") {
		t.Fatalf("Narrative() = %q, no debería tener separador de detalle si el detalle está vacío", n)
	}
}

// TestParseInternalStateQuery_Positivos confirma que los triggers
// reales disparan la detección.
func TestParseInternalStateQuery_Positivos(t *testing.T) {
	positives := []string{
		"que estas haciendo",
		"en que andas",
		"estas ocupada",
		"cual es tu estado interno",
		"como estas ahora mismo",
		"que estado tenes",
	}
	for _, p := range positives {
		if !ParseInternalStateQuery(p) {
			t.Errorf("ParseInternalStateQuery(%q) = false, esperaba true", p)
		}
	}
}

// TestParseInternalStateQuery_NoPisaOtrosTriggers confirma que no
// colisiona con las metacogniciones de los bloques anteriores de esta
// misma fase (Pipeline, Attention, Working Memory, Context, Trust).
func TestParseInternalStateQuery_NoPisaOtrosTriggers(t *testing.T) {
	negatives := []string{
		"en que etapa estas",
		"que te importa ahora",
		"de que hablabamos",
		"contexto completo",
		"confias en eso",
		"abrí chrome",
	}
	for _, n := range negatives {
		if ParseInternalStateQuery(n) {
			t.Errorf("ParseInternalStateQuery(%q) = true, no debería disparar (es trigger de otro bloque)", n)
		}
	}
}
