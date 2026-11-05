package solving

import "ahmelia/interaction"

import "testing"

// TestSolve_OrdenSecuencialSinGaps confirma la ruta central: con un
// plan donde Knowledge Mapper no encontró ningún gap, el grafo sale
// completo (Order/Edges) y nada queda Blocked.
func TestSolve_OrdenSecuencialSinGaps(t *testing.T) {
	plan := &TaskPlan{
		Goal: "x",
		Tasks: []*Task{
			{ID: "T1", Description: "Crea carpeta de proyecto"},
			{ID: "T2", Description: "Abre el programa"},
		},
	}
	km := &KnowledgeMap{Goal: "x", Matches: []KnowledgeMatch{
		{TaskID: "T1", Known: true},
		{TaskID: "T2", Known: true},
	}}

	s := NewSolver()
	dg := s.Solve(plan, km)

	if len(dg.Order) != 2 || dg.Order[0] != "T1" || dg.Order[1] != "T2" {
		t.Fatalf("esperaba Order=[T1 T2], obtuve %v", dg.Order)
	}
	if len(dg.Edges) != 1 || dg.Edges[0].TaskID != "T2" || dg.Edges[0].DependsOnID != "T1" {
		t.Fatalf("esperaba un edge T2->T1, obtuve %+v", dg.Edges)
	}
	if dg.BlockedCount() != 0 {
		t.Fatalf("sin gaps no debería haber Tasks bloqueadas, obtuve %d", dg.BlockedCount())
	}
}

// TestSolve_GapBloqueaLoQueSigue confirma el caso central del bloque:
// una Task marcada como gap por Knowledge Mapper deja bloqueadas todas
// las que vienen después en el orden, apuntando a ese gap.
func TestSolve_GapBloqueaLoQueSigue(t *testing.T) {
	plan := &TaskPlan{
		Goal: "x",
		Tasks: []*Task{
			{ID: "T1", Description: "Diseñar el circuito del PCB"}, // gap
			{ID: "T2", Description: "Crea carpeta de proyecto"},
			{ID: "T3", Description: "Abre el programa"},
		},
	}
	km := &KnowledgeMap{Goal: "x", Matches: []KnowledgeMatch{
		{TaskID: "T1", Known: false},
		{TaskID: "T2", Known: true},
		{TaskID: "T3", Known: true},
	}}

	s := NewSolver()
	dg := s.Solve(plan, km)

	if !dg.Nodes["T1"].IsGap {
		t.Fatal("T1 debería quedar marcado como gap")
	}
	if dg.Nodes["T1"].Blocked {
		t.Fatal("T1 es la causa raíz, no debería estar Blocked él mismo")
	}
	if !dg.Nodes["T2"].Blocked || dg.Nodes["T2"].BlockedByID != "T1" {
		t.Fatalf("T2 debería estar bloqueado por T1, obtuve %+v", dg.Nodes["T2"])
	}
	if !dg.Nodes["T3"].Blocked || dg.Nodes["T3"].BlockedByID != "T1" {
		t.Fatalf("T3 debería estar bloqueado por T1 (transitivo), obtuve %+v", dg.Nodes["T3"])
	}
	if dg.BlockedCount() != 2 {
		t.Fatalf("esperaba 2 Tasks bloqueadas, obtuve %d", dg.BlockedCount())
	}
	if len(dg.RootGaps()) != 1 || dg.RootGaps()[0].TaskID != "T1" {
		t.Fatalf("RootGaps() debería devolver solo T1, obtuve %+v", dg.RootGaps())
	}
}

// TestSolve_GapMasCercanoGanaAlAnterior confirma que, con dos gaps en
// la cadena, cada Task bloqueada apunta al gap MÁS CERCANO, no al
// primero de todos.
func TestSolve_GapMasCercanoGanaAlAnterior(t *testing.T) {
	plan := &TaskPlan{
		Goal: "x",
		Tasks: []*Task{
			{ID: "T1", Description: "gap uno"},
			{ID: "T2", Description: "gap dos"},
			{ID: "T3", Description: "conocida"},
		},
	}
	km := &KnowledgeMap{Goal: "x", Matches: []KnowledgeMatch{
		{TaskID: "T1", Known: false},
		{TaskID: "T2", Known: false},
		{TaskID: "T3", Known: true},
	}}

	s := NewSolver()
	dg := s.Solve(plan, km)

	if dg.Nodes["T3"].BlockedByID != "T2" {
		t.Fatalf("T3 debería apuntar al gap más cercano (T2), obtuve %q", dg.Nodes["T3"].BlockedByID)
	}
}

// TestSolve_SinKnowledgeMapNoRompe confirma que Solve con km=nil (mismo
// caso que Mapper(nil) en Bloque 23) no genera panic y no marca nada
// como bloqueado, ya que sin KnowledgeMap no hay forma de saber qué
// Task es gap.
func TestSolve_SinKnowledgeMapNoRompe(t *testing.T) {
	plan := &TaskPlan{
		Goal:  "x",
		Tasks: []*Task{{ID: "T1", Description: "algo"}, {ID: "T2", Description: "otra cosa"}},
	}

	s := NewSolver()
	dg := s.Solve(plan, nil)

	if dg.BlockedCount() != 0 {
		t.Fatalf("sin KnowledgeMap no debería marcarse ningún bloqueo, obtuve %d", dg.BlockedCount())
	}
	if len(dg.Order) != 2 {
		t.Fatalf("el grafo debería seguir armando Order aunque km sea nil, obtuve %v", dg.Order)
	}
}

// TestParseDependencySolveQuery_DistintoDeLosOtrosTres confirma que los
// triggers de Dependency Solver no se pisan con los tres bloques
// anteriores de Fase 3.
func TestParseDependencySolveQuery_DistintoDeLosOtrosTres(t *testing.T) {
	if !ParseDependencySolveQuery(interaction.Analyze("que orden debo seguir para armar una tienda online", interaction.Context{})) {
		t.Error("esperaba detectar el trigger de Dependency Solver")
	}
	if ParseDependencySolveQuery(interaction.Analyze("analiza este objetivo: crear un juego", interaction.Context{})) {
		t.Error("un pedido de Goal Analyzer no debería disparar Dependency Solver")
	}
	if ParseDependencySolveQuery(interaction.Analyze("que tareas necesito para armar una tienda", interaction.Context{})) {
		t.Error("un pedido de Problem Decomposer no debería disparar Dependency Solver")
	}
	if ParseDependencySolveQuery(interaction.Analyze("cuanto sabes de armar una tienda online", interaction.Context{})) {
		t.Error("un pedido de Knowledge Mapper no debería disparar Dependency Solver")
	}
}
