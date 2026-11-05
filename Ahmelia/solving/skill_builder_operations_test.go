package solving

import (
	"testing"
)

// TestDetectOperations_StepConVerboWired confirma que un Step cuyo
// texto matchea un trigger de shared.ActionPatterns Y ese Action está
// en shared.WiredActions produce una Operation Wired.
func TestDetectOperations_StepConVerboWired(t *testing.T) {
	ops := detectOperations([]string{"Abre el programa antes de continuar"})
	if len(ops) != 1 {
		t.Fatalf("esperaba 1 Operation, obtuve %v", ops)
	}
	if ops[0].Status != OpWired {
		t.Errorf("Status = %q, esperaba %q (abrir_programa está en WiredActions)", ops[0].Status, OpWired)
	}
	if ops[0].Action != "abrir_programa" {
		t.Errorf("Action = %q, esperaba %q", ops[0].Action, "abrir_programa")
	}
}

// TestDetectOperations_StepConVerboReconocidoYConectado confirma que la
// resolución semántica y el binding de ejecución convergen en el mismo
// contrato canónico.
func TestDetectOperations_StepConVerboReconocidoYConectado(t *testing.T) {
	ops := detectOperations([]string{"Renombra el archivo al nombre final"})
	if len(ops) != 1 {
		t.Fatalf("esperaba 1 Operation, obtuve %v", ops)
	}
	if ops[0].Status != OpWired {
		t.Errorf("Status = %q, esperaba %q — renombrar_archivo debe derivar de la capability canónica", ops[0].Status, OpWired)
	}
	if ops[0].Action != "renombrar_archivo" {
		t.Errorf("Action = %q, esperaba %q", ops[0].Action, "renombrar_archivo")
	}
}

// TestDetectOperations_VerboDeActionsEsSiempreNamed confirma que un
// verbo de actions/blender (ej. "renderizar") nunca sale Wired — ese
// paquete es un esqueleto sin Execute() real, ver
// actions/blender/render.go.
func TestDetectOperations_VerboDeActionsEsSiempreNamed(t *testing.T) {
	ops := detectOperations([]string{"Renderizar la escena final"})
	if len(ops) != 1 {
		t.Fatalf("esperaba 1 Operation, obtuve %v", ops)
	}
	if ops[0].Status != OpNamed {
		t.Errorf("Status = %q, esperaba %q — actions/blender es un esqueleto sin implementación real", ops[0].Status, OpNamed)
	}
	if ops[0].Action != "renderizar" {
		t.Errorf("Action = %q, esperaba %q", ops[0].Action, "renderizar")
	}
}

// TestDetectOperations_StepSinMatchEsUnknown confirma que un Step sin
// ningún verbo reconocido produce una única Operation Unknown, con el
// Step COMPLETO como Text (no un fragmento inventado).
func TestDetectOperations_StepSinMatchEsUnknown(t *testing.T) {
	step := "Configura las propiedades del material segun el diseno deseado"
	ops := detectOperations([]string{step})
	if len(ops) != 1 {
		t.Fatalf("esperaba 1 Operation, obtuve %v", ops)
	}
	if ops[0].Status != OpUnknown {
		t.Errorf("Status = %q, esperaba %q", ops[0].Status, OpUnknown)
	}
	if ops[0].Action != "" {
		t.Errorf("Action = %q, esperaba vacío para Unknown", ops[0].Action)
	}
	if ops[0].Text != step {
		t.Errorf("Text = %q, esperaba el Step completo %q", ops[0].Text, step)
	}
}

// TestDetectOperations_StepConVariosMatchesProduceVariasOperations
// confirma que un Step con más de un verbo conocido aporta varias
// Operations, no una sola.
func TestDetectOperations_StepConVariosMatchesProduceVariasOperations(t *testing.T) {
	ops := detectOperations([]string{"Abre el programa y luego exportar el proyecto"})
	if len(ops) < 2 {
		t.Fatalf("esperaba al menos 2 Operations (abrir_programa + exportar de blender), obtuve %v", ops)
	}
	var hasWiredAbrir, hasNamedExportar bool
	for _, op := range ops {
		if op.Action == "abrir_programa" && op.Status == OpWired {
			hasWiredAbrir = true
		}
		if op.Action == "exportar" && op.Status == OpNamed {
			hasNamedExportar = true
		}
	}
	if !hasWiredAbrir {
		t.Errorf("esperaba una Operation Wired para abrir_programa, obtuve %v", ops)
	}
	if !hasNamedExportar {
		t.Errorf("esperaba una Operation Named para exportar (actions/blender), obtuve %v", ops)
	}
}

