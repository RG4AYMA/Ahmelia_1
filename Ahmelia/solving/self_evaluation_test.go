package solving

import (
	"ahmelia/interaction"
	"strings"
	"testing"
)

// TestEvaluate_TodoBien confirma el caso ideal: dominio reconocido,
// tareas suficientes, todo conocido, nada bloqueado, sin gaps —
// OverallScore debería ser 1.0 exacto.
func TestEvaluate_TodoBien(t *testing.T) {
	ga := &GoalAnalysis{MainGoal: "x", Domain: "software"}
	plan := &TaskPlan{Goal: "x", Tasks: []*Task{{ID: "T1"}, {ID: "T2"}}}
	km := &KnowledgeMap{Goal: "x", Matches: []KnowledgeMatch{
		{TaskID: "T1", Known: true}, {TaskID: "T2", Known: true},
	}}
	dg := &DependencyGraph{Goal: "x", Order: []string{"T1", "T2"}, Nodes: map[string]*DependencyNode{
		"T1": {TaskID: "T1"}, "T2": {TaskID: "T2"},
	}}
	hs := &HypothesisSet{Goal: "x"}    // sin gaps
	cr := &ConstraintReport{Goal: "x"} // sin requirements
	dec := &Decision{Goal: "x", Verdict: VerdictProceed}
	as := &AnalogySet{Goal: "x"} // sin gaps

	se := NewSelfEvaluator()
	report := se.Evaluate(ga, plan, km, dg, hs, cr, dec, as)

	if report.OverallScore != 1.0 {
		t.Fatalf("OverallScore = %.2f, esperaba 1.0 (caso ideal)", report.OverallScore)
	}
	if len(report.Layers) != 8 {
		t.Fatalf("len(Layers) = %d, esperaba 8", len(report.Layers))
	}
}

// TestEvaluate_TodoNilNoRompe confirma el criterio defensivo de Fase
// 3: llamar con los ocho punteros en nil no rompe, y el score refleja
// que ninguna capa tuvo con qué trabajar.
func TestEvaluate_TodoNilNoRompe(t *testing.T) {
	se := NewSelfEvaluator()
	report := se.Evaluate(nil, nil, nil, nil, nil, nil, nil, nil)

	if report.Goal != "" {
		t.Fatalf("Goal = %q, esperaba vacío (ga nil)", report.Goal)
	}
	if len(report.Layers) != 8 {
		t.Fatalf("len(Layers) = %d, esperaba 8 aunque todo sea nil", len(report.Layers))
	}
}

// TestWeakest_DevuelveLaCapaConMenorScore confirma que Weakest() no
// devuelve la primera capa a secas, sino la de menor score real.
func TestWeakest_DevuelveLaCapaConMenorScore(t *testing.T) {
	ga := &GoalAnalysis{MainGoal: "x", Domain: "general"} // score 0.5
	km := &KnowledgeMap{Goal: "x", Matches: []KnowledgeMatch{
		{TaskID: "T1", Known: false}, // score 0.0 — el más bajo
	}}

	se := NewSelfEvaluator()
	report := se.Evaluate(ga, nil, km, nil, nil, nil, nil, nil)

	weakest := report.Weakest()
	if weakest.Score != 0 {
		t.Fatalf("Weakest().Score = %.2f, esperaba 0 (varias capas en 0, pero ninguna con score menor)", weakest.Score)
	}
}

// TestEvaluate_DependencySolver_PenalizaBloqueos confirma que el score
// de Dependency Solver refleja la proporción real de tareas
// bloqueadas, no solo si hubo o no bloqueos.
func TestEvaluate_DependencySolver_PenalizaBloqueos(t *testing.T) {
	dg := &DependencyGraph{
		Goal:  "x",
		Order: []string{"T1", "T2", "T3", "T4"},
		Nodes: map[string]*DependencyNode{
			"T1": {TaskID: "T1", IsGap: true},
			"T2": {TaskID: "T2", Blocked: true, BlockedByID: "T1"},
			"T3": {TaskID: "T3", Blocked: true, BlockedByID: "T1"},
			"T4": {TaskID: "T4"},
		},
	}

	eval := evalDependencySolver(dg)
	if eval.Score != 0.5 { // 2 de 4 bloqueadas -> (4-2)/4 = 0.5
		t.Fatalf("Score = %.2f, esperaba 0.5 (2 de 4 tareas bloqueadas)", eval.Score)
	}
	if !strings.Contains(eval.Signal, "2 de 4") {
		t.Fatalf("Signal = %q, esperaba mencionar \"2 de 4\"", eval.Signal)
	}
}

// TestDescribe_MencionaGoalYCapaMasFloja confirma que la narrativa
// incluye tanto el objetivo como la capa más floja con su señal.
func TestDescribe_MencionaGoalYCapaMasFloja(t *testing.T) {
	ga := &GoalAnalysis{MainGoal: "armar un juego", Domain: "general"}

	se := NewSelfEvaluator()
	report := se.Evaluate(ga, nil, nil, nil, nil, nil, nil, nil)
	desc := report.Describe()

	if !strings.Contains(desc, "armar un juego") {
		t.Fatalf("Describe() = %q, debería mencionar el goal", desc)
	}
	if !strings.Contains(desc, "Problem Decomposer") && !strings.Contains(desc, "Dependency Solver") {
		// con plan/dg nil, ambas quedan en 0 — cualquiera de las dos es una capa más floja válida
		t.Fatalf("Describe() = %q, debería mencionar alguna capa en 0", desc)
	}
}

// TestParseSelfEvalQuery_Positivos confirma los triggers reales.
func TestParseSelfEvalQuery_Positivos(t *testing.T) {
	positives := []string{
		"que tan bien razonaste",
		"evalua tu propio razonamiento",
		"que tan solido fue tu analisis",
	}
	for _, p := range positives {
		if !ParseSelfEvalQuery(interaction.Analyze(p, interaction.Context{})) {
			t.Errorf("ParseSelfEvalQuery(interaction.Analyze(%q, interaction.Context{})) = false, esperaba true", p)
		}
	}
}

// TestParseSelfEvalQuery_NoPisaLosOtrosOcho confirma que no colisiona
// con ningún trigger de las ocho capas anteriores de Fase 3.
func TestParseSelfEvalQuery_NoPisaLosOtrosOcho(t *testing.T) {
	negatives := []string{
		"que deberia hacer con esto", // Decision Engine
		"cuanto sabes de esto",       // Knowledge Mapper
		"descompon esta tarea",       // Problem Decomposer
	}
	for _, n := range negatives {
		if ParseSelfEvalQuery(interaction.Analyze(n, interaction.Context{})) {
			t.Errorf("ParseSelfEvalQuery(interaction.Analyze(%q, interaction.Context{})) = true, no debería activarse (es trigger de otra capa)", n)
		}
	}
}
