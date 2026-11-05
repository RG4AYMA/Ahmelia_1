package solving

import (
	"ahmelia/interaction"
	"strings"
	"testing"
)

// TestExtractProcedureTarget_SacaElTriggerYDejaSoloElTarget
// (Integration Fix) reproduce el bug real en vivo: sin esto, engine.go
// pasaba analysis.MainGoal (goalAnalyzer, que no conoce estos
// triggers) a ExplainProcedure, y "como instalo go" llegaba a Search()
// como el string COMPLETO en vez de solo "go" — nunca encontraba la
// Skill guardada aunque existiera.
func TestExtractProcedureTarget_SacaElTriggerYDejaSoloElTarget(t *testing.T) {
	casos := map[string]string{
		"como instalo go":                       "go",
		"cuales son los pasos para instalar go": "instalar go",
		"dame el procedimiento de go":           "go",
		"explicame como usar blender":           "blender",
	}
	for input, want := range casos {
		if got := ExtractProcedureTarget(input); got != want {
			t.Errorf("ExtractProcedureTarget(%q) = %q, esperaba %q", input, got, want)
		}
	}
}

// TestParseSkillProcedureQuery_DetectaTriggersSinConfundirseConOtrosBloques
// confirma los triggers principales y que no colisiona con 54/57.
func TestParseSkillProcedureQuery_DetectaTriggersSinConfundirseConOtrosBloques(t *testing.T) {
	casos := []string{
		"como instalo go",
		"cuales son los pasos para instalar go",
		"muestrame los pasos de go",
		"dame el procedimiento de go",
		"explicame como usar blender",
	}
	for _, c := range casos {
		if !ParseSkillProcedureQuery(interaction.Analyze(c, interaction.Context{})) {
			t.Errorf("ParseSkillProcedureQuery(interaction.Analyze(%q, interaction.Context{})) debería ser true", c)
		}
	}
	if ParseSkillProcedureQuery(interaction.Analyze("que sabes sobre go", interaction.Context{})) {
		t.Error("una consulta de conocimiento (57) no debería disparar Skill Procedure Explanation")
	}
	if ParseSkillProcedureQuery(interaction.Analyze("aprende go", interaction.Context{})) {
		t.Error("un pedido de aprendizaje nuevo (54) no debería disparar Skill Procedure Explanation")
	}
}

// TestExplainProcedure_SkillProceduralEnumeraLosPasosReales confirma el
// caso central: con Steps reales, la narrativa los enumera de verdad,
// más Prerequisites/Pitfalls/FailureRecovery/ExpectedOutcome cuando
// existen.
func TestExplainProcedure_SkillProceduralEnumeraLosPasosReales(t *testing.T) {
	skills := NewSkillLibrary()
	skills.Install(&Skill{
		SkillID: "Go", Label: "Go", ProcedureTitle: "Tutorial: Get started with Go",
		State:           StateProcedural,
		Prerequisites:   []string{"Go instalado"},
		Steps:           []string{"Install Go", "Write Hello World", "Run the code", "Call functions of an external module"},
		Pitfalls:        []string{"no uses GOPATH viejo"},
		FailureRecovery: []string{"revisa la variable PATH"},
		ExpectedOutcome: []string{"el programa imprime Hello, World"},
		Sources:         []string{"Go Docs (go.dev)"},
	})

	got := ExplainProcedure("go", skills)

	for _, want := range []string{
		"Tutorial: Get started with Go",
		"Antes necesitás: Go instalado",
		"1. Install Go",
		"2. Write Hello World",
		"3. Run the code",
		"4. Call functions of an external module",
		"Ojo con esto: no uses GOPATH viejo",
		"Si algo falla: revisa la variable PATH",
		"Resultado esperado: el programa imprime Hello, World",
		"Fuente: Go Docs (go.dev)",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("ExplainProcedure() no contiene %q — respuesta completa: %q", want, got)
		}
	}
}

