package solving

import "ahmelia/interaction"

import "testing"

// TestGenerate_EncuentraPistaPorSolapamiento confirma la ruta central:
// una Task gap cuya descripción comparte palabras con las keywords de
// una Action real (aunque no matcheó exacto en Knowledge Mapper) recibe
// esa Action como ClosestAction.
func TestGenerate_EncuentraPistaPorSolapamiento(t *testing.T) {
	km := &KnowledgeMap{Goal: "x", Matches: []KnowledgeMatch{
		{TaskID: "T1", TaskDesc: "Abre volumen del programa nuevo", Known: false},
	}}

	g := NewGenerator()
	hs := g.Generate(km, nil)

	if len(hs.Hypotheses) != 1 {
		t.Fatalf("esperaba 1 hipótesis, obtuve %d", len(hs.Hypotheses))
	}
	h := hs.Hypotheses[0]
	if !h.HasLead() {
		t.Fatalf("esperaba encontrar alguna pista por solapamiento, obtuve %+v", h)
	}
	if h.ClosestAction != "abrir_programa" {
		t.Errorf("esperaba ClosestAction='abrir_programa' (comparte 'programa'), obtuve %q", h.ClosestAction)
	}
}

// TestGenerate_SinPistaQuedaHonesto confirma que una Task gap sin
// ninguna palabra en común con ninguna Action queda con ClosestAction
// vacío — HasLead()==false, no un match forzado de baja calidad.
func TestGenerate_SinPistaQuedaHonesto(t *testing.T) {
	km := &KnowledgeMap{Goal: "x", Matches: []KnowledgeMatch{
		{TaskID: "T1", TaskDesc: "Soldar componentes electronicos", Known: false},
	}}

	g := NewGenerator()
	hs := g.Generate(km, nil)

	h := hs.Hypotheses[0]
	if h.HasLead() {
		t.Fatalf("no debería encontrar ninguna pista para 'soldar componentes electronicos', obtuve %+v", h)
	}
}

// TestGenerate_UsaGapsNoTasksConocidas confirma que Generate solo genera
// hipótesis para los Gaps() del KnowledgeMap, no para Tasks ya conocidas
// (esas no las necesita: Knowledge Mapper ya las resolvió).
func TestGenerate_UsaGapsNoTasksConocidas(t *testing.T) {
	km := &KnowledgeMap{Goal: "x", Matches: []KnowledgeMatch{
		{TaskID: "T1", TaskDesc: "crea carpeta nueva", Known: true, Action: "crear_carpeta"},
		{TaskID: "T2", TaskDesc: "Diseñar el circuito del PCB", Known: false},
	}}

	g := NewGenerator()
	hs := g.Generate(km, nil)

	if len(hs.Hypotheses) != 1 || hs.Hypotheses[0].TaskID != "T2" {
		t.Fatalf("esperaba una sola hipótesis para T2 (el gap), obtuve %+v", hs.Hypotheses)
	}
}

// TestGenerate_PriorizaPorDownstreamBlocked confirma que PriorityOrder()
// pone primero el gap que más Tasks deja bloqueadas, usando el
// DependencyGraph de Dependency Solver (Bloque 24).
func TestGenerate_PriorizaPorDownstreamBlocked(t *testing.T) {
	km := &KnowledgeMap{Goal: "x", Matches: []KnowledgeMatch{
		{TaskID: "T1", TaskDesc: "gap chico", Known: false},
		{TaskID: "T4", TaskDesc: "gap grande", Known: false},
	}}

	dg := &DependencyGraph{
		Nodes: map[string]*DependencyNode{
			"T1": {TaskID: "T1", IsGap: true},
			"T2": {TaskID: "T2", Blocked: true, BlockedByID: "T4"},
			"T3": {TaskID: "T3", Blocked: true, BlockedByID: "T4"},
			"T4": {TaskID: "T4", IsGap: true},
		},
		Order: []string{"T1", "T2", "T3", "T4"},
	}

	g := NewGenerator()
	hs := g.Generate(km, dg)

	ordered := hs.PriorityOrder()
	if ordered[0].TaskID != "T4" {
		t.Fatalf("esperaba T4 primero (deja 2 bloqueadas vs 0 de T1), obtuve orden %+v", ordered)
	}
	if ordered[0].DownstreamBlocked != 2 {
		t.Errorf("esperaba DownstreamBlocked=2 para T4, obtuve %d", ordered[0].DownstreamBlocked)
	}
}

// TestGenerate_SinDependencyGraphNoRompe confirma que Generate(km, nil)
// no rompe — mismo criterio defensivo que los bloques anteriores. Sin
// grafo de dependencias no hay forma de saber cuánto bloquea cada gap,
// así que DownstreamBlocked queda en 0 para todos.
func TestGenerate_SinDependencyGraphNoRompe(t *testing.T) {
	km := &KnowledgeMap{Goal: "x", Matches: []KnowledgeMatch{
		{TaskID: "T1", TaskDesc: "Diseñar el circuito del PCB", Known: false},
	}}

	g := NewGenerator()
	hs := g.Generate(km, nil)

	if hs.Hypotheses[0].DownstreamBlocked != 0 {
		t.Fatalf("sin DependencyGraph, DownstreamBlocked debería quedar en 0, obtuve %d", hs.Hypotheses[0].DownstreamBlocked)
	}
}

// TestParseHypothesisQuery_DistintoDeLosOtrosCuatro confirma que los
// triggers de Hypothesis Generator no se pisan con los cuatro bloques
// anteriores de Fase 3.
func TestParseHypothesisQuery_DistintoDeLosOtrosCuatro(t *testing.T) {
	if !ParseHypothesisQuery(interaction.Analyze("que hipotesis tienes para armar una tienda online", interaction.Context{})) {
		t.Error("esperaba detectar el trigger de Hypothesis Generator")
	}
	if ParseHypothesisQuery(interaction.Analyze("analiza este objetivo: crear un juego", interaction.Context{})) {
		t.Error("un pedido de Goal Analyzer no debería disparar Hypothesis Generator")
	}
	if ParseHypothesisQuery(interaction.Analyze("que tareas necesito para armar una tienda", interaction.Context{})) {
		t.Error("un pedido de Problem Decomposer no debería disparar Hypothesis Generator")
	}
	if ParseHypothesisQuery(interaction.Analyze("cuanto sabes de armar una tienda online", interaction.Context{})) {
		t.Error("un pedido de Knowledge Mapper no debería disparar Hypothesis Generator")
	}
	if ParseHypothesisQuery(interaction.Analyze("que orden debo seguir para armar una tienda", interaction.Context{})) {
		t.Error("un pedido de Dependency Solver no debería disparar Hypothesis Generator")
	}
}
