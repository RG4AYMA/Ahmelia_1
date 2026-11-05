package memory

import (
	"testing"
	"time"
)

// TestPendingClarification_CicloCompleto confirma el ciclo completo:
// SetAskingClarification deja WaitingForClarification() en true y
// guarda lo necesario; ClearClarification vuelve a idle, borra el
// puntero y arma skipNextAsk (consumible una sola vez).
func TestPendingClarification_CicloCompleto(t *testing.T) {
	s := NewSession()

	if s.WaitingForClarification() {
		t.Fatal("sesión recién creada no debería estar esperando clarificación")
	}

	s.SetAskingClarification("aprende blender", []string{"no se detectó un dominio específico"})

	if !s.WaitingForClarification() {
		t.Fatal("después de SetAskingClarification, WaitingForClarification() debería ser true")
	}
	pc := s.GetPendingClarification()
	if pc == nil {
		t.Fatal("GetPendingClarification() no debería ser nil mientras está pendiente")
	}
	if pc.OriginalGoal != "aprende blender" {
		t.Errorf("OriginalGoal = %q, esperaba %q", pc.OriginalGoal, "aprende blender")
	}
	if len(pc.PendingAssumptions) != 1 {
		t.Errorf("PendingAssumptions = %v, esperaba 1 elemento", pc.PendingAssumptions)
	}

	s.ClearClarification()

	if s.WaitingForClarification() {
		t.Error("después de ClearClarification, WaitingForClarification() debería ser false")
	}
	if s.GetPendingClarification() != nil {
		t.Error("después de ClearClarification, GetPendingClarification() debería ser nil")
	}
}

// TestConsumeSkipNextAsk_SeConsumeUnaSolaVez confirma el flag de un
// solo uso: la primera lectura después de ClearClarification devuelve
// true (evita repetir la pregunta sobre el objetivo recién fusionado),
// cualquier lectura posterior devuelve false (turnos futuros, sin
// relación con esa clarificación, pueden volver a preguntar).
func TestConsumeSkipNextAsk_SeConsumeUnaSolaVez(t *testing.T) {
	s := NewSession()
	s.SetAskingClarification("aprende blender", nil)
	s.ClearClarification()

	if !s.ConsumeSkipNextAsk() {
		t.Fatal("primer ConsumeSkipNextAsk() después de ClearClarification debería ser true")
	}
	if s.ConsumeSkipNextAsk() {
		t.Error("segundo ConsumeSkipNextAsk() debería ser false — el flag se consume una sola vez")
	}
}

// TestReset_LimpiaPendingClarification confirma que una cancelación
// explícita (Reset, mismo camino que "olvídalo") borra también el
// estado de clarificación pendiente, no solo los otros campos de
// sesión.
func TestReset_LimpiaPendingClarification(t *testing.T) {
	s := NewSession()
	s.SetAskingClarification("aprende blender", []string{"x"})

	s.Reset()

	if s.WaitingForClarification() {
		t.Error("Reset() debería salir de stateAskingClarification")
	}
	if s.GetPendingClarification() != nil {
		t.Error("Reset() debería borrar el PendingClarification guardado")
	}
	if s.ConsumeSkipNextAsk() {
		t.Error("Reset() debería dejar skipNextAsk en false — una cancelación explícita no es una clarificación resuelta")
	}
}

// TestRefreshTimeout_ReiniciaElContador (Integration Fix) documenta el
// invariante del que dependía un bug real: SetAskingClarification()
// marca stateSetAt ANTES de que la pregunta se hable por TTS. Si nadie
// llama RefreshTimeout() después de que el TTS termina, el reloj de
// los 45s del timeout ya incluye el tiempo de habla — con una
// pregunta larga más un par de intentos fallidos de reconocimiento de
// voz, la sesión podía expirar sin que el usuario hubiera tenido
// tiempo real de responder (visto en vivo: "Estado cancelado por
// timeout" con el usuario respondiendo activamente). Se simula el
// tiempo transcurrido escribiendo stateSetAt directamente (mismo
// package, campo no exportado) en vez de dormir de verdad, para que
// el test sea instantáneo y determinístico.
func TestRefreshTimeout_ReiniciaElContador(t *testing.T) {
	s := NewSession()
	s.SetAskingClarification("aprende blender", []string{"x"})

	s.stateSetAt = time.Now().Add(-sessionTimeout - time.Second)
	if !s.IsExpired() {
		t.Fatal("sanity check: sin RefreshTimeout(), con stateSetAt viejo la sesión debería estar expirada")
	}

	s.RefreshTimeout()
	if s.IsExpired() {
		t.Error("después de RefreshTimeout(), la sesión no debería estar expirada aunque el stateSetAt original ya hubiera superado el timeout")
	}
}
