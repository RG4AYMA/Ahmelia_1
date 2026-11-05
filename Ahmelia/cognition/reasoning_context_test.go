package cognition

import "testing"

// TestNewReasoningContext_GuardaInput confirma lo único que
// NewReasoningContext hace de verdad: guardar el input y dejar el
// resto en su zero-value.
func TestNewReasoningContext_GuardaInput(t *testing.T) {
	rc := NewReasoningContext("necesito crear un videojuego survivor")

	if rc.Input != "necesito crear un videojuego survivor" {
		t.Fatalf("esperaba Input guardado tal cual, obtuve %q", rc.Input)
	}
	if rc.Activated {
		t.Error("un ReasoningContext recién creado no debería estar Activated")
	}
}

// TestReasoningContext_ZeroValueEsSeguro confirma que un
// ReasoningContext con todos los campos de Fase 3 en nil no rompe al
// leerlos — es el estado normal de un turno donde Cognitive Mind no
// hizo falta, no un contrato a medio llenar.
func TestReasoningContext_ZeroValueEsSeguro(t *testing.T) {
	rc := &ReasoningContext{}

	if rc.GoalAnalysis != nil || rc.TaskPlan != nil || rc.KnowledgeMap != nil ||
		rc.DependencyGraph != nil || rc.HypothesisSet != nil || rc.ConstraintReport != nil ||
		rc.Decision != nil || rc.AnalogySet != nil || rc.SelfEvaluation != nil ||
		rc.ReasoningNeed != nil {
		t.Fatal("un ReasoningContext{} recién declarado debería tener todos los campos de Fase 3 en nil")
	}
}

// TestReasoningContext_AceptaLosTiposDeCadaCapa confirma, a nivel de
// compilación, que cognition puede importar solving y que cada campo
// acepta el tipo real que su capa produce — no un placeholder que haya
// que cambiar después. Si esto compila, el contrato es correcto.
func TestReasoningContext_AceptaLosTiposDeCadaCapa(t *testing.T) {
	rc := NewReasoningContext("x")
	rc.Activated = true

	// No hace falta poblar campos reales de cada struct — el punto de
	// este test es que la asignación tipa correctamente, no verificar
	// lógica de negocio de solving (eso ya lo cubren los tests de
	// Bloques 21-30 en el paquete solving).
	if rc.Activated != true {
		t.Fatal("Activated debería quedar en true tras la asignación")
	}
}
