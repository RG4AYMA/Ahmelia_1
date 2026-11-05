package language

import (
	"strings"
	"testing"

	"ahmelia/cognition"
	"ahmelia/solving"
)

// ============================================================================
// RESPONSE_COMPOSER_BLOQUE34_TEST.GO — Tests para Bloque 34
// ============================================================================
//
// Validar que ResponseComposer 2.0 lee correctamente ReasoningContext
// y genera decoraciones inteligentes basadas en lo que Cognitive Mind calculó.
//
// Tests acotados: enfoque en el contrato (SetReasoningContext, cognitiveMindInsight,
// decisionConfidenceHint) y cómo se integran en ComposeResponse. La lógica de
// CreatePlan y ComposeResponse (Bloque 20) ya está testeada; estos tests confirman
// que el Bloque 34 enriquece esos métodos sin romperlos.

// TestSetReasoningContext valida que ResponseComposer almacena el RC correctamente
func TestSetReasoningContext(t *testing.T) {
	rc := NewResponseComposer(nil, nil, nil, nil, nil, nil)
	
	// Inicialmente nil
	if rc.reasoningContext != nil {
		t.Error("reasoningContext debe inicializar en nil")
	}
	
	// Después de SetReasoningContext, debe tener el RC
	rctx := cognition.NewReasoningContext("¿cómo hago X?")
	rc.SetReasoningContext(rctx)
	
	if rc.reasoningContext == nil {
		t.Error("SetReasoningContext debe asignar el RC")
	}
	if rc.reasoningContext.Input != "¿cómo hago X?" {
		t.Errorf("RC.Input = %q, quería %q", rc.reasoningContext.Input, "¿cómo hago X?")
	}
}

// TestSetReasoningContextNil valida que SetReasoningContext ignora nil (no lo asigna)
func TestSetReasoningContextNil(t *testing.T) {
	rc := NewResponseComposer(nil, nil, nil, nil, nil, nil)
	rctx := cognition.NewReasoningContext("test")
	rc.SetReasoningContext(rctx)
	
	// Intentar sobrescribir con nil no debe cambiar nada
	rc.SetReasoningContext(nil)
	
	if rc.reasoningContext == nil {
		t.Error("SetReasoningContext(nil) no debe limpiar el RC previamente asignado")
	}
	if rc.reasoningContext.Input != "test" {
		t.Errorf("RC debe seguir siendo 'test', no %q", rc.reasoningContext.Input)
	}
}

// TestCognitiveMindInsightDeactivated valida que cognitiveMindInsight devuelve ""
// cuando Cognitive Mind no se activó (rc=nil o Activated=false)
func TestCognitiveMindInsightDeactivated(t *testing.T) {
	rc := NewResponseComposer(nil, nil, nil, nil, nil, nil)
	
	// rc es nil
	insight := rc.cognitiveMindInsight()
	if insight != "" {
		t.Errorf("cognitiveMindInsight con RC=nil debe devolver '', no %q", insight)
	}
	
	// rc existe pero Activated=false
	rctx := cognition.NewReasoningContext("test")
	rctx.Activated = false
	rc.SetReasoningContext(rctx)
	
	insight = rc.cognitiveMindInsight()
	if insight != "" {
		t.Errorf("cognitiveMindInsight con Activated=false debe devolver '', no %q", insight)
	}
}

// TestCognitiveMindInsightComplexity valida que insight varía según Score y Signals
func TestCognitiveMindInsightComplexity(t *testing.T) {
	rc := NewResponseComposer(nil, nil, nil, nil, nil, nil)
	rctx := cognition.NewReasoningContext("¿cómo automatizo un sistema complicado?")
	rctx.Activated = true
	
	// Caso 1: Muchas señales + puntaje alto (>= 4, >= 3 signals)
	rctx.ReasoningNeed = &solving.ReasoningNeed{
		Score: 4,
		Signals: []solving.NeedSignal{
			{Reason: "dominio reconocido: automatización", Points: 2},
			{Reason: "requisito detectado: seguridad", Points: 1},
			{Reason: "objetivo largo (12 palabras)", Points: 1},
		},
	}
	rc.SetReasoningContext(rctx)
	
	insight := rc.cognitiveMindInsight()
	if insight != "Esto requiere análisis más profundo. " {
		t.Errorf("insight con Score=4 y 3 signals debe ser 'Esto requiere análisis...', no %q", insight)
	}
	
	// Caso 2: Dos señales + puntaje 2+
	rctx.ReasoningNeed = &solving.ReasoningNeed{
		Score: 3,
		Signals: []solving.NeedSignal{
			{Reason: "dominio reconocido: programación", Points: 2},
			{Reason: "requisito detectado: rendimiento", Points: 1},
		},
	}
	insight = rc.cognitiveMindInsight()
	if insight != "Esto necesita un plan. " {
		t.Errorf("insight con Score=3 y 2 signals debe ser 'Esto necesita un plan. ', no %q", insight)
	}
	
	// Caso 3: Puntaje mínimo (2)
	rctx.ReasoningNeed = &solving.ReasoningNeed{
		Score: 2,
		Signals: []solving.NeedSignal{
			{Reason: "dominio reconocido: general", Points: 2},
		},
	}
	insight = rc.cognitiveMindInsight()
	if insight != "Esto involucra varios aspectos. " {
		t.Errorf("insight con Score=2 debe ser 'Esto involucra varios aspectos. ', no %q", insight)
	}
}

