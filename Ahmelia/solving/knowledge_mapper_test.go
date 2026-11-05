package solving

import (
	"ahmelia/interaction"
	"testing"

	memoryintelligence "ahmelia/memory_intelligence"
)

// TestMap_DetectaTaskConocida confirma la ruta central: una Task cuya
// descripción contiene una keyword de una Action real (shared.ActionPatterns)
// se marca Known=true con la Action correcta.
func TestMap_DetectaTaskConocida(t *testing.T) {
	plan := &TaskPlan{
		Goal: "x",
		Tasks: []*Task{
			{ID: "T1", Description: "Crea carpeta de proyecto"},
		},
	}

	m := NewMapper(nil)
	km := m.Map(plan)

	if len(km.Matches) != 1 {
		t.Fatalf("esperaba 1 match, obtuve %d", len(km.Matches))
	}
	match := km.Matches[0]
	if !match.Known {
		t.Fatal("esperaba Known=true: 'crea carpeta' matchea la Action crear_carpeta")
	}
	if match.Action != "crear_carpeta" {
		t.Errorf("esperaba Action='crear_carpeta', obtuve %q", match.Action)
	}
}

// TestMap_TaskDesconocidaEsGap confirma que una Task sin ninguna
// keyword conocida queda marcada como gap (Known=false), no como error.
func TestMap_TaskDesconocidaEsGap(t *testing.T) {
	plan := &TaskPlan{
		Goal: "x",
		Tasks: []*Task{
			{ID: "T1", Description: "Diseñar el circuito del PCB"},
		},
	}

	m := NewMapper(nil)
	km := m.Map(plan)

	if km.KnownCount() != 0 || km.GapCount() != 1 {
		t.Fatalf("esperaba 0 conocidas y 1 gap, obtuve known=%d gap=%d", km.KnownCount(), km.GapCount())
	}
	if len(km.Gaps()) != 1 || km.Gaps()[0].TaskID != "T1" {
		t.Fatalf("Gaps() debería devolver la Task T1, obtuve: %+v", km.Gaps())
	}
}

// TestMap_UsaConfianzaDeLearningState confirma que, cuando SÍ hay un
// LearningState real con historial, Confidence se completa con
// ActionConfidence en vez de quedar en -1.
func TestMap_UsaConfianzaDeLearningState(t *testing.T) {
	ls := memoryintelligence.NewLearningState()
	ls.ActionConfidence["crear_carpeta"] = 82

	plan := &TaskPlan{
		Goal:  "x",
		Tasks: []*Task{{ID: "T1", Description: "crea carpeta nueva"}},
	}

	m := NewMapper(memoryintelligence.NewLearningStore(ls))
	km := m.Map(plan)

	if km.Matches[0].Confidence != 82 {
		t.Fatalf("esperaba Confidence=82 desde LearningState, obtuve %.0f", km.Matches[0].Confidence)
	}
}

// TestMap_SinLearningStateNoRompe confirma que Mapper con nil Learning
// (mismo caso que los dos primeros tests) no genera panic ni deja
// Confidence en un valor engañoso — debe quedar en -1 explícito.
func TestMap_SinLearningStateNoRompe(t *testing.T) {
	plan := &TaskPlan{
		Goal:  "x",
		Tasks: []*Task{{ID: "T1", Description: "crea carpeta nueva"}},
	}
	m := NewMapper(nil)
	km := m.Map(plan)

	if km.Matches[0].Confidence != -1 {
		t.Fatalf("sin LearningState, Confidence debería quedar en -1 (sin historial), obtuve %.0f", km.Matches[0].Confidence)
	}
}

// TestParseKnowledgeMapQuery_DistintoDeLosOtrosDos confirma que los
// triggers de Knowledge Mapper no se pisan con Goal Analyzer ni Problem
// Decomposer.
func TestParseKnowledgeMapQuery_DistintoDeLosOtrosDos(t *testing.T) {
	if !ParseKnowledgeMapQuery(interaction.Analyze("cuanto sabes de armar una tienda online", interaction.Context{})) {
		t.Error("esperaba detectar el trigger de Knowledge Mapper")
	}
	if ParseKnowledgeMapQuery(interaction.Analyze("analiza este objetivo: crear un juego", interaction.Context{})) {
		t.Error("un pedido de Goal Analyzer no debería disparar Knowledge Mapper")
	}
	if ParseKnowledgeMapQuery(interaction.Analyze("que tareas necesito para armar una tienda", interaction.Context{})) {
		t.Error("un pedido de Problem Decomposer no debería disparar Knowledge Mapper")
	}
}
