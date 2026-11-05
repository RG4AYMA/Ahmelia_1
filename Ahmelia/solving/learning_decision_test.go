package solving

import (
	"testing"
)

// TestDecide_NoAprendeSiHayCapabilityGap verifica que NO se vaya a Internet si el motor no sabe ejecutar la acción.
func TestDecide_NoAprendeSiHayCapabilityGap(t *testing.T) {
	gap := &GapAnalysis{Type: CapabilityGap}
	ctx := LearningContext{
		Action:        "reproducir_audio",
		IsOnline:      true,
		WorthLearning: true,
		Gap:           gap,
	}

	dec := Decide(ctx)
	if dec.Type != DecisionReportCapabilityGap {
		t.Errorf("esperaba DecisionReportCapabilityGap, dio %q", dec.Type)
	}
}

// TestDecide_PriorizaRiesgoAlto verifica que si hay alto riesgo pida confirmación antes de investigar.
func TestDecide_PriorizaRiesgoAlto(t *testing.T) {
	gap := &GapAnalysis{Type: KnowledgeGap}
	ctx := LearningContext{
		Action:        "formatear_disco",
		IsOnline:      true,
		IsHighRisk:    true,
		WorthLearning: true,
		Gap:           gap,
	}

	dec := Decide(ctx)
	if dec.Type != DecisionRequestConfirmation {
		t.Errorf("esperaba DecisionRequestConfirmation, dio %q", dec.Type)
	}
}

// TestDecide_UsaMemoriaSiOffline verifica que sin red se recurre a la memoria interna.
func TestDecide_UsaMemoriaSiOffline(t *testing.T) {
	gap := &GapAnalysis{Type: KnowledgeGap}
	ctx := LearningContext{
		Action:        "crear_script",
		IsOnline:      false,
		WorthLearning: true,
		Gap:           gap,
	}

	dec := Decide(ctx)
	if dec.Type != DecisionUseMemory {
		t.Errorf("esperaba DecisionUseMemory, dio %q", dec.Type)
	}
}

// TestDecide_InvestigaSiKnowledgeGapYRedValida cubre el camino feliz de investigación en Internet.
func TestDecide_InvestigaSiKnowledgeGapYRedValida(t *testing.T) {
	gap := &GapAnalysis{Type: KnowledgeGap}
	ctx := LearningContext{
		Action:        "convertir_pdf_a_txt",
		IsOnline:      true,
		WorthLearning: true,
		Gap:           gap,
	}

	dec := Decide(ctx)
	if dec.Type != DecisionExecuteResearch {
		t.Errorf("esperaba DecisionExecuteResearch, dio %q", dec.Type)
	}
}

// TestDecide_FailSafeGapNil verifica que si Gap es nil no ocurra un panic y use la memoria por defecto.
func TestDecide_FailSafeGapNil(t *testing.T) {
	ctx := LearningContext{
		Action:   "algo_desconocido",
		IsOnline: true,
	}

	dec := Decide(ctx)
	if dec.Type != DecisionUseMemory {
		t.Errorf("esperaba DecisionUseMemory como fail-safe, dio %q", dec.Type)
	}
}
