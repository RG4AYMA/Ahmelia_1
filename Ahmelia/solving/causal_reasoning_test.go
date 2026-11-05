package solving

import (
	"ahmelia/interaction"
	"strings"
	"testing"
)

// buildTestGraph arma un DependencyGraph a mano, sin pasar por
// Solve(), para testear ExplainBlock/ProjectResolution en aislamiento
// con datos controlados.
func buildTestGraph() *DependencyGraph {
	return &DependencyGraph{
		Goal:  "x",
		Order: []string{"T1", "T2", "T3", "T4"},
		Nodes: map[string]*DependencyNode{
			"T1": {TaskID: "T1", TaskDesc: "tarea conocida", IsGap: false},
			"T2": {TaskID: "T2", TaskDesc: "tarea nueva", IsGap: true},
			"T3": {TaskID: "T3", TaskDesc: "depende de T2", Blocked: true, BlockedByID: "T2"},
			"T4": {TaskID: "T4", TaskDesc: "también depende de T2", Blocked: true, BlockedByID: "T2"},
		},
	}
}

// TestExplainBlock_TaskNoBloqueada confirma que una Task sin Blocked
// da IsBlocked=false y ningún dato de causa inventado.
func TestExplainBlock_TaskNoBloqueada(t *testing.T) {
	dg := buildTestGraph()
	cr := NewCausalReasoner()
	link := cr.ExplainBlock(dg, "T1")

	if link.IsBlocked {
		t.Fatal("IsBlocked = true para T1, esperaba false")
	}
	if link.CauseID != "" {
		t.Fatalf("CauseID = %q, esperaba vacío sin bloqueo", link.CauseID)
	}
}

// TestExplainBlock_TaskBloqueadaTraeCausaReal confirma que la causa
// viene de BlockedByID, con la descripción real del nodo causante.
func TestExplainBlock_TaskBloqueadaTraeCausaReal(t *testing.T) {
	dg := buildTestGraph()
	cr := NewCausalReasoner()
	link := cr.ExplainBlock(dg, "T3")

	if !link.IsBlocked {
		t.Fatal("IsBlocked = false para T3, esperaba true")
	}
	if link.CauseID != "T2" {
		t.Fatalf("CauseID = %q, esperaba \"T2\"", link.CauseID)
	}
	if link.CauseDesc != "tarea nueva" {
		t.Fatalf("CauseDesc = %q, esperaba \"tarea nueva\"", link.CauseDesc)
	}
}

// TestExplainBlock_TaskInexistenteONil confirma el criterio defensivo.
func TestExplainBlock_TaskInexistenteONil(t *testing.T) {
	cr := NewCausalReasoner()
	if cr.ExplainBlock(nil, "T1") != nil {
		t.Fatal("ExplainBlock(nil, ...) debería devolver nil")
	}
	if cr.ExplainBlock(buildTestGraph(), "T99") != nil {
		t.Fatal("ExplainBlock con TaskID inexistente debería devolver nil")
	}
}

// TestProjectResolution_CuentaLasTareasQueDejariandeBloquearse
// confirma el aporte central del bloque: proyectar hacia adelante.
func TestProjectResolution_CuentaLasTareasQueDejariandeBloquearse(t *testing.T) {
	dg := buildTestGraph()
	cr := NewCausalReasoner()
	proj := cr.ProjectResolution(dg, "T2")

	if len(proj.Unblocks) != 2 {
		t.Fatalf("len(Unblocks) = %d, esperaba 2 (T3 y T4, ambas BlockedByID=T2)", len(proj.Unblocks))
	}
}

// TestProjectResolution_GapIDQueNoEsGapDaNil confirma que proyectar
// sobre una Task que no es gap (aunque exista) no inventa nada.
func TestProjectResolution_GapIDQueNoEsGapDaNil(t *testing.T) {
	dg := buildTestGraph()
	cr := NewCausalReasoner()
	if cr.ProjectResolution(dg, "T1") != nil {
		t.Fatal("ProjectResolution sobre una Task que no es gap debería devolver nil")
	}
}

// TestProjectResolution_SinTareasQueDesbloquearDaSliceVacio confirma
// que un gap sin nada bloqueado por él da Unblocks vacío, no nil
// inconsistente ni error.
func TestProjectResolution_SinTareasQueDesbloquearDaSliceVacio(t *testing.T) {
	dg := &DependencyGraph{
		Order: []string{"T1"},
		Nodes: map[string]*DependencyNode{"T1": {TaskID: "T1", TaskDesc: "solitario", IsGap: true}},
	}
	cr := NewCausalReasoner()
	proj := cr.ProjectResolution(dg, "T1")

	if len(proj.Unblocks) != 0 {
		t.Fatalf("len(Unblocks) = %d, esperaba 0", len(proj.Unblocks))
	}
	if !strings.Contains(proj.Describe(), "no desbloquearía") {
		t.Fatalf("Describe() = %q, esperaba que avise que no desbloquea nada", proj.Describe())
	}
}

// TestDescribeProjections_OrdenaPorImpacto confirma que el gap que
// desbloquea más tareas aparece primero en la narrativa conjunta.
func TestDescribeProjections_OrdenaPorImpacto(t *testing.T) {
	chico := &CausalProjection{GapID: "G1", GapDesc: "gap chico", Unblocks: []*DependencyNode{{TaskDesc: "uno"}}}
	grande := &CausalProjection{GapID: "G2", GapDesc: "gap grande", Unblocks: []*DependencyNode{
		{TaskDesc: "uno"}, {TaskDesc: "dos"}, {TaskDesc: "tres"},
	}}

	desc := DescribeProjections([]*CausalProjection{chico, grande})
	posGrande := strings.Index(desc, "gap grande")
	posChico := strings.Index(desc, "gap chico")
	if posGrande == -1 || posChico == -1 || posGrande > posChico {
		t.Fatalf("DescribeProjections() = %q, esperaba que \"gap grande\" aparezca antes que \"gap chico\"", desc)
	}
}

// TestExplainAllBlocks_SoloListaLoBloqueado confirma que Tasks no
// bloqueadas (como T1) no aparecen en la lista.
func TestExplainAllBlocks_SoloListaLoBloqueado(t *testing.T) {
	dg := buildTestGraph()
	cr := NewCausalReasoner()
	links := cr.ExplainAllBlocks(dg)

	if len(links) != 2 {
		t.Fatalf("len(links) = %d, esperaba 2 (T3 y T4)", len(links))
	}
}

// TestParseCausalQueries_NoSeConfunden confirma que explicar y
// proyectar son triggers distintos, sin superposición.
func TestParseCausalQueries_NoSeConfunden(t *testing.T) {
	if !ParseCausalExplainQuery(interaction.Analyze("por que esta bloqueada", interaction.Context{})) {
		t.Error("ParseCausalExplainQuery debería activarse con su propio trigger")
	}
	if ParseCausalExplainQuery(interaction.Analyze("que se desbloquearia si resuelvo", interaction.Context{})) {
		t.Error("ParseCausalExplainQuery no debería activarse con el trigger de proyección")
	}
	if !ParseCausalProjectQuery(interaction.Analyze("cual gap conviene resolver primero", interaction.Context{})) {
		t.Error("ParseCausalProjectQuery debería activarse con su propio trigger")
	}
	if ParseCausalProjectQuery(interaction.Analyze("por que esta bloqueada", interaction.Context{})) {
		t.Error("ParseCausalProjectQuery no debería activarse con el trigger de explicación")
	}
}