// TestDecisionConfidenceHintDeactivated valida que decisionConfidenceHint devuelve ""
// cuando Cognitive Mind no se activó
func TestDecisionConfidenceHintDeactivated(t *testing.T) {
	rc := NewResponseComposer(nil, nil, nil, nil, nil, nil)
	
	// rc es nil
	hint := rc.decisionConfidenceHint()
	if hint != "" {
		t.Errorf("decisionConfidenceHint con RC=nil debe devolver '', no %q", hint)
	}
	
	// rc existe pero Activated=false
	rctx := cognition.NewReasoningContext("test")
	rctx.Activated = false
	rc.SetReasoningContext(rctx)
	
	hint = rc.decisionConfidenceHint()
	if hint != "" {
		t.Errorf("decisionConfidenceHint con Activated=false debe devolver '', no %q", hint)
	}
}

// TestDecisionConfidenceHintLevels valida que hint varía según Verdict y RiskCount/NoLeadGaps
func TestDecisionConfidenceHintLevels(t *testing.T) {
	rc := NewResponseComposer(nil, nil, nil, nil, nil, nil)
	rctx := cognition.NewReasoningContext("test")
	rctx.Activated = true
	
	// Caso 1: VerdictSeekInfo = máxima incertidumbre
	rctx.Decision = &solving.Decision{
		Goal:       "hacer X",
		Verdict:    "pedir mas informacion antes de empezar",
		Reason:     "hay riesgos Y gaps sin pista",
		RiskCount:  2,
		NoLeadGaps: 1,
	}
	rc.SetReasoningContext(rctx)
	
	hint := rc.decisionConfidenceHint()
	if !strings.Contains(hint, "bastantes puntos grises") {
		t.Errorf("hint con VerdictSeekInfo debe mencionar puntos grises, no %q", hint)
	}
	
	// Caso 2: VerdictAdjust con riesgos
	rctx.Decision = &solving.Decision{
		Goal:       "hacer Y",
		Verdict:    "ajustar el plan",
		Reason:     "hay requisitos en riesgo",
		RiskCount:  1,
		NoLeadGaps: 0,
	}
	hint = rc.decisionConfidenceHint()
	if !strings.Contains(hint, "no es absoluta") {
		t.Errorf("hint con VerdictAdjust/RiskCount debe mencionar confianza no absoluta, no %q", hint)
	}
	
	// Caso 3: VerdictAdjust con gaps sin pista
	rctx.Decision = &solving.Decision{
		Goal:       "hacer Z",
		Verdict:    "ajustar el plan",
		Reason:     "hay gaps sin pista",
		RiskCount:  0,
		NoLeadGaps: 1,
	}
	hint = rc.decisionConfidenceHint()
	if !strings.Contains(hint, "con cuidado") {
		t.Errorf("hint con VerdictAdjust/NoLeadGaps debe mencionar cuidado, no %q", hint)
	}
	
	// Caso 4: VerdictProceed = máxima confianza
	rctx.Decision = &solving.Decision{
		Goal:       "hacer A",
		Verdict:    "avanzar",
		Reason:     "sin riesgos ni gaps",
		RiskCount:  0,
		NoLeadGaps: 0,
	}
	hint = rc.decisionConfidenceHint()
	if hint != "" {
		t.Errorf("hint con VerdictProceed debe devolver '', no %q", hint)
	}
}

// TestComposeResponseIntegration valida que ComposeResponse integra los insights de RC
func TestComposeResponseIntegration(t *testing.T) {
	rc := NewResponseComposer(nil, nil, nil, nil, nil, nil)
	rctx := cognition.NewReasoningContext("test")
	rctx.Activated = true
	rctx.ReasoningNeed = &solving.ReasoningNeed{
		Score: 4,
		Signals: []solving.NeedSignal{
			{Reason: "dominio: automatización", Points: 2},
			{Reason: "requisito: seguridad", Points: 1},
			{Reason: "objetivo largo", Points: 1},
		},
	}
	rctx.Decision = &solving.Decision{
		Goal:       "automatizar",
		Verdict:    "pedir mas informacion antes de empezar",
		Reason:     "hay riesgos y gaps",
		RiskCount:  2,
		NoLeadGaps: 1,
	}
	rc.SetReasoningContext(rctx)
	
	baseText := "Esta es una respuesta base."
	plan := ResponsePlan{}
	
	composed := rc.ComposeResponse(baseText, plan)
	
	// Debe contener los insights de Cognitive Mind
	if !contains(composed, "requiere análisis más profundo") {
		t.Errorf("ComposeResponse debe incluir insight de complejidad, got: %q", composed)
	}
	if !contains(composed, "bastantes puntos grises") {
		t.Errorf("ComposeResponse debe incluir hint de incertidumbre, got: %q", composed)
	}
}

// TestComposeResponseNoActivation valida que sin activación de Cognitive Mind
// los insights no se agregan
func TestComposeResponseNoActivation(t *testing.T) {
	rc := NewResponseComposer(nil, nil, nil, nil, nil, nil)
	rctx := cognition.NewReasoningContext("test")
	rctx.Activated = false
	rc.SetReasoningContext(rctx)
	
	baseText := "Esta es una respuesta base."
	plan := ResponsePlan{}
	
	composed := rc.ComposeResponse(baseText, plan)
	
	// No debe agregar insights de Cognitive Mind
	if contains(composed, "requiere análisis") || 
	   contains(composed, "puntos grises") ||
	   contains(composed, "confianza") {
		t.Errorf("ComposeResponse sin activación no debe agregar insights, got: %q", composed)
	}
}

// ============================================================================
// HELPER
// ============================================================================

// contains es un wrapper para strings.Contains (para brevedad en tests)
func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}
