package solving

import (
	"ahmelia/interaction"
	"strings"
	"testing"
)

// TestPlan_SoloTentativasEntranComoReforzar confirma que solo las
// entradas StatusTentative de Knowledge Memory se convierten en ítems
// KindReinforce — las validadas y sin-datos no aportan nada al plan.
func TestPlan_SoloTentativasEntranComoReforzar(t *testing.T) {
	km := NewKnowledgeMemory()
	km.Remember(&Validation{Action: "crear_carpeta", Label: "Crear carpetas", Status: StatusTentative, Samples: 2})
	km.Remember(&Validation{Action: "buscar_archivo", Label: "Buscar archivos", Status: StatusValidated, Samples: 10})
	km.Remember(&Validation{Action: "abrir_programa", Label: "Abrir programas", Status: StatusNoData})

	p := NewPlanner()
	plan := p.Plan(km, nil)

	reinforceCount := 0
	for _, item := range plan.Items {
		if item.Kind == KindReinforce {
			reinforceCount++
			if item.Label != "Crear carpetas" {
				t.Errorf("esperaba que el único ítem de refuerzo sea 'Crear carpetas', obtuve %q", item.Label)
			}
		}
	}
	if reinforceCount != 1 {
		t.Fatalf("esperaba exactamente 1 ítem KindReinforce, obtuve %d", reinforceCount)
	}
}

// TestPlan_CurriculumEntraComoAprender confirma que los Items de
// Curriculum se traducen 1:1 a ítems KindLearn, preservando Priority.
func TestPlan_CurriculumEntraComoAprender(t *testing.T) {
	c := &Curriculum{Goal: "x", Items: []LearningItem{
		{TaskID: "T1", TaskDesc: "renderizar en blender", Reason: "sin pista ni antecedente", Priority: 3},
	}}

	p := NewPlanner()
	plan := p.Plan(nil, c)

	if len(plan.Items) != 1 || plan.Items[0].Kind != KindLearn {
		t.Fatalf("esperaba 1 ítem KindLearn, obtuve %+v", plan.Items)
	}
	if plan.Items[0].Priority != 3 {
		t.Fatalf("esperaba que Priority se preserve desde Curriculum (3), obtuve %d", plan.Items[0].Priority)
	}
}

// TestPlan_ReforzarVaAntesQueAprender confirma el orden entre grupos:
// los ítems de refuerzo (inversión más chica) van antes que los de
// aprendizaje de cero, sin importar el orden en que se agregaron acá.
func TestPlan_ReforzarVaAntesQueAprender(t *testing.T) {
	km := NewKnowledgeMemory()
	km.Remember(&Validation{Action: "crear_carpeta", Label: "Crear carpetas", Status: StatusTentative, Samples: 2})
	c := &Curriculum{Goal: "x", Items: []LearningItem{
		{TaskID: "T1", TaskDesc: "renderizar en blender", Reason: "gap genuino", Priority: 1},
	}}

	p := NewPlanner()
	plan := p.Plan(km, c)

	if len(plan.Items) != 2 {
		t.Fatalf("esperaba 2 ítems en total, obtuve %d", len(plan.Items))
	}
	if plan.Items[0].Kind != KindReinforce || plan.Items[1].Kind != KindLearn {
		t.Fatalf("esperaba Reforzar primero y Aprender después, obtuve orden: %v, %v", plan.Items[0].Kind, plan.Items[1].Kind)
	}
}

// TestPlan_SinDatosDaPlanVacio confirma que, sin Knowledge Memory ni
// Curriculum (ambos nil), el plan sale vacío y Describe() lo dice
// explícito, sin inventar contenido.
func TestPlan_SinDatosDaPlanVacio(t *testing.T) {
	p := NewPlanner()
	plan := p.Plan(nil, nil)

	if len(plan.Items) != 0 {
		t.Fatalf("esperaba 0 ítems sin datos de entrada, obtuve %d", len(plan.Items))
	}
	desc := plan.Describe()
	if !strings.Contains(desc, "No tengo nada pendiente") {
		t.Fatalf("esperaba que Describe() lo diga explícito, obtuve: %q", desc)
	}
}

// TestDescribe_AgrupaPorTipo confirma que la narrativa separa
// claramente Reforzar de Aprender, no los mezcla en una sola lista.
func TestDescribe_AgrupaPorTipo(t *testing.T) {
	plan := &LearningPlan{Goal: "x", Items: []PlanItem{
		{Kind: KindReinforce, Label: "Crear carpetas", Description: "consolidar con más intentos (tiene 2 de 5 necesarios)"},
		{Kind: KindLearn, Label: "renderizar en blender", Description: "gap genuino"},
	}}
	desc := plan.Describe()

	if !strings.Contains(desc, "Para reforzar") || !strings.Contains(desc, "Para aprender de cero") {
		t.Fatalf("esperaba las dos secciones claramente separadas, obtuve: %q", desc)
	}
}

// TestParsePlannerQuery_DistintoDeCurriculumYMemoria confirma que los
// triggers de Learning Planner no se pisan con Curriculum (47) ni
// Knowledge Memory (49).
func TestParsePlannerQuery_DistintoDeCurriculumYMemoria(t *testing.T) {
	if !ParsePlannerQuery(interaction.Analyze("cual es tu plan de aprendizaje", interaction.Context{})) {
		t.Error("esperaba detectar el trigger de Learning Planner")
	}
	if ParsePlannerQuery(interaction.Analyze("que necesito aprender para armar un juego", interaction.Context{})) {
		t.Error("un pedido de Curriculum Builder no debería disparar Learning Planner")
	}
	if ParsePlannerQuery(interaction.Analyze("desde cuando confias en buscar archivos", interaction.Context{})) {
		t.Error("un pedido de Knowledge Memory no debería disparar Learning Planner")
	}
}