// TestExplainProcedure_SkillSinStepsNoInventaProcedimiento confirma el
// límite honesto central: sin Steps, no se fabrica un procedimiento —
// se dice explícitamente que no hay uno estructurado.
func TestExplainProcedure_SkillSinStepsNoInventaProcedimiento(t *testing.T) {
	skills := NewSkillLibrary()
	skills.Install(&Skill{SkillID: "Unity", Label: "Unity", Sources: []string{"Unity Docs"}})

	got := ExplainProcedure("unity", skills)

	if strings.Contains(got, "1.") {
		t.Errorf("sin Steps, no debería aparecer ningún paso numerado: %q", got)
	}
	if !strings.Contains(got, "no tengo un procedimiento suficientemente estructurado") {
		t.Errorf("esperaba el mensaje honesto de límite, obtuve: %q", got)
	}
}

// TestExplainProcedure_SkillSoloStructuredComparteWhenToUse confirma
// el caso intermedio: sin Steps pero con WhenToUse, comparte esa
// información real en vez de solo decir "no tengo nada".
func TestExplainProcedure_SkillSoloStructuredComparteWhenToUse(t *testing.T) {
	skills := NewSkillLibrary()
	skills.Install(&Skill{
		SkillID: "ObjectPooling", Label: "Object Pooling", State: StateStructured,
		WhenToUse: []string{"cuando necesites levantar muchos objetos seguido"},
		Sources:   []string{"Unity Docs"},
	})

	got := ExplainProcedure("object pooling", skills)

	if !strings.Contains(got, "cuando necesites levantar muchos objetos seguido") {
		t.Errorf("esperaba que comparta el WhenToUse real, obtuve: %q", got)
	}
}

// TestExplainProcedure_SinNingunMatchLoDiceHonestamente confirma el
// caso de 0 resultados.
func TestExplainProcedure_SinNingunMatchLoDiceHonestamente(t *testing.T) {
	got := ExplainProcedure("cobol", NewSkillLibrary())
	if !strings.Contains(got, "No tengo ningún procedimiento guardado") {
		t.Errorf("esperaba el mensaje de 0 resultados, obtuve: %q", got)
	}
}

// TestExplainProcedure_ConVariosMatchesNoAdivinaCual confirma que, con
// ambigüedad real, no elige arbitrariamente una Skill — mismo
// criterio que SkillSearchResult.Describe() (57).
func TestExplainProcedure_ConVariosMatchesNoAdivinaCual(t *testing.T) {
	skills := NewSkillLibrary()
	skills.Install(&Skill{SkillID: "Go", Label: "Go", Content: "contenido sobre go", Sources: []string{"Go Docs"}})
	skills.Install(&Skill{SkillID: "Godot", Label: "Godot", Content: "contenido sobre go y godot", Sources: []string{"Godot Docs"}})

	got := ExplainProcedure("go", skills)

	if strings.Contains(got, "1.") {
		t.Errorf("con ambigüedad, no debería enumerar pasos de ninguna de las dos: %q", got)
	}
	if !strings.Contains(got, "no quiero adivinar") {
		t.Errorf("esperaba que declare la ambigüedad explícitamente, obtuve: %q", got)
	}
}

// TestExplainProcedure_MencionaLimitationsCuandoExisten (Integration
// Fix, Skill Model Refinement) confirma que describeProcedure avisa
// las limitaciones conocidas junto al resto del procedimiento.
func TestExplainProcedure_MencionaLimitationsCuandoExisten(t *testing.T) {
	skills := NewSkillLibrary()
	skills.Install(&Skill{
		SkillID: "Go", Label: "Go", State: StateProcedural,
		Steps:       []string{"Install Go"},
		Limitations: []string{"no funciona en Windows 7"},
		Sources:     []string{"Go Docs"},
	})

	got := ExplainProcedure("go", skills)

	if !strings.Contains(got, "Limitaciones conocidas: no funciona en Windows 7") {
		t.Errorf("esperaba que mencione las limitaciones conocidas, obtuve: %q", got)
	}
}