// TestDetectOperations_DeterministicoEntreCorridas confirma que el
// orden de las Operations detectadas no depende del orden de
// iteración de los maps internos (ActionPatterns, namedActionsVocabulary).
func TestDetectOperations_DeterministicoEntreCorridas(t *testing.T) {
	steps := []string{"Abre el editor y exporta el proyecto", "Renderiza la escena"}
	first := detectOperations(steps)
	for i := 0; i < 20; i++ {
		again := detectOperations(steps)
		if len(again) != len(first) {
			t.Fatalf("corrida %d: longitud distinta, %d vs %d", i, len(again), len(first))
		}
		for j := range first {
			if again[j] != first[j] {
				t.Fatalf("corrida %d: Operation[%d] distinta — %v vs %v", i, j, again[j], first[j])
			}
		}
	}
}

// ── Métodos de razonamiento sobre Operations ────────────────────────

func TestSkill_HasExecutableOperations(t *testing.T) {
	conWired := &Skill{Operations: []Operation{{Status: OpWired}, {Status: OpUnknown}}}
	if !conWired.HasExecutableOperations() {
		t.Error("debería ser true con al menos una Operation Wired")
	}
	sinWired := &Skill{Operations: []Operation{{Status: OpNamed}, {Status: OpUnknown}}}
	if sinWired.HasExecutableOperations() {
		t.Error("debería ser false sin ninguna Operation Wired")
	}
}

func TestSkill_AllOperationsWired(t *testing.T) {
	todasWired := &Skill{Operations: []Operation{{Status: OpWired}, {Status: OpWired}}}
	if !todasWired.AllOperationsWired() {
		t.Error("debería ser true cuando todas las Operations son Wired")
	}
	mezcla := &Skill{Operations: []Operation{{Status: OpWired}, {Status: OpUnknown}}}
	if mezcla.AllOperationsWired() {
		t.Error("debería ser false con una mezcla de estados")
	}
	vacia := &Skill{Operations: nil}
	if vacia.AllOperationsWired() {
		t.Error("debería ser false sin ninguna Operation — no hay nada que afirmar")
	}
}

func TestSkill_CapabilityCoverage(t *testing.T) {
	s := &Skill{Operations: []Operation{{Status: OpWired}, {Status: OpWired}, {Status: OpNamed}, {Status: OpUnknown}}}
	wired, total := s.CapabilityCoverage()
	if wired != 2 || total != 4 {
		t.Errorf("CapabilityCoverage() = (%d, %d), esperaba (2, 4)", wired, total)
	}
}

func TestSkill_OperabilityLabel(t *testing.T) {
	casos := []struct {
		nombre string
		ops    []Operation
		quiero string
	}{
		{"sin operations", nil, "KnowledgeOnly"},
		{"todo unknown", []Operation{{Status: OpUnknown}}, "KnowledgeOnly"},
		{"parcial", []Operation{{Status: OpWired}, {Status: OpUnknown}}, "OperationsPartiallyMapped"},
		{"completo", []Operation{{Status: OpWired}, {Status: OpWired}}, "OperationsFullyMapped"},
	}
	for _, c := range casos {
		s := &Skill{Operations: c.ops}
		if got := s.OperabilityLabel(); got != c.quiero {
			t.Errorf("%s: OperabilityLabel() = %q, esperaba %q", c.nombre, got, c.quiero)
		}
	}
}

// TestBuild_OperationsSeCalculanDesdeSteps confirma la integración
// completa: Build() puebla Operations a partir de los Steps extraídos
// del texto fuente, sin que el llamador tenga que invocar nada aparte.
func TestBuild_OperationsSeCalculanDesdeSteps(t *testing.T) {
	text := "Para preparar el proyecto, segui estos pasos:\n" +
		"1. Abre el programa principal\n" +
		"2. Renombra el archivo de configuracion\n" +
		"3. Configura las opciones avanzadas del sistema\n"
	skill := buildFrom(t, "preparar-proyecto", text)

	if len(skill.Operations) != 3 {
		t.Fatalf("esperaba 3 Operations (una por Step), obtuve %v", skill.Operations)
	}
	if skill.Operations[0].Status != OpWired {
		t.Errorf("Operations[0] (abrir_programa) = %q, esperaba %q", skill.Operations[0].Status, OpWired)
	}
	if skill.Operations[1].Status != OpWired {
		t.Errorf("Operations[1] (renombrar_archivo) = %q, esperaba %q", skill.Operations[1].Status, OpWired)
	}
	if skill.Operations[2].Status != OpUnknown {
		t.Errorf("Operations[2] (sin verbo reconocido) = %q, esperaba %q", skill.Operations[2].Status, OpUnknown)
	}
	if skill.OperabilityLabel() != "OperationsPartiallyMapped" {
		t.Errorf("OperabilityLabel() = %q, esperaba OperationsPartiallyMapped (2 wired de 3)", skill.OperabilityLabel())
	}
}
