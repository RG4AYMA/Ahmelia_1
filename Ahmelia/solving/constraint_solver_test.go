package solving

import "ahmelia/interaction"

import "testing"

// TestCheck_VelocidadSatisfiableSinGaps confirma la ruta central: un
// Requirement de velocidad, con un KnowledgeMap sin gaps, queda
// StatusSatisfiable.
func TestCheck_VelocidadSatisfiableSinGaps(t *testing.T) {
	ga := &GoalAnalysis{
		MainGoal:     "x",
		Requirements: []Requirement{{Description: "Priorizar velocidad de entrega sobre exhaustividad", Implicit: true}},
	}
	km := &KnowledgeMap{Matches: []KnowledgeMatch{{TaskID: "T1", Known: true}}}

	cs := NewConstraintSolver()
	report := cs.Check(ga, km, nil)

	if len(report.Checks) != 1 {
		t.Fatalf("esperaba 1 check, obtuve %d", len(report.Checks))
	}
	if report.Checks[0].Status != StatusSatisfiable {
		t.Fatalf("esperaba StatusSatisfiable sin gaps, obtuve %q", report.Checks[0].Status)
	}
}

// TestCheck_VelocidadEnRiesgoConBloqueos confirma el caso central del
// bloque: un Requirement de velocidad con gaps que además dejan Tasks
// bloqueadas queda StatusAtRisk.
func TestCheck_VelocidadEnRiesgoConBloqueos(t *testing.T) {
	ga := &GoalAnalysis{
		MainGoal:     "x",
		Requirements: []Requirement{{Description: "Priorizar velocidad de entrega sobre exhaustividad", Implicit: true}},
	}
	km := &KnowledgeMap{Matches: []KnowledgeMatch{
		{TaskID: "T1", Known: false},
		{TaskID: "T2", Known: true},
	}}
	dg := &DependencyGraph{
		Nodes: map[string]*DependencyNode{
			"T1": {TaskID: "T1", IsGap: true},
			"T2": {TaskID: "T2", Blocked: true, BlockedByID: "T1"},
		},
	}

	cs := NewConstraintSolver()
	report := cs.Check(ga, km, dg)

	if report.Checks[0].Status != StatusAtRisk {
		t.Fatalf("esperaba StatusAtRisk con gaps que bloquean, obtuve %q", report.Checks[0].Status)
	}
	if report.RiskCount() != 1 {
		t.Fatalf("esperaba RiskCount()==1, obtuve %d", report.RiskCount())
	}
}

// TestCheck_RequisitoSinReglaQuedaSinSenal confirma que un Requirement
// sin regla conocida (ej. escalabilidad) queda StatusUnverifiable, sin
// inventar un veredicto.
func TestCheck_RequisitoSinReglaQuedaSinSenal(t *testing.T) {
	ga := &GoalAnalysis{
		MainGoal:     "x",
		Requirements: []Requirement{{Description: "Debe soportar escala (más de un usuario o carga concurrente)", Implicit: true}},
	}
	km := &KnowledgeMap{Matches: []KnowledgeMatch{{TaskID: "T1", Known: true}}}

	cs := NewConstraintSolver()
	report := cs.Check(ga, km, nil)

	if report.Checks[0].Status != StatusUnverifiable {
		t.Fatalf("esperaba StatusUnverifiable para un requisito sin regla, obtuve %q", report.Checks[0].Status)
	}
}

// TestCheck_SinKnowledgeMapQuedaSinSenal confirma que, sin
// KnowledgeMap, un Requirement de velocidad queda Unverifiable en vez
// de arriesgar un veredicto sin base — mismo criterio defensivo que el
// resto de Fase 3.
func TestCheck_SinKnowledgeMapQuedaSinSenal(t *testing.T) {
	ga := &GoalAnalysis{
		MainGoal:     "x",
		Requirements: []Requirement{{Description: "Priorizar velocidad de entrega sobre exhaustividad", Implicit: true}},
	}

	cs := NewConstraintSolver()
	report := cs.Check(ga, nil, nil)

	if report.Checks[0].Status != StatusUnverifiable {
		t.Fatalf("sin KnowledgeMap, esperaba StatusUnverifiable, obtuve %q", report.Checks[0].Status)
	}
}

// TestParseConstraintQuery_DistintoDeLosOtrosCinco confirma que los
// triggers de Constraint Solver no se pisan con los cinco bloques
// anteriores de Fase 3.
func TestParseConstraintQuery_DistintoDeLosOtrosCinco(t *testing.T) {
	if !ParseConstraintQuery(interaction.Analyze("que restricciones tiene armar una tienda online", interaction.Context{})) {
		t.Error("esperaba detectar el trigger de Constraint Solver")
	}
	if ParseConstraintQuery(interaction.Analyze("analiza este objetivo: crear un juego", interaction.Context{})) {
		t.Error("un pedido de Goal Analyzer no debería disparar Constraint Solver")
	}
	if ParseConstraintQuery(interaction.Analyze("que tareas necesito para armar una tienda", interaction.Context{})) {
		t.Error("un pedido de Problem Decomposer no debería disparar Constraint Solver")
	}
	if ParseConstraintQuery(interaction.Analyze("cuanto sabes de armar una tienda online", interaction.Context{})) {
		t.Error("un pedido de Knowledge Mapper no debería disparar Constraint Solver")
	}
	if ParseConstraintQuery(interaction.Analyze("que orden debo seguir para armar una tienda", interaction.Context{})) {
		t.Error("un pedido de Dependency Solver no debería disparar Constraint Solver")
	}
	if ParseConstraintQuery(interaction.Analyze("que hipotesis tienes para armar una tienda", interaction.Context{})) {
		t.Error("un pedido de Hypothesis Generator no debería disparar Constraint Solver")
	}
}
